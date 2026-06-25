"""vivify.commands.episode — episode lifecycle in the DB.

Wraps render_episode.py so each render becomes:
  - 1 episode row (cost, duration, status, output path)
  - N shot rows (one per storyboard shot)
  - 1 render_job row (start/end, error, retries)
  - K asset rows (generated images — best-effort)

This is the surface that justifies the DB. After `episode render`
returns, the user can `episode show` to see the full audit trail of
that render — what shots ran, what they cost, where the files are.

Commands:
  list        [-c char] [-s status]
  show        <character> <episode>
  shots       <character> <episode>
  add         <character> <episode>
  render      <character> <episode> [--storyboard] [--script] ...
  status      <character> <episode>
  delete      <character> <episode>
"""

import json
import os
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

import click

from ..db import connect, init_db
from ..pricing import (
    estimate_episode_cost,
    format_yuan,
    image_cost_yuan,
    tts_cost_yuan,
    video_cost_yuan,
)
from ..episode_driver import (
    render_episode_assets as _render_episode_assets,
    _resolve_canonical_ref as _driver_resolve_canonical_ref,
    _resolve_voice_profile as _driver_resolve_voice_profile,
)


# ---- helpers --------------------------------------------------------------

def _now_iso() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def _resolve_character(conn, character_id: str) -> dict | None:
    """Look up character row, return None if not registered."""
    row = conn.execute(
        "SELECT id, name, english_name, dir_path FROM characters WHERE id = ?",
        (character_id,),
    ).fetchone()
    return dict(row) if row else None


def _resolve_episode(conn, character_id: str, episode_id: str) -> dict | None:
    row = conn.execute(
        "SELECT * FROM episodes WHERE character_id = ? AND episode_id = ?",
        (character_id, episode_id),
    ).fetchone()
    return dict(row) if row else None


def _ensure_episode_row(conn, character_id: str, episode_id: str,
                       storyboard: str = None, script: str = None,
                       voice: str = None, platform: str = None,
                       quality_tier: str = None, model_used: str = None,
                       target_dur: int = None,
                       status: str = "pending") -> int:
    """Insert or update an episode row, return its id."""
    existing = _resolve_episode(conn, character_id, episode_id)
    if existing:
        # Update only fields that are non-None
        updates = {}
        for k, v in [("storyboard_path", storyboard), ("script_path", script),
                     ("tone", voice), ("platform", platform),
                     ("quality_tier", quality_tier), ("model_used", model_used),
                     ("target_dur_sec", target_dur), ("status", status)]:
            if v is not None:
                updates[k] = v
        if updates:
            sets = ", ".join(f"{k} = ?" for k in updates)
            conn.execute(
                f"UPDATE episodes SET {sets}, updated_at = datetime('now') "
                f"WHERE id = ?",
                list(updates.values()) + [existing["id"]],
            )
        return existing["id"]
    cur = conn.execute(
        """INSERT INTO episodes (
            character_id, episode_id, tone, platform,
            storyboard_path, script_path, quality_tier, model_used,
            target_dur_sec, status, updated_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))""",
        (character_id, episode_id, voice, platform, storyboard, script,
         quality_tier, model_used, target_dur, status),
    )
    return cur.lastrowid


def _populate_shots_from_storyboard(conn, episode_pk: int,
                                    storyboard_path: str,
                                    script_path: str = None) -> int:
    """Parse storyboard, insert shot rows. Returns number of shots written.

    Best-effort: we import render_episode's parsers when available; if not,
    we fall back to a minimal regex. Either way, this never blocks the
    render — it just pre-fills the shots table.
    """
    try:
        sys.path.insert(0, str(Path.cwd()))
        from render_episode import parse_storyboard, parse_script
        shots = parse_storyboard(storyboard_path)
        voiceovers = parse_script(script_path) if script_path else []
    except Exception as e:
        click.echo(f"[warn] could not parse storyboard: {e}", err=True)
        return 0

    # Wipe existing shots for this episode (re-render: start fresh)
    conn.execute("DELETE FROM shots WHERE episode_id = ?", (episode_pk,))

    for shot in shots:
        vo = None
        if voiceovers:
            best = None
            best_delta = 999.0
            for v in voiceovers:
                delta = abs(v["start_sec"] - shot["start_sec"])
                if delta <= 1.0 and delta < best_delta:
                    best = v
                    best_delta = delta
            vo = best
        conn.execute(
            """INSERT INTO shots (
                episode_id, shot_number, duration_sec, kling_prompt,
                voiceover_text
            ) VALUES (?, ?, ?, ?, ?)""",
            (episode_pk, shot["n"], shot["duration_sec"],
             shot["kling_prompt"],
             (vo or {}).get("text", "")),
        )
    return len(shots)


def _get_shots(conn, episode_pk: int) -> list[dict]:
    rows = conn.execute(
        "SELECT * FROM shots WHERE episode_id = ? ORDER BY shot_number",
        (episode_pk,),
    ).fetchall()
    return [dict(r) for r in rows]


def _update_shot_after_render(conn, shot_pk: int, *, image_path: str = None,
                              video_path: str = None, cost_yuan: float = None,
                              model_used: str = None, qa_passed: bool = None,
                              qa_issues: list = None) -> None:
    sets, vals = [], []
    if image_path is not None:
        sets.append("image_path = ?"); vals.append(image_path)
    if video_path is not None:
        sets.append("video_path = ?"); vals.append(video_path)
    if cost_yuan is not None:
        sets.append("cost_yuan = ?"); vals.append(cost_yuan)
    if model_used is not None:
        sets.append("model_used = ?"); vals.append(model_used)
    if qa_passed is not None:
        sets.append("qa_passed = ?"); vals.append(1 if qa_passed else 0)
    if qa_issues is not None:
        sets.append("qa_issues = ?"); vals.append(json.dumps(qa_issues, ensure_ascii=False))
    if not sets:
        return
    vals.append(shot_pk)
    conn.execute(f"UPDATE shots SET {', '.join(sets)} WHERE id = ?", vals)


def _ffprobe_duration(path: str) -> float | None:
    """Best-effort: read duration from final MP4. Returns None on failure.

    Tries (in order): $FFMPEG_DIR/ffprobe, the ffmpeg installer's sibling
    ffprobe, /usr/bin/ffprobe, then PATH. The ffmpeg-installer at
    /root/.openclaw/.../ffmpeg/ bundles ffprobe next to ffmpeg.
    """
    if not path or not Path(path).exists():
        return None
    ffprobe_candidates = [
        os.environ.get("FFPROBE"),
        "/root/.openclaw/extensions/dingtalk-connector/node_modules/@ffmpeg-installer/linux-x64/ffprobe",
        "/usr/bin/ffprobe",
        "/usr/local/bin/ffprobe",
        "ffprobe",
    ]
    for c in ffprobe_candidates:
        if not c:
            continue
        try:
            r = subprocess.run(
                [c, "-v", "error", "-show_entries", "format=duration",
                 "-of", "default=noprint_wrappers=1:nokey=1", path],
                capture_output=True, text=True, timeout=10,
            )
            if r.returncode == 0 and r.stdout.strip():
                return float(r.stdout.strip())
        except (subprocess.TimeoutExpired, FileNotFoundError, ValueError, OSError):
            continue
    return None


def _file_size(path: str) -> int | None:
    if not path or not Path(path).exists():
        return None
    return Path(path).stat().st_size


# ---- Click group ----------------------------------------------------------

@click.group("episode")
def cli():
    """Episode lifecycle: register, render, show history."""
    pass


# ---- list -----------------------------------------------------------------

@cli.command("list")
@click.option("--character", "-c", default=None, help="Filter by character id.")
@click.option("--status", "-s", default=None,
              type=click.Choice(["pending", "rendering", "completed", "failed"]),
              help="Filter by status.")
@click.option("--json", "as_json", is_flag=True, help="Output as JSON.")
@click.pass_obj
def list_cmd(obj, character, status, as_json):
    """List all episodes (optionally filtered)."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        q = "SELECT * FROM episodes WHERE 1=1"
        params = []
        if character:
            q += " AND character_id = ?"; params.append(character)
        if status:
            q += " AND status = ?"; params.append(status)
        q += " ORDER BY character_id, episode_id"
        rows = [dict(r) for r in conn.execute(q, params).fetchall()]
        if as_json:
            click.echo(json.dumps(rows, indent=2, ensure_ascii=False, default=str))
            return
        if not rows:
            click.echo("(no episodes registered yet — try `vivify episode add <char> <ep>`)")
            return
        click.echo(f"{'CHAR':<10} {'EP':<8} {'STATUS':<10} {'VOICE':<6} {'PLATFORM':<8} "
                   f"{'DUR':<6} {'COST':<8} {'UPDATED':<20}")
        click.echo("─" * 90)
        for r in rows:
            click.echo(
                f"{r['character_id']:<10} "
                f"{r['episode_id']:<8} "
                f"{(r['status'] or '—'):<10} "
                f"{(r['tone'] or '—'):<6} "
                f"{(r['platform'] or '—'):<8} "
                f"{((str(r['target_dur_sec']) + 's') if r['target_dur_sec'] else '—'):<6} "
                f"{format_yuan(r['cost_yuan']):<8} "
                f"{(r.get('render_completed_at') or r.get('render_started_at') or r['created_at']):<20}"
            )


# ---- show -----------------------------------------------------------------

@cli.command("show")
@click.argument("character_id")
@click.argument("episode_id")
@click.option("--json", "as_json", is_flag=True)
@click.pass_obj
def show_cmd(obj, character_id, episode_id, as_json):
    """Show episode details + shots + render jobs + publishes."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        ep = _resolve_episode(conn, character_id, episode_id)
        if not ep:
            raise click.ClickException(f"episode {character_id}/{episode_id} not found "
                                       f"(try `vivify episode list`)")
        shots = _get_shots(conn, ep["id"])
        jobs = [dict(r) for r in conn.execute(
            "SELECT * FROM render_jobs WHERE episode_id = ? ORDER BY id",
            (ep["id"],),
        ).fetchall()]
        pubs = [dict(r) for r in conn.execute(
            "SELECT * FROM publishes WHERE episode_id = ? ORDER BY id",
            (ep["id"],),
        ).fetchall()]

    if as_json:
        click.echo(json.dumps({"episode": ep, "shots": shots,
                               "render_jobs": jobs, "publishes": pubs},
                              indent=2, ensure_ascii=False, default=str))
        return

    click.echo(f"═══ Episode {character_id}/{episode_id} ═══")
    click.echo(f"  status:       {ep['status'] or '—'}")
    click.echo(f"  voice/tone:   {ep['tone'] or '—'}")
    click.echo(f"  platform:     {ep['platform'] or '—'}")
    click.echo(f"  quality_tier: {ep['quality_tier'] or '—'}")
    click.echo(f"  model_used:   {ep['model_used'] or '—'}")
    click.echo(f"  target_dur:   {ep['target_dur_sec'] or '—'}s")
    click.echo(f"  actual_dur:   {ep['actual_dur_sec'] or '—'}s")
    click.echo(f"  cost:         {format_yuan(ep['cost_yuan'])}")
    click.echo(f"  output:       {ep['output_path'] or '—'}")
    click.echo(f"  file_size:    {(ep['file_size_bytes'] or 0) / 1_000_000:.1f} MB"
               if ep['file_size_bytes'] else "  file_size:    —")
    click.echo(f"  storyboard:   {ep['storyboard_path'] or '—'}")
    click.echo(f"  script:       {ep['script_path'] or '—'}")
    click.echo(f"  created:      {ep['created_at']}")
    if ep.get("render_started_at"):
        click.echo(f"  started:      {ep['render_started_at']}")
    if ep.get("render_completed_at"):
        click.echo(f"  completed:    {ep['render_completed_at']}")
    if ep.get("error_message"):
        click.echo(f"  ⚠ error:      {ep['error_message']}", err=True)

    click.echo("")
    click.echo(f"── shots ({len(shots)}) ──")
    if shots:
        click.echo(f"  {'#':<3} {'DUR':<5} {'IMG':<5} {'VID':<5} {'QA':<3} {'COST':<8} PROMPT")
        for s in shots:
            qa_mark = "—" if s.get("qa_passed") is None else ("✓" if s["qa_passed"] else "✗")
            prompt_head = (s["kling_prompt"] or "")[:60].replace("\n", " ")
            click.echo(
                f"  {s['shot_number']:<3} "
                f"{(str(s['duration_sec']) + 's'):<5} "
                f"{('Y' if s.get('image_path') else '—'):<5} "
                f"{('Y' if s.get('video_path') else '—'):<5} "
                f"{qa_mark:<3} "
                f"{format_yuan(s['cost_yuan']):<8} {prompt_head}…"
            )

    click.echo("")
    click.echo(f"── render_jobs ({len(jobs)}) ──")
    for j in jobs:
        click.echo(f"  #{j['id']} {j['status']:<10} "
                   f"start={j.get('started_at', '—') or '—'} "
                   f"end={j.get('completed_at', '—') or '—'} "
                   f"retries={j['retries']}")
        if j.get("error_message"):
            click.echo(f"    ⚠ {j['error_message']}", err=True)

    if pubs:
        click.echo("")
        click.echo(f"── publishes ({len(pubs)}) ──")
        for p in pubs:
            click.echo(f"  {p['platform']:<10} {p['published_at']} "
                       f"views={p['view_count']} likes={p['like_count']} "
                       f"comments={p['comment_count']}")


# ---- shots ----------------------------------------------------------------

@cli.command("shots")
@click.argument("character_id")
@click.argument("episode_id")
@click.option("--json", "as_json", is_flag=True)
@click.pass_obj
def shots_cmd(obj, character_id, episode_id, as_json):
    """Show shot-by-shot table for an episode."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        ep = _resolve_episode(conn, character_id, episode_id)
        if not ep:
            raise click.ClickException(f"episode {character_id}/{episode_id} not found")
        shots = _get_shots(conn, ep["id"])

    if as_json:
        click.echo(json.dumps(shots, indent=2, ensure_ascii=False, default=str))
        return

    if not shots:
        click.echo(f"(no shots — was storyboard parsed? "
                   f"storyboard_path={ep.get('storyboard_path') or '—'})")
        return
    click.echo(f"{'#':<3} {'DUR':<5} {'IMG':<5} {'VID':<5} {'QA':<3} {'COST':<8} "
               f"{'MODEL':<32} PROMPT")
    click.echo("─" * 110)
    for s in shots:
        qa_mark = "—" if s.get("qa_passed") is None else ("✓" if s["qa_passed"] else "✗")
        prompt_head = (s["kling_prompt"] or "")[:55].replace("\n", " ")
        click.echo(
            f"{s['shot_number']:<3} "
            f"{(str(s['duration_sec']) + 's'):<5} "
            f"{('Y' if s.get('image_path') else '—'):<5} "
            f"{('Y' if s.get('video_path') else '—'):<5} "
            f"{qa_mark:<3} "
            f"{format_yuan(s['cost_yuan']):<8} "
            f"{(s.get('model_used') or '—')[:32]:<32} {prompt_head}…"
        )


# ---- add ------------------------------------------------------------------

@cli.command("add")
@click.argument("character_id")
@click.argument("episode_id")
@click.option("--storyboard", default=None, help="Path to STORYBOARD.md")
@click.option("--script", default=None, help="Path to SCRIPT-*.md")
@click.option("--voice", default=None, type=click.Choice(["治愈", "御宅", "哲学", "国潮"]))
@click.option("--platform", default=None, type=click.Choice(["抖音", "哔哩哔哩", "小红书"]))
@click.option("--quality-tier", default="standard",
              type=click.Choice(["draft", "standard", "premium"]))
@click.option("--video-model", default=None, help="Override the video model.")
@click.option("--target-dur", type=int, default=None,
              help="Target episode duration in seconds.")
@click.pass_obj
def add_cmd(obj, character_id, episode_id, storyboard, script, voice,
            platform, quality_tier, video_model, target_dur):
    """Register an episode in the DB without rendering."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        char = _resolve_character(conn, character_id)
        if not char:
            raise click.ClickException(f"character '{character_id}' not registered "
                                       f"(try `vivify character add {character_id} ...`)")
        if storyboard and not Path(storyboard).exists():
            raise click.ClickException(f"storyboard not found: {storyboard}")
        if script and not Path(script).exists():
            raise click.ClickException(f"script not found: {script}")
        ep_pk = _ensure_episode_row(
            conn, character_id, episode_id,
            storyboard=storyboard, script=script, voice=voice,
            platform=platform, quality_tier=quality_tier,
            model_used=video_model, target_dur=target_dur,
            status="pending",
        )
        n_shots = 0
        if storyboard:
            n_shots = _populate_shots_from_storyboard(
                conn, ep_pk, storyboard, script)
    click.echo(f"✓ episode #{ep_pk} registered: {character_id}/{episode_id}")
    if n_shots:
        click.echo(f"  shots parsed from storyboard: {n_shots}")
    click.echo(f"  status: pending")
    click.echo("")
    click.echo("Next: `vivify episode render {cid} {eid}` to render it.".format(
        cid=character_id, eid=episode_id))


# ---- render ---------------------------------------------------------------

@cli.command("render")
@click.argument("character_id")
@click.argument("episode_id")
@click.option("--storyboard", default=None)
@click.option("--script", default=None)
@click.option("--voice", default=None, type=click.Choice(["治愈", "御宅", "哲学", "国潮"]))
@click.option("--platform", default=None, type=click.Choice(["抖音", "哔哩哔哩", "小红书"]))
@click.option("--quality-tier", default=None,
              type=click.Choice(["draft", "standard", "premium"]))
@click.option("--video-provider", default=None,
              type=click.Choice(["auto", "ark", "minimax"]),
              help="Which video gen provider: auto (router), ark (Seedance), "
                   "minimax (Hailuo via mmx CLI). Default: auto.")
@click.option("--video-model", default=None,
              help="Force a specific model id (e.g. 'doubao-seedance-1-5-pro-251215' "
                   "or 'MiniMax-S2V-01'). Provider filter still applies.")
@click.option("--image-model", default="doubao-seedream-4-0-250828")
@click.option("--reference-image", default=None)
@click.option("--target-dur", type=int, default=None)
@click.option("--out", "out_dir", default=".tmp/renders",
              help="Where to write the final MP4.")
@click.option("--require-lip-sync", is_flag=True)
@click.option("--qa-skip", is_flag=True)
@click.option("--title", default=None)
@click.option("--next-episode", default="下集预告")
@click.option("--dry-run", is_flag=True,
              help="Write DB rows + estimate cost, but DO NOT call render_episode.py. "
                   "Useful for testing the pipeline without burning API quota.")
@click.option("--force", is_flag=True,
              help="Bypass cost-cap enforcement (per-video + monthly hard limits). "
                   "Use only when intentional.")
@click.option("--per-video-cap", type=float, default=None,
              help="Override per-video hard cap in ¥ (default 100).")
@click.option("--parallel", "-j", type=int, default=1,
              help="Number of shots to render in parallel (default 1 = serial).")
@click.option("--max-retries", type=int, default=3,
              help="Max retries per shot before marking failed (default 3).")
@click.option("--retry-only", is_flag=True,
              help="Only re-run failed/queued jobs for this episode "
                   "(use after `workflow retry` requeues).")
@click.option("--use-driver", is_flag=True,
              help="Use the new in-process driver (vivify.episode_driver) "
                   "instead of the legacy render_episode.py subprocess. "
                   "Default: False (subprocess). Driver writes real per-asset "
                   "cost to the DB and emits ledger rows; subprocess path "
                   "still works as a manual escape.")
@click.pass_obj
def render_cmd(obj, character_id, episode_id, storyboard, script, voice,
               platform, quality_tier, video_provider, video_model, image_model,
               reference_image, target_dur, out_dir, require_lip_sync,
               qa_skip, title, next_episode, dry_run, force, per_video_cap,
               parallel, max_retries, retry_only, use_driver):
    """Render an episode via render_episode.py, recording the full run in DB.

    With --use-driver, the per-shot assets are generated in-process via
    vivify.episode_driver.render_episode_assets (which calls generate_asset
    + TTS + DB update). Without it, falls back to the legacy subprocess.
    """
    db_path = obj.get("db_path")
    init_db(db_path)

    # 1. Resolve character
    with connect(db_path) as conn:
        char = _resolve_character(conn, character_id)
        if not char:
            raise click.ClickException(f"character '{character_id}' not registered")

    # 2. Default args from existing episode row if present
    with connect(db_path) as conn:
        existing = _resolve_episode(conn, character_id, episode_id)
        if existing:
            storyboard = storyboard or existing.get("storyboard_path")
            script = script or existing.get("script_path")
            voice = voice or existing.get("tone")
            platform = platform or existing.get("platform")
            quality_tier = quality_tier or existing.get("quality_tier") or "standard"
            video_model = video_model or existing.get("model_used")
            target_dur = target_dur or existing.get("target_dur_sec") or 58
        else:
            quality_tier = quality_tier or "standard"
            target_dur = target_dur or 58

    # 3. Validate inputs
    if not storyboard:
        raise click.ClickException("--storyboard required (or pre-register via "
                                   "`vivify episode add`)")
    if not Path(storyboard).exists():
        raise click.ClickException(f"storyboard not found: {storyboard}")
    if script and not Path(script).exists():
        raise click.ClickException(f"script not found: {script}")
    if not voice:
        raise click.ClickException("--voice required (or pre-register via `episode add`)")
    if not platform:
        raise click.ClickException("--platform required (or pre-register via `episode add`)")

    # 4. Pre-write episode + shots + render_job rows
    out_path = str(Path(out_dir).expanduser() / f"{character_id}-{episode_id}.mp4")
    Path(out_dir).expanduser().mkdir(parents=True, exist_ok=True)

    with connect(db_path) as conn:
        ep_pk = _ensure_episode_row(
            conn, character_id, episode_id,
            storyboard=str(storyboard), script=str(script) if script else None,
            voice=voice, platform=platform,
            quality_tier=quality_tier, model_used=video_model,
            target_dur=target_dur, status="rendering",
        )
        n_shots = _populate_shots_from_storyboard(
            conn, ep_pk, str(storyboard), str(script) if script else None)
        cur = conn.execute(
            """INSERT INTO render_jobs (episode_id, status, started_at)
               VALUES (?, 'running', datetime('now'))""",
            (ep_pk,),
        )
        job_pk = cur.lastrowid
        conn.execute(
            "UPDATE episodes SET render_started_at = datetime('now') WHERE id = ?",
            (ep_pk,),
        )

    # 5. Estimate cost (we'll know real cost only after the call returns)
    # Pull the shot breakdown we just wrote so we can sum durations
    with connect(db_path) as conn:
        shots = _get_shots(conn, ep_pk)
    total_video_sec = sum((s["duration_sec"] or 0) for s in shots if s.get("voiceover_text") or True)
    n_voiceovers = sum(1 for s in shots if s.get("voiceover_text"))
    avg_vo_chars = 0
    if n_voiceovers:
        lengths = [len(s["voiceover_text"]) for s in shots if s.get("voiceover_text")]
        avg_vo_chars = sum(lengths) // n_voiceovers
    est = estimate_episode_cost(
        n_shots=n_shots,
        total_video_sec=total_video_sec,
        n_voiceovers=n_voiceovers,
        avg_vo_chars=avg_vo_chars,
        video_model=video_model or "doubao-seedance-2-0-260128",
    )

    click.echo(f"╭─ rendering {character_id}/{episode_id} {'(DRY RUN) ' if dry_run else ''}─╮")
    click.echo(f"│ shots:        {n_shots}")
    click.echo(f"│ video_sec:    {total_video_sec}")
    click.echo(f"│ voiceovers:   {n_voiceovers} (avg {avg_vo_chars} chars)")
    click.echo(f"│ voice/tone:   {voice}")
    click.echo(f"│ platform:     {platform}")
    click.echo(f"│ quality_tier: {quality_tier}")
    click.echo(f"│ video_model:  {video_model or '(router default)'}")
    click.echo(f"│ parallel:     {parallel}")
    click.echo(f"│ max-retries:  {max_retries}")
    click.echo(f"│ output:       {out_path}")
    click.echo(f"│ est cost:     {format_yuan(est['total'])} "
               f"(img {format_yuan(est['images'])} + "
               f"vid {format_yuan(est['videos'])} + "
               f"tts {format_yuan(est['tts'])})")
    click.echo("╰──────────────────────────────────────╯")

    # Cost-cap gate (skipped for dry-run — nothing to spend)
    if not dry_run:
        from ..cost_cap import (
            DEFAULT_MONTHLY_HARD, DEFAULT_PER_VIDEO_HARD,
            check_cost_caps, render_cost_summary,
        )
        with connect(db_path) as conn:
            cap_check = check_cost_caps(
                conn,
                estimate_yuan=est["total"],
                per_video_hard=per_video_cap or DEFAULT_PER_VIDEO_HARD,
                monthly_hard=DEFAULT_MONTHLY_HARD,
                force=force,
            )
        click.echo(render_cost_summary(cap_check))
        if not cap_check["allowed"]:
            # Should have raised, but defensive
            raise click.ClickException(
                f"cost cap blocked this render (use --force to override)")

    if dry_run and not use_driver:
        # Mark as completed (dry-run), with estimated cost
        # Note: when use_driver is set, dry-run takes the driver branch
        # above (with generate_asset/_call_tts stubbed) and runs the full
        # pipeline end-to-end without spending money.
        with connect(db_path) as conn:
            conn.execute(
                """UPDATE episodes SET
                    status = 'completed',
                    output_path = ?,
                    cost_yuan = ?,
                    actual_dur_sec = ?,
                    render_completed_at = datetime('now')
                   WHERE id = ?""",
                (out_path, est["total"], float(target_dur), ep_pk),
            )
            # Update per-shot cost rows
            for shot in shots:
                _update_shot_after_render(
                    conn, shot["id"],
                    cost_yuan=est["videos"] / max(1, len(shots)),
                    model_used=video_model or "doubao-seedance-2-0-260128",
                    qa_passed=True,
                )
            conn.execute(
                """UPDATE render_jobs SET status = 'completed',
                                          completed_at = datetime('now')
                   WHERE id = ?""",
                (job_pk,),
            )
        click.echo(f"✓ DRY RUN complete — DB rows written for {character_id}/{episode_id}")
        click.echo(f"  run `vivify episode show {character_id} {episode_id}` to inspect")
        return

    # 6. Driver path (new — in-process per-shot asset pipeline)
    if use_driver:
        char_dir = Path(char["dir_path"])
        char_yaml = char.get("character_yaml_path")
        canonical_ref = _driver_resolve_canonical_ref(char_dir, char_yaml)
        voice_profile_dict = _driver_resolve_voice_profile(char_yaml, voice)
        env = obj.get("env") or _load_vendor_env()
        click.echo(f"\n[vivify] driver path: "
                   f"{n_shots} shots, parallel={parallel}, "
                   f"canonical={'yes' if canonical_ref else 'no'}\n")
        t0 = time.time()

        # 6a. Dry-run-with-driver: stub generate_asset + _call_tts so the full
        # per-shot pipeline runs end-to-end (DB writes, cost aggregation,
        # driver ↔ orchestrator interface) without spending money.
        # Without this, `--dry-run --use-driver` would still call real
        # Ark/海螺 APIs.
        _stub_active = False
        _orig_generate_asset = None
        _orig_call_tts = None
        if dry_run:
            from vivify.providers.base import GenerateResult
            from vivify import episode_driver as _driver_mod

            _work_dir = Path(out_dir) / "work"
            _work_dir.mkdir(parents=True, exist_ok=True)

            def _fake_generate_asset(req, **kwargs):
                ext = "jpg" if req.asset_type == "image" else "mp4"
                local = _work_dir / f"{req.scene_id}.{ext}"
                local.parent.mkdir(parents=True, exist_ok=True)
                local.write_bytes(b"fake")
                cost = 0.20 if req.asset_type == "image" else 1.05
                return GenerateResult(
                    ok=True, provider="ark", model="fake",
                    local_path=local, cost_yuan=cost, duration_ms=100,
                )

            def _fake_call_tts(req, env, voice_profile, out_path):
                out_path = Path(out_path)
                out_path.parent.mkdir(parents=True, exist_ok=True)
                out_path.write_bytes(b"fake-mp3")
                return GenerateResult(
                    ok=True, provider="minimax", model="fake-tts",
                    local_path=out_path, cost_yuan=0.01, duration_ms=100,
                )

            # Save originals BEFORE patching so we can restore in finally.
            _orig_generate_asset = _driver_mod.generate_asset
            _orig_call_tts = _driver_mod._call_tts
            _driver_mod.generate_asset = _fake_generate_asset
            _driver_mod._call_tts = _fake_call_tts
            _stub_active = True
            click.echo("  [dry-run] generate_asset + _call_tts stubbed "
                       "(no API calls, no spend)")

        try:
            asset_results = _render_episode_assets(
                shots=shots,
                voiceovers=_parse_voiceovers(script, storyboard),
                episode_id=episode_id,
                character_id=character_id,
                env=env,
                out_dir=Path(out_dir) / "work",
                db_path=db_path,
                voice_profile=voice_profile_dict,
                canonical_ref=canonical_ref,
                episode_pk=ep_pk,
                monthly_hard=60000.0,
            )
        finally:
            # Restore real generate_asset + _call_tts so the stubs don't
            # leak into subsequent calls (pytest test ordering breaks if
            # we don't).
            if _stub_active:
                from vivify import episode_driver as _dm
                _dm.generate_asset = _orig_generate_asset
                _dm._call_tts = _orig_call_tts

        real_total_cost = sum(r.total_cost_yuan for r in asset_results)
        elapsed = time.time() - t0
        click.echo(f"\n[driver] {n_shots} shots done in {elapsed:.0f}s, "
                   f"real cost ¥{real_total_cost:.2f}")

        # 6a. Mux with ffmpeg
        final_mp4 = Path(out_path)
        mux_ok = _mux_episode(
            asset_results,
            voiceovers=_parse_voiceovers(script, storyboard),
            env=env,
            out_path=final_mp4,
            target_dur=target_dur,
            title=title,
            next_episode=next_episode,
            voice=voice,
        )
        # 6b. DB finalize with REAL cost
        actual_dur = _ffprobe_duration(str(final_mp4)) if mux_ok else None
        file_size = _file_size(str(final_mp4)) if mux_ok else None
        with connect(db_path) as conn:
            if mux_ok and final_mp4.exists():
                conn.execute(
                    """UPDATE episodes SET
                        status = 'completed',
                        output_path = ?, actual_dur_sec = ?,
                        file_size_bytes = ?, cost_yuan = ?,
                        render_completed_at = datetime('now')
                       WHERE id = ?""",
                    (str(final_mp4), actual_dur, file_size,
                     real_total_cost, ep_pk),
                )
                conn.execute(
                    """UPDATE render_jobs SET status = 'completed',
                                              completed_at = datetime('now')
                       WHERE id = ?""",
                    (job_pk,),
                )
                click.echo(f"\n✅ driver render OK in {elapsed:.0f}s — "
                           f"{final_mp4} (¥{real_total_cost:.2f})")
            else:
                err_msg = "ffmpeg mux produced no output (driver path)"
                conn.execute(
                    """UPDATE episodes SET status = 'failed',
                        error_message = ?,
                        render_completed_at = datetime('now')
                       WHERE id = ?""",
                    (err_msg, ep_pk),
                )
                conn.execute(
                    """UPDATE render_jobs SET status = 'failed',
                                              completed_at = datetime('now'),
                                              error_message = ?
                       WHERE id = ?""",
                    (err_msg, job_pk),
                )
                conn.commit()
                click.echo(f"\n❌ driver render FAILED — mux produced no output",
                           err=True)
                sys.exit(1)
        return

    # 6. Spawn render_episode.py as subprocess
    cmd = [
        sys.executable, "render_episode.py",
        "--storyboard", str(storyboard),
        "--voice", voice,
        "--platform", platform,
        "--target-dur", str(target_dur),
        "--out", out_path,
        "--image-model", image_model,
        "--quality-tier", quality_tier,
        "--character-dir", char["dir_path"],
    ]
    if video_provider:
        cmd += ["--video-provider", video_provider]
    if script:
        cmd += ["--script", str(script)]
    if video_model:
        cmd += ["--video-model", video_model]
    if reference_image:
        cmd += ["--reference-image", reference_image]
    if require_lip_sync:
        cmd += ["--require-lip-sync"]
    if qa_skip:
        cmd += ["--qa-skip"]
    if title:
        cmd += ["--title", title]
    cmd += ["--next-episode", next_episode]
    if parallel and parallel > 1:
        cmd += ["--parallel", str(parallel)]
    if max_retries and max_retries != 3:
        cmd += ["--max-retries", str(max_retries)]
    if retry_only:
        cmd += ["--retry-only"]

    click.echo(f"\n[vivify] spawning: {' '.join(cmd)}\n")
    t0 = time.time()
    rc = subprocess.call(cmd)
    elapsed = time.time() - t0

    # 7. Post-render: ffprobe + DB finalize
    actual_dur = _ffprobe_duration(out_path)
    file_size = _file_size(out_path)

    # Update shot rows with computed file paths (best-effort)
    # render_episode.py writes to $VIVIFY_RENDER_DIR/work/ (default /tmp/vivify-render/work/).
    with connect(db_path) as conn:
        shots = _get_shots(conn, ep_pk)
        render_work = Path(os.environ.get("VIVIFY_RENDER_DIR", "/tmp/vivify-render")) / "work"
        for shot in shots:
            img_p = render_work / "01-image" / f"shot-{shot['shot_number']:02d}.jpg"
            vid_p = render_work / "02-video" / f"shot-{shot['shot_number']:02d}.mp4"
            _update_shot_after_render(
                conn, shot["id"],
                image_path=str(img_p) if img_p.exists() else None,
                video_path=str(vid_p) if vid_p.exists() else None,
                cost_yuan=est["videos"] / max(1, len(shots)),
                model_used=video_model or "doubao-seedance-2-0-260128",
            )

        if rc == 0:
            conn.execute(
                """UPDATE episodes SET
                    status = 'completed',
                    output_path = ?,
                    actual_dur_sec = ?,
                    file_size_bytes = ?,
                    cost_yuan = ?,
                    render_completed_at = datetime('now')
                   WHERE id = ?""",
                (out_path, actual_dur, file_size, est["total"], ep_pk),
            )
            conn.execute(
                """UPDATE render_jobs SET status = 'completed',
                                          completed_at = datetime('now')
                   WHERE id = ?""",
                (job_pk,),
            )
            click.echo(f"\n✅ render OK in {elapsed:.0f}s — {out_path}")
        else:
            err_msg = f"render_episode.py exited with code {rc}"
            conn.execute(
                """UPDATE episodes SET
                    status = 'failed',
                    error_message = ?,
                    render_completed_at = datetime('now')
                   WHERE id = ?""",
                (err_msg, ep_pk),
            )
            conn.execute(
                """UPDATE render_jobs SET status = 'failed',
                                          completed_at = datetime('now'),
                                          error_message = ?,
                                          retries = retries + 1
                   WHERE id = ?""",
                (err_msg, job_pk),
            )
            conn.commit()  # explicit commit before sys.exit (SystemExit skips the with-block's auto-commit)
            click.echo(f"\n❌ render FAILED (rc={rc}) — episode marked failed in DB",
                       err=True)
            sys.exit(rc)


# ---- status ---------------------------------------------------------------

@cli.command("status")
@click.argument("character_id")
@click.argument("episode_id")
@click.pass_obj
def status_cmd(obj, character_id, episode_id):
    """Quick status check (concise alternative to `show`)."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        ep = _resolve_episode(conn, character_id, episode_id)
        if not ep:
            raise click.ClickException(f"episode {character_id}/{episode_id} not found")
        n_shots = conn.execute(
            "SELECT COUNT(*) FROM shots WHERE episode_id = ?", (ep["id"],)
        ).fetchone()[0]
        n_jobs = conn.execute(
            "SELECT COUNT(*) FROM render_jobs WHERE episode_id = ?", (ep["id"],)
        ).fetchone()[0]
        last_job = conn.execute(
            "SELECT * FROM render_jobs WHERE episode_id = ? ORDER BY id DESC LIMIT 1",
            (ep["id"],),
        ).fetchone()

    color = {"pending": "yellow", "rendering": "blue", "completed": "green",
             "failed": "red"}.get(ep["status"], "white")
    click.echo(click.style(f"{ep['status']}", fg=color, bold=True) +
               f"  {character_id}/{episode_id}")
    click.echo(f"  shots:      {n_shots}")
    click.echo(f"  jobs:       {n_jobs}")
    click.echo(f"  cost:       {format_yuan(ep['cost_yuan'])}")
    click.echo(f"  duration:   {ep['actual_dur_sec'] or '—'}s "
               f"(target {ep['target_dur_sec'] or '—'}s)")
    click.echo(f"  output:     {ep['output_path'] or '—'}")
    if last_job:
        job = dict(last_job)
        click.echo(f"  last job:   #{job['id']} {job['status']} "
                   f"({job.get('started_at') or '—'} → "
                   f"{job.get('completed_at') or 'running…'})")
    if ep.get("error_message"):
        click.echo(f"  ⚠ {ep['error_message']}", err=True)


# ---- delete ---------------------------------------------------------------

@cli.command("delete")
@click.argument("character_id")
@click.argument("episode_id")
@click.option("--yes", "-y", is_flag=True, help="Skip confirmation.")
@click.pass_obj
def delete_cmd(obj, character_id, episode_id, yes):
    """Delete an episode and all its shots/jobs from the DB."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        ep = _resolve_episode(conn, character_id, episode_id)
        if not ep:
            raise click.ClickException(f"episode {character_id}/{episode_id} not found")
        n_shots = conn.execute(
            "SELECT COUNT(*) FROM shots WHERE episode_id = ?", (ep["id"],)
        ).fetchone()[0]
        n_jobs = conn.execute(
            "SELECT COUNT(*) FROM render_jobs WHERE episode_id = ?", (ep["id"],)
        ).fetchone()[0]
        if not yes:
            click.confirm(
                f"Delete {character_id}/{episode_id}? "
                f"({n_shots} shots, {n_jobs} jobs will also be deleted)",
                abort=True,
            )
        conn.execute("DELETE FROM render_jobs WHERE episode_id = ?", (ep["id"],))
        conn.execute("DELETE FROM shots WHERE episode_id = ?", (ep["id"],))
        conn.execute("DELETE FROM publishes WHERE episode_id = ?", (ep["id"],))
        conn.execute("DELETE FROM episodes WHERE id = ?", (ep["id"],))
    click.echo(f"✓ deleted {character_id}/{episode_id} "
               f"({n_shots} shots, {n_jobs} jobs)")


# --- driver-path helpers ----------------------------------------------------

def _load_vendor_env() -> dict:
    """Read vendor credentials from env. Returns dict with whatever the
    parent process has in os.environ (ARK_API_KEY, MINIMAX_API_KEY, etc.).
    Optionally overlays ~/.claude/config/vivify-volcengine.env if present.
    """
    env = dict(os.environ)
    env_file = Path.home() / ".claude" / "config" / "vivify-volcengine.env"
    if env_file.exists():
        for line in env_file.read_text(encoding="utf-8").splitlines():
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, _, v = line.partition("=")
            env[k.strip()] = v.strip().strip('"').strip("'")
    return env


def _parse_voiceovers(script_path: str | None, storyboard_path: str) -> list[dict]:
    """Parse voiceovers from script via render_episode.parse_script.
    Returns [] on missing script or parse error (best-effort)."""
    if not script_path or not Path(script_path).exists():
        return []
    try:
        import sys as _sys
        fw_dir = str(Path(__file__).resolve().parent.parent.parent)
        if fw_dir not in _sys.path:
            _sys.path.insert(0, fw_dir)
        from render_episode import parse_script  # type: ignore
        return parse_script(script_path)
    except Exception as e:
        click.echo(f"[warn] could not parse script: {e}", err=True)
        return []


def _mux_episode(asset_results, *, voiceovers, env, out_path, target_dur,
                 title, next_episode, voice) -> bool:
    """Concatenate per-shot image+video+tts into the final MP4 using
    render_episode.ff_* helpers. Best-effort: returns False if mux fails
    (caller marks the episode as failed). Returns True on success."""
    try:
        import sys as _sys
        fw_dir = str(Path(__file__).resolve().parent.parent.parent)
        if fw_dir not in _sys.path:
            _sys.path.insert(0, fw_dir)
        from render_episode import (  # type: ignore
            ff_concat, ff_apply_subtitles, ff_mix_audio, ff_mux_final,
            ff_text_card, ff_synth_bgm,
        )
    except ImportError as e:
        click.echo(f"[warn] render_episode helpers not importable: {e}; "
                   f"skipping mux", err=True)
        return False

    out_path = Path(out_path)
    work = out_path.parent / "work"
    work.mkdir(parents=True, exist_ok=True)
    concat_list = work / "concat.txt"
    audio_inputs = []
    lines = []
    cum_dur = 0.0

    # Title card (optional)
    if title:
        title_path = work / "title.jpg"
        try:
            ff_text_card(env, title, dur=2, out_path=str(title_path))
            lines.append(f"file '{title_path}'\nduration 2.0\n")
            cum_dur += 2.0
        except Exception as e:
            click.echo(f"[warn] title card failed: {e}", err=True)

    for r in asset_results:
        if r.video_path and Path(r.video_path).exists():
            lines.append(f"file '{r.video_path}'\n")
            cum_dur += 5.0  # approximation; per-shot duration is in shot row
        if r.tts_path and Path(r.tts_path).exists():
            audio_inputs.append((int(cum_dur), str(r.tts_path)))

    # End card
    end_path = work / "end.jpg"
    try:
        ff_text_card(env, next_episode, dur=2, out_path=str(end_path))
        lines.append(f"file '{end_path}'\nduration 2.0\n")
    except Exception as e:
        click.echo(f"[warn] end card failed: {e}", err=True)

    if not lines:
        click.echo("[warn] no per-shot assets to mux", err=True)
        return False

    concat_list.write_text("".join(lines))
    raw_concat = work / "raw.mp4"
    try:
        ff_concat(env, str(concat_list), str(raw_concat))
    except Exception as e:
        click.echo(f"[warn] ff_concat failed: {e}", err=True)
        return False

    subbed = work / "subbed.mp4"
    try:
        vo_text = "\n".join(v.get("text", "") for v in voiceovers)
        ff_apply_subtitles(env, str(raw_concat), str(subbed),
                           voiceover_text=vo_text)
    except Exception as e:
        click.echo(f"[warn] ff_apply_subtitles failed: {e}", err=True)
        # Fall back: just rename raw → subbed
        subbed.write_bytes(raw_concat.read_bytes())

    if audio_inputs:
        try:
            bgm_path = work / "bgm.mp3"
            ff_synth_bgm(env, str(bgm_path),
                         dur_sec=int(cum_dur + 4), tone=voice)
            mixed = work / "mixed.mp3"
            ff_mix_audio(env, str(bgm_path), audio_inputs, str(mixed))
            ff_mux_final(env, str(subbed), str(mixed), str(out_path))
        except Exception as e:
            click.echo(f"[warn] audio mux failed: {e}; "
                       f"writing video without audio", err=True)
            out_path.write_bytes(subbed.read_bytes())
    else:
        out_path.write_bytes(subbed.read_bytes())

    return out_path.exists()


__all__ = ["cli"]
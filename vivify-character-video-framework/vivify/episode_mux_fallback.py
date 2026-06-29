"""vivify.episode_mux_fallback — graceful failure path when final MP4 mux breaks.

When ffmpeg is missing OR the concat/mux step errors out, the per-shot
assets are still on disk and represent real spend (Ark/海螺 API calls).
Marking the episode as `failed` is misleading — the user got usable
artifacts, just not a concatenated MP4.

This module lets the render driver record the partial-success state:

  1. write_manifest(out_dir, asset_results, mux_failure_reason)
       Writes <out_dir>/MANIFEST.json describing each per-shot asset
       plus a copy-paste ffmpeg command to retry the mux manually.
  2. mark_episode_assets_only(db_path, episode_pk, manifest_path, mux_error)
       Flips the episode row to status='assets_only' and inserts a
       render_job row recording the partial success.

The companion CLI subcommand `vivify episode mux <ip> <ep>` reads the
manifest and re-runs only the concat step — useful right after the user
installs ffmpeg.

Pure logic: no Click dependency, no DB connection held beyond the
single update. Safe to import from drivers, retries, or tests.
"""

import json
import shutil
import sqlite3
import subprocess
from pathlib import Path
from typing import Iterable, Union


# Where the work directory lives relative to the rendered final MP4.
# render_episode.ff_* helpers put per-shot assets under <out>/work/.
# This constant is intentionally duplicated from episode.py rather than
# imported — episode.py owns it, and we want this module standalone.
WORK_DIR_NAME = "work"


def _install_hint() -> str:
    """Best-effort platform-specific install hint for ffmpeg."""
    # We don't actually shell out for `uname` — the message is generic enough
    # that 'apt' / 'brew' cover 95% of cases.
    return (
        "apt install ffmpeg (Debian/Ubuntu) | "
        "brew install ffmpeg (macOS) | "
        "choco install ffmpeg (Windows)"
    )


def _ffmpeg_command_template(out_dir: Path) -> str:
    """Return a copy-paste ffmpeg command the user can run by hand.

    We use the concat demuxer with the concat.txt + audio.mp3 layout
    that render_episode.ff_mux_final produces. This is informational —
    the real `episode mux` subcommand wraps the same operation with
    better error handling.
    """
    out_dir = Path(out_dir).expanduser()
    concat_list = out_dir / WORK_DIR_NAME / "concat.txt"
    audio = out_dir / WORK_DIR_NAME / "audio.mp3"
    final = out_dir / "final.mp4"
    return (
        f"ffmpeg -f concat -safe 0 -i {concat_list} "
        f"-i {audio} -c:v copy -c:a aac {final}"
    )


def write_manifest(
    out_dir: Union[str, Path],
    asset_results: Iterable,
    mux_failure_reason: str,
    *,
    episode_id: str = None,
    character_id: str = None,
) -> Path:
    """Write <out_dir>/MANIFEST.json describing per-shot assets.

    Args:
        out_dir: Directory that would have held the final MP4. The
            manifest is written as <out_dir>/MANIFEST.json. The work
            directory is expected at <out_dir>/work/.
        asset_results: Iterable of ShotAssetResult (or any object with
            `shot_number`, `image_path`, `video_path`, `tts_path`).
            We don't import ShotAssetResult to keep this module
            decoupled from episode_driver.
        mux_failure_reason: Human-readable reason the mux failed
            (e.g. "ffmpeg not found in PATH", "concat returned no
            output"). Goes into the manifest verbatim.
        episode_id, character_id: Optional metadata. Pulled from the
            episode row when called from the driver; safe to omit.

    Returns:
        Path to the written MANIFEST.json.
    """
    out_dir = Path(out_dir).expanduser()
    out_dir.mkdir(parents=True, exist_ok=True)

    shots_json = []
    for r in asset_results:
        # Tolerate dict-like and dataclass-like results. dataclass gives
        # `r.shot_number`; the test suite may pass dicts.
        if isinstance(r, dict):
            n = r.get("shot_number") or r.get("n")
            image = r.get("image_path")
            video = r.get("video_path")
            tts = r.get("tts_path")
        else:
            n = getattr(r, "shot_number", None)
            image = getattr(r, "image_path", None)
            video = getattr(r, "video_path", None)
            tts = getattr(r, "tts_path", None)

        def _rel(p):
            if p is None:
                return None
            p = Path(p)
            if not p.is_absolute():
                return str(p)
            # Make relative to out_dir when possible so the manifest is
            # portable (the user can move the work tree and the manifest
            # still references the right files).
            try:
                return str(p.relative_to(out_dir))
            except ValueError:
                return str(p)

        shots_json.append({
            "n": int(n) if n is not None else None,
            "image": _rel(image),
            "video": _rel(video),
            "tts": _rel(tts),
            # duration_sec isn't on ShotAssetResult itself; the caller
            # patches it in via the asset_results list. We leave it
            # null here — callers can override by passing dicts with
            # the field set.
            "duration_sec": (r.get("duration_sec") if isinstance(r, dict) else None),
        })

    manifest = {
        "episode_id": episode_id,
        "character_id": character_id,
        "mux_status": "failed",
        "mux_failure_reason": str(mux_failure_reason),
        "shots": shots_json,
        "ffmpeg_command_template": _ffmpeg_command_template(out_dir),
        "install_hint": _install_hint(),
        "out_dir": str(out_dir),
    }
    manifest_path = out_dir / "MANIFEST.json"
    manifest_path.write_text(
        json.dumps(manifest, indent=2, ensure_ascii=False),
        encoding="utf-8",
    )
    return manifest_path


def mark_episode_assets_only(
    db_path: str,
    episode_pk: int,
    manifest_path: Union[str, Path],
    mux_error: str,
) -> int:
    """Flip an episode to `assets_only` status + log a render_job row.

    The episode's `output_path` is set to the manifest path so users can
    find the partial-success package from `episode show`.

    Returns the render_jobs.id of the inserted row.
    """
    db_path = str(db_path)
    manifest_path = str(Path(manifest_path))
    conn = sqlite3.connect(db_path)
    try:
        conn.row_factory = sqlite3.Row
        conn.execute("PRAGMA foreign_keys = ON")
        # Build the UPDATE dynamically so it works whether or not migration
        # 002 (episodes.updated_at) has been applied. Tests that init a
        # bare-bones schema shouldn't have to run the migrator.
        ep_cols = {
            row["name"] for row in conn.execute("PRAGMA table_info(episodes)")
        }
        set_clauses = [
            "status = 'assets_only'",
            "output_path = ?",
            "error_message = ?",
            "render_completed_at = datetime('now')",
        ]
        params: list = [manifest_path, str(mux_error)]
        if "updated_at" in ep_cols:
            set_clauses.append("updated_at = datetime('now')")
        sql = (
            f"UPDATE episodes SET {', '.join(set_clauses)} WHERE id = ?"
        )
        params.append(int(episode_pk))
        conn.execute(sql, params)
        cur = conn.execute(
            """INSERT INTO render_jobs (
                episode_id, status, started_at, completed_at,
                error_message, retries
            ) VALUES (
                ?, 'assets_only', datetime('now'), datetime('now'),
                ?, 0
            )""",
            (int(episode_pk), f"mux failed; assets_only. {mux_error}"),
        )
        job_pk = cur.lastrowid
        conn.commit()
        return int(job_pk)
    finally:
        conn.close()


def _resolve_manifest(manifest_path: Union[str, Path]) -> dict:
    """Read MANIFEST.json. Raises FileNotFoundError if missing."""
    p = Path(manifest_path)
    if not p.exists():
        raise FileNotFoundError(f"manifest not found: {p}")
    return json.loads(p.read_text(encoding="utf-8"))


def _find_ffmpeg() -> str | None:
    """Locate ffmpeg on PATH. Returns None if missing."""
    found = shutil.which("ffmpeg")
    return found


def run_mux_from_manifest(
    manifest_path: Union[str, Path],
    final_mp4: Union[str, Path] = None,
    *,
    db_path: str = None,
    episode_pk: int = None,
) -> dict:
    """Re-run only the mux step from a MANIFEST.json.

    This is the engine behind `vivify episode mux`. It:

      1. Locates ffmpeg (returns an error dict if missing).
      2. Builds the concat list from the manifest's `shots` array.
      3. Calls ffmpeg.
      4. On success: optionally updates the episode row to `completed`.

    Returns a dict with keys: `ok` (bool), `output_path` (str | None),
    `error` (str | None). The CLI subcommand formats this for the user.
    """
    try:
        manifest = _resolve_manifest(manifest_path)
    except FileNotFoundError as e:
        return {"ok": False, "output_path": None, "error": str(e)}

    manifest_dir = Path(manifest_path).expanduser().parent
    out_dir = Path(manifest.get("out_dir") or manifest_dir)
    work = out_dir / WORK_DIR_NAME
    work.mkdir(parents=True, exist_ok=True)

    ffmpeg = _find_ffmpeg()
    if not ffmpeg:
        return {
            "ok": False,
            "output_path": None,
            "error": "ffmpeg not found in PATH",
        }

    # Build concat list from manifest entries. Only shots with a video
    # file are concatenated. TTS audio is muxed in a second pass if
    # present.
    concat_list = work / "concat.txt"
    lines = []
    audio_inputs = []
    cum_dur = 0.0
    for s in manifest.get("shots", []):
        video_rel = s.get("video")
        if not video_rel:
            continue
        video_abs = (out_dir / video_rel).resolve() \
            if not Path(video_rel).is_absolute() \
            else Path(video_rel)
        if not video_abs.exists():
            continue
        lines.append(f"file '{video_abs}'\n")
        # We don't have per-shot duration in the manifest's shots; default
        # to 5s (matches render_episode.ff_mux_final). End cards are added
        # by render_episode; the retry mux here just stitches shots.
        cum_dur += 5.0
        tts_rel = s.get("tts")
        if tts_rel:
            tts_abs = (out_dir / tts_rel).resolve() \
                if not Path(tts_rel).is_absolute() \
                else Path(tts_rel)
            if tts_abs.exists():
                audio_inputs.append((int(cum_dur), str(tts_abs)))

    if not lines:
        return {
            "ok": False,
            "output_path": None,
            "error": "manifest has no per-shot video entries",
        }

    concat_list.write_text("".join(lines), encoding="utf-8")

    final = Path(final_mp4).expanduser() if final_mp4 else (out_dir / "final.mp4")
    final.parent.mkdir(parents=True, exist_ok=True)

    # Pass 1: concat video
    raw = work / "raw.mp4"
    cmd1 = [ffmpeg, "-y", "-f", "concat", "-safe", "0",
            "-i", str(concat_list), "-c", "copy", str(raw)]
    r1 = subprocess.run(cmd1, capture_output=True, text=True, timeout=600)
    if r1.returncode != 0 or not raw.exists():
        return {
            "ok": False,
            "output_path": None,
            "error": f"ffmpeg concat failed: {r1.stderr[-500:]}",
        }

    # Pass 2 (optional): mux audio. If no TTS, just copy raw → final.
    if audio_inputs:
        # Build -itsoffset tags for each audio input.
        audio_args = []
        filter_parts = []
        for offset, p in audio_inputs:
            audio_args += ["-ss", str(offset), "-i", p]
            filter_parts.append(f"[{len(audio_args) // 2}:a]")
        # amix all inputs; 5s dropout-ignore at start.
        if len(audio_inputs) > 1:
            filter_complex = (
                f"{''.join(filter_parts)}amix=inputs={len(audio_inputs)}:"
                f"dropout_transition=0[aout]"
            )
            cmd2 = [ffmpeg, "-y", "-i", str(raw)] + audio_args + [
                "-filter_complex", filter_complex,
                "-map", "0:v", "-map", "[aout]",
                "-c:v", "copy", "-c:a", "aac", str(final),
            ]
        else:
            cmd2 = [ffmpeg, "-y", "-i", str(raw)] + audio_args + [
                "-map", "0:v", "-map", "1:a",
                "-c:v", "copy", "-c:a", "aac", str(final),
            ]
        r2 = subprocess.run(cmd2, capture_output=True, text=True, timeout=600)
        if r2.returncode != 0:
            # Fall back to video-only final
            final.write_bytes(raw.read_bytes())
    else:
        final.write_bytes(raw.read_bytes())

    if not final.exists():
        return {
            "ok": False,
            "output_path": None,
            "error": "ffmpeg produced no output file",
        }

    # Optional DB finalize
    if db_path is not None and episode_pk is not None:
        try:
            file_size = final.stat().st_size
            conn = sqlite3.connect(str(db_path))
            try:
                conn.row_factory = sqlite3.Row
                ep_cols = {
                    row["name"] for row in conn.execute(
                        "PRAGMA table_info(episodes)")
                }
                set_clauses = [
                    "status = 'completed'",
                    "output_path = ?",
                    "file_size_bytes = ?",
                    "render_completed_at = datetime('now')",
                ]
                params: list = [str(final), file_size]
                if "updated_at" in ep_cols:
                    set_clauses.append("updated_at = datetime('now')")
                params.append(int(episode_pk))
                conn.execute(
                    f"UPDATE episodes SET {', '.join(set_clauses)} "
                    f"WHERE id = ?",
                    params,
                )
                conn.execute(
                    """INSERT INTO render_jobs (
                        episode_id, status, started_at, completed_at
                    ) VALUES (
                        ?, 'completed', datetime('now'), datetime('now')
                    )""",
                    (int(episode_pk),),
                )
                conn.commit()
            finally:
                conn.close()
        except Exception as e:
            # The file is fine; DB update is best-effort.
            return {
                "ok": True,
                "output_path": str(final),
                "error": f"file OK but DB update failed: {e}",
            }

    return {"ok": True, "output_path": str(final), "error": None}


__all__ = [
    "WORK_DIR_NAME",
    "write_manifest",
    "mark_episode_assets_only",
    "run_mux_from_manifest",
]
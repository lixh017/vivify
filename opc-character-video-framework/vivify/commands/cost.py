"""vivify.commands.cost — cost-cap status + history commands.

Lets the user see:
  - this month's running spend (sum of episodes.cost_yuan)
  - per-episode costs (with breakdown by provider/model)
  - cap status (per-video soft/hard, monthly hard)

Reads from the DB. Does NOT mutate anything.

Commands:
  status                 Current month spend + cap headroom
  history  [--month M]   Per-episode spend, sorted newest-first
  estimate <vid>         What would this model/duration cost?
"""

from datetime import datetime, timezone

import click

from ..cost_cap import (
    DEFAULT_MONTHLY_HARD,
    DEFAULT_PER_VIDEO_HARD,
    DEFAULT_PER_VIDEO_SOFT,
    current_month,
    monthly_spend_yuan,
    render_cost_summary,
)
from ..db import connect, init_db
from ..pricing import format_yuan


@click.group("cost")
def cli():
    """Cost tracking + cap enforcement."""
    pass


@cli.command("status")
@click.option("--month", "-m", default=None, help="YYYY-MM (default: current).")
@click.option("--per-video-hard", default=DEFAULT_PER_VIDEO_HARD,
              help="Per-video hard cap in ¥ (default 100).")
@click.option("--monthly-hard", default=DEFAULT_MONTHLY_HARD,
              help="Monthly hard cap in ¥ (default 60000).")
@click.pass_obj
def status_cmd(obj, month, per_video_hard, monthly_hard):
    """Show this month's running spend vs cap."""
    db_path = obj.get("db_path")
    init_db(db_path)
    month = month or current_month()
    with connect(db_path) as conn:
        spent = monthly_spend_yuan(conn, month)
        n_done = conn.execute(
            "SELECT COUNT(*) FROM episodes WHERE render_completed_at LIKE ?",
            (f"{month}%",),
        ).fetchone()[0]
        n_failed = conn.execute(
            "SELECT COUNT(*) FROM episodes WHERE render_completed_at LIKE ? AND status='failed'",
            (f"{month}%",),
        ).fetchone()[0]
        # Cheapest per-episode cost this month
        rows = conn.execute(
            """SELECT episode_id, character_id, cost_yuan, model_used,
                      render_completed_at
               FROM episodes
               WHERE render_completed_at LIKE ?
                 AND cost_yuan IS NOT NULL
               ORDER BY cost_yuan DESC
               LIMIT 3""",
            (f"{month}%",),
        ).fetchall()

    headroom = monthly_hard - spent
    pct = (spent / monthly_hard * 100) if monthly_hard else 0
    color = "green" if pct < 60 else ("yellow" if pct < 90 else "red")

    click.echo(f"═══ Cost status ({month}) ═══")
    click.echo(click.style(f"  spent:        {format_yuan(spent)} / {format_yuan(monthly_hard)} "
                           f"({pct:.1f}%)", fg=color, bold=True))
    click.echo(f"  headroom:     {format_yuan(headroom)}")
    click.echo(f"  episodes:     {n_done} done, {n_failed} failed")
    click.echo(f"  caps:         per_video_soft=¥{DEFAULT_PER_VIDEO_SOFT:.0f}  "
               f"per_video_hard=¥{per_video_hard:.0f}  monthly_hard=¥{monthly_hard:.0f}")
    if rows:
        click.echo("")
        click.echo(f"  Top 3 most expensive (this month):")
        for r in rows:
            click.echo(f"    {r['character_id']}/{r['episode_id']}  "
                       f"{format_yuan(r['cost_yuan'])}  "
                       f"{(r['model_used'] or '—')[:30]}  {r['render_completed_at']}")


@cli.command("history")
@click.option("--month", "-m", default=None)
@click.option("--character", "-c", default=None)
@click.option("--limit", "-n", default=20, type=int)
@click.pass_obj
def history_cmd(obj, month, character, limit):
    """Per-episode cost history."""
    db_path = obj.get("db_path")
    init_db(db_path)
    month = month or current_month()
    with connect(db_path) as conn:
        q = """SELECT character_id, episode_id, model_used, cost_yuan,
                      actual_dur_sec, render_completed_at, status
               FROM episodes
               WHERE render_completed_at LIKE ?"""
        params: list = [f"{month}%"]
        if character:
            q += " AND character_id = ?"
            params.append(character)
        q += " ORDER BY render_completed_at DESC LIMIT ?"
        params.append(limit)
        rows = conn.execute(q, params).fetchall()
        total = monthly_spend_yuan(conn, month)

    click.echo(f"═══ Cost history — {month}"
               f"{f' (filter: {character})' if character else ''} ═══")
    click.echo(f"{'CHAR':<10} {'EP':<8} {'STATUS':<10} {'COST':<10} "
               f"{'DUR':<7} {'MODEL':<32} COMPLETED")
    click.echo("─" * 95)
    for r in rows:
        click.echo(
            f"{r['character_id']:<10} "
            f"{r['episode_id']:<8} "
            f"{(r['status'] or '—'):<10} "
            f"{format_yuan(r['cost_yuan']):<10} "
            f"{(str(r['actual_dur_sec']) + 's' if r['actual_dur_sec'] else '—'):<7} "
            f"{(r['model_used'] or '—')[:32]:<32} "
            f"{r['render_completed_at'] or '—'}"
        )
    click.echo(f"\n  total {month}: {format_yuan(total)}")


@cli.command("estimate")
@click.option("--model", "-m", required=True,
              help="Model id (e.g. 'doubao-seedance-1-5-pro-251215' or 'MiniMax-Hailuo-2.3').")
@click.option("--shots", "-s", required=True, type=int,
              help="Number of shots (image gen calls).")
@click.option("--video-sec", "-v", required=True, type=float,
              help="Total seconds of video to generate.")
@click.option("--voiceover-chars", default=0, type=int,
              help="Total voiceover characters (for TTS cost estimate).")
@click.pass_obj
def estimate_cmd(obj, model, shots, video_sec, voiceover_chars):
    """Estimate cost for a planned render."""
    from ..pricing import estimate_episode_cost
    est = estimate_episode_cost(
        n_shots=shots,
        total_video_sec=video_sec,
        n_voiceovers=voiceover_chars // 30 or 1,  # rough: avg 30 chars/shot
        avg_vo_chars=voiceover_chars // max(1, voiceover_chars // 30 or 1),
        video_model=model,
    )
    click.echo(f"Cost estimate for model={model}:")
    click.echo(f"  images ({shots} shots):  {format_yuan(est['images'])}")
    click.echo(f"  video  ({video_sec}s):  {format_yuan(est['videos'])}")
    click.echo(f"  tts    ({voiceover_chars} chars):  {format_yuan(est['tts'])}")
    click.echo(f"  ─────────────────────")
    click.echo(f"  total:                  {format_yuan(est['total'])}")
    click.echo(f"")
    click.echo(f"  vs caps: per_video_soft=¥{DEFAULT_PER_VIDEO_SOFT:.0f}  "
               f"per_video_hard=¥{DEFAULT_PER_VIDEO_HARD:.0f}")
    if est["total"] > DEFAULT_PER_VIDEO_HARD:
        click.echo(click.style(
            f"  ⚠ ABOVE per_video_hard (¥{DEFAULT_PER_VIDEO_HARD:.0f}) — "
            f"episode render will be blocked without --force", fg="red"))
    elif est["total"] > DEFAULT_PER_VIDEO_SOFT:
        click.echo(click.style(
            f"  ⚠ above per_video_soft (¥{DEFAULT_PER_VIDEO_SOFT:.0f}) — within hard limit",
            fg="yellow"))


__all__ = ["cli"]
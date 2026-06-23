"""vivify.commands.workflow — parallel execution + smart retry surface.

Uses ThreadPoolExecutor for shot-level parallelism within a render.
Uses vivify.retry for error classification (transient/permanent).

Commands:
  status                Show all in-flight + recently-failed jobs
  list   [--status X]   List render_jobs (any filter)
  retry  <job-id>       Requeue a failed job (with smart backoff classification)
  config                Show parallel/worker configuration
  retry-failed <ep>     Requeue all failed jobs for an episode

The actual parallel rendering is triggered via:
    vivify episode render --parallel N <char> <ep>

When --parallel N > 1, render_episode.py runs shots in a thread pool,
each with retry-aware gen_video() that classifies errors before retrying.
"""

from __future__ import annotations

import json
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass

import click

from ..db import connect, init_db
from ..retry import (
    FailureClass,
    RetryPolicy,
    classify_error,
    should_retry,
    sleep_for_retry,
)


DEFAULT_MAX_WORKERS = 4
DEFAULT_MAX_RETRIES = 3


@dataclass
class WorkflowConfig:
    """Runtime config for parallel rendering."""
    max_workers: int = DEFAULT_MAX_WORKERS
    max_retries: int = DEFAULT_MAX_RETRIES
    per_shot_timeout_sec: int = 900  # 15 min per shot

    def as_dict(self) -> dict:
        return {
            "max_workers": self.max_workers,
            "max_retries": self.max_retries,
            "per_shot_timeout_sec": self.per_shot_timeout_sec,
        }


def _gen_shot_with_retry(env, image_url, action, duration_sec, model,
                         out_path, audio_url=None, character_ref=None,
                         policy: RetryPolicy = None):
    """Call gen_video() with retry classification.

    Imports render_episode.gen_video at call time to avoid circular deps.
    Re-imported inside the function so each thread has clean module state.
    """
    import render_episode  # imported inside, not at module load
    policy = policy or RetryPolicy()

    last_err = None
    for attempt in range(1, policy.max_retries + 2):  # initial + retries
        try:
            render_episode.gen_video(
                env, image_url, action, duration_sec, model, out_path,
                audio_url=audio_url, character_ref=character_ref,
            )
            if attempt > 1:
                click.echo(f"  [retry {attempt}] shot succeeded after retries")
            return {"ok": True, "attempts": attempt}
        except SystemExit as e:
            # fatal() calls sys.exit(1) — capture message from stderr
            msg = str(e) if str(e) else "unknown fatal"
            last_err = msg
            retry, reason = should_retry(msg, attempt, policy)
            cls, sub_reason = classify_error(msg)
            if not retry:
                return {
                    "ok": False,
                    "attempts": attempt,
                    "error_class": cls.value,
                    "error_reason": sub_reason,
                    "error_message": msg[:500],
                    "will_retry": False,
                }
            click.echo(f"  [retry {attempt}] {cls.value}:{sub_reason} — sleeping "
                       f"{policy.delay_for_attempt(attempt):.0f}s")
            sleep_for_retry(attempt, policy)
        except Exception as e:
            last_err = str(e)
            retry, reason = should_retry(last_err, attempt, policy)
            cls, sub_reason = classify_error(last_err)
            if not retry:
                return {
                    "ok": False,
                    "attempts": attempt,
                    "error_class": cls.value,
                    "error_reason": sub_reason,
                    "error_message": last_err[:500],
                    "will_retry": False,
                }
            click.echo(f"  [retry {attempt}] {cls.value}:{sub_reason} — sleeping "
                       f"{policy.delay_for_attempt(attempt):.0f}s")
            sleep_for_retry(attempt, policy)
    return {
        "ok": False,
        "attempts": policy.max_retries + 1,
        "error_class": classify_error(last_err or "")[0].value,
        "error_reason": classify_error(last_err or "")[1],
        "error_message": (last_err or "")[:500],
        "will_retry": False,
    }


@click.group("workflow")
def cli():
    """Parallel rendering + smart retry surface."""
    pass


@cli.command("config")
@click.option("--max-workers", type=int, default=DEFAULT_MAX_WORKERS,
              help="Parallel shots per render (default 4).")
@click.option("--max-retries", type=int, default=DEFAULT_MAX_RETRIES,
              help="Max retries per shot before giving up (default 3).")
@click.pass_obj
def config_cmd(obj, max_workers, max_retries):
    """Show workflow configuration."""
    cfg = WorkflowConfig(max_workers=max_workers, max_retries=max_retries)
    click.echo("═══ Workflow config ═══")
    for k, v in cfg.as_dict().items():
        click.echo(f"  {k:<22} {v}")
    click.echo("")
    click.echo("Set on render:")
    click.echo("  vivify episode render ... --parallel 4 --max-retries 3")


@cli.command("list")
@click.option("--status", "-s", default=None,
              type=click.Choice(["queued", "running", "completed", "failed"]))
@click.option("--character", "-c", default=None)
@click.option("--limit", "-n", default=30, type=int)
@click.option("--as-json", "as_json", is_flag=True)
@click.pass_obj
def list_cmd(obj, status, character, limit, as_json):
    """List render_jobs (any filter)."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        # Aliases avoid render_jobs.episode_id (FK int) vs
        # episodes.episode_id (user-visible string) collision.
        q = """SELECT j.id, j.status, j.retries, j.started_at, j.completed_at,
                      j.error_message, j.episode_id AS fk_episode_id,
                      e.character_id, e.episode_id AS ep_code
               FROM render_jobs j
               JOIN episodes e ON e.id = j.episode_id
               WHERE 1=1"""
        params: list = []
        if status:
            q += " AND j.status = ?"; params.append(status)
        if character:
            q += " AND e.character_id = ?"; params.append(character)
        q += " ORDER BY j.id DESC LIMIT ?"
        params.append(limit)
        rows = [dict(r) for r in conn.execute(q, params).fetchall()]
    if as_json:
        click.echo(json.dumps(rows, indent=2, ensure_ascii=False, default=str))
        return
    if not rows:
        click.echo("(no jobs match — try without filters)")
        return
    click.echo(f"{'ID':<5} {'CHAR':<10} {'EP':<8} {'STATUS':<10} "
               f"{'RETRIES':<8} {'STARTED':<20} ERROR")
    click.echo("─" * 100)
    for r in rows:
        err = (r.get("error_message") or "")[:50]
        click.echo(
            f"{r['id']:<5} {r['character_id']:<10} {r['ep_code']:<8} "
            f"{r['status']:<10} {r['retries']:<8} "
            f"{(r.get('started_at') or '—'):<20} {err}"
        )


@cli.command("status")
@click.pass_obj
def status_cmd(obj):
    """Show in-flight + recently-failed jobs (concise dashboard)."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        running = [dict(r) for r in conn.execute(
            """SELECT j.id, e.character_id, e.episode_id, j.started_at
               FROM render_jobs j JOIN episodes e ON e.id = j.episode_id
               WHERE j.status = 'running'
               ORDER BY j.started_at DESC LIMIT 10""",
        ).fetchall()]
        failed = [dict(r) for r in conn.execute(
            """SELECT j.id, e.character_id, e.episode_id, j.retries,
                      j.error_message, j.completed_at
               FROM render_jobs j JOIN episodes e ON e.id = j.episode_id
               WHERE j.status = 'failed'
               ORDER BY j.id DESC LIMIT 10""",
        ).fetchall()]
        queued = [dict(r) for r in conn.execute(
            """SELECT j.id, e.character_id, e.episode_id
               FROM render_jobs j JOIN episodes e ON e.id = j.episode_id
               WHERE j.status = 'queued'""",
        ).fetchall()]

    click.echo("═══ Workflow status ═══")
    click.echo(f"  running: {len(running)}    queued: {len(queued)}    "
               f"failed (last 10): {len(failed)}")
    if running:
        click.echo("")
        click.echo(f"  Running:")
        for r in running:
            click.echo(f"    #{r['id']}  {r['character_id']}/{r['episode_id']}  "
                       f"started {r['started_at']}")
    if queued:
        click.echo("")
        click.echo(f"  Queued:")
        for r in queued:
            click.echo(f"    #{r['id']}  {r['character_id']}/{r['episode_id']}")
    if failed:
        click.echo("")
        click.echo(f"  Recently failed:")
        for r in failed:
            err = (r.get("error_message") or "")[:60]
            cls, reason = classify_error(err) if err else (None, "")
            cls_str = f"[{cls.value}:{reason}]" if cls else ""
            click.echo(f"    #{r['id']}  {r['character_id']}/{r['episode_id']}  "
                       f"retries={r['retries']}  {cls_str}")
            if err:
                click.echo(f"      {err}")


@cli.command("retry")
@click.argument("job_id", type=int)
@click.option("--max-retries", type=int, default=DEFAULT_MAX_RETRIES)
@click.option("--yes", "-y", is_flag=True, help="Skip confirmation.")
@click.pass_obj
def retry_cmd(obj, job_id, max_retries, yes):
    """Requeue a failed job. Smart-retry: permanent failures are NOT retried.

    Sets status='queued' so it can be picked up by the next render that
    targets this episode. Use `vivify episode render --retry-only <char> <ep>`
    to actually re-run.
    """
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        row = conn.execute(
            "SELECT * FROM render_jobs WHERE id = ?", (job_id,),
        ).fetchone()
        if not row:
            raise click.ClickException(f"job #{job_id} not found")
        if row["status"] != "failed":
            click.echo(f"[warn] job #{job_id} status is '{row['status']}', "
                       f"not 'failed'. Resetting to queued anyway.")
        err = row["error_message"] or ""
        cls, reason = classify_error(err) if err else (None, "")
        if cls == FailureClass.PERMANENT:
            click.echo(click.style(
                f"⚠ job #{job_id} previously failed with PERMANENT error "
                f"({reason}):\n  {err[:200]}\n"
                f"  Re-running will likely fail the same way. "
                f"Fix the root cause (e.g. upgrade Token Plan, "
                f"activate model on account) before retrying.",
                fg="yellow"))
            if not click.confirm("Requeue anyway?", default=False):
                click.echo("aborted")
                return

        conn.execute(
            """UPDATE render_jobs SET
                status = 'queued',
                error_message = NULL,
                completed_at = NULL
               WHERE id = ?""",
            (job_id,),
        )
        click.echo(f"✓ job #{job_id} requeued")
        click.echo(f"  run `vivify episode render ... --retry-only` to actually execute")


@cli.command("retry-failed")
@click.argument("character_id")
@click.argument("episode_id")
@click.option("--yes", "-y", is_flag=True)
@click.pass_obj
def retry_failed_cmd(obj, character_id, episode_id, yes):
    """Requeue all failed jobs for one episode."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        ep = conn.execute(
            "SELECT id FROM episodes WHERE character_id=? AND episode_id=?",
            (character_id, episode_id),
        ).fetchone()
        if not ep:
            raise click.ClickException(f"episode {character_id}/{episode_id} not found")
        failed_ids = [r["id"] for r in conn.execute(
            "SELECT id FROM render_jobs WHERE episode_id=? AND status='failed'",
            (ep["id"],),
        ).fetchall()]
        if not failed_ids:
            click.echo(f"(no failed jobs for {character_id}/{episode_id})")
            return
        if not yes:
            click.confirm(
                f"Requeue {len(failed_ids)} failed job(s) for "
                f"{character_id}/{episode_id}?", abort=True)
        for jid in failed_ids:
            conn.execute(
                """UPDATE render_jobs SET
                    status='queued', error_message=NULL, completed_at=NULL
                   WHERE id = ?""",
                (jid,),
            )
        click.echo(f"✓ requeued {len(failed_ids)} failed job(s) for "
                   f"{character_id}/{episode_id}")


__all__ = [
    "cli",
    "WorkflowConfig",
    "_gen_shot_with_retry",
]

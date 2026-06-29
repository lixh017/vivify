"""vivify.commands.doctor — `vivify doctor` environment diagnosis.

Outputs a one-line summary per check (✓ / ✗), followed by a summary line.
Exit code 0 if all critical checks pass, 1 if any critical check fails,
2 if only non-critical checks failed.
"""

import sys
from pathlib import Path

import click

from ..doctor import CheckResult, run_all_checks


def _style_kwargs(color: str) -> dict:
    """Apply color/bold only when stdout is a real TTY (so tests/CI get plain text)."""
    if sys.stdout.isatty():
        return {"fg": color, "bold": True}
    return {}


def _render_line(result: CheckResult) -> list[str]:
    """Build the lines we'll click.echo for one check result."""
    mark = "✓" if result.ok else "✗"
    color = "green" if result.ok else ("red" if result.critical else "yellow")
    line = f"{click.style(mark, **_style_kwargs(color))} {result.name}: {result.detail}"
    out = [line]
    if not result.ok and result.fix_hint:
        for hint_line in result.fix_hint.splitlines():
            out.append("   " + click.style(hint_line, **_style_kwargs("red")))
    return out


@click.command("doctor")
@click.option("--verbose", "-v", is_flag=True,
              help="Show full detail for passing checks too.")
@click.option("--out-dir", default=None,
              help="Disk-space check path (default: /tmp).")
@click.option("--self-install", "-i", is_flag=True,
              help="Attempt to install missing critical deps before reporting.")
@click.pass_obj
def doctor_cmd(obj, verbose, out_dir, self_install):
    """Diagnose your vivify environment. Run this first if anything is broken.

    With --self-install, doctor will try `apt install ffmpeg` / `pip install
    minimax-mmx` / etc. for each missing dep before re-checking. Best-effort:
    requires sudo on Linux, may fail without network access.
    """
    db_path = obj.get("db_path") if obj else None
    results = run_all_checks(out_dir=Path(out_dir) if out_dir else None,
                             db_path=db_path)

    if self_install:
        from vivify.doctor_install import attempt_install
        # Only attempt install for critical failures
        for r in results:
            if not r.ok and r.critical and r.install_cmd:
                click.echo(f"\n[install] attempting: {r.install_cmd}")
                ok, msg = attempt_install(r.install_cmd)
                if ok:
                    click.echo(click.style(f"  ✓ installed {r.name}",
                                            **_style_kwargs("green")))
                else:
                    click.echo(click.style(f"  ✗ install failed: {msg}",
                                            **_style_kwargs("red")))
        # Re-check after install attempts
        results = run_all_checks(out_dir=Path(out_dir) if out_dir else None,
                                 db_path=db_path)

    click.echo("═══ vivify doctor ═══")
    for r in results:
        for line in _render_line(r):
            click.echo(line)

    n_ok = sum(1 for r in results if r.ok)
    n_total = len(results)
    failed = [r for r in results if not r.ok]
    critical_failed = [r for r in results if not r.ok and r.critical]

    if not failed:
        click.echo(click.style(
            f"\nSummary: {n_ok}/{n_total} ok, all green.",
            **_style_kwargs("green")))
        return

    if critical_failed:
        names = ", ".join(r.name for r in critical_failed)
        click.echo(click.style(
            f"\nSummary: {n_ok}/{n_total} ok, "
            f"{len(critical_failed)} CRITICAL issue(s) ({names}).",
            **_style_kwargs("red")))
        sys.exit(1)

    names = ", ".join(r.name for r in failed)
    click.echo(click.style(
        f"\nSummary: {n_ok}/{n_total} ok, {len(failed)} issue(s) ({names}).",
        **_style_kwargs("yellow")))
    sys.exit(2)


# Backward-compat: a few callers may still expect a `cli` symbol — it's
# the same command object so `add_command` works the same way.
cli = doctor_cmd


__all__ = ["cli", "doctor_cmd"]

"""vivify.commands.db — DB migration + diagnostics.

Commands:
  status                  Show applied + pending migrations
  migrate [--dry-run]     Apply pending migrations
  schema-version          Print the current schema version (max applied)
  inspect                 Dump table list + row counts + sizes
"""

from pathlib import Path

import click

from ..db import connect, init_db
from ..migrator import (
    MIGRATIONS_DIR,
    apply_pending,
    get_status,
    schema_version,
)


@click.group("db")
def cli():
    """Database maintenance: migrations, diagnostics."""
    pass


@cli.command("status")
@click.option("--migrations-dir", default=None,
              help="Override migrations directory (default: vivify/migrations).")
@click.pass_obj
def status_cmd(obj, migrations_dir):
    """Show applied + pending migrations."""
    db_path = obj.get("db_path")
    md = Path(migrations_dir) if migrations_dir else MIGRATIONS_DIR
    applied, pending = get_status(db_path, md)
    click.echo(f"═══ DB migrations ═══")
    click.echo(f"  schema version: {schema_version(db_path)}")
    click.echo(f"  migrations dir: {md}")
    click.echo("")
    if applied:
        click.echo(f"  Applied ({len(applied)}):")
        for m in applied:
            click.echo(f"    ✓ {m.version:03d}_{m.name}")
    if pending:
        click.echo(f"  Pending ({len(pending)}):")
        for m in pending:
            click.echo(f"    → {m.version:03d}_{m.name}")
    if not applied and not pending:
        click.echo("  (no migrations found — check vivify/migrations/)")


@cli.command("migrate")
@click.option("--dry-run", is_flag=True,
              help="Show what would be applied, but don't actually run.")
@click.option("--migrations-dir", default=None)
@click.pass_obj
def migrate_cmd(obj, dry_run, migrations_dir):
    """Apply pending migrations to bring the DB up to current schema."""
    db_path = obj.get("db_path")
    md = Path(migrations_dir) if migrations_dir else MIGRATIONS_DIR

    applied_existing, pending = get_status(db_path, md)
    click.echo(f"Current schema version: {schema_version(db_path)}")
    click.echo(f"Pending migrations:     {len(pending)}")
    click.echo("")
    if not pending:
        click.echo("✓ nothing to do")
        return

    if dry_run:
        click.echo("(dry-run — would apply:)")
        for m in pending:
            click.echo(f"  → {m.version:03d}_{m.name}")
        return

    applied_now = apply_pending(db_path, md, verbose=True)
    click.echo("")
    click.echo(f"✓ applied {len(applied_now)} migration(s). "
               f"schema is now v{schema_version(db_path)}.")


@cli.command("schema-version")
@click.pass_obj
def schema_version_cmd(obj):
    """Print the current schema version (max applied migration)."""
    db_path = obj.get("db_path")
    v = schema_version(db_path)
    click.echo(f"schema version: {v}")


@cli.command("inspect")
@click.pass_obj
def inspect_cmd(obj):
    """Show table list + row counts + DB file size."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        tables = [r["name"] for r in conn.execute(
            "SELECT name FROM sqlite_master WHERE type='table' "
            "AND name NOT LIKE 'sqlite_%' ORDER BY name"
        ).fetchall()]
        click.echo(f"═══ DB inspection — {db_path or '.tmp/data/vivify.db'} ═══")
        click.echo(f"  tables: {len(tables)}")
        click.echo("")
        click.echo(f"  {'TABLE':<20} {'ROWS':<8}")
        click.echo("  " + "─" * 30)
        for t in tables:
            n = conn.execute(f"SELECT COUNT(*) AS n FROM {t}").fetchone()["n"]
            click.echo(f"  {t:<20} {n:<8}")
    # File size
    from ..db import get_db_path
    p = get_db_path(db_path)
    if p.exists():
        size_mb = p.stat().st_size / 1_000_000
        click.echo(f"\n  file size: {size_mb:.2f} MB  ({p})")


__all__ = ["cli"]

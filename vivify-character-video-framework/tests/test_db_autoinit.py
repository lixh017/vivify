"""Tests for DB auto-init: every CLI invocation should apply pending migrations
and bring a fresh DB up to current schema version.

These tests do NOT depend on the project's .tmp/data/vivify.db — they use
throwaway paths under tmp_path to simulate a fresh install.
"""

from pathlib import Path

import pytest
from click.testing import CliRunner

from vivify.cli import main
from vivify.db import connect, init_db, get_db_path
from vivify.migrator import apply_pending, schema_version


def test_init_db_creates_empty_schema(tmp_path: Path):
    """init_db on a fresh path creates the DB file and all 8 base tables."""
    db = str(tmp_path / "fresh.db")
    p = init_db(db)
    assert p.exists()
    with connect(db) as conn:
        rows = conn.execute(
            "SELECT name FROM sqlite_master WHERE type='table' "
            "AND name NOT LIKE 'sqlite_%' ORDER BY name"
        ).fetchall()
        names = {r["name"] for r in rows}
    # 7 application tables + migrations table (created on demand by migrator
    # when first queried; init_db alone should give us the 7 base tables).
    expected = {"characters", "episodes", "shots", "assets",
                "publishes", "render_jobs", "lessons"}
    assert expected.issubset(names)


def test_apply_pending_brings_fresh_db_to_current_schema(tmp_path: Path):
    """apply_pending on a fresh DB creates migrations table + applies 001, 002."""
    db = str(tmp_path / "fresh2.db")
    applied = apply_pending(db, verbose=False)
    # Should apply all migrations found in vivify/migrations/
    assert len(applied) >= 2
    assert schema_version(db) == max(m.version for m in applied)

    # The 002 migration's updated_at column should now exist on episodes.
    with connect(db) as conn:
        cols = {row["name"] for row in conn.execute(
            "PRAGMA table_info(episodes)"
        ).fetchall()}
    assert "updated_at" in cols


def test_apply_pending_is_idempotent(tmp_path: Path):
    """Running apply_pending twice does not double-apply."""
    db = str(tmp_path / "idem.db")
    first = apply_pending(db, verbose=False)
    second = apply_pending(db, verbose=False)
    assert len(first) >= 2
    assert second == []  # nothing to apply the second time


def test_cli_auto_applies_pending_migrations(tmp_path: Path, monkeypatch):
    """A fresh DB is migrated to current schema just by invoking the CLI."""
    db = str(tmp_path / "cli_fresh.db")
    runner = CliRunner()
    # Use --db to point the CLI at our throwaway DB.
    result = runner.invoke(main, ["--db", db, "db", "schema-version"])
    assert result.exit_code == 0, result.output
    assert "schema version: 2" in result.output
    # The auto-apply message should appear on first run (no quiet flag).
    assert "auto-applied" in result.output


def test_cli_quiet_init_suppresses_auto_migration_message(tmp_path: Path):
    """--quiet-init hides auto-migration noise (useful for scripted use)."""
    db = str(tmp_path / "cli_quiet.db")
    runner = CliRunner()
    result = runner.invoke(main, ["--quiet-init", "--db", db,
                                  "db", "schema-version"])
    assert result.exit_code == 0, result.output
    assert "auto-applied" not in result.output
    assert "schema version: 2" in result.output


def test_cli_second_run_does_not_re_announce(tmp_path: Path):
    """After auto-init, subsequent CLI runs are silent (no migrations to apply)."""
    db = str(tmp_path / "cli_second.db")
    runner = CliRunner()
    # First run — should announce auto-applied.
    r1 = runner.invoke(main, ["--db", db, "db", "schema-version"])
    assert "auto-applied" in r1.output
    # Second run — schema already current, no announcement.
    r2 = runner.invoke(main, ["--db", db, "db", "schema-version"])
    assert "auto-applied" not in r2.output
    assert "schema version: 2" in r2.output
"""Tests for --db global flag honoring in subcommands.

Pre-existing bug (B1, surfaced 2026-06): `connect()` was called with no
args in 15 places (character.py, asset.py, lesson.py, cost.py, memory.py,
episode.py, db.py). Each ignored the CLI `--db` flag, forcing the user
to use the default DB path.

Fix: `connect()` / `init_db()` now consult `click.get_current_context()`
for `obj['db_path']` when no explicit path is given. Library callers
(tests, scripts) still pass an explicit path.

These tests pin the fix so future refactors don't reintroduce the bug.
"""
import sqlite3
from pathlib import Path

import pytest
from click.testing import CliRunner

from vivify.cli import main as cli
from vivify.db import connect, init_db, get_db_path


# --- unit tests --------------------------------------------------------------

def test_connect_uses_explicit_db_path_when_passed(tmp_path):
    """When caller passes db_path, it's used regardless of context."""
    db = tmp_path / "explicit.db"
    init_db(str(db))
    with connect(str(db)) as conn:
        conn.execute("CREATE TABLE t (x INTEGER)")
        conn.execute("INSERT INTO t VALUES (1)")
    assert db.exists()
    rows = sqlite3.connect(str(db)).execute("SELECT * FROM t").fetchall()
    assert rows == [(1,)]


def test_connect_falls_back_to_ctx_obj_db_path(tmp_path):
    """When caller passes no db_path, connect() reads from Click context.

    This is the fix: `with connect() as conn` now respects `--db`.
    """
    db = tmp_path / "from-ctx.db"
    runner = CliRunner()
    # Use a CLI command that triggers `connect()` internally. We don't care
    # if the command succeeds — we care that the DB file got created.
    result = runner.invoke(cli, [
        "--db", str(db),
        "db", "inspect",
    ])
    assert db.exists(), (
        f"--db was not honored: {db} not created. "
        f"CLI output: {result.output}"
    )


def test_get_db_path_precedence_explicit_over_ctx(tmp_path):
    """Explicit db_path arg wins over Click context."""
    import click
    from vivify.db import _ctx_db_path, get_db_path

    @click.command()
    @click.pass_context
    def fake(ctx):
        ctx.obj = {"db_path": str(tmp_path / "ctx.db")}
        # Explicit path should override ctx
        assert get_db_path("/explicit/path.db") == Path("/explicit/path.db")
        # No arg → ctx
        assert get_db_path() == Path(str(tmp_path / "ctx.db"))

    runner = CliRunner()
    runner.invoke(fake, [])


def test_get_db_path_default_when_no_ctx():
    """No explicit arg + no Click context → default .tmp/data/vivify.db."""
    # Outside any Click command
    p = get_db_path()
    assert p == Path.cwd() / ".tmp" / "data" / "vivify.db"


# --- integration test --------------------------------------------------------

def test_character_add_respects_db_flag(tmp_path, monkeypatch):
    """End-to-end: `vivify character add ... --db /tmp/x.db` writes to /tmp/x.db,
    not to the default .tmp/data/vivify.db.

    This is the user-visible bug — register_character was always going to
    the default DB regardless of --db.
    """
    db = tmp_path / "user-specified.db"
    char_dir = tmp_path / "characters" / "testchar"
    char_dir.mkdir(parents=True)
    (char_dir / "character.yaml").write_text(
        "character:\n  name: Test\n  species: human\n",
        encoding="utf-8",
    )

    runner = CliRunner()
    result = runner.invoke(cli, [
        "--db", str(db),
        "character", "add", "testchar", str(char_dir),
    ])

    assert db.exists(), (
        f"--db /tmp/...db was ignored: {db} not created. "
        f"Output: {result.output}"
    )
    conn = sqlite3.connect(str(db))
    row = conn.execute(
        "SELECT id, name, dir_path FROM characters WHERE id='testchar'"
    ).fetchone()
    conn.close()
    assert row is not None, f"testchar not in user-specified DB: {result.output}"
    assert row[1] == "Test"
    assert row[2] == str(char_dir.resolve())
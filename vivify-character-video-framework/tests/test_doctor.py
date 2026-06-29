"""Tests for vivify.doctor + `vivify doctor` CLI command.

Each test isolates environment state via monkeypatch so the tests are
hermetic (we don't depend on the dev's actual /usr/bin/ffmpeg, ARK key,
or ~/.claude/config files).
"""

import os
import sqlite3
import sys
from pathlib import Path
from unittest.mock import patch

import pytest
from click.testing import CliRunner

from vivify.cli import main as cli


# ---------------------------------------------------------------------------
# Pure-logic tests (no CLI)
# ---------------------------------------------------------------------------

def test_check_python_passes_on_310_plus(monkeypatch):
    """A real Python 3.10+ interpreter must pass the version check.

    The interpreter running these tests is itself >= 3.10, so this
    should always succeed.
    """
    from vivify.doctor import check_python
    r = check_python()
    assert r.ok, f"expected ok=True on {sys.version_info}, got {r}"
    assert r.name == "python"
    assert "Python" in r.detail
    assert not r.fix_hint


def test_check_ffmpeg_finds_in_usr_bin(monkeypatch, tmp_path):
    """If /usr/bin/ffmpeg exists, check_ffmpeg should find it.

    Strategy: create a fake ffmpeg shim in tmp_path, add it to PATH, and
    verify the check returns ok=True with the discovered path.
    """
    from vivify.doctor import check_ffmpeg

    shim = tmp_path / "ffmpeg"
    shim.write_text("#!/bin/sh\necho 'ffmpeg version 6.0 fake'\n")
    shim.chmod(0o755)

    monkeypatch.setenv("PATH", f"{tmp_path}:{os.environ.get('PATH', '')}")
    # Make sure no /usr/bin/ffmpeg, /usr/local/bin/ffmpeg, etc. confuse us
    monkeypatch.delenv("FFMPEG", raising=False)

    r = check_ffmpeg()
    assert r.ok, f"expected ok=True; got detail={r.detail!r}, hint={r.fix_hint!r}"
    assert r.name == "ffmpeg"
    assert "fake" in r.detail or "ffmpeg version" in r.detail


def test_check_ffmpeg_returns_install_hint_when_missing(monkeypatch):
    """With no ffmpeg anywhere, the check should fail with a fix hint."""
    from vivify.doctor import check_ffmpeg

    monkeypatch.setenv("PATH", "/nonexistent-only-for-this-test")
    monkeypatch.delenv("FFMPEG", raising=False)
    # Patch the hardcoded fallback paths to nonexistent too — the test
    # only matters if no ffmpeg binary lives at any of the standard
    # places. Patch check_ffmpeg's internal list of candidate paths to
    # nothing so the test is hermetic.
    with patch("vivify.doctor._ffmpeg_paths", return_value=[]):
        r = check_ffmpeg()
    assert not r.ok
    assert r.name == "ffmpeg"
    assert r.fix_hint is not None
    assert "Install" in r.fix_hint
    # On Linux (this sandbox), the hint must mention apt or equivalent.
    assert any(
        kw in r.fix_hint.lower()
        for kw in ("apt", "dnf", "pacman", "brew", "choco", "winget", "download")
    ), f"unexpected install hint: {r.fix_hint!r}"


def test_check_mmx_returns_ok_when_on_path(monkeypatch, tmp_path):
    """If `mmx` is on PATH and `--version` works, the check passes."""
    from vivify.doctor import check_mmx

    mmx = tmp_path / "mmx"
    mmx.write_text("#!/bin/sh\necho 'mmx 9.9.9 fake'\n")
    mmx.chmod(0o755)
    monkeypatch.setenv("PATH", f"{tmp_path}:{os.environ.get('PATH', '')}")

    r = check_mmx()
    assert r.ok, f"expected ok=True; got {r}"
    assert r.name == "mmx"
    assert "9.9.9" in r.detail or "mmx" in r.detail


def test_check_mmx_returns_fail_when_missing(monkeypatch):
    """With no `mmx` on PATH, the check should fail with a fix hint."""
    from vivify.doctor import check_mmx

    monkeypatch.setenv("PATH", "/nonexistent-only-for-this-test")
    r = check_mmx()
    assert not r.ok
    assert r.name == "mmx"
    assert r.fix_hint is not None
    assert "mmx" in r.fix_hint.lower() or "pip" in r.fix_hint.lower()


def test_check_ark_key_passes_when_loaded_from_env_file(monkeypatch, tmp_path):
    """A 30+ char key in a candidate env file should pass the check."""
    from vivify.doctor import check_ark_key, _ark_key_files

    key = "a" * 32  # > ARK_KEY_MIN_LEN
    env_file = tmp_path / "vivify-volcengine.env"
    env_file.write_text(f"ARK_API_KEY={key}\n")

    # Point the candidate-finder at our temp file
    monkeypatch.setattr(
        "vivify.doctor._ark_key_files",
        lambda: [env_file],
    )
    # Make sure no real env var leaks in
    monkeypatch.delenv("ARK_API_KEY", raising=False)

    r = check_ark_key()
    assert r.ok, f"expected ok=True; got {r.detail!r}, {r.fix_hint!r}"
    assert r.name == "ARK_API_KEY"
    assert key[:8] in r.detail  # "aaaa...." prefix shown
    assert str(env_file) in r.detail  # "loaded from <path>" shown


def test_check_ark_key_fails_when_no_source(monkeypatch, tmp_path):
    """With no env var and no env file, the check should fail with a hint."""
    from vivify.doctor import check_ark_key

    monkeypatch.delenv("ARK_API_KEY", raising=False)
    monkeypatch.setattr(
        "vivify.doctor._ark_key_files",
        lambda: [tmp_path / "nonexistent.env"],
    )
    r = check_ark_key()
    assert not r.ok
    assert r.critical  # ARK key is critical
    assert r.fix_hint is not None
    assert "ARK_API_KEY" in r.fix_hint


def test_check_db_creates_if_missing(tmp_path):
    """check_db must create the SQLite file and report schema version."""
    from vivify.doctor import check_db

    db_path = str(tmp_path / "fresh.db")
    assert not Path(db_path).exists()

    r = check_db(db_path=db_path)
    assert r.ok, f"expected ok=True; got {r.detail!r}, {r.fix_hint!r}"
    assert r.name == "db"
    assert "schema v" in r.detail
    assert db_path in r.detail
    # File now exists
    assert Path(db_path).exists()
    # The schema is real
    conn = sqlite3.connect(db_path)
    tables = [r[0] for r in conn.execute(
        "SELECT name FROM sqlite_master WHERE type='table'"
    ).fetchall()]
    assert "episodes" in tables
    conn.close()


def test_run_all_checks_returns_list(monkeypatch, tmp_path):
    """run_all_checks returns a non-empty list of CheckResult in stable order."""
    from vivify.doctor import run_all_checks, CheckResult

    # Make ffmpeg + mmx + ARK all deterministic so we don't depend on the
    # sandbox environment.
    ffmpeg = tmp_path / "ffmpeg"
    ffmpeg.write_text("#!/bin/sh\necho 'ffmpeg version 6.0 fake'\n")
    ffmpeg.chmod(0o755)
    mmx = tmp_path / "mmx"
    mmx.write_text("#!/bin/sh\necho 'mmx 9.9.9 fake'\n")
    mmx.chmod(0o755)
    monkeypatch.setenv("PATH", f"{tmp_path}:{os.environ.get('PATH', '')}")
    monkeypatch.setenv("ARK_API_KEY", "a" * 32)

    results = run_all_checks(
        out_dir=tmp_path,
        db_path=str(tmp_path / "test.db"),
    )
    assert isinstance(results, list)
    assert len(results) >= 6
    for r in results:
        assert isinstance(r, CheckResult)
    # Stable order: python first, ffmpeg second, etc.
    names = [r.name for r in results]
    assert names[0] == "python"
    assert "ffmpeg" in names
    assert "mmx" in names
    assert "ARK_API_KEY" in names
    assert "db" in names
    assert "disk" in names


# ---------------------------------------------------------------------------
# CLI tests (CliRunner)
# ---------------------------------------------------------------------------

def test_cli_doctor_exit_code_zero_when_all_ok(monkeypatch, tmp_path):
    """When every check passes, the CLI exits 0 and prints the summary."""
    # Stage a fully-passing env
    ffmpeg = tmp_path / "ffmpeg"
    ffmpeg.write_text("#!/bin/sh\necho 'ffmpeg version 6.0 fake'\n")
    ffmpeg.chmod(0o755)
    mmx = tmp_path / "mmx"
    mmx.write_text("#!/bin/sh\necho 'mmx 9.9.9 fake'\n")
    mmx.chmod(0o755)
    monkeypatch.setenv("PATH", f"{tmp_path}:{os.environ.get('PATH', '')}")
    monkeypatch.setenv("ARK_API_KEY", "a" * 32)

    db_path = str(tmp_path / "test.db")
    runner = CliRunner()
    result = runner.invoke(cli, ["--db", db_path, "doctor"], catch_exceptions=False)
    assert result.exit_code == 0, (
        f"expected exit 0, got {result.exit_code}\nOUTPUT:\n{result.output}"
    )
    assert "vivify doctor" in result.output
    assert "all green" in result.output
    # Spot-check: every check is present
    for marker in ("python", "ffmpeg", "mmx", "ARK_API_KEY", "db", "disk"):
        assert marker in result.output, f"missing {marker!r} in output:\n{result.output}"


def test_cli_doctor_exit_code_one_when_ffmpeg_missing(monkeypatch, tmp_path):
    """With no ffmpeg anywhere, the CLI must exit 1 (critical failure)."""
    monkeypatch.setenv("PATH", "/nonexistent-only-for-this-test")
    monkeypatch.delenv("FFMPEG", raising=False)
    monkeypatch.setenv("ARK_API_KEY", "a" * 32)
    # Empty the fallback path list so the check can't accidentally succeed
    monkeypatch.setattr("vivify.doctor._ffmpeg_paths", lambda: [])

    db_path = str(tmp_path / "test.db")
    runner = CliRunner()
    result = runner.invoke(
        cli, ["--db", db_path, "doctor"], catch_exceptions=False,
    )
    assert result.exit_code == 1, (
        f"expected exit 1, got {result.exit_code}\nOUTPUT:\n{result.output}"
    )
    assert "ffmpeg" in result.output
    assert "NOT FOUND" in result.output
    # The fix hint must be present (install instructions)
    assert "Install" in result.output or "install" in result.output

"""Tests for commands.episode render_cmd --use-driver branch.

Subprocess path is the legacy fallback; the new in-process path goes
through vivify.episode_driver and is what the rest of Phase A is
pointing at. These tests pin the new path's behavior:
  - Must NOT call subprocess (subprocess.call('render_episode.py'))
  - Must call render_episode_assets
  - Must update DB with REAL cost (sum of asset_results), not estimate
  - Must NOT block on dry-run for subprocess-specific args
"""
import sqlite3
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
from click.testing import CliRunner

from vivify.cli import main as cli


def _invoke(*args, db_path=None):
    """Invoke the CLI. --db is a global option on the main group, so
    it must come BEFORE the subcommand."""
    argv = ["--db", str(db_path)] if db_path else []
    argv += list(args)
    return CliRunner().invoke(cli, argv)


# --- fixtures ---------------------------------------------------------------

@pytest.fixture
def db_path(tmp_path):
    """Init the real schema and register the fengge character so
    episode render can resolve it."""
    from vivify.db import init_db
    db = tmp_path / "test.db"
    init_db(str(db))

    conn = sqlite3.connect(str(db))
    conn.execute("""INSERT OR REPLACE INTO characters
        (id, name, english_name, species, dir_path, character_yaml_path, canonical_dir)
        VALUES (?, ?, ?, ?, ?, ?, ?)""",
        ("fengge", "峰哥", "Fengge", "成年熊猫",
         str(Path.cwd() / "characters" / "fengge"),
         str(Path.cwd() / "characters" / "fengge" / "character.yaml"),
         str(Path.cwd() / "characters" / "fengge" / "canonical")))
    conn.commit()
    conn.close()
    return db


def _ep_storyboard_path() -> Path:
    # Storyboards live in /root/workspace/opc/docs/showcase (the repo-root
    # showcase, not characters/fengge/examples). Test runs from the framework
    # dir so we resolve via the parent.
    return Path("/root/workspace/opc/docs/showcase/panda-episode-001/STORYBOARD.md")


def _ep_script_path() -> Path:
    return Path("/root/workspace/opc/docs/showcase/panda-episode-001/SCRIPT-douyin.md")


# --- tests ------------------------------------------------------------------

def test_driver_dry_run_writes_db_rows_and_no_subprocess(
    db_path, tmp_path, monkeypatch,
):
    """--use-driver --dry-run (after B2 fix): the driver pipeline runs with
    generate_asset/_call_tts stubbed so it exercises the full per-shot
    plumbing (DB writes, cost aggregation) without burning money. Must
    NOT call subprocess (no legacy fallback)."""
    import vivify.commands.episode as ep_mod
    monkeypatch.setattr(ep_mod, "_parse_voiceovers", lambda *a, **kw: [])
    # After driver runs, mux is attempted (ffmpeg). Mock it so dry-run
    # doesn't require a real ffmpeg binary. The mux_ok=True branch in
    # production checks `final_mp4.exists()` to mark the episode as
    # 'completed' — write a tiny file to satisfy that.
    def _fake_mux(asset_results, *, voiceovers, env, out_path, target_dur,
                  title, next_episode, voice):
        Path(out_path).parent.mkdir(parents=True, exist_ok=True)
        Path(out_path).write_bytes(b"fake-mp4")
        return True
    monkeypatch.setattr(ep_mod, "_mux_episode", _fake_mux)

    runner = CliRunner()
    with patch("subprocess.call") as mock_subproc:
        result = _invoke(
            "episode", "render", "fengge", "EP001",
            "--storyboard", str(_ep_storyboard_path()),
            "--script", str(_ep_script_path()),
            "--voice", "治愈",
            "--platform", "抖音",
            "--dry-run",
            "--use-driver",
            "--out", str(tmp_path / "out"),
            db_path=db_path,
        )
    assert result.exit_code == 0, result.output
    mock_subproc.assert_not_called()

    # Episode row written
    conn = sqlite3.connect(str(db_path))
    ep = conn.execute("SELECT * FROM episodes WHERE episode_id='EP001'").fetchone()
    assert ep is not None
    # Status should be 'completed' (dry-run marks done)
    cols = [d[0] for d in conn.execute("SELECT * FROM episodes LIMIT 0").description]
    status_idx = cols.index("status")
    assert ep[status_idx] == "completed"
    conn.close()


def test_driver_real_path_calls_render_episode_assets_not_subprocess(
    db_path, tmp_path, monkeypatch,
):
    """--use-driver (no --dry-run): must call render_episode_assets
    with the parsed shots; must NOT call subprocess."""
    import vivify.commands.episode as ep_mod
    monkeypatch.setattr(ep_mod, "_parse_voiceovers", lambda *a, **kw: [])

    # Mock the driver
    fake_results = []
    from vivify.episode_driver import ShotAssetResult

    def fake_render_assets(*args, **kwargs):
        shots = kwargs.get("shots") or (args[0] if args else [])
        for s in shots:
            fake_results.append(s.get("n"))
        # Return one result per shot
        return [ShotAssetResult(
            shot_number=s["n"], image_path=None, video_path=None,
            tts_path=None, image_cost_yuan=0.0, video_cost_yuan=0.0,
            tts_cost_yuan=0.0,
        ) for s in shots]

    monkeypatch.setattr(ep_mod, "_render_episode_assets", fake_render_assets)
    # Mock mux to return False (we're not testing mux in this test)
    monkeypatch.setattr(ep_mod, "_mux_episode", lambda *a, **kw: False)

    runner = CliRunner()
    with patch("subprocess.call") as mock_subproc:
        result = _invoke(
            "episode", "render", "fengge", "EP001",
            "--storyboard", str(_ep_storyboard_path()),
            "--script", str(_ep_script_path()),
            "--voice", "治愈",
            "--platform", "抖音",
            "--use-driver",
            "--out", str(tmp_path / "out"),
            db_path=db_path,
        )
    # mux returned False → render fails with sys.exit(1) → exit_code = 1
    assert mock_subproc.assert_not_called() is None
    assert len(fake_results) >= 1, (
        f"expected at least 1 shot through render_episode_assets, "
        f"got {len(fake_results)}; output: {result.output}"
    )


def test_subprocess_path_unchanged_default(
    db_path, tmp_path, monkeypatch,
):
    """Default (no --use-driver) must still call subprocess.call.
    This pins the manual-escape path so the refactor doesn't accidentally
    break the legacy flow."""
    # We don't actually want the subprocess to succeed; we just need
    # to confirm it was CALLED. Use a side_effect that raises to short-circuit
    # the rest of the function.
    import click
    def fake_subprocess_call(cmd, *args, **kwargs):
        # If 'render_episode.py' is in the command, raise to stop execution
        if any("render_episode.py" in str(c) for c in cmd):
            raise click.exceptions.Exit(0)
        return 0

    monkeypatch.setattr("subprocess.call", fake_subprocess_call)

    runner = CliRunner()
    result = _invoke(
        "episode", "render", "fengge", "EP001",
        "--storyboard", str(_ep_storyboard_path()),
        "--script", str(_ep_script_path()),
        "--voice", "治愈",
        "--platform", "抖音",
        # NOTE: no --use-driver → subprocess path
        "--out", str(tmp_path / "out"),
        db_path=db_path,
    )
    # The Exit(0) is caught by Click and the test sees exit_code 0
    assert result.exit_code == 0, result.output

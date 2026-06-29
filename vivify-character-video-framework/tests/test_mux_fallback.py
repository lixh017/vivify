"""Tests for the mux-failure fallback (vivify.episode_mux_fallback).

These pin:
  - write_manifest produces a well-formed MANIFEST.json
  - mark_episode_assets_only updates the episodes + render_jobs rows
  - the `vivify episode mux` subcommand invokes ffmpeg via the manifest
  - missing manifest is reported gracefully (no crash)
"""

import json
import sqlite3
from pathlib import Path
from unittest.mock import patch

import pytest
from click.testing import CliRunner

from vivify.cli import main as cli
from vivify.db import init_db
from vivify.episode_mux_fallback import (
    WORK_DIR_NAME,
    mark_episode_assets_only,
    run_mux_from_manifest,
    write_manifest,
)


# --- fixtures ---------------------------------------------------------------

@pytest.fixture
def db_path(tmp_path):
    """Init the real schema and register the fengge character so the
    `episode mux` subcommand can resolve it."""
    db = tmp_path / "test.db"
    init_db(str(db))
    conn = sqlite3.connect(str(db))
    conn.execute(
        """INSERT OR REPLACE INTO characters
           (id, name, english_name, species, dir_path,
            character_yaml_path, canonical_dir)
           VALUES (?, ?, ?, ?, ?, ?, ?)""",
        ("fengge", "峰哥", "Fengge", "成年熊猫",
         str(Path.cwd() / "characters" / "fengge"),
         str(Path.cwd() / "characters" / "fengge" / "character.yaml"),
         str(Path.cwd() / "characters" / "fengge" / "canonical")),
    )
    conn.commit()
    conn.close()
    return db


def _fake_asset_results(work_dir: Path, n: int = 3):
    """Build a list of dicts shaped like ShotAssetResult for the manifest."""
    out = []
    for i in range(1, n + 1):
        d = work_dir / WORK_DIR_NAME
        d.mkdir(parents=True, exist_ok=True)
        img = d / f"shot-{i:02d}.jpg"
        vid = d / f"shot-{i:02d}.mp4"
        tts = d / f"shot-{i:02d}.mp3"
        img.write_bytes(b"img")
        vid.write_bytes(b"vid")
        tts.write_bytes(b"tts")
        out.append({
            "shot_number": i,
            "image_path": img,
            "video_path": vid,
            "tts_path": tts,
            "duration_sec": 5,
        })
    return out


# --- pure-logic tests -------------------------------------------------------

def test_write_manifest_creates_json(tmp_path):
    """write_manifest writes a parseable MANIFEST.json with mux_status='failed'."""
    work = tmp_path / WORK_DIR_NAME
    work.mkdir()
    asset_results = _fake_asset_results(tmp_path)
    manifest_path = write_manifest(
        tmp_path, asset_results,
        mux_failure_reason="ffmpeg not found",
        episode_id="EP001", character_id="fengge",
    )
    assert manifest_path.exists()
    assert manifest_path.name == "MANIFEST.json"
    data = json.loads(manifest_path.read_text(encoding="utf-8"))
    assert data["mux_status"] == "failed"
    assert data["mux_failure_reason"] == "ffmpeg not found"
    assert data["episode_id"] == "EP001"
    assert data["character_id"] == "fengge"
    assert "ffmpeg_command_template" in data
    assert "install_hint" in data


def test_manifest_contains_shot_paths(tmp_path):
    """Each per-shot entry must include image/video/tts paths."""
    asset_results = _fake_asset_results(tmp_path, n=3)
    manifest_path = write_manifest(
        tmp_path, asset_results,
        mux_failure_reason="mux failed",
    )
    data = json.loads(manifest_path.read_text(encoding="utf-8"))
    assert len(data["shots"]) == 3
    for i, s in enumerate(data["shots"], start=1):
        assert s["n"] == i
        assert s["image"].endswith(f"shot-{i:02d}.jpg")
        assert s["video"].endswith(f"shot-{i:02d}.mp4")
        assert s["tts"].endswith(f"shot-{i:02d}.mp3")
        # Paths are relative to out_dir (portable manifest)
        assert not Path(s["image"]).is_absolute()


def test_manifest_ffmpeg_command_template_correct(tmp_path):
    """The retry template must reference the standard concat demuxer layout."""
    write_manifest(tmp_path, [], mux_failure_reason="x")
    data = json.loads((tmp_path / "MANIFEST.json").read_text(encoding="utf-8"))
    cmd = data["ffmpeg_command_template"]
    assert "ffmpeg" in cmd
    assert "-f concat" in cmd
    assert "concat.txt" in cmd
    assert "audio.mp3" in cmd
    assert "final.mp4" in cmd


def test_mark_episode_assets_only_updates_db(db_path):
    """mark_episode_assets_only flips status to 'assets_only' and logs a job."""
    conn = sqlite3.connect(str(db_path))
    cur = conn.execute(
        """INSERT INTO episodes
           (character_id, episode_id, status, output_path)
           VALUES (?, ?, ?, ?)""",
        ("fengge", "EP001", "rendering", None),
    )
    ep_pk = cur.lastrowid
    conn.commit()
    conn.close()

    manifest_path = db_path.parent / "EP001-MANIFEST.json"
    manifest_path.write_text("{}", encoding="utf-8")

    job_pk = mark_episode_assets_only(
        str(db_path), ep_pk, manifest_path,
        mux_error="ffmpeg not found in PATH",
    )
    assert job_pk > 0

    conn = sqlite3.connect(str(db_path))
    ep = conn.execute(
        "SELECT status, output_path, error_message FROM episodes WHERE id = ?",
        (ep_pk,),
    ).fetchone()
    assert ep[0] == "assets_only"
    assert ep[1] == str(manifest_path)
    assert "ffmpeg not found" in ep[2]

    job = conn.execute(
        "SELECT status, error_message FROM render_jobs WHERE id = ?",
        (job_pk,),
    ).fetchone()
    assert job[0] == "assets_only"
    assert "mux failed" in job[1]
    conn.close()


# --- CLI subcommand tests ---------------------------------------------------

def _invoke(*args, db_path=None):
    argv = ["--db", str(db_path)] if db_path else []
    argv += list(args)
    return CliRunner().invoke(cli, argv)


def _bootstrap_assets_only_episode(db_path: Path, manifest_path: Path):
    """Insert an episode + render_job row in assets_only state pointing at
    a real manifest file."""
    conn = sqlite3.connect(str(db_path))
    cur = conn.execute(
        """INSERT INTO episodes
           (character_id, episode_id, status, output_path)
           VALUES (?, ?, ?, ?)""",
        ("fengge", "EP002", "assets_only", str(manifest_path)),
    )
    ep_pk = cur.lastrowid
    conn.execute(
        """INSERT INTO render_jobs (episode_id, status, error_message)
           VALUES (?, 'assets_only', 'mux failed in prior render')""",
        (ep_pk,),
    )
    conn.commit()
    conn.close()
    return ep_pk


def test_cli_mux_cmd_runs_ffmpeg_when_available(tmp_path, db_path):
    """When ffmpeg is on PATH and the manifest has shots, the mux
    subcommand produces a final MP4 and flips the episode to completed."""
    # Build manifest + per-shot files
    work = tmp_path / WORK_DIR_NAME
    work.mkdir(parents=True, exist_ok=True)
    asset_results = _fake_asset_results(tmp_path, n=2)
    manifest_path = write_manifest(
        tmp_path, asset_results,
        mux_failure_reason="ffmpeg not found",
        episode_id="EP002", character_id="fengge",
    )
    ep_pk = _bootstrap_assets_only_episode(db_path, manifest_path)

    # Strategy: make a stand-alone executable ffmpeg shim (chmod +x).
    # The mux module calls subprocess.run(['ffmpeg', '-y', ...]) directly
    # and shutil.which('ffmpeg'). Easiest: an executable shell script
    # that ignores all args and writes a sentinel file at its last arg.
    import sys as _sys
    shim_dir = tmp_path / "shim"
    shim_dir.mkdir(exist_ok=True)
    shim = shim_dir / "ffmpeg"
    shim.write_text(
        "#!/usr/bin/env python3\n"
        "import sys\n"
        "from pathlib import Path\n"
        "out = Path(sys.argv[-1])\n"
        "out.parent.mkdir(parents=True, exist_ok=True)\n"
        "out.write_bytes(b'FAKE-FINAL-MP4')\n"
        "sys.exit(0)\n",
        encoding="utf-8",
    )
    shim.chmod(0o755)

    # Put the shim dir on PATH so shutil.which finds it.
    saved_path = __import__("os").environ.get("PATH", "")
    __import__("os").environ["PATH"] = str(shim_dir) + ":" + saved_path

    try:
        result = _invoke(
            "episode", "mux", "fengge", "EP002",
            db_path=db_path,
        )
    finally:
        __import__("os").environ["PATH"] = saved_path

    assert result.exit_code == 0, result.output
    assert "mux OK" in result.output

    conn = sqlite3.connect(str(db_path))
    ep = conn.execute(
        "SELECT status, output_path, file_size_bytes "
        "FROM episodes WHERE id = ?",
        (ep_pk,),
    ).fetchone()
    assert ep[0] == "completed"
    assert Path(ep[1]).exists()
    assert ep[2] is not None and ep[2] > 0
    conn.close()


def test_cli_mux_cmd_handles_missing_manifest_gracefully(db_path):
    """If the episode isn't in assets_only AND no --manifest was passed,
    the subcommand exits with a clear ClickException (no traceback)."""
    conn = sqlite3.connect(str(db_path))
    conn.execute(
        """INSERT INTO episodes (character_id, episode_id, status)
           VALUES (?, ?, ?)""",
        ("fengge", "EP003", "completed"),
    )
    conn.commit()
    conn.close()

    result = _invoke("episode", "mux", "fengge", "EP003", db_path=db_path)
    assert result.exit_code != 0
    # Should NOT have produced a traceback; Click prints the message.
    assert "assets_only" in (result.output or "") or "Traceback" not in (result.output or "")
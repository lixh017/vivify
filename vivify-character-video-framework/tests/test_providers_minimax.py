"""Tests for vivify.providers.minimax — 海螺 Hailuo (mmx CLI) adapter.

The adapter shells out to `mmx video generate`. Tests mock subprocess.run
to inspect the constructed command and simulate exit codes.
"""

import base64
import subprocess
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from vivify.providers.minimax import MiniMaxProvider
from vivify.providers.base import GenerateRequest


# --- fixtures ---------------------------------------------------------------

@pytest.fixture
def env():
    return {"MINIMAX_API_KEY": "test-minimax-key"}


def _make_fake_run(captured: list, out_path: str, rc: int = 0, stderr: str = ""):
    """Return a fake subprocess.run that records cmd and writes a fake file."""
    def fake_run(cmd, *args, **kwargs):
        captured.append(cmd)
        # Find --download and write a fake mp4 marker there
        for i, c in enumerate(cmd):
            if c == "--download" and i + 1 < len(cmd):
                p = Path(cmd[i + 1])
                p.parent.mkdir(parents=True, exist_ok=True)
                p.write_bytes(b"\x00\x00\x00\x18ftypFAKE")
                break
        m = MagicMock()
        m.returncode = rc
        m.stdout = ""
        m.stderr = stderr
        return m
    return fake_run


# --- I2V mode (most common) -------------------------------------------------

def test_i2v_mode_passes_first_frame(env, tmp_path, monkeypatch):
    """I2V: mmx gets --first-frame, model, --download, --timeout 900."""
    captured = []
    monkeypatch.setattr(subprocess, "run", _make_fake_run(captured, str(tmp_path / "out.mp4")))
    p = MiniMaxProvider(env=env)
    req = GenerateRequest(
        scene_id="s1", asset_type="video", prompt="a panda walks",
        reference_image="https://ark/face.jpeg", duration_sec=5,
    )
    result = p.generate_video(req, out_path=tmp_path / "out.mp4",
                                model="MiniMax-Hailuo-2.3")
    assert result.ok is True
    assert result.provider == "minimax"
    assert result.model == "MiniMax-Hailuo-2.3"
    cmd = captured[0]
    assert "mmx" in cmd and "video" in cmd and "generate" in cmd
    assert "--first-frame" in cmd
    assert cmd[cmd.index("--first-frame") + 1] == "https://ark/face.jpeg"
    assert "--download" in cmd
    assert "--timeout" in cmd
    assert cmd[cmd.index("--timeout") + 1] == "900"


# --- S2V mode (character-locked) -------------------------------------------

def test_s2v_mode_passes_subject_image(env, tmp_path, monkeypatch):
    """S2V: mmx gets --subject-image + --first-frame (from ref in req.options)."""
    captured = []
    monkeypatch.setattr(subprocess, "run", _make_fake_run(captured, str(tmp_path / "out.mp4")))
    p = MiniMaxProvider(env=env)
    req = GenerateRequest(
        scene_id="s1", asset_type="video", prompt="a panda walks",
        reference_image="/tmp/face.jpg", duration_sec=6,
        options={"character_ref": "/tmp/canonical.jpg"},
    )
    result = p.generate_video(req, out_path=tmp_path / "out.mp4",
                                model="MiniMax-S2V-01")
    assert result.ok is True
    cmd = captured[0]
    assert "--subject-image" in cmd
    assert cmd[cmd.index("--subject-image") + 1] == "/tmp/canonical.jpg"
    assert "--first-frame" in cmd
    assert cmd[cmd.index("--first-frame") + 1] == "/tmp/face.jpg"


# --- SEF mode (start-end frame) --------------------------------------------

def test_sef_mode_passes_last_frame_equal_to_first(env, tmp_path, monkeypatch):
    """SEF: --first-frame and --last-frame both set to the same source."""
    captured = []
    monkeypatch.setattr(subprocess, "run", _make_fake_run(captured, str(tmp_path / "out.mp4")))
    p = MiniMaxProvider(env=env)
    req = GenerateRequest(
        scene_id="s1", asset_type="video", prompt="a panda walks",
        reference_image="/tmp/face.jpg", duration_sec=6,
    )
    p.generate_video(req, out_path=tmp_path / "out.mp4",
                       model="MiniMax-Hailuo-02")  # mmx_mode=sef
    cmd = captured[0]
    assert "--first-frame" in cmd
    assert "--last-frame" in cmd
    assert cmd[cmd.index("--first-frame") + 1] == "/tmp/face.jpg"
    assert cmd[cmd.index("--last-frame") + 1] == "/tmp/face.jpg"


# --- error handling --------------------------------------------------------

def test_mmx_nonzero_exit_returns_failure(env, tmp_path, monkeypatch):
    """mmx rc != 0 → adapter returns ok=False with stderr in error."""
    captured = []
    monkeypatch.setattr(subprocess, "run",
                        _make_fake_run(captured, str(tmp_path / "out.mp4"),
                                        rc=1, stderr="rate limit"))
    p = MiniMaxProvider(env=env)
    req = GenerateRequest(
        scene_id="s1", asset_type="video", prompt="x",
        reference_image="/tmp/face.jpg", duration_sec=5,
    )
    result = p.generate_video(req, out_path=tmp_path / "out.mp4",
                                model="MiniMax-Hailuo-2.3")
    assert result.ok is False
    assert "rate limit" in result.error or "rc=1" in result.error


def test_mmx_timeout_returns_failure(env, tmp_path, monkeypatch):
    """subprocess.TimeoutExpired → adapter returns ok=False with timeout message."""
    def fake_run(cmd, *args, **kwargs):
        raise subprocess.TimeoutExpired(cmd, timeout=1200)
    monkeypatch.setattr(subprocess, "run", fake_run)
    p = MiniMaxProvider(env=env)
    req = GenerateRequest(
        scene_id="s1", asset_type="video", prompt="x",
        reference_image="/tmp/face.jpg", duration_sec=5,
    )
    result = p.generate_video(req, out_path=tmp_path / "out.mp4",
                                model="MiniMax-Hailuo-2.3")
    assert result.ok is False
    assert "timeout" in result.error.lower() or "timed out" in result.error.lower()


# --- cost_estimate ---------------------------------------------------------

def test_cost_estimate_uses_model_catalog(env):
    """Video cost = model.cost_per_sec_yuan × duration_sec."""
    p = MiniMaxProvider(env=env)
    req = GenerateRequest(
        scene_id="s1", asset_type="video", prompt="x", duration_sec=6,
        options={"model": "MiniMax-Hailuo-2.3"},
    )
    cost = p.cost_estimate(req)
    # MiniMax-Hailuo-2.3 is ¥1.0/s × 6s = ¥6.0
    assert cost == pytest.approx(6.0)

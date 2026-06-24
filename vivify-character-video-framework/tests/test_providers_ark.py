"""Tests for vivify.providers.ark — 火山方舟 Ark adapter.

The adapter wraps two real endpoints:
  - POST /images/generations (sync, returns URL → download)
  - POST /contents/generations/tasks (async, poll until succeeded)

These tests mock `subprocess.run` at the curl boundary, so no real HTTP
is ever made. We assert on the curl args to confirm correct vendor API
contract; we assert on the return to confirm correct adapter contract.
"""

import base64
import json
import subprocess
from pathlib import Path
from unittest.mock import patch, MagicMock

import pytest

from vivify.providers.ark import ArkProvider
from vivify.providers.base import GenerateRequest, GenerateResult


# --- shared fixtures --------------------------------------------------------

@pytest.fixture
def env():
    return {
        "ARK_API_KEY": "test-key-315f0bff-f4d9-4b6d-ab98-6ef66a4195d2",
        "ARK_BASE_URL": "https://ark.cn-beijing.volces.com/api/v3",
    }


def _fake_curl(http_code: int, body: str = "", stderr: str = ""):
    """Returns a mock subprocess.run result compatible with curl()'s parsing."""
    m = MagicMock()
    m.returncode = 0
    if body:
        # curl appends __HTTP_CODE__NNN__ marker
        m.stdout = f"{body}\n__HTTP_CODE__{http_code}__"
    else:
        m.stdout = f"__HTTP_CODE__{http_code}__"
    m.stderr = stderr
    return m


def _mock_curl_success(http_code=200, body_dict=None, downloaded_path=None):
    """Return a side_effect function for subprocess.run mocking."""
    def side_effect(cmd, *args, **kwargs):
        out_file = None
        for i, c in enumerate(cmd):
            if c == "-o" and i + 1 < len(cmd):
                out_file = cmd[i + 1]
                break
        if out_file:
            # download path — write a tiny JPEG marker
            Path(out_file).parent.mkdir(parents=True, exist_ok=True)
            Path(out_file).write_bytes(b"\xff\xd8\xff\xe0FAKEJPEG")
            return _fake_curl(http_code, body="")
        body = json.dumps(body_dict or {})
        return _fake_curl(http_code, body=body)
    return side_effect


# --- image generation -------------------------------------------------------

def test_generate_image_happy_path(env, tmp_path, monkeypatch):
    """Image gen: POST → 200 with URL → GET → download to out_path."""
    out = tmp_path / "img.jpg"
    responses = iter([
        _fake_curl(200, body=json.dumps({"data": [{"url": "https://ark/x.jpeg"}]})),
        _fake_curl(200, body=""),  # download
    ])

    def fake_run(cmd, *args, **kwargs):
        # Detect download phase by presence of -o
        is_download = "-o" in cmd
        if is_download:
            out_file = cmd[cmd.index("-o") + 1]
            Path(out_file).write_bytes(b"\xff\xd8\xff\xe0FAKE")
            return _fake_curl(200, body="")
        return next(responses)

    monkeypatch.setattr(subprocess, "run", fake_run)
    p = ArkProvider(env=env)
    req = GenerateRequest(scene_id="s1", asset_type="image", prompt="red circle")
    result = p.generate_image(req, out_path=out)
    assert result.ok is True
    assert result.provider == "ark"
    assert result.model == "doubao-seedream-4-0-250828"
    assert result.local_path == out
    assert result.public_url == "https://ark/x.jpeg"
    assert out.exists()
    assert out.read_bytes()[:2] == b"\xff\xd8"


def test_generate_image_api_error_raises(env, tmp_path, monkeypatch):
    """Non-200 from Ark: adapter returns ok=False with error message."""
    out = tmp_path / "img.jpg"
    monkeypatch.setattr(
        subprocess, "run",
        lambda *a, **k: _fake_curl(401, body=json.dumps({"error": "InvalidAPIKey"})),
    )
    p = ArkProvider(env=env)
    req = GenerateRequest(scene_id="s1", asset_type="image", prompt="x")
    result = p.generate_image(req, out_path=out)
    assert result.ok is False
    assert "401" in result.error
    assert "InvalidAPIKey" in result.error


def test_generate_image_with_local_reference(env, tmp_path, monkeypatch):
    """Local reference image gets base64-encoded inline as data URI."""
    ref = tmp_path / "face.jpg"
    ref.write_bytes(b"\xff\xd8\xff\xe0FACE")
    captured_cmd = []

    def fake_run(cmd, *args, **kwargs):
        captured_cmd.append(cmd)
        is_download = "-o" in cmd
        if is_download:
            out_file = cmd[cmd.index("-o") + 1]
            Path(out_file).write_bytes(b"\xff\xd8\xff\xe0FAKE")
            return _fake_curl(200, body="")
        return _fake_curl(200, body=json.dumps({"data": [{"url": "https://ark/y.jpeg"}]}))

    monkeypatch.setattr(subprocess, "run", fake_run)
    p = ArkProvider(env=env)
    req = GenerateRequest(
        scene_id="s1", asset_type="image", prompt="x",
        reference_image=str(ref),
    )
    p.generate_image(req, out_path=tmp_path / "out.jpg")
    # The POST body should have reference_image as a data: URI
    post_cmd = captured_cmd[0]
    # The body is sent via -d or --data-binary @tmpfile
    assert any("-d" in c or "data-binary" in c for c in post_cmd)
    # Find the body argument
    body_str = None
    if "-d" in post_cmd:
        body_str = post_cmd[post_cmd.index("-d") + 1]
    else:
        # data-binary @tmpfile
        idx = post_cmd.index("--data-binary")
        path = post_cmd[idx + 1].lstrip("@")
        body_str = Path(path).read_text()
    body = json.loads(body_str)
    assert body["reference_image"].startswith("data:image/jpeg;base64,")


# --- video generation -------------------------------------------------------

def test_generate_video_happy_path(env, tmp_path, monkeypatch):
    """Video gen: POST task → 200 (task_id) → poll → succeeded → download."""
    task_id = "cgt-20260608140817-l579p"
    responses = iter([
        # POST task
        _fake_curl(200, body=json.dumps({"id": task_id})),
        # GET poll #1: running
        _fake_curl(200, body=json.dumps({"status": "running", "id": task_id})),
        # GET poll #2: succeeded
        _fake_curl(200, body=json.dumps({
            "status": "succeeded", "id": task_id,
            "content": {"video_url": "https://ark-content/v.mp4"},
        })),
        # GET download
        _fake_curl(200, body=""),
    ])

    def fake_run(cmd, *args, **kwargs):
        is_download = "-o" in cmd
        if is_download:
            out_file = cmd[cmd.index("-o") + 1]
            Path(out_file).write_bytes(b"\x00\x00\x00\x18ftypFAKE")
            return _fake_curl(200, body="")
        return next(responses)

    monkeypatch.setattr(subprocess, "run", fake_run)
    # Make poll fast — patch time.sleep
    import time as _time
    monkeypatch.setattr(_time, "sleep", lambda *a, **k: None)

    p = ArkProvider(env=env)
    req = GenerateRequest(
        scene_id="s1", asset_type="video", prompt="a panda walks",
        reference_image="https://ark/face.jpeg", duration_sec=5,
    )
    out = tmp_path / "v.mp4"
    result = p.generate_video(req, out_path=out, model="doubao-seedance-1-5-pro-251215")
    assert result.ok is True
    assert result.asset_id == task_id
    assert result.local_path == out
    assert result.public_url == "https://ark-content/v.mp4"
    assert result.model == "doubao-seedance-1-5-pro-251215"
    assert out.exists()


def test_generate_video_poll_fails(env, tmp_path, monkeypatch):
    """Poll returns status='failed' → adapter returns ok=False with error."""
    task_id = "cgt-FAIL"
    responses = iter([
        _fake_curl(200, body=json.dumps({"id": task_id})),
        _fake_curl(200, body=json.dumps({"status": "failed", "error": "ContentPolicy"})),
    ])

    def fake_run(cmd, *args, **kwargs):
        return next(responses)

    monkeypatch.setattr(subprocess, "run", fake_run)
    import time as _time
    monkeypatch.setattr(_time, "sleep", lambda *a, **k: None)

    p = ArkProvider(env=env)
    req = GenerateRequest(scene_id="s1", asset_type="video", prompt="x", duration_sec=5)
    result = p.generate_video(req, out_path=tmp_path / "v.mp4", model="doubao-seedance-1-5-pro-251215")
    assert result.ok is False
    assert "failed" in result.error.lower()
    assert "ContentPolicy" in result.error


def test_generate_video_poll_timeout(env, tmp_path, monkeypatch):
    """Poll never returns succeeded → adapter times out and returns ok=False."""
    task_id = "cgt-SLOW"
    responses = iter([
        _fake_curl(200, body=json.dumps({"id": task_id})),
        # Always running
    ] * 5)

    def fake_run(cmd, *args, **kwargs):
        return next(responses)

    monkeypatch.setattr(subprocess, "run", fake_run)
    import time as _time
    # Make the timeout effectively instant
    monkeypatch.setattr(_time, "sleep", lambda *a, **k: None)

    # Patch the adapter's poll timeout to be 0
    p = ArkProvider(env=env)
    p._poll_timeout_sec = 0
    req = GenerateRequest(scene_id="s1", asset_type="video", prompt="x", duration_sec=5)
    result = p.generate_video(req, out_path=tmp_path / "v.mp4", model="doubao-seedance-1-5-pro-251215")
    assert result.ok is False
    assert "timeout" in result.error.lower() or "timed out" in result.error.lower()


# --- cost_estimate ---------------------------------------------------------

def test_cost_estimate_image(env):
    """Image cost is fixed ¥0.20 (matches vivify.pricing.COST_IMAGE_YUAN)."""
    p = ArkProvider(env=env)
    req = GenerateRequest(scene_id="s1", asset_type="image", prompt="x")
    assert p.cost_estimate(req) == pytest.approx(0.20)


def test_cost_estimate_video_uses_model_catalog(env, monkeypatch):
    """Video cost = model.cost_per_sec_yuan × duration_sec."""
    p = ArkProvider(env=env)
    req = GenerateRequest(
        scene_id="s1", asset_type="video", prompt="x", duration_sec=10,
        options={"model": "doubao-seedance-1-5-pro-251215"},
    )
    cost = p.cost_estimate(req)
    # 1.5-pro is ¥0.4/s × 10s = ¥4.0
    assert cost == pytest.approx(4.0)


def test_cost_estimate_video_no_duration_is_zero(env):
    """If duration_sec is None, cost_estimate returns 0 (caller should fill it)."""
    p = ArkProvider(env=env)
    req = GenerateRequest(scene_id="s1", asset_type="video", prompt="x")
    assert p.cost_estimate(req) == 0.0


# --- character_ref passthrough (Task 3) ------------------------------------

def test_generate_video_without_character_ref_has_one_image_entry(env, tmp_path, monkeypatch):
    """When character_ref is None, content array has exactly 1 image_url entry
    (the first-frame) — confirms the second-image path is opt-in, not always-on.
    """
    task_id = "cgt-NOREF"
    captured_post_body = {}

    def fake_run(cmd, *args, **kwargs):
        is_download = "-o" in cmd
        if is_download:
            out_file = cmd[cmd.index("-o") + 1]
            Path(out_file).write_bytes(b"\x00\x00\x00\x18ftypFAKE")
            return _fake_curl(200, body="")
        if "-d" in cmd:
            d_idx = cmd.index("-d") + 1
            captured_post_body.update(json.loads(cmd[d_idx]))
            return _fake_curl(200, body=json.dumps({"id": task_id}))
        if "--data-binary" in cmd:
            body_path = cmd[cmd.index("--data-binary") + 1].lstrip("@")
            captured_post_body.update(json.loads(Path(body_path).read_text()))
            return _fake_curl(200, body=json.dumps({"id": task_id}))
        return _fake_curl(200, body=json.dumps({
            "status": "succeeded", "id": task_id,
            "content": {"video_url": "https://ark-content/v.mp4"},
        }))

    monkeypatch.setattr(subprocess, "run", fake_run)
    import time as _time
    monkeypatch.setattr(_time, "sleep", lambda *a, **k: None)

    p = ArkProvider(env=env)
    req = GenerateRequest(
        scene_id="s1", asset_type="video", prompt="x",
        reference_image="https://ark/first-frame.jpg",
        duration_sec=5,
    )
    p.generate_video(
        req, out_path=tmp_path / "v.mp4",
        model="doubao-seedance-1-5-pro-251215",
        # NO character_ref
    )

    content = captured_post_body["content"]
    image_entries = [c for c in content if c.get("type") == "image_url"]
    assert len(image_entries) == 1, (
        f"expected 1 image_url entry (first-frame only), got {len(image_entries)}: {content}"
    )


def test_generate_video_with_character_ref_adds_second_image_to_content(env, tmp_path, monkeypatch):
    """When character_ref is passed (in addition to reference_image first-frame),
    the POST body must include TWO image_url content entries — one for the
    first-frame and one for the canonical character ref. This is the ID-drift
    fix path: Seedance gets both the per-shot frame AND the locked character
    reference.

    Without character_ref, content array has 1 image_url entry (the first-frame).
    """
    # Write a real canonical face jpeg so _resolve_reference can base64-encode it
    canonical_face = tmp_path / "face.jpg"
    canonical_face.write_bytes(b"\xff\xd8\xff\xe0fake-jpeg")

    task_id = "cgt-CHARREF"
    captured_post_body = {}

    def fake_run(cmd, *args, **kwargs):
        is_download = "-o" in cmd
        if is_download:
            out_file = cmd[cmd.index("-o") + 1]
            Path(out_file).write_bytes(b"\x00\x00\x00\x18ftypFAKE")
            return _fake_curl(200, body="")
        # Read the JSON body — either via -d (small) or --data-binary @file (large)
        if "-d" in cmd:
            d_idx = cmd.index("-d") + 1
            captured_post_body.update(json.loads(cmd[d_idx]))
            return _fake_curl(200, body=json.dumps({"id": task_id}))
        if "--data-binary" in cmd:
            body_path = cmd[cmd.index("--data-binary") + 1].lstrip("@")
            captured_post_body.update(json.loads(Path(body_path).read_text()))
            return _fake_curl(200, body=json.dumps({"id": task_id}))
        return _fake_curl(200, body=json.dumps({
            "status": "succeeded", "id": task_id,
            "content": {"video_url": "https://ark-content/v.mp4"},
        }))

    monkeypatch.setattr(subprocess, "run", fake_run)
    import time as _time
    monkeypatch.setattr(_time, "sleep", lambda *a, **k: None)

    p = ArkProvider(env=env)
    req = GenerateRequest(
        scene_id="s1", asset_type="video", prompt="a panda walks",
        reference_image="https://ark/first-frame.jpg",
        duration_sec=5,
    )
    p.generate_video(
        req,
        out_path=tmp_path / "v.mp4",
        model="doubao-seedance-1-5-pro-251215",
        character_ref=str(canonical_face),
    )

    # Content must have: text + 2 image_url entries (first-frame + character_ref)
    content = captured_post_body["content"]
    image_entries = [c for c in content if c.get("type") == "image_url"]
    assert len(image_entries) == 2, (
        f"expected 2 image_url entries (first-frame + character_ref), "
        f"got {len(image_entries)}: {content}"
    )
    # The character_ref path should be the second one — verify it's NOT the first-frame
    urls = [c["image_url"]["url"] for c in image_entries]
    # face.jpg is base64-encoded so look for "data:image/" prefix
    assert any("data:image/" in u for u in urls), (
        f"character_ref base64 data URI not found in image entries: {urls}"
    )
    # The first-frame is still https:// — both should be present
    assert any("https://" in u for u in urls), (
        f"first-frame https URL missing from image entries: {urls}"
    )

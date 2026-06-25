"""Tests for vivify.episode_driver — per-shot asset pipeline.

The driver is the bridge between the old subprocess-based render_episode.py
and the new in-process orchestrator pipeline. Per shot it drives:
  1. generate_asset(asset_type='image')       — seed frame
  2. generate_asset(asset_type='video')       — image → video, with character_ref
  3. TTS                                       — direct adapter call (海螺)
and writes real results (paths + cost) into the shots table.

The driver is the only place in the codebase that knows how a storyboard
shot maps to three independent asset generations and one DB row.
"""
import sqlite3
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from vivify.episode_driver import (
    render_episode_assets,
    ShotAssetResult,
    _call_tts,
    _resolve_canonical_ref,
    _resolve_voice_profile,
)
from vivify.providers.base import GenerateRequest, GenerateResult


# --- fixtures ---------------------------------------------------------------

@pytest.fixture
def empty_db(tmp_path):
    """A SQLite DB with just the episodes+shots tables, suitable for the
    driver's update_shot_row to write into."""
    db = tmp_path / "test.db"
    conn = sqlite3.connect(db)
    conn.execute("""CREATE TABLE episodes (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        cost_yuan REAL, render_completed_at TEXT, status TEXT
    )""")
    conn.execute("""CREATE TABLE shots (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        episode_id INTEGER, shot_number INTEGER,
        image_path TEXT, video_path TEXT, cost_yuan REAL,
        model_used TEXT, kling_prompt TEXT
    )""")
    conn.commit()
    conn.close()
    return db


@pytest.fixture
def two_shot_storyboard():
    return [
        {"n": 1, "start_sec": 0.0, "duration_sec": 4,
         "kling_prompt": "镜头1: 推镜头 峰哥 走入竹林", "scene_id": "bamboo_courtyard"},
        {"n": 2, "start_sec": 4.0, "duration_sec": 4,
         "kling_prompt": "镜头2: 拉镜头 峰哥 转身回眸", "scene_id": "bamboo_courtyard"},
    ]


@pytest.fixture
def two_voiceovers():
    return [
        {"n": 1, "start_sec": 0.5, "text": "嘿,你看那山"},
        {"n": 2, "start_sec": 4.5, "text": "嗯……风停了"},
    ]


# --- core happy path --------------------------------------------------------

def test_render_episode_assets_serializes_shot_results(
    empty_db, two_shot_storyboard, two_voiceovers, tmp_path, monkeypatch,
):
    """Each shot must produce one ShotAssetResult with image_path,
    video_path, tts_path populated, and a total_cost_yuan = sum of
    image + video + tts costs (NOT the estimate from pricing)."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")

    # Stub generate_asset: return deterministic GenerateResult per call
    import vivify.episode_driver as driver_mod
    call_log = []

    def fake_generate_asset(req, **kwargs):
        call_log.append((req.asset_type, req.scene_id))
        if req.asset_type == "image":
            return GenerateResult(
                ok=True, provider="ark", model="seedream",
                local_path=tmp_path / f"img-{req.scene_id}.jpg",
                cost_yuan=1.5, duration_ms=2000,
            )
        if req.asset_type == "video":
            return GenerateResult(
                ok=True, provider="ark", model="seedance",
                local_path=tmp_path / f"vid-{req.scene_id}.mp4",
                cost_yuan=3.0, duration_ms=15000,
            )
        raise AssertionError(f"unexpected asset_type: {req.asset_type}")

    # Stub TTS: return success
    def fake_call_tts(req, env, voice_profile, out_path):
        out_path.write_bytes(b"fake-mp3")
        return GenerateResult(
            ok=True, provider="minimax", model="speech-02-hd",
            local_path=out_path, cost_yuan=0.05, duration_ms=500,
        )

    monkeypatch.setattr(driver_mod, "generate_asset", fake_generate_asset)
    monkeypatch.setattr(driver_mod, "_call_tts", fake_call_tts)

    results = render_episode_assets(
        shots=two_shot_storyboard,
        voiceovers=two_voiceovers,
        episode_id="EP001",
        character_id="fengge",
        env={},
        out_dir=tmp_path / "render",
        db_path=str(empty_db),
        voice_profile={"voice_id": "male-qn-jingying", "speed": 0.78,
                       "pitch": -2, "emotion": "neutral", "vol": 1.0},
        canonical_ref="/abs/canonical/face.jpg",
    )

    # 2 shots × (image + video) = 4 generate_asset calls; 2 TTS calls
    assert len(call_log) == 4
    asset_types_in_order = [t for t, _ in call_log]
    assert asset_types_in_order == ["image", "video", "image", "video"]

    # 2 ShotAssetResults returned
    assert len(results) == 2

    # Real cost = image 1.5 + video 3.0 + tts 0.05 = 4.55 per shot
    expected_per_shot = 1.5 + 3.0 + 0.05
    for r in results:
        assert r.image_path is not None
        assert r.video_path is not None
        assert r.tts_path is not None
        assert abs(r.total_cost_yuan - expected_per_shot) < 0.01, (
            f"shot {r.shot_number} total_cost_yuan={r.total_cost_yuan} "
            f"expected={expected_per_shot}"
        )


def test_render_episode_assets_writes_per_shot_db_rows(
    empty_db, two_shot_storyboard, two_voiceovers, tmp_path, monkeypatch,
):
    """After render, the shots table must have its image_path + video_path
    + cost_yuan columns populated for each shot (real, not estimated)."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")

    import vivify.episode_driver as driver_mod

    def fake_generate_asset(req, **kwargs):
        ext = "jpg" if req.asset_type == "image" else "mp4"
        return GenerateResult(
            ok=True, provider="ark", model="x",
            local_path=tmp_path / f"{req.asset_type}-{req.scene_id}.{ext}",
            cost_yuan=1.0, duration_ms=100,
        )

    def fake_call_tts(req, env, voice_profile, out_path):
        out_path.write_bytes(b"x")
        return GenerateResult(
            ok=True, provider="minimax", model="tts",
            local_path=out_path, cost_yuan=0.0, duration_ms=100,
        )

    monkeypatch.setattr(driver_mod, "generate_asset", fake_generate_asset)
    monkeypatch.setattr(driver_mod, "_call_tts", fake_call_tts)

    # Pre-insert episode + 2 shot rows so we can update them
    conn = sqlite3.connect(str(empty_db))
    ep_pk = conn.execute("INSERT INTO episodes DEFAULT VALUES").lastrowid
    for s in two_shot_storyboard:
        conn.execute(
            "INSERT INTO shots (episode_id, shot_number, kling_prompt) "
            "VALUES (?, ?, ?)",
            (ep_pk, s["n"], s["kling_prompt"]),
        )
    conn.commit()
    conn.close()

    render_episode_assets(
        shots=two_shot_storyboard, voiceovers=two_voiceovers,
        episode_id="EP001", character_id="fengge", env={},
        out_dir=tmp_path / "r", db_path=str(empty_db),
        episode_pk=ep_pk,
        voice_profile={"voice_id": "x", "speed": 1.0, "pitch": 0,
                       "emotion": "neutral", "vol": 1.0},
        canonical_ref=None,
    )

    conn = sqlite3.connect(str(empty_db))
    rows = conn.execute(
        "SELECT shot_number, image_path, video_path, cost_yuan FROM shots "
        "WHERE episode_id = ? ORDER BY shot_number", (ep_pk,)
    ).fetchall()
    conn.close()
    assert len(rows) == 2
    for n, ip, vp, cost in rows:
        assert ip is not None, f"shot {n} image_path is None"
        assert vp is not None, f"shot {n} video_path is None"
        # cost_yuan on shot row = image (1.0) + video (1.0) = 2.0
        # (TTS not tracked on shot row; tracked in ledger + total)
        assert cost == 2.0, f"shot {n} cost_yuan={cost} expected 2.0"


def test_render_episode_assets_accepts_db_shaped_shot_dicts(empty_db, tmp_path, monkeypatch):
    """commands/episode._get_shots returns dicts with keys like
    'shot_number' (DB column), 'voiceover_text', etc. — NOT 'n' / 'kling_prompt'.
    The driver must accept BOTH shapes (parse_storyboard-style and DB-style)
    so the same driver works whether called from the CLI or from a script
    that reads from SQLite.
    """
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")

    import vivify.episode_driver as driver_mod

    def fake_generate_asset(req, **kwargs):
        return GenerateResult(
            ok=True, provider="ark", model="x",
            local_path=tmp_path / f"x-{req.scene_id}", cost_yuan=1.0,
        )
    monkeypatch.setattr(driver_mod, "generate_asset", fake_generate_asset)
    monkeypatch.setattr(driver_mod, "_call_tts", lambda *a, **k: GenerateResult(
        ok=True, provider="minimax", model="tts", cost_yuan=0.0,
    ))

    db_shaped_shots = [
        {"shot_number": 1, "duration_sec": 5,
         "prompt": "镜头1: 推镜头 峰哥 走入竹林", "scene_id": "bamboo_1"},
        {"shot_number": 2, "duration_sec": 5,
         "prompt": "镜头2: 拉镜头 峰哥 转身回眸", "scene_id": "bamboo_2"},
    ]
    # No voiceovers → TTS skipped
    results = render_episode_assets(
        shots=db_shaped_shots, voiceovers=[],
        episode_id="EP-DB", character_id="fengge", env={},
        out_dir=tmp_path / "r", db_path=str(empty_db),
        voice_profile={"voice_id": "x", "speed": 1.0, "pitch": 0,
                       "emotion": "neutral", "vol": 1.0},
        canonical_ref=None,
    )
    # Both shots processed
    assert len(results) == 2
    assert [r.shot_number for r in results] == [1, 2]


def test_render_episode_assets_skips_tts_when_no_voiceover(
    empty_db, two_shot_storyboard, tmp_path, monkeypatch,
):
    """If no voiceover text matches a shot, TTS is not called and
    tts_path stays None for that shot."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")

    import vivify.episode_driver as driver_mod
    tts_call_count = {"n": 0}

    def fake_generate_asset(req, **kwargs):
        ext = "jpg" if req.asset_type == "image" else "mp4"
        return GenerateResult(
            ok=True, provider="ark", model="x",
            local_path=tmp_path / f"x-{req.scene_id}.{ext}",
            cost_yuan=1.0, duration_ms=100,
        )

    def fake_call_tts(req, env, voice_profile, out_path):
        tts_call_count["n"] += 1
        out_path.write_bytes(b"x")
        return GenerateResult(
            ok=True, provider="minimax", model="tts",
            local_path=out_path, cost_yuan=0.0, duration_ms=100,
        )

    monkeypatch.setattr(driver_mod, "generate_asset", fake_generate_asset)
    monkeypatch.setattr(driver_mod, "_call_tts", fake_call_tts)

    # Empty voiceovers → no TTS calls
    results = render_episode_assets(
        shots=two_shot_storyboard, voiceovers=[],
        episode_id="EP001", character_id="fengge", env={},
        out_dir=tmp_path / "r", db_path=str(empty_db),
        voice_profile={"voice_id": "x", "speed": 1.0, "pitch": 0,
                       "emotion": "neutral", "vol": 1.0},
        canonical_ref=None,
    )
    assert tts_call_count["n"] == 0
    for r in results:
        assert r.tts_path is None
        assert r.tts_cost_yuan == 0.0
        # total = image (1.0) + video (1.0) = 2.0
        assert abs(r.total_cost_yuan - 2.0) < 0.01


# --- resolver helpers --------------------------------------------------------

def test_resolve_canonical_ref_with_character_yaml(tmp_path):
    """If a character.yaml exists with character.canonical.primary, return
    the resolved absolute path. Otherwise return None."""
    char_dir = tmp_path / "characters" / "fengge"
    char_dir.mkdir(parents=True)
    canonical_dir = char_dir / "canonical"
    canonical_dir.mkdir()
    (canonical_dir / "face.jpg").write_bytes(b"jpg")
    (char_dir / "character.yaml").write_text(
        "character:\n"
        "  canonical:\n"
        "    primary: canonical/face.jpg\n"
    )

    from vivify.episode_driver import _resolve_canonical_ref
    result = _resolve_canonical_ref(char_dir, str(char_dir / "character.yaml"))
    assert result is not None
    assert Path(result).name == "face.jpg"
    assert Path(result).exists()


def test_resolve_canonical_ref_missing_yaml_returns_none(tmp_path):
    char_dir = tmp_path / "characters" / "fengge"
    char_dir.mkdir(parents=True)
    from vivify.episode_driver import _resolve_canonical_ref
    # No character.yaml at all
    assert _resolve_canonical_ref(char_dir, None) is None


def test_resolve_voice_profile_from_yaml(tmp_path):
    """Resolve character.yaml's voice_profiles[tone] to a profile dict.
    Falls back to a sane default if tone is missing."""
    char_dir = tmp_path / "characters" / "fengge"
    char_dir.mkdir(parents=True)
    yaml_path = char_dir / "character.yaml"
    yaml_path.write_text(
        "character:\n"
        "  voice_profiles:\n"
        "    治愈:\n"
        "      voice_id: male-qn-jingying\n"
        "      speed: 0.78\n"
        "      pitch: -2\n"
        "      emotion: neutral\n"
        "      vol: 1.0\n"
    )
    from vivify.episode_driver import _resolve_voice_profile
    p = _resolve_voice_profile(yaml_path, "治愈")
    assert p["voice_id"] == "male-qn-jingying"
    assert p["speed"] == 0.78
    # Unknown tone → fallback defaults
    p2 = _resolve_voice_profile(yaml_path, "不存在的调")
    assert p2["voice_id"]  # truthy default
    # No yaml at all → fallback defaults
    p3 = _resolve_voice_profile(None, "治愈")
    assert p3["voice_id"]


# --- tts adapter shim --------------------------------------------------------

def test_call_tts_uses_mmx_cli_when_available(tmp_path, monkeypatch):
    """Preferred path: shell out to `mmx speech synthesize` (uses mmx's
    self-managed auth, no MINIMAX_API_KEY env needed). Mock subprocess.run
    to capture the argv and assert voice_profile fields map to flags.
    """
    from vivify.episode_driver import _call_tts
    from vivify.providers.base import GenerateRequest

    out_path = tmp_path / "tts.mp3"
    req = GenerateRequest(scene_id="s1", asset_type="tts", prompt="嘿")
    profile = {"voice_id": "male-qn-jingying", "speed": 0.78,
               "pitch": -2, "emotion": "neutral", "vol": 1.0}

    captured_cmd = {}

    def fake_run(cmd, *args, **kwargs):
        captured_cmd["cmd"] = cmd
        # Simulate mmx writing the mp3 to --out
        out_idx = cmd.index("--out") + 1
        Path(cmd[out_idx]).write_bytes(b"mp3-bytes-from-mmx")
        return MagicMock(returncode=0, stdout="", stderr="")

    monkeypatch.setattr("vivify.episode_driver.subprocess.run", fake_run)

    result = _call_tts(req, env={},    # empty env — mmx handles its own auth
                       voice_profile=profile, out_path=out_path)

    assert result.ok is True, f"result.error: {result.error}"
    assert result.provider == "minimax"
    assert result.local_path == out_path
    assert Path(out_path).read_bytes() == b"mp3-bytes-from-mmx"

    # Verify mmx CLI was invoked with correct argv
    cmd = captured_cmd["cmd"]
    assert cmd[0:3] == ["mmx", "speech", "synthesize"], f"unexpected argv: {cmd}"
    assert "--text" in cmd
    text_idx = cmd.index("--text") + 1
    assert cmd[text_idx] == "嘿"
    assert "--voice" in cmd
    voice_idx = cmd.index("--voice") + 1
    assert cmd[voice_idx] == "male-qn-jingying"
    assert "--speed" in cmd
    speed_idx = cmd.index("--speed") + 1
    assert float(cmd[speed_idx]) == 0.78
    assert "--pitch" in cmd
    pitch_idx = cmd.index("--pitch") + 1
    assert int(cmd[pitch_idx]) == -2
    assert "--volume" in cmd
    vol_idx = cmd.index("--volume") + 1
    assert float(cmd[vol_idx]) == 1.0
    out_idx = cmd.index("--out") + 1
    assert cmd[out_idx] == str(out_path)


def test_call_tts_falls_back_to_legacy_when_mmx_missing(tmp_path, monkeypatch):
    """When mmx binary is not on PATH (FileNotFoundError), the shim must
    fall back to render_episode.gen_tts (the legacy path that needs
    MINIMAX_API_KEY env). We mock the legacy function to avoid a real call.
    """
    from vivify.episode_driver import _call_tts
    from vivify.providers.base import GenerateRequest

    out_path = tmp_path / "tts.mp3"
    req = GenerateRequest(scene_id="s1", asset_type="tts", prompt="嘿")
    profile = {"voice_id": "male-qn-jingying", "speed": 0.78,
               "pitch": -2, "emotion": "neutral", "vol": 1.0}

    # Simulate mmx not on PATH
    def fake_run_raises(cmd, *args, **kwargs):
        raise FileNotFoundError("[Errno 2] No such file or directory: 'mmx'")

    monkeypatch.setattr("vivify.episode_driver.subprocess.run", fake_run_raises)

    # Mock the legacy gen_tts as the fallback target
    captured = {}

    def fake_gen_tts(env_arg, text_arg, out_arg, tone=None):
        captured["env"] = env_arg
        captured["text"] = text_arg
        captured["out"] = out_arg
        captured["tone"] = tone
        Path(out_arg).write_bytes(b"mp3-bytes-legacy")

    import sys as _sys
    fake_module = MagicMock()
    fake_module.gen_tts = fake_gen_tts
    monkeypatch.setitem(_sys.modules, "render_episode", fake_module)

    result = _call_tts(req, env={"MINIMAX_API_KEY": "test"},
                       voice_profile=profile, out_path=out_path)

    assert result.ok is True
    assert result.provider == "minimax"
    assert Path(out_path).read_bytes() == b"mp3-bytes-legacy"
    # Legacy gen_tts was called with our shimmed env
    assert "CHARACTER" in captured["env"]
    assert captured["text"] == "嘿"
    assert captured["tone"] == "male-qn-jingying"


def test_call_tts_returns_error_when_both_paths_fail(tmp_path, monkeypatch):
    """If mmx is missing AND legacy gen_tts raises, return ok=False with
    a clear error (don't crash the render)."""
    from vivify.episode_driver import _call_tts
    from vivify.providers.base import GenerateRequest

    out_path = tmp_path / "tts.mp3"
    req = GenerateRequest(scene_id="s1", asset_type="tts", prompt="嘿")
    profile = {"voice_id": "x", "speed": 1.0, "pitch": 0,
               "emotion": "neutral", "vol": 1.0}

    monkeypatch.setattr("vivify.episode_driver.subprocess.run",
                        lambda *a, **k: (_ for _ in ()).throw(FileNotFoundError("no mmx")))

    # No legacy gen_tts available
    import sys as _sys
    fake_module = MagicMock(spec=[])  # no gen_tts attr
    monkeypatch.setitem(_sys.modules, "render_episode", fake_module)

    result = _call_tts(req, env={}, voice_profile=profile, out_path=out_path)
    assert result.ok is False
    assert "TTS" in (result.error or "") or "tts" in (result.error or "")


# --- legacy fallback tests (kept for regression) ----------------------------

def test_call_tts_writes_mp3_file_legacy(tmp_path, monkeypatch):
    """Legacy path: invoke render_episode.gen_tts (the original API)."""
    from vivify.episode_driver import _call_tts
    from vivify.providers.base import GenerateRequest

    out_path = tmp_path / "tts.mp3"
    req = GenerateRequest(scene_id="s1", asset_type="tts", prompt="嘿")
    profile = {"voice_id": "x", "speed": 1.0, "pitch": 0,
               "emotion": "neutral", "vol": 1.0}

    # Force mmx missing + legacy present
    monkeypatch.setattr("vivify.episode_driver.subprocess.run",
                        lambda *a, **k: (_ for _ in ()).throw(FileNotFoundError("no mmx")))

    captured = {}

    def fake_gen_tts(env_arg, text_arg, out_arg, tone=None):
        captured["text"] = text_arg
        captured["tone"] = tone
        Path(out_arg).write_bytes(b"mp3")

    import sys as _sys
    fake_module = MagicMock()
    fake_module.gen_tts = fake_gen_tts
    monkeypatch.setitem(_sys.modules, "render_episode", fake_module)

    result = _call_tts(req, env={"MINIMAX_API_KEY": "test"},
                       voice_profile=profile, out_path=out_path)
    assert result.ok is True
    assert captured["text"] == "嘿"


def test_call_tts_falls_back_to_legacy_gen_tts(tmp_path, monkeypatch):
    """When MiniMaxProvider has no generate_tts method (Task 2 not done),
    the shim must fall back to render_episode.gen_tts. We mock the legacy
    function to avoid a real API call, and assert the shim wires voice_profile
    through env['CHARACTER']['voice_profiles'] + tone arg."""
    from vivify.episode_driver import _call_tts
    from vivify.providers.base import GenerateRequest

    out_path = tmp_path / "tts.mp3"
    req = GenerateRequest(scene_id="s1", asset_type="tts", prompt="嘿")
    profile = {"voice_id": "male-qn-jingying", "speed": 0.78,
               "pitch": -2, "emotion": "neutral", "vol": 1.0}

    # Simulate mmx binary missing (force fallback to legacy path)
    monkeypatch.setattr("vivify.episode_driver.subprocess.run",
                        lambda *a, **k: (_ for _ in ()).throw(FileNotFoundError("no mmx")))

    # Mock the legacy import path
    captured = {}

    def fake_gen_tts(env_arg, text_arg, out_arg, tone=None):
        captured["env"] = env_arg
        captured["text"] = text_arg
        captured["out"] = out_arg
        captured["tone"] = tone
        Path(out_arg).write_bytes(b"mp3-bytes")

    # Inject fake gen_tts into the module so the shim's import picks it up
    import sys as _sys
    fake_module = MagicMock()
    fake_module.gen_tts = fake_gen_tts
    monkeypatch.setitem(_sys.modules, "render_episode", fake_module)

    result = _call_tts(req, env={"MINIMAX_API_KEY": "test"},
                       voice_profile=profile, out_path=out_path)

    assert result.ok is True
    assert result.provider == "minimax"
    assert result.local_path == out_path
    # Legacy gen_tts was called with our shimmed env that has CHARACTER
    assert "CHARACTER" in captured["env"]
    assert captured["text"] == "嘿"
    assert captured["tone"] == "male-qn-jingying"
    profiles = captured["env"]["CHARACTER"]["voice_profiles"]
    assert profiles["male-qn-jingying"]["speed"] == 0.78
    assert Path(captured["out"]).read_bytes() == b"mp3-bytes"

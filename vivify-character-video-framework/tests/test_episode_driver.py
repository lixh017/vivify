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

def test_call_tts_writes_mp3_file(tmp_path):
    """The _call_tts shim must invoke MiniMaxProvider.generate_tts and
    return its result. We mock the provider so no real API call."""
    from vivify.episode_driver import _call_tts
    from vivify.providers.base import GenerateRequest

    out_path = tmp_path / "tts.mp3"
    req = GenerateRequest(
        scene_id="s1", asset_type="tts", prompt="嘿",
    )
    profile = {"voice_id": "x", "speed": 1.0, "pitch": 0,
               "emotion": "neutral", "vol": 1.0}

    # Simulate Task 2 having shipped: the provider has generate_tts
    with patch("vivify.providers.minimax.MiniMaxProvider") as MockProvider:
        mock_instance = MagicMock()
        mock_instance.generate_tts.return_value = GenerateResult(
            ok=True, provider="minimax", model="speech-02-hd",
            local_path=out_path, cost_yuan=0.05, duration_ms=500,
        )
        MockProvider.return_value = mock_instance

        result = _call_tts(req, env={"MINIMAX_API_KEY": "test"},
                           voice_profile=profile, out_path=out_path)

    assert result.ok is True
    mock_instance.generate_tts.assert_called_once()
    call_kwargs = mock_instance.generate_tts.call_args.kwargs
    assert call_kwargs["voice_profile"] == profile
    assert call_kwargs["out_path"] == out_path


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

    # Simulate Task 2 NOT done: MiniMaxProvider has no generate_tts attribute
    with patch("vivify.providers.minimax.MiniMaxProvider") as MockProvider:
        mock_instance = MagicMock(spec=[])  # spec=[] means no attrs/methods
        MockProvider.return_value = mock_instance

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

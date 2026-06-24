"""Tests for vivify.asset_orchestrator — end-to-end pipeline.

The orchestrator coordinates: router → cost-cap → provider → retry → ledger.
Tests mock the provider and ledger to focus on the orchestration logic.
"""

import json
from pathlib import Path
from unittest.mock import patch, MagicMock

import pytest

from vivify.asset_orchestrator import generate_asset
from vivify.asset_router import RouterConfig
from vivify.asset_ledger import LedgerEntry
from vivify.providers.base import GenerateRequest, GenerateResult
from vivify.providers.ark import ArkProvider
from vivify.providers.stubs import KlingProvider


# --- fixtures ---------------------------------------------------------------

@pytest.fixture
def cfg():
    return RouterConfig(
        default_providers={"video": "ark", "image": "ark"},
        fallback_chains={"video": ["ark", "kling", "manual-pending"],
                          "image": ["ark", "jimeng", "manual-pending"]},
        cost_caps={"per_video_cny": 50, "per_asset_cny": 5, "monthly_cny": 60000},
    )


@pytest.fixture
def env():
    return {"ARK_API_KEY": "test-key", "ARK_BASE_URL": "https://ark/api/v3"}


@pytest.fixture
def isolated_ledger(tmp_path, monkeypatch):
    """Point ledger at tmp so tests don't pollute ~/.claude/agents/."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")
    # Re-import the orchestrator's reference to LEDGER_PATH at call time
    return tmp_path / "ledger.jsonl"


@pytest.fixture
def cfg_path(tmp_path, monkeypatch):
    """Point router config at tmp."""
    p = tmp_path / "router.yaml"
    p.write_text("""\
default_providers:
  video: ark
  image: ark
fallback_chains:
  video: [ark, kling, manual-pending]
  image: [ark, jimeng, manual-pending]
cost_caps:
  per_video_cny: 50
  per_asset_cny: 5
  monthly_cny: 60000
""")
    monkeypatch.setattr("vivify.asset_router.DEFAULT_CONFIG_PATH", p)
    return p


# --- happy path -------------------------------------------------------------

def test_generate_asset_happy_path(cfg_path, env, isolated_ledger):
    """End-to-end: pick ark → call adapter → write success ledger row."""
    req = GenerateRequest(scene_id="s1", asset_type="image", prompt="red circle",
                           options={"size": "1024x1024"})
    fake_result = GenerateResult(
        ok=True, provider="ark", model="doubao-seedream-4-0-250828",
        local_path=Path("/tmp/img.jpg"), cost_yuan=0.20, duration_ms=2500,
        public_url="https://ark/x.jpeg",
    )
    with patch("vivify.asset_orchestrator._build_adapter",
                return_value=MagicMock(generate=MagicMock(return_value=fake_result))):
        result = generate_asset(req, env=env, config_path=cfg_path)

    assert result.ok is True
    assert result.provider == "ark"
    # Ledger has 1 success row
    text = isolated_ledger.read_text()
    assert "success" in text
    assert "s1" in text
    assert "ark" in text
    assert '"cost_cny": 0.2' in text or '"cost_cny":0.2' in text


# --- budget gate ------------------------------------------------------------

def test_generate_asset_budget_exceeded_writes_ledger(cfg_path, env, isolated_ledger, monkeypatch):
    """If cost-cap rejects the call, return ok=False + write ledger with budget-exceeded."""
    req = GenerateRequest(scene_id="s1", asset_type="video", prompt="x",
                           duration_sec=5)

    # Force the per_asset_cap to be tiny so any model is over
    def fake_load_config(path=None):
        return RouterConfig(
            default_providers={"video": "ark"},
            fallback_chains={"video": ["ark", "manual-pending"]},
            cost_caps={"per_video_cny": 0.001, "per_asset_cny": 0.001, "monthly_cny": 0.001},
        )
    monkeypatch.setattr("vivify.asset_orchestrator.load_config", fake_load_config)
    # Mock _default_model_for to return provider_id verbatim (so the mock
    # _model_cost_yuan is consistent). Then mock _model_cost_yuan to huge.
    monkeypatch.setattr("vivify.asset_orchestrator._default_model_for",
                        lambda pid, cfg: pid)
    monkeypatch.setattr("vivify.asset_orchestrator._model_cost_yuan",
                        lambda *a, **k: 999.0)

    # The adapter should NEVER be called. Mock pick() to return 'ark'
    # directly (bypassing the chain walk, which would short-circuit to
    # 'manual-pending' if the chain ends in that sentinel — that's a
    # different code path). This test specifically exercises the
    # orchestrator's per-asset cap gate, not pick()'s chain logic.
    monkeypatch.setattr("vivify.asset_orchestrator.pick", lambda *a, **k: "ark")
    with patch("vivify.asset_orchestrator._build_adapter") as mock_build:
        result = generate_asset(req, env=env, config_path=cfg_path)
        mock_build.assert_not_called()

    assert result.ok is False
    assert result.status_hint == "budget-exceeded"  # orchestrator-suggested status
    # Ledger has 1 budget-exceeded row
    text = isolated_ledger.read_text()
    assert "budget-exceeded" in text


# --- provider 4xx (permanent) ------------------------------------------------

def test_generate_asset_provider_4xx_writes_ledger(cfg_path, env, isolated_ledger):
    """Provider returns ok=False with HTTP 4xx error → ledger gets provider-failed."""
    req = GenerateRequest(scene_id="s1", asset_type="image", prompt="x")
    fake_result = GenerateResult(
        ok=False, provider="ark", model="doubao-seedream-4-0-250828",
        error="HTTP 400: bad size",
    )
    with patch("vivify.asset_orchestrator._build_adapter",
                return_value=MagicMock(generate=MagicMock(return_value=fake_result))):
        result = generate_asset(req, env=env, config_path=cfg_path)

    assert result.ok is False
    text = isolated_ledger.read_text()
    assert "provider-failed" in text


# --- provider 5xx (retry) ----------------------------------------------------

def test_generate_asset_provider_5xx_retries_then_succeeds(cfg_path, env, isolated_ledger):
    """5xx → retry up to max_retries → success on 2nd attempt."""
    req = GenerateRequest(scene_id="s1", asset_type="image", prompt="x")
    fail = GenerateResult(ok=False, provider="ark", model="m", error="HTTP 500: server")
    ok = GenerateResult(ok=True, provider="ark", model="m", cost_yuan=0.2,
                          local_path=Path("/tmp/img.jpg"), public_url="https://ark/x")
    adapter = MagicMock()
    adapter.generate = MagicMock(side_effect=[fail, ok])

    # Mock time.sleep to avoid waiting
    import time as _time
    with patch("vivify.asset_orchestrator._build_adapter", return_value=adapter), \
         patch("vivify.asset_orchestrator.time.sleep", lambda *a, **k: None), \
         patch("vivify.asset_orchestrator.time.time", side_effect=[0, 0, 0, 5, 5]):
        result = generate_asset(req, env=env, config_path=cfg_path)

    assert result.ok is True
    # Should have called the adapter twice
    assert adapter.generate.call_count == 2
    # Ledger has 1 provider-retry + 1 success = 2 rows
    text = isolated_ledger.read_text()
    assert "provider-retry" in text
    assert "success" in text


# --- ledger is always written ------------------------------------------------

def test_generate_asset_always_writes_ledger_even_on_unexpected_exception(cfg_path, env, isolated_ledger):
    """Even if the adapter raises, a ledger row should be written."""
    req = GenerateRequest(scene_id="s1", asset_type="image", prompt="x")

    def boom(*a, **k):
        raise RuntimeError("kaboom")
    with patch("vivify.asset_orchestrator._build_adapter",
                return_value=MagicMock(generate=boom)), \
         patch("vivify.asset_orchestrator.time.sleep", lambda *a, **k: None):
        result = generate_asset(req, env=env, config_path=cfg_path)

    assert result.ok is False
    # Ledger has at least one row (error captured)
    text = isolated_ledger.read_text()
    assert "s1" in text
    # Some non-success status is recorded
    lines = [json.loads(l) for l in text.strip().split("\n") if l.strip()]
    assert any(e["status"] != "success" for e in lines)


# --- provider filter --------------------------------------------------------

def test_generate_asset_respects_provider_filter(cfg_path, env, isolated_ledger):
    """If provider_filter='ark', router never picks kling even as fallback."""
    req = GenerateRequest(scene_id="s1", asset_type="image", prompt="x")
    fake_result = GenerateResult(ok=True, provider="ark", model="m",
                                   local_path=Path("/tmp/i.jpg"), cost_yuan=0.2)
    with patch("vivify.asset_orchestrator._build_adapter",
                return_value=MagicMock(generate=MagicMock(return_value=fake_result))) as mock_build:
        result = generate_asset(req, env=env, config_path=cfg_path,
                                  provider_filter="ark")
    assert result.ok is True
    # Build was called with 'ark' exactly
    args, kwargs = mock_build.call_args
    assert kwargs.get("provider_id") == "ark" or args[0] == "ark"


# --- monthly cap ------------------------------------------------------------

class TestMonthlyCap:
    def test_monthly_cap_blocks_when_db_total_plus_estimate_exceeds(self, monkeypatch, tmp_path):
        """If db_path is given and this call's estimate would push monthly total
        over the cap, generate_asset must write budget-exceeded and return ok=False."""
        import sqlite3
        db = tmp_path / "vivify.db"
        conn = sqlite3.connect(db)
        conn.execute("CREATE TABLE episodes (cost_yuan REAL, render_completed_at TEXT)")
        conn.execute("INSERT INTO episodes VALUES (?, ?)", (60001.0, "2026-06-15T10:00:00Z"))
        conn.commit()
        conn.close()

        from vivify.asset_orchestrator import generate_asset
        from vivify.providers.base import GenerateRequest

        # Force-override the ledger path so we don't pollute real home
        monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")

        req = GenerateRequest(
            scene_id="s1", asset_type="video", prompt="test",
            duration_sec=10,
        )
        result = generate_asset(
            req, env={}, config_path=None, provider_filter="ark",
            db_path=str(db), monthly_hard=60000.0,
        )
        assert result.ok is False
        assert result.status_hint == "budget-exceeded"
        assert "monthly" in (result.error or "").lower()

    def test_monthly_cap_passes_when_db_total_low(self, monkeypatch, tmp_path):
        """If monthly total is low, generate_asset proceeds and reaches adapter."""
        import sqlite3
        db = tmp_path / "vivify.db"
        conn = sqlite3.connect(db)
        conn.execute("CREATE TABLE episodes (cost_yuan REAL, render_completed_at TEXT)")
        conn.execute("INSERT INTO episodes VALUES (?, ?)", (100.0, "2026-06-01T00:00:00Z"))
        conn.commit()
        conn.close()
        monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")

        from vivify.asset_orchestrator import generate_asset
        from vivify.providers.base import GenerateRequest, GenerateResult
        from unittest.mock import MagicMock

        # Stub adapter that returns ok
        fake = MagicMock()
        fake.name = "ark"
        fake.asset_type = "video"
        fake.generate.return_value = GenerateResult(
            ok=True, provider="ark", model="seedance", local_path=tmp_path / "x.mp4",
            cost_yuan=10.0, duration_ms=1000,
        )
        monkeypatch.setattr("vivify.asset_orchestrator._build_adapter", lambda *a, **kw: fake)

        req = GenerateRequest(scene_id="s1", asset_type="video",
                              prompt="t", duration_sec=10)
        result = generate_asset(req, env={}, provider_filter="ark",
                                db_path=str(db), monthly_hard=60000.0)
        assert result.ok is True
        fake.generate.assert_called_once()

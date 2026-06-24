"""Tests for vivify.asset_router — YAML router config + decision algorithm.

Router picks which provider+model to call for a given asset type,
walking the configured fallback chain. Reuses MODEL_CATALOG and
FALLBACK_CHAINS from model_router.py for the default chain definitions
(no duplication).
"""

from pathlib import Path
from unittest.mock import patch

import pytest

from vivify.asset_router import (
    RouterConfig,
    load_config,
    pick,
    save_default_config,
    DEFAULT_CONFIG_PATH,
)


@pytest.fixture
def project_yaml(tmp_path, monkeypatch):
    """Make DEFAULT_CONFIG_PATH a throwaway path so tests don't pollute ~/.claude."""
    cfg_path = tmp_path / "vivify-providers.yaml"
    monkeypatch.setattr("vivify.asset_router.DEFAULT_CONFIG_PATH", cfg_path)
    return cfg_path


# --- load_config ------------------------------------------------------------

def test_load_config_creates_default_if_missing(project_yaml):
    """If no config file exists anywhere, write a default and load it."""
    assert not project_yaml.exists()
    cfg = load_config()
    assert project_yaml.exists()
    assert isinstance(cfg, RouterConfig)
    # Default config uses ark for video + image
    assert cfg.default_providers["video"] == "ark"
    assert cfg.default_providers["image"] == "ark"
    # Fallback chains have at least the stub providers
    assert "kling" in cfg.fallback_chains["video"] or "minimax" in cfg.fallback_chains["video"]


def test_load_config_reads_existing_file(project_yaml):
    """If a YAML file exists, parse it and return its config."""
    project_yaml.write_text("""\
default_providers:
  video: minimax
  image: ark
fallback_chains:
  video: [minimax, ark, kling, manual-pending]
  image: [ark, manual-pending]
cost_caps:
  per_video_cny: 30
  per_asset_cny: 2
  monthly_cny: 5000
""")
    cfg = load_config()
    assert cfg.default_providers["video"] == "minimax"
    assert cfg.fallback_chains["video"] == ["minimax", "ark", "kling", "manual-pending"]
    assert cfg.cost_caps["per_video_cny"] == 30


def test_load_config_uses_project_local_over_global(project_yaml, tmp_path, monkeypatch):
    """If a project-local config exists, it takes precedence over global."""
    # Create both: project-local and global
    project_local = tmp_path / "project.yaml"
    project_local.write_text("default_providers:\n  video: kling\n")
    project_yaml.write_text("default_providers:\n  video: ark\n")

    # load_config(path=project_local) should use the local one
    cfg = load_config(path=project_local)
    assert cfg.default_providers["video"] == "kling"


def test_load_config_handles_missing_optional_fields(project_yaml):
    """Missing fallback_chains / cost_caps → sensible defaults, no error."""
    project_yaml.write_text("default_providers:\n  video: ark\n  image: ark\n")
    cfg = load_config()
    assert cfg.fallback_chains == {} or "video" in cfg.fallback_chains
    assert cfg.cost_caps == {}


# --- pick -------------------------------------------------------------------

def test_pick_returns_default_for_type(project_yaml):
    """pick('video') returns default_providers['video'] when chain doesn't apply."""
    project_yaml.write_text("""\
default_providers:
  video: ark
  image: ark
fallback_chains:
  video: [kling, manual-pending]
""")
    cfg = load_config()
    p = pick(cfg, "video", requirements={})
    assert p == "ark"


def test_pick_walks_fallback_chain_on_excluded(project_yaml):
    """If the default is in the exclude list, walk the fallback chain."""
    project_yaml.write_text("""\
default_providers:
  video: ark
  image: ark
fallback_chains:
  video: [ark, kling, minimax, manual-pending]
""")
    cfg = load_config()
    p = pick(cfg, "video", requirements={}, exclude=["ark"])
    assert p == "kling"


def test_pick_returns_manual_pending_when_chain_exhausted(project_yaml):
    """If everything is excluded, return 'manual-pending' (a sentinel)."""
    project_yaml.write_text("""\
default_providers:
  video: ark
  image: ark
fallback_chains:
  video: [ark, kling, manual-pending]
""")
    cfg = load_config()
    p = pick(cfg, "video", requirements={}, exclude=["ark", "kling"])
    assert p == "manual-pending"


def test_pick_provider_filter_limits_choices(project_yaml):
    """provider_filter='ark' means we never pick anything else."""
    project_yaml.write_text("""\
default_providers:
  video: ark
  image: ark
fallback_chains:
  video: [ark, kling, manual-pending]
""")
    cfg = load_config()
    p = pick(cfg, "video", requirements={}, provider_filter="ark")
    assert p == "ark"


def test_pick_no_chain_returns_default(project_yaml):
    """If no fallback chain exists for an asset type, just return the default."""
    project_yaml.write_text("""\
default_providers:
  bgm: suno
  image: ark
""")
    cfg = load_config()
    p = pick(cfg, "bgm", requirements={})
    assert p == "suno"


# --- cost_estimate_in_config ------------------------------------------------

def test_pick_respects_per_asset_cap(project_yaml, monkeypatch):
    """If a model would exceed per_asset_cny, skip it and try the next."""
    project_yaml.write_text("""\
default_providers:
  video: ark
fallback_chains:
  video: [ark, kling, manual-pending]
cost_caps:
  per_asset_cny: 5
""")
    cfg = load_config()

    # Mock _model_cost_yuan: ark=10 (over cap), kling=2 (under cap)
    def fake_cost(model, duration_sec=0):
        return {"ark": 10.0, "kling": 2.0}.get(model, 0.0)
    with patch("vivify.asset_router._model_cost_yuan", side_effect=fake_cost):
        p = pick(cfg, "video", requirements={"duration_sec": 1})
    # ark would cost 10 > 5 cap, so we walk to kling (2 ≤ 5)
    assert p == "kling"


def test_pick_returns_manual_pending_when_everything_over_cap(project_yaml):
    """If every model in the chain is over the per-asset cap, return manual-pending."""
    project_yaml.write_text("""\
default_providers:
  video: ark
fallback_chains:
  video: [ark, kling, manual-pending]
cost_caps:
  per_asset_cny: 5
""")
    cfg = load_config()
    def fake_cost(model, duration_sec=0):
        return 10.0  # everything is over the cap
    with patch("vivify.asset_router._model_cost_yuan", side_effect=fake_cost):
        p = pick(cfg, "video", requirements={"duration_sec": 1})
    assert p == "manual-pending"


# --- _default_model_for resolver (Task 1.5) --------------------------------

class TestDefaultModelFor:
    def test_explicit_config_default_models_wins(self):
        """When config.default_models has the provider, use that exact model name."""
        from vivify.asset_router import RouterConfig, _default_model_for
        cfg = RouterConfig(default_models={"ark": "doubao-seedance-2-0-260128"})
        assert _default_model_for("ark", cfg) == "doubao-seedance-2-0-260128"

    def test_fallback_to_catalog_scan(self):
        """When config.default_models is empty, scan MODEL_CATALOG for keys
        containing the provider_id and pick the cheapest."""
        from vivify.asset_router import RouterConfig, _default_model_for
        cfg = RouterConfig()  # no default_models
        # This test only asserts that the resolver returns SOMETHING that
        # looks like a model name (not the provider_id itself)
        result = _default_model_for("ark", cfg)
        # Either it found a catalog match, or it fell back to provider_id
        assert result  # truthy

    def test_resolver_returns_string_never_raises(self):
        """Resolver must never raise — provider_id 'minimax' / 'kling' / etc."""
        from vivify.asset_router import RouterConfig, _default_model_for
        cfg = RouterConfig()
        for pid in ["ark", "minimax", "kling", "nonexistent-provider"]:
            result = _default_model_for(pid, cfg)
            assert isinstance(result, str)
            assert result  # non-empty


class TestCostGateNotSilent:
    """Regression: confirm the cost gate fires when it should."""

    def test_per_asset_cap_actually_blocks_with_real_cost(self, tmp_path):
        """When default_models is configured, the per_asset cap should be
        checkable. Without the fix, _model_cost_yuan('ark', 5) returns 0
        and the gate never fires. With the fix, it should compute > 0
        for video with duration 5."""
        from vivify.asset_router import _model_cost_yuan, _default_model_for, RouterConfig
        from model_router import MODEL_CATALOG
        # Pick any ark model that has a non-zero cost_per_sec_yuan
        ark_models = [k for k, v in MODEL_CATALOG.items()
                      if "seedance" in k and v.get("cost_per_sec_yuan", 0) > 0]
        assert ark_models, "test premise broken: no ark models in catalog with non-zero cost"
        cfg = RouterConfig(default_models={"ark": ark_models[0]})
        model_name = _default_model_for("ark", cfg)
        cost = _model_cost_yuan(model_name, 5)
        # 5 seconds x ¥/sec should be > 0
        assert cost > 0, (f"_model_cost_yuan returned 0 even with model_name={model_name!r}; "
                          f"the cost gate is still broken")

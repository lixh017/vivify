"""vivify.asset_router — YAML config + provider-selection algorithm.

This is the L1 backing for the L2 `vivify-asset-router` SKILL.md. The
router picks which provider to call for a given asset type, walking a
configured fallback chain when the preferred one fails or is excluded.

Config file: ~/.claude/config/vivify-providers.yaml (auto-created on
first load if missing). Project-local override at
./vivify-providers.yaml takes precedence.

YAML shape:
  default_providers:    {video: ark, image: ark, tts: ark, bgm: suno}
  fallback_chains:      {video: [ark, kling, manual-pending], ...}
  cost_caps:            {per_video_cny: 50, per_asset_cny: 5, monthly_cny: 60000}
  scene_overrides:      {video_short_character: ark, ...}   # optional
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

import yaml


# --- paths ------------------------------------------------------------------

def _home() -> Path:
    return Path(os.environ.get("HOME", str(Path.home())))


def _default_config_path() -> Path:
    """Canonical config path. Override via $VIVIFY_PROVIDERS_CONFIG (tests)."""
    override = os.environ.get("VIVIFY_PROVIDERS_CONFIG")
    if override:
        return Path(override)
    return _home() / ".claude" / "config" / "vivify-providers.yaml"


# Module-level default (overridable via monkeypatch in tests).
DEFAULT_CONFIG_PATH = _default_config_path()


# --- config dataclass -------------------------------------------------------

@dataclass
class RouterConfig:
    default_providers: dict = field(default_factory=dict)
    fallback_chains: dict = field(default_factory=dict)
    cost_caps: dict = field(default_factory=dict)
    scene_overrides: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        return {
            "default_providers": self.default_providers,
            "fallback_chains": self.fallback_chains,
            "cost_caps": self.cost_caps,
            "scene_overrides": self.scene_overrides,
        }


# --- defaults ---------------------------------------------------------------

# Default config — used when no YAML file exists anywhere.
# Mirrors the L2 SKILL.md spec, with project-actual cost caps
# (¥60,000/month, not ¥3,000) and real provider ids.
DEFAULT_CONFIG_YAML = """\
# Vivify asset router config. Auto-generated on first `vivify asset router` call.
# Edit freely; loaded on every invocation.

# Provider to use by default for each asset type.
default_providers:
  video: ark
  image: ark
  tts: ark        # see note below — ark here means a future 火山 TTS adapter;
                  # for now, TTS goes through render_episode.py gen_tts (海螺).
  bgm: suno       # stub — `suno` provider raises NotImplementedError today

# Fallback chains walked in order. The first provider in the chain is
# tried; on failure or exclusion, the next is tried. `manual-pending` is
# a sentinel meaning "stop and queue for human review" — never auto-pick
# anything beyond it (preserves IP consistency per L2 spec).
fallback_chains:
  video: [ark, minimax, kling, manual-pending]
  image: [ark, jimeng, manual-pending]
  tts:   [ark, manual-pending]
  bgm:   [suno, udio, manual-pending]

# Cost caps. Used by the orchestrator before each call. The orchestrator
# refuses to dispatch if a model would push us over the relevant cap.
cost_caps:
  per_video_cny: 50      # soft cap; orchestrator warns but may override
  per_asset_cny: 5       # hard cap for any single asset
  monthly_cny: 60000     # hard cap per calendar month (Phase 1 budget)

# Optional: per-scene-type overrides. Keys are scene categories from
# vivify-scene-decomposition (e.g. 'video_short_character').
scene_overrides: {}
"""


# --- I/O --------------------------------------------------------------------

def save_default_config(path: Optional[Path] = None) -> Path:
    """Write the default config to `path` (or DEFAULT_CONFIG_PATH). Returns the path."""
    p = path or DEFAULT_CONFIG_PATH
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(DEFAULT_CONFIG_YAML, encoding="utf-8")
    return p


def load_config(path: Optional[Path] = None) -> RouterConfig:
    """Load the router config. Resolution order:
      1. `path` argument (if given)
      2. ./vivify-providers.yaml (project-local)
      3. ~/.claude/config/vivify-providers.yaml (global)
      4. Write default to global and load it
    """
    candidates = []
    if path is not None:
        candidates.append(path)
    candidates.append(Path.cwd() / "vivify-providers.yaml")
    candidates.append(DEFAULT_CONFIG_PATH)

    for c in candidates:
        if c.exists():
            return _parse_yaml(c)

    # No config anywhere — write default and load it
    save_default_config(DEFAULT_CONFIG_PATH)
    return _parse_yaml(DEFAULT_CONFIG_PATH)


def _parse_yaml(path: Path) -> RouterConfig:
    raw = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    return RouterConfig(
        default_providers=raw.get("default_providers", {}) or {},
        fallback_chains=_str_keys(raw.get("fallback_chains", {}) or {}),
        cost_caps={k: float(v) for k, v in (raw.get("cost_caps", {}) or {}).items()},
        scene_overrides=_str_keys(raw.get("scene_overrides", {}) or {}),
    )


def _str_keys(d: dict) -> dict:
    """Coerce dict keys to strings (YAML may parse 'video_short' as not str)."""
    return {str(k): list(v) for k, v in d.items()}


# --- model cost lookup (testable seam) --------------------------------------

def _model_cost_yuan(model: str, duration_sec: int = 0) -> float:
    """Cost in ¥ for a single call. Pulled from MODEL_CATALOG; ¥/sec × duration."""
    try:
        from model_router import MODEL_CATALOG
        rate = MODEL_CATALOG.get(model, {}).get("cost_per_sec_yuan", 0.0)
    except Exception:
        rate = 0.0
    return rate * max(1, duration_sec)


# --- pick -------------------------------------------------------------------

def pick(config: RouterConfig, asset_type: str, *,
         requirements: Optional[dict] = None,
         exclude: Optional[list[str]] = None,
         provider_filter: Optional[str] = None) -> str:
    """Choose a provider for `asset_type`. Returns the provider id (e.g. 'ark').

    Resolution order:
      1. scene_overrides[scene_category] (if a category is in requirements)
      2. default_providers[asset_type] (subject to cap + filter)
      3. Walk fallback_chains[asset_type], skipping excluded and over-cap
      4. Return 'manual-pending' if everything is excluded
    """
    requirements = requirements or {}
    exclude = set(exclude or [])
    per_asset_cap = config.cost_caps.get("per_asset_cny")
    duration = requirements.get("duration_sec", 0)

    def _acceptable(candidate: str) -> bool:
        if candidate in exclude:
            return False
        if not _passes_filter(candidate, provider_filter):
            return False
        if per_asset_cap and _model_cost_yuan(candidate, duration) > per_asset_cap:
            return False
        return True

    # 1. Scene override
    scene_category = requirements.get("scene_category")
    if scene_category and scene_category in config.scene_overrides:
        candidate = config.scene_overrides[scene_category]
        if _acceptable(candidate):
            return candidate

    # 2. Default
    default = config.default_providers.get(asset_type)
    if default and _acceptable(default):
        return default

    # 3. Walk the fallback chain
    chain = config.fallback_chains.get(asset_type, [])
    for candidate in chain:
        if candidate == "manual-pending":
            return "manual-pending"
        if _acceptable(candidate):
            return candidate

    # 4. Nothing worked
    return "manual-pending"


def _passes_filter(provider: str, provider_filter: Optional[str]) -> bool:
    """A provider_filter='ark' means only 'ark' is acceptable; 'minimax' means
    only 'minimax'; None means no filter. The 'manual-pending' sentinel
    always passes (it represents a human, not a real provider)."""
    if provider_filter is None:
        return True
    if provider == "manual-pending":
        return True  # sentinel — human review always allowed
    return provider == provider_filter


__all__ = [
    "RouterConfig",
    "DEFAULT_CONFIG_PATH",
    "DEFAULT_CONFIG_YAML",
    "load_config",
    "save_default_config",
    "pick",
]

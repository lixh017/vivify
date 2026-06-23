"""model_router.py — smart video model selection with auto-fallback.

Replaces manual `--video-model X` calls with a tier-based system that:
  1. Knows the model catalog (tier, cost, features, quality)
  2. Detects what the account has activated
  3. Picks the best model based on requirements (tier + lip_sync + budget)
  4. Falls back to next-best if the preferred model fails
  5. Reports actual usage at the end

Usage:
    from model_router import ModelRouter, MODEL_CATALOG

    router = ModelRouter(env)
    model_id, reason = router.route_video_model(
        tier="premium",
        requirements={"needs_lip_sync": True},
    )
    # Try the preferred model; on failure, walk the fallback chain.
    try:
        gen_video(env, ..., model=model_id, ...)
    except Exception as e:
        fallback_model, _ = router.fallback_from(model_id)
        gen_video(env, ..., model=fallback_model, ...)
"""

import json
from pathlib import Path


# === Model catalog ===
# Fields:
#   tier:         "draft" | "standard" | "premium"
#   cost/sec ¥:   approximate cost (varies by duration, resolution)
#   quality:      0-10 score (subjective; lip sync = major bonus)
#   features:     ["lip_sync", "audio_input", "chinese_aesthetic", ...]
#   provider:     "火山方舟" / "快手" / etc
#   activated_on_demo_account: True if our test account has it

MODEL_CATALOG = {
    # === 火山方舟 (ByteDance) ===
    "doubao-seedance-2-0-260128": {
        "name": "Seedance 2.0",
        "provider": "火山方舟",
        "tier": "premium",
        "cost_per_sec_yuan": 1.0,
        "quality": 9.5,
        "features": ["lip_sync", "audio_input", "chinese_aesthetic", "character_ref", "2k_resolution", "long_video"],
        "max_duration_sec": 15,
        "best_for": ["lip_sync_required", "premium_content", "character_animation"],
        "notes": "唯一支持嘴型同步的模型,质量最高,最贵",
    },
    "doubao-seedance-2-0-fast-260128": {
        "name": "Seedance 2.0 Fast",
        "provider": "火山方舟",
        "tier": "premium",
        "cost_per_sec_yuan": 0.8,
        "quality": 9.0,
        "features": ["lip_sync", "audio_input", "chinese_aesthetic", "character_ref"],
        "max_duration_sec": 15,
        "best_for": ["lip_sync_required", "premium_content_fast"],
        "notes": "2.0 速度优化版,质量略低于 full",
    },
    "doubao-seedance-2-0-mini-260615": {
        "name": "Seedance 2.0 Mini",
        "provider": "火山方舟",
        "tier": "premium",
        "cost_per_sec_yuan": 0.5,
        "features": ["lip_sync", "audio_input", "chinese_aesthetic", "character_ref"],
        "max_duration_sec": 10,
        "quality": 8.5,
        "best_for": ["budget_premium", "lip_sync_required"],
        "notes": "2.0 mini,性价比最高的 lip sync 选项",
    },
    "doubao-seedance-1-5-pro-251215": {
        "name": "Seedance 1.5 Pro",
        "provider": "火山方舟",
        "tier": "standard",
        "cost_per_sec_yuan": 0.4,
        "quality": 8.5,
        "features": ["character_ref", "chinese_aesthetic"],
        "max_duration_sec": 12,
        "best_for": ["general_use", "default"],
        "notes": "当前默认,质量好但无嘴型同步",
    },
    "doubao-seedance-1-0-pro-250528": {
        "name": "Seedance 1.0 Pro",
        "provider": "火山方舟",
        "tier": "standard",
        "cost_per_sec_yuan": 0.75,
        "quality": 7.5,
        "features": ["character_ref"],
        "max_duration_sec": 12,
        "best_for": ["legacy"],
        "notes": "老版本,被 1.5-pro 替代",
    },
    "doubao-seedance-1-0-pro-fast-251015": {
        "name": "Seedance 1.0 Pro Fast",
        "provider": "火山方舟",
        "tier": "draft",
        "cost_per_sec_yuan": 0.21,
        "quality": 6.5,
        "features": ["character_ref"],
        "max_duration_sec": 10,
        "best_for": ["draft_iteration", "preview"],
        "notes": "便宜快速,适合草稿迭代",
    },
    "doubao-seedance-1-0-lite-i2v-250428": {
        "name": "Seedance 1.0 Lite (i2v)",
        "provider": "火山方舟",
        "tier": "draft",
        "cost_per_sec_yuan": 0.5,
        "quality": 6.0,
        "features": [],
        "max_duration_sec": 10,
        "best_for": ["ultra_budget"],
        "notes": "极致便宜,质量一般",
    },
    # === 阿里 ===
    "wan2-1-14b-i2v-250225": {
        "name": "Wan 2.1 14B (i2v)",
        "provider": "火山方舟",
        "tier": "standard",
        "cost_per_sec_yuan": 0.3,
        "quality": 7.5,
        "features": ["chinese_aesthetic"],
        "max_duration_sec": 10,
        "best_for": ["chinese_aesthetic", "open_source_alt"],
        "notes": "阿里开源,可自部署",
    },
    # === MiniMax 海螺 (mmx CLI) ===
    # These models route through `mmx video generate`, NOT the ark curl
    # path. Mark with provider="minimax" so gen_video() dispatches correctly.
    "MiniMax-Hailuo-2.3": {
        "name": "Hailuo 2.3 (I2V)",
        "provider": "minimax",
        "tier": "premium",
        "cost_per_sec_yuan": 1.0,
        "quality": 9.0,
        "features": ["character_ref", "chinese_aesthetic"],
        "max_duration_sec": 6,
        "best_for": ["minimax_provider", "premium_alt"],
        "notes": "MiniMax 旗舰,默认 I2V 模式 (--first-frame)。质量稳定,无嘴型同步。",
        "mmx_mode": "i2v",
    },
    "MiniMax-Hailuo-2.3-Fast": {
        "name": "Hailuo 2.3 Fast (I2V)",
        "provider": "minimax",
        "tier": "standard",
        "cost_per_sec_yuan": 0.6,
        "quality": 8.5,
        "features": ["character_ref", "chinese_aesthetic"],
        "max_duration_sec": 6,
        "best_for": ["minimax_provider", "fast_iteration"],
        "notes": "海螺 Fast 版,出片更快、便宜 40%。",
        "mmx_mode": "i2v",
    },
    "MiniMax-Hailuo-02": {
        "name": "Hailuo 02 (SEF: start-end frame)",
        "provider": "minimax",
        "tier": "premium",
        "cost_per_sec_yuan": 0.8,
        "quality": 8.5,
        "features": ["character_ref"],
        "max_duration_sec": 6,
        "best_for": ["minimax_provider", "transition_shots"],
        "notes": "起始+结束帧插值 (SEF),自动选 --first-frame + --last-frame 模式。",
        "mmx_mode": "sef",
    },
    "MiniMax-S2V-01": {
        "name": "S2V-01 (Subject Reference — character-locked)",
        "provider": "minimax",
        "tier": "premium",
        "cost_per_sec_yuan": 1.2,
        "quality": 9.5,
        "features": ["lip_sync", "character_ref", "subject_lock"],
        "max_duration_sec": 6,
        "best_for": ["character_consistency", "id_drift_fix"],
        "notes": "角色锁定 — 用 --subject-image 传 canonical ref,跨镜头脸不漂移。解决 Seedance ID drift 问题。",
        "mmx_mode": "s2v",
    },
}


# Provider identifiers — used by vivify episode render --video-provider
PROVIDERS = {
    "ark": "火山方舟 (Seedance)",
    "minimax": "MiniMax 海螺 (Hailuo)",
    "auto": "auto-pick (model router decides)",
}


# === Fallback chains (in order of preference) ===
# Each list is ordered: try the first, then the next, etc.
FALLBACK_CHAINS = {
    "premium": [
        "doubao-seedance-2-0-mini-260615",   # cheap premium
        "doubao-seedance-2-0-fast-260128",   # faster premium
        "doubao-seedance-2-0-260128",        # full quality
        "doubao-seedance-1-5-pro-251215",    # standard fallback
        "wan2-1-14b-i2v-250225",             # alt provider
    ],
    "standard": [
        "doubao-seedance-1-5-pro-251215",
        "wan2-1-14b-i2v-250225",
        "doubao-seedance-1-0-pro-fast-251015",
    ],
    "draft": [
        "doubao-seedance-1-0-pro-fast-251015",
        "doubao-seedance-1-0-lite-i2v-250428",
        "doubao-seedance-1-5-pro-251215",
    ],
}


class ModelRouter:
    """Decides which video model to use + handles fallback."""

    def __init__(self, env: dict):
        self.env = env
        self._activated_cache = None
        self._usage = {"calls": [], "total_cost_yuan": 0.0}

    def get_activated_models(self) -> set:
        """Hit /models endpoint, return set of model IDs the account has activated."""
        if self._activated_cache is not None:
            return self._activated_cache
        try:
            from render_episode import curl
            code, err, body_b = curl(
                "GET",
                f"{self.env['ARK_BASE_URL']}/models",
                {"Authorization": f"Bearer {self.env['ARK_API_KEY']}"},
            )
            if code != 200:
                return set(MODEL_CATALOG.keys())  # assume all available
            d = json.loads(body_b)
            activated = {m["id"] for m in d.get("data", [])}
            # intersect with known catalog
            self._activated_cache = activated & set(MODEL_CATALOG.keys())
            return self._activated_cache
        except Exception:
            return set(MODEL_CATALOG.keys())

    def route_video_model(self, tier: str = "standard",
                          requirements: dict = None,
                          explicit_model: str = None,
                          provider: str = None) -> tuple[str, str]:
        """Pick the best video model. Returns (model_id, reasoning).

        Args:
            tier: "draft" | "standard" | "premium"
            requirements: e.g. {"needs_lip_sync": True}
            explicit_model: force a specific model id (skips routing)
            provider: "ark" | "minimax" | None (auto)
                - "ark" → only consider Seedance models
                - "minimax" → only consider Hailuo models
                - None → all models (current behavior)

        Priority:
          1. explicit_model (if user forced one) — provider filter still applies
          2. If requirements["needs_lip_sync"], prefer models with lip_sync feature
          3. Use the tier's first activated model (within provider filter)
          4. Fall back to any activated model (within provider filter)
        """
        requirements = requirements or {}
        provider_filter = (provider or "").lower() or None

        def _match(mid: str) -> bool:
            m = MODEL_CATALOG.get(mid, {})
            if provider_filter and provider_filter != "auto":
                p = m.get("provider", "")
                # Normalize: "minimax" provider OR "MiniMax" provider OR "火山方舟"
                if provider_filter == "ark":
                    return p == "火山方舟"
                if provider_filter == "minimax":
                    return p == "minimax"
                return False
            return True

        # 1. Explicit override (with provider guard)
        if explicit_model:
            if not _match(explicit_model):
                raise ValueError(
                    f"explicit_model={explicit_model} doesn't match provider={provider_filter}. "
                    f"Use --video-provider auto/ark/minimax to match the model."
                )
            return explicit_model, f"explicit override ({explicit_model}, provider={provider_filter or 'any'})"

        # 2. Special: lip sync requirement
        if requirements.get("needs_lip_sync"):
            lip_sync_models = [
                mid for mid, m in MODEL_CATALOG.items()
                if "lip_sync" in m.get("features", [])
                and mid in self.get_activated_models()
                and _match(mid)
            ]
            if lip_sync_models:
                # pick the cheapest premium tier that has lip sync
                lip_sync_models.sort(key=lambda mid: MODEL_CATALOG[mid]["cost_per_sec_yuan"])
                chosen = lip_sync_models[0]
                return chosen, f"lip_sync required, picked cheapest lip-sync model ({chosen}, provider={MODEL_CATALOG[chosen]['provider']})"
            else:
                # No lip sync model in this provider — fall back to standard
                return self.route_video_model(
                    tier="standard",
                    requirements={k: v for k, v in requirements.items() if k != "needs_lip_sync"},
                    provider=provider,
                )[0], f"lip_sync requested but no model in provider={provider_filter or 'any'} — fell back to standard (no lip sync)"

        # 3. Tier-based selection
        chain = FALLBACK_CHAINS.get(tier, FALLBACK_CHAINS["standard"])
        activated = self.get_activated_models()
        for mid in chain:
            if mid in activated and _match(mid):
                m = MODEL_CATALOG[mid]
                return mid, f"tier={tier}, picked {mid} (¥{m['cost_per_sec_yuan']}/s, provider={m['provider']})"

        # 3b. Provider-scoped chain (for minimax etc., where FALLBACK_CHAINS
        # doesn't have entries). Build a tier-aware list from MODEL_CATALOG.
        if provider_filter:
            tier_rank = {"draft": 0, "standard": 1, "premium": 2}
            target_rank = tier_rank.get(tier, 1)
            in_provider = [
                mid for mid, m in MODEL_CATALOG.items()
                if _match(mid) and mid in activated
            ]
            # Pick the model with tier closest to the target
            in_provider.sort(key=lambda mid: (
                abs(tier_rank.get(MODEL_CATALOG[mid].get("tier", "standard"), 1) - target_rank),
                MODEL_CATALOG[mid]["cost_per_sec_yuan"],
            ))
            if in_provider:
                mid = in_provider[0]
                m = MODEL_CATALOG[mid]
                return mid, (f"tier={tier}, no chain match in provider={provider_filter}, "
                             f"picked closest-tier {mid} (¥{m['cost_per_sec_yuan']}/s)")

        # 4. Last resort: any activated model in this provider
        available = activated & set(MODEL_CATALOG.keys())
        available_filtered = [mid for mid in available if _match(mid)]
        if available_filtered:
            mid = sorted(available_filtered,
                         key=lambda m: MODEL_CATALOG[m]["cost_per_sec_yuan"])[0]
            return mid, f"no tier match, picked cheapest available ({mid})"

        # 5. Nothing activated — fallback to known standard
        return FALLBACK_CHAINS["standard"][0], "no activated models, using default"

    def fallback_from(self, failed_model: str) -> tuple[str, str]:
        """Given a model that just failed, return the next one in the chain.

        Searches all chains for the failed model and returns the next entry.
        If not found, returns the default standard model.
        """
        for chain_name, chain in FALLBACK_CHAINS.items():
            if failed_model in chain:
                idx = chain.index(failed_model)
                activated = self.get_activated_models()
                # Walk forward through the chain, pick the next activated one
                for next_model in chain[idx + 1:]:
                    if next_model in activated:
                        return next_model, f"fallback from {failed_model} → {next_model} (chain={chain_name})"
        return FALLBACK_CHAINS["standard"][0], f"no chain found for {failed_model}, using default"

    def record_call(self, model_id: str, duration_sec: float, success: bool):
        """Track usage for the final report."""
        cost = MODEL_CATALOG.get(model_id, {}).get("cost_per_sec_yuan", 0) * duration_sec
        self._usage["calls"].append({
            "model": model_id,
            "duration_sec": duration_sec,
            "cost_yuan": cost,
            "success": success,
        })
        if success:
            self._usage["total_cost_yuan"] += cost

    def print_report(self):
        """Print a summary at the end of an episode."""
        print()
        print("=" * 60)
        print("  MODEL USAGE REPORT")
        print("=" * 60)
        calls = self._usage["calls"]
        if not calls:
            print("  No video calls recorded.")
            return
        successful = [c for c in calls if c["success"]]
        total_sec = sum(c["duration_sec"] for c in successful)
        total_cost = self._usage["total_cost_yuan"]
        # Group by model
        by_model = {}
        for c in successful:
            by_model.setdefault(c["model"], {"count": 0, "sec": 0.0, "cost": 0.0})
            by_model[c["model"]]["count"] += 1
            by_model[c["model"]]["sec"] += c["duration_sec"]
            by_model[c["model"]]["cost"] += c["cost_yuan"]
        print(f"  Total shots: {len(calls)} ({len(successful)} succeeded)")
        print(f"  Total video: {total_sec:.1f}s")
        print(f"  Total cost:  ¥{total_cost:.2f}")
        print()
        print(f"  {'Model':<45} {'#':>3} {'Sec':>6} {'¥':>8}")
        print(f"  {'-'*45} {'-'*3} {'-'*6} {'-'*8}")
        for mid, stats in sorted(by_model.items(), key=lambda x: -x[1]["cost"]):
            print(f"  {mid:<45} {stats['count']:>3} {stats['sec']:>5.1f}s ¥{stats['cost']:>7.2f}")
        print("=" * 60)


# === CLI helper ===

def parse_tier(value: str) -> str:
    """Normalize tier value (case-insensitive, with aliases)."""
    v = (value or "standard").lower().strip()
    aliases = {
        "low": "draft", "cheap": "draft", "fast": "draft", "preview": "draft",
        "default": "standard", "normal": "standard", "med": "standard",
        "high": "premium", "best": "premium", "lip_sync": "premium",
    }
    return aliases.get(v, v)


if __name__ == "__main__":
    # Self-test
    print("MODEL CATALOG:")
    for mid, m in MODEL_CATALOG.items():
        print(f"  {mid:<45} tier={m['tier']:<9} ¥{m['cost_per_sec_yuan']}/s  q={m['quality']}")
    print()
    print("FALLBACK CHAINS:")
    for tier, chain in FALLBACK_CHAINS.items():
        print(f"  {tier}: {' → '.join(c.split('-')[-2] for c in chain)}")
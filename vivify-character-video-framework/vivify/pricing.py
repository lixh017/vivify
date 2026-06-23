"""vivify.pricing — estimate rendering cost in ¥.

Used by `vivify episode render` to pre-fill `episodes.cost_yuan` and
`shots.cost_yuan` based on the model + duration. The actual billed
amount may differ (model pricing changes, promos, retries), so the DB
column is a snapshot — not a billing system.

Pricing source: model_router.MODEL_CATALOG[cost_per_sec_yuan]. When
that module isn't importable (older installs), we fall back to
hardcoded constants here so `vivify episode add` always works.
"""

# Image gen cost (¥ per call, not per second — Seedream is flat-fee per image)
COST_IMAGE_YUAN = 0.20

# TTS cost (¥ per 1000 chars — 海螺 speech-02-hd is roughly this)
COST_TTS_PER_KCHAR_YUAN = 0.50

# Fallback video cost (¥/sec) by tier, when MODEL_CATALOG isn't available
FALLBACK_VIDEO_COST_PER_SEC = {
    "draft": 0.20,
    "standard": 0.40,
    "premium": 0.80,
}


def video_cost_yuan(model_id: str, duration_sec: float) -> float:
    """Estimate video gen cost. Tries MODEL_CATALOG first, falls back."""
    try:
        from model_router import MODEL_CATALOG
        entry = MODEL_CATALOG.get(model_id, {})
        rate = entry.get("cost_per_sec_yuan")
        if rate is not None:
            return round(rate * duration_sec, 4)
    except (ImportError, ModuleNotFoundError):
        pass
    # Fallback: guess from model name (rough)
    if "2-0" in model_id:
        return round(0.80 * duration_sec, 4)
    if "1-5" in model_id:
        return round(0.40 * duration_sec, 4)
    return round(0.20 * duration_sec, 4)


def tts_cost_yuan(text_or_chars) -> float:
    """Estimate TTS cost. Accepts either a string (uses len()) or an int
    (treated as char count directly)."""
    n = len(text_or_chars) if isinstance(text_or_chars, str) else int(text_or_chars)
    return round(n / 1000.0 * COST_TTS_PER_KCHAR_YUAN, 4)


def image_cost_yuan(n_shots: int = 1) -> float:
    """Estimate image gen cost. ¥0.20 per Seedream call."""
    return round(n_shots * COST_IMAGE_YUAN, 4)


def estimate_episode_cost(
    n_shots: int,
    total_video_sec: float,
    n_voiceovers: int,
    avg_vo_chars: int,
    video_model: str,
) -> dict:
    """Estimate full episode cost breakdown.

    Returns dict with: images, videos, tts, total (all ¥).
    """
    images = image_cost_yuan(n_shots)
    videos = video_cost_yuan(video_model, total_video_sec)
    tts = tts_cost_yuan(avg_vo_chars * max(1, n_voiceovers))
    total = round(images + videos + tts, 4)
    return {"images": images, "videos": videos, "tts": tts, "total": total}


def format_yuan(amount: float) -> str:
    """Render ¥ amounts the way 抖音 ops people read them."""
    if amount is None:
        return "—"
    if amount < 1:
        return f"¥{amount:.2f}"
    return f"¥{amount:.2f}"


__all__ = [
    "video_cost_yuan",
    "tts_cost_yuan",
    "image_cost_yuan",
    "estimate_episode_cost",
    "format_yuan",
    "COST_IMAGE_YUAN",
    "COST_TTS_PER_KCHAR_YUAN",
    "FALLBACK_VIDEO_COST_PER_SEC",
]
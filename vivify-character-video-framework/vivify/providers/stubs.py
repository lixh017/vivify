"""vivify.providers.stubs — placeholder adapters for vendors not yet implemented.

These exist so the router YAML can reference future providers without
crashing. They raise `NotImplementedError` with a clear message at
generate-time, not at import-time, so the router can list them in
fallback chains but never accidentally call them in production.

When a real adapter is built, move it out of this file into its own
module (kling.py, suno.py, udio.py) and remove the stub.
"""

from __future__ import annotations

from typing import Optional

from .base import AssetType, GenerateRequest, GenerateResult, ProviderAdapter


class _NotYetImplementedProvider(ProviderAdapter):
    """Base for stub providers. Override `name` and `asset_type`."""

    name: str = "stub"
    asset_type: AssetType = "image"

    def __init__(self, *args, **kwargs):
        pass

    def generate(self, req: GenerateRequest, *, out_path: Optional = None,
                 model: Optional[str] = None, **kwargs) -> GenerateResult:
        return GenerateResult(
            ok=False, provider=self.name, model=model or "",
            error=(f"{self.name} adapter not yet implemented — "
                   f"see vivify/providers/{self.name}.py (currently a stub). "
                   f"Use a different provider or wait for implementation."),
        )

    def cost_estimate(self, req: GenerateRequest) -> float:
        return 0.0


class KlingProvider(_NotYetImplementedProvider):
    """可灵 (Kling) — placeholder. Real impl: vivify/providers/kling.py."""
    name = "kling"
    asset_type = "video"


class SunoProvider(_NotYetImplementedProvider):
    """Suno (BGM) — placeholder. Real impl: vivify/providers/suno.py."""
    name = "suno"
    asset_type = "bgm"


class UdioProvider(_NotYetImplementedProvider):
    """Udio (BGM) — placeholder. Real impl: vivify/providers/udio.py."""
    name = "udio"
    asset_type = "bgm"


class JimengProvider(_NotYetImplementedProvider):
    """即梦 (Jimeng) image — placeholder. Real impl: vivify/providers/jimeng.py.
    Note: the L2 spec lists 'volcengine-jimeng-4.5' as the default image
    provider, but the L1 implementation in this repo routes all image
    generation through 火山方舟 Ark (Seedream) instead. If you need the
    actual Jimeng API, implement it here."""
    name = "jimeng"
    asset_type = "image"


__all__ = [
    "KlingProvider",
    "SunoProvider",
    "UdioProvider",
    "JimengProvider",
]

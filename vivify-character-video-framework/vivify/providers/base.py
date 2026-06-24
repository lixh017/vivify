"""vivify.providers.base — ProviderAdapter ABC + request/result dataclasses.

The contract every vendor adapter must satisfy. The orchestrator calls
`adapter.generate(req)` and gets a `GenerateResult` back; it doesn't care
which vendor the adapter wraps.

Field shapes mirror the L2 SKILL.md input contract from
`vivify-asset-orchestrator` §1, with these renames for project consistency:
- `anchor` (TypeScript object in spec) → split into `outfit` + `tone`
  string fields on the request, plus `profile_version` on the ledger
- `scene` (TypeScript object in spec) → flattened: `prompt` is the
  built prompt string the orchestrator has already assembled; scene
  metadata lives in the orchestrator, not on the request

This keeps the adapter API small and vendor-agnostic.
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from pathlib import Path
from typing import Literal, Optional


# Allowed asset types. Keep in sync with vivify/commands/asset.py help text.
AssetType = Literal["video", "image", "tts", "bgm"]


@dataclass
class GenerateRequest:
    """What the orchestrator hands to an adapter.

    `prompt` is the full, already-built prompt string (with IP anchor
    embedded by the orchestrator). The adapter is responsible only for
    translating this into vendor-specific API calls.
    """
    scene_id: str
    asset_type: AssetType
    prompt: str

    # Optional context — used by some vendors / some asset types only
    reference_image: Optional[str] = None    # local path, http(s)://, or data: URI
    duration_sec: Optional[int] = None        # video only
    size: Optional[str] = None                # image only, e.g. "1024x1792"
    outfit: Optional[str] = None              # IP anchor field (e.g. "hufu_red")
    tone: Optional[str] = None                # IP anchor field (e.g. "国潮")

    # Vendor-specific extras (ratio, resolution, seed, voice_id, etc.)
    options: dict = field(default_factory=dict)


@dataclass
class GenerateResult:
    """What an adapter hands back to the orchestrator.

    `ok=False` means the call failed; `error` carries the reason. On
    success, at least one of `local_path` / `public_url` is set, and
    `cost_yuan` is the actual billed amount (not an estimate).

    `status_hint` is an orchestrator-suggested ledger status string,
    set when the orchestrator synthesises a non-success result (e.g.
    'budget-exceeded', 'provider-failed'). Adapters can leave it None.
    """
    ok: bool

    asset_id: Optional[str] = None            # vendor task id (e.g. cgt-…)
    local_path: Optional[Path] = None         # downloaded file (set if downloaded)
    public_url: Optional[str] = None         # vendor URL (24h TTL for video)

    cost_yuan: float = 0.0
    duration_ms: int = 0
    provider: str = ""                        # "ark" | "minimax" | "kling" | …
    model: str = ""                           # vendor model id (e.g. doubao-seedance-1-5-pro-251215)
    error: Optional[str] = None
    status_hint: Optional[str] = None         # orchestrator-set: budget-exceeded, etc.


class ProviderAdapter(ABC):
    """Contract every vendor adapter must implement.

    Subclasses must define:
      - `name: str`        — short id used in router config ("ark", "minimax", …)
      - `asset_type`       — which asset type this adapter handles
      - `generate(req)`    — actually call the vendor
      - `cost_estimate(req)` — pre-call cost estimate (for budget gates)
    """
    name: str
    asset_type: AssetType

    @abstractmethod
    def generate(self, req: GenerateRequest) -> GenerateResult: ...

    @abstractmethod
    def cost_estimate(self, req: GenerateRequest) -> float: ...


__all__ = [
    "AssetType",
    "GenerateRequest",
    "GenerateResult",
    "ProviderAdapter",
]

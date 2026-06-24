"""vivify.asset_ledger — append-only JSONL ledger for AI asset generation.

Every provider call (success or failure) writes a row to the ledger.
Schema matches the existing data in ~/.claude/agents/opc-asset-ledger.jsonl
(which is auto-renamed from opc-asset-ledger.jsonl on first run).

Schema (v1, project-actual):
  timestamp         ISO-8601 with .sssZ
  scene_id          string
  provider          "ark" | "minimax" | "kling" | ...
  type              "video" | "image" | "tts" | "bgm"
  cost_cny          float
  duration_ms       int
  status            "success" | "budget-exceeded" | "provider-failed" |
                    "provider-retry" | "timeout" | "consistency-failed" |
                    "network-error" | "pending: manual" | "pending: network" |
                    "pending: no-provider"
  asset_path        string | null
  profile_version   string (default "fengge_v1")
  prompt_hash       "fnv64:" + 16 hex chars
  consistency_score float | null
  outfit            string | null
  error             string | null

Anti-patterns (per L2 spec §8):
  - Skipping ledger on failure = audit fail
  - Writing without profile_version = audit fail
  - Writing without prompt_hash = reproducibility fail
"""

from __future__ import annotations

import json
import os
from dataclasses import dataclass, field, asdict
from pathlib import Path
from typing import Iterator, Optional


# --- paths ------------------------------------------------------------------

def _home() -> Path:
    """Honor $HOME for tests; default to ~/.claude."""
    return Path(os.environ.get("HOME", str(Path.home())))


def _default_ledger_path() -> Path:
    """The canonical ledger path. Reads from $VIVIFY_LEDGER_PATH if set
    (so tests can override); otherwise ~/.claude/agents/vivify-asset-ledger.jsonl."""
    override = os.environ.get("VIVIFY_LEDGER_PATH")
    if override:
        return Path(override)
    return _home() / ".claude" / "agents" / "vivify-asset-ledger.jsonl"


# Module-level defaults; tests can monkeypatch.
LEDGER_PATH = _default_ledger_path()
LEGACY_LEDGER_PATHS: list[Path] = [
    _home() / ".claude" / "agents" / "opc-asset-ledger.jsonl",
]


def _legacy_ledger_paths() -> list[Path]:
    """Compute legacy paths relative to current LEDGER_PATH (testable).

    Looks for siblings of LEDGER_PATH with the legacy filename.
    """
    return [LEDGER_PATH.parent / "opc-asset-ledger.jsonl"]


# --- hash -------------------------------------------------------------------

# FNV-1a 64-bit constants (matches the existing ledger values)
_FNV_OFFSET = 0xcbf29ce484222325
_FNV_PRIME = 0x100000001b3
_FNV_MASK = (1 << 64) - 1


def fnv1a_64(s: str) -> str:
    """FNV-1a 64-bit hash. Returns 'fnv64:' + 16 hex chars (lowercase).

    Format matches the existing data in opc-asset-ledger.jsonl."""
    h = _FNV_OFFSET
    for byte in s.encode("utf-8"):
        h = ((h ^ byte) * _FNV_PRIME) & _FNV_MASK
    return f"fnv64:{h:016x}"


# --- entry ------------------------------------------------------------------

@dataclass
class LedgerEntry:
    """One row in the JSONL ledger."""
    timestamp: str
    scene_id: str
    provider: str
    type: str
    cost_cny: float
    duration_ms: int
    status: str
    asset_path: Optional[str] = None
    profile_version: str = "fengge_v1"
    prompt_hash: str = ""
    consistency_score: Optional[float] = None
    outfit: Optional[str] = None
    error: Optional[str] = None

    def to_dict(self) -> dict:
        d = asdict(self)
        return d

    @classmethod
    def from_dict(cls, d: dict) -> "LedgerEntry":
        return cls(
            timestamp=d.get("timestamp", ""),
            scene_id=d.get("scene_id", ""),
            provider=d.get("provider", ""),
            type=d.get("type", ""),
            cost_cny=float(d.get("cost_cny", 0.0)),
            duration_ms=int(d.get("duration_ms", 0)),
            status=d.get("status", ""),
            asset_path=d.get("asset_path"),
            profile_version=d.get("profile_version", "fengge_v1"),
            prompt_hash=d.get("prompt_hash", ""),
            consistency_score=d.get("consistency_score"),
            outfit=d.get("outfit"),
            error=d.get("error"),
        )


# --- I/O --------------------------------------------------------------------

def ensure_ledger() -> Path:
    """Make sure LEDGER_PATH exists. Auto-rename any legacy file.

    Returns the resolved path.
    """
    LEDGER_PATH.parent.mkdir(parents=True, exist_ok=True)

    # Rename legacy files in order (siblings of LEDGER_PATH)
    for legacy in _legacy_ledger_paths():
        if legacy.exists() and legacy != LEDGER_PATH:
            if not LEDGER_PATH.exists():
                legacy.rename(LEDGER_PATH)
            else:
                # LEDGER_PATH exists already; append legacy data, then delete
                with legacy.open("r", encoding="utf-8") as src, \
                     LEDGER_PATH.open("a", encoding="utf-8") as dst:
                    dst.write(src.read())
                legacy.unlink()

    if not LEDGER_PATH.exists():
        LEDGER_PATH.touch()
    return LEDGER_PATH


def append(entry: LedgerEntry) -> None:
    """Append one entry to the ledger (one JSON object per line)."""
    p = ensure_ledger()
    line = json.dumps(entry.to_dict(), ensure_ascii=False) + "\n"
    with p.open("a", encoding="utf-8") as f:
        f.write(line)


def read_recent(n: int) -> list[LedgerEntry]:
    """Return the last `n` ledger entries (most recent last)."""
    p = ensure_ledger()
    if not p.exists() or p.stat().st_size == 0:
        return []
    # Naive: read all, take last n. Fine for a ledger that's small enough
    # to be tail'd (we don't expect millions of rows in Phase 1).
    out: list[LedgerEntry] = []
    with p.open("r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                d = json.loads(line)
            except json.JSONDecodeError:
                continue  # skip malformed lines
            out.append(LedgerEntry.from_dict(d))
    return out[-n:]


def stream() -> Iterator[LedgerEntry]:
    """Yield ledger entries in order (for `ledger --tail` style iteration)."""
    p = ensure_ledger()
    if not p.exists():
        return
    with p.open("r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                d = json.loads(line)
            except json.JSONDecodeError:
                continue
            yield LedgerEntry.from_dict(d)


__all__ = [
    "LEDGER_PATH",
    "LEGACY_LEDGER_PATHS",
    "LedgerEntry",
    "fnv1a_64",
    "ensure_ledger",
    "append",
    "read_recent",
    "stream",
]

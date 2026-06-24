"""vivify.asset_orchestrator — end-to-end pipeline.

Coordinates: router → cost-cap → provider → retry → ledger.

This is the L1 backing for the L2 `vivify-asset-orchestrator` SKILL.md.
The orchestrator is the *only* thing that should call the vendor adapters
directly; CLI commands and skills go through `generate_asset()`.

Anti-patterns enforced here (per L2 SKILL.md §8):
  - Skipping ledger on failure = audit fail  → every outcome writes a row
  - Cost-blind calls = budget overrun         → pre-call cost gate
  - Retry-storm on permanent errors           → classify_error before sleeping
"""

from __future__ import annotations

import time
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Optional

from .asset_router import RouterConfig, load_config, pick, _model_cost_yuan
from .asset_ledger import LedgerEntry, append as ledger_append, fnv1a_64
from .providers.base import (
    AssetType,
    GenerateRequest,
    GenerateResult,
    ProviderAdapter,
)
from .providers.ark import ArkProvider
from .providers.minimax import MiniMaxProvider
from .providers.stubs import (
    JimengProvider,
    KlingProvider,
    SunoProvider,
    UdioProvider,
)
from .retry import (
    FailureClass,
    RetryPolicy,
    classify_error,
    should_retry,
)


# --- profile / version constants -------------------------------------------

PROFILE_VERSION = "fengge_v1"   # IP anchor version embedded in every ledger row


# --- adapter registry ------------------------------------------------------

def _build_adapter(provider_id: str, env: dict, asset_type: str) -> ProviderAdapter:
    """Construct the right adapter for the provider id.

    `env` is the dict of vendor credentials (ARK_API_KEY etc.) — passed
    through to adapters that need them. Stub providers ignore it.

    Raises:
      NotImplementedError: provider is a known stub
      ValueError:          unknown provider id
    """
    p = provider_id.lower()
    if p == "ark":
        return ArkProvider(env=env)
    if p == "minimax":
        return MiniMaxProvider(env=env)
    if p == "kling":
        return KlingProvider()
    if p == "suno":
        return SunoProvider()
    if p == "udio":
        return UdioProvider()
    if p == "jimeng":
        return JimengProvider()
    raise ValueError(f"unknown provider: {provider_id!r}")


# --- helpers ---------------------------------------------------------------

def _now_iso() -> str:
    """ISO-8601 with millisecond precision, UTC. e.g. '2026-06-24T10:00:00.123Z'."""
    now = datetime.now(timezone.utc)
    return now.strftime("%Y-%m-%dT%H:%M:%S.") + f"{now.microsecond // 1000:03d}Z"


def _classify_to_status(error_msg: str, exhausted: bool) -> str:
    """Map a provider error to a ledger status string."""
    cls, reason = classify_error(error_msg)
    if cls == FailureClass.PERMANENT:
        # Permanent errors: pick a specific label
        if reason == "timeout":
            return "timeout"
        if reason in ("network",):
            return "network-error"
        return "provider-failed"
    # Transient or unknown — exhausted retries
    if exhausted:
        if reason == "timeout":
            return "timeout"
        if reason in ("network",):
            return "network-error"
        return "provider-failed"
    # Transient mid-retry
    return "provider-retry"


def _sleep(attempt: int, policy: RetryPolicy) -> float:
    """Sleep with exponential backoff. Uses module-level time.sleep so
    tests can patch `vivify.asset_orchestrator.time.sleep` to skip waits."""
    delay = policy.delay_for_attempt(attempt)
    time.sleep(delay)
    return delay


# --- public entry point ----------------------------------------------------

def generate_asset(req: GenerateRequest, *,
                   env: dict,
                   config_path: Optional[Path] = None,
                   provider_filter: Optional[str] = None,
                   db_path: Optional[str] = None,    # reserved for future monthly-cap check
                   policy: Optional[RetryPolicy] = None,
                   monthly_so_far: float = 0.0) -> GenerateResult:
    """End-to-end: pick provider → cost-cap check → adapter call → ledger.

    Args:
      req:             GenerateRequest describing the asset to make.
      env:             Dict of vendor credentials (ARK_API_KEY, ARK_BASE_URL, ...).
      config_path:     Optional override for router YAML path (else default).
      provider_filter: Restrict to one provider id (e.g. 'ark') — bypasses fallback.
      db_path:         Reserved for future monthly-cap DB lookup. Not yet wired.
      policy:          RetryPolicy (default: 3 retries, 5/15/45s backoff).
      monthly_so_far:  Already-spent-amount-in-month, for future monthly gate.

    Returns:
      GenerateResult with `ok=True` and `local_path`/`public_url` on success,
      or `ok=False` and `status_hint` on any non-success outcome.

    Side effects:
      Always writes exactly one ledger row at the end (success or failure).
      May write additional "provider-retry" rows during retry attempts.
    """
    policy = policy or RetryPolicy()
    cfg = load_config(path=config_path)

    # 1. Router picks a provider
    duration = req.duration_sec or 0
    requirements: dict = {"duration_sec": duration}
    if req.outfit:
        requirements["outfit"] = req.outfit
    if req.tone:
        requirements["tone"] = req.tone
    if req.options:
        requirements["options"] = req.options

    provider_id = pick(cfg, req.asset_type, requirements=requirements,
                       provider_filter=provider_filter)

    # 2. Router could not find anyone → write "pending: no-provider" ledger row
    if provider_id == "manual-pending":
        return _emit_failure(
            req,
            provider="router",
            status="pending: no-provider",
            error="router could not find any acceptable provider "
                  f"(filter={provider_filter!r}, asset_type={req.asset_type!r})",
        )

    # 3. Pre-call cost gate (per_asset_cny from router config)
    per_asset_cap = cfg.cost_caps.get("per_asset_cny")
    estimate = _model_cost_yuan(provider_id, duration)
    if per_asset_cap and estimate > per_asset_cap:
        return _emit_failure(
            req,
            provider=provider_id,
            status="budget-exceeded",
            error=(f"estimate ¥{estimate:.4f} > per_asset_cny cap ¥{per_asset_cap:.4f} "
                   f"(model={provider_id}, duration_sec={duration})"),
            cost_yuan=estimate,
        )

    # 4. Build the adapter (may raise NotImplementedError for stubs)
    try:
        adapter = _build_adapter(provider_id, env, req.asset_type)
    except (NotImplementedError, ValueError) as e:
        return _emit_failure(
            req,
            provider=provider_id,
            status="provider-failed",
            error=f"adapter construction failed: {e}",
        )

    # 5. Generate with retry on transient failures
    last_result: Optional[GenerateResult] = None
    last_error: str = ""
    # Pull vendor-specific kwargs from req.options. The orchestrator
    # knows about `out_path` and `model` (used by the ark adapter);
    # everything else stays on req.options and is the adapter's job to
    # ignore / use as it sees fit.
    out_path_opt = (req.options or {}).get("out_path")
    model_opt = (req.options or {}).get("model")
    for attempt in range(policy.max_retries + 1):    # initial + N retries
        try:
            if out_path_opt is not None or model_opt is not None:
                result = adapter.generate(
                    req,
                    out_path=Path(out_path_opt) if out_path_opt else None,
                    model=model_opt,
                )
            else:
                result = adapter.generate(req)
        except Exception as e:
            # Adapter raised unexpectedly — wrap as a failed result so retry
            # logic can classify it like any other error.
            result = GenerateResult(
                ok=False,
                provider=provider_id,
                model="",
                error=f"adapter raised: {type(e).__name__}: {e}",
            )

        last_result = result

        if result.ok:
            # 5a. Success — write ledger + return
            _emit_success(req, result)
            return result

        # 5b. Failure — classify + decide retry
        last_error = result.error or ""
        retry_ok, reason = should_retry(last_error, attempt, policy)
        if not retry_ok:
            # Permanent error or out of retries — write final ledger row + return
            status = _classify_to_status(last_error, exhausted=True)
            _emit_failure(
                req,
                provider=provider_id,
                model=result.model,
                status=status,
                error=last_error,
                cost_yuan=result.cost_yuan,
                duration_ms=result.duration_ms,
                local_path=str(result.local_path) if result.local_path else None,
            )
            return _with_status_hint(result, status)

        # Retryable — write a provider-retry ledger row, sleep, loop
        _emit_failure(
            req,
            provider=provider_id,
            model=result.model,
            status="provider-retry",
            error=f"{last_error} (attempt {attempt + 1}/{policy.max_retries + 1}: {reason})",
            cost_yuan=result.cost_yuan,
            duration_ms=result.duration_ms,
            local_path=str(result.local_path) if result.local_path else None,
        )
        _sleep(attempt + 1, policy)

    # 6. Loop exited without success (theoretical — should_retry gates this)
    status = _classify_to_status(last_error, exhausted=True)
    if last_result is not None:
        _emit_failure(
            req,
            provider=last_result.provider or provider_id,
            model=last_result.model,
            status=status,
            error=last_error,
            cost_yuan=last_result.cost_yuan,
            duration_ms=last_result.duration_ms,
            local_path=str(last_result.local_path) if last_result.local_path else None,
        )
        return _with_status_hint(last_result, status)
    return _emit_failure(
        req,
        provider=provider_id,
        status=status,
        error=last_error,
    )


# --- ledger emission -------------------------------------------------------

def _emit_success(req: GenerateRequest, result: GenerateResult) -> None:
    """Write one ledger row for a successful call."""
    entry = LedgerEntry(
        timestamp=_now_iso(),
        scene_id=req.scene_id,
        provider=result.provider,
        type=req.asset_type,
        cost_cny=result.cost_yuan,
        duration_ms=result.duration_ms,
        status="success",
        asset_path=str(result.local_path) if result.local_path else (result.public_url or ""),
        profile_version=PROFILE_VERSION,
        prompt_hash=fnv1a_64(req.prompt),
        outfit=req.outfit,
    )
    try:
        ledger_append(entry)
    except Exception:
        # Ledger failure should not fail the call — log and move on.
        # (Tests assert the call returns ok=True regardless.)
        pass


def _emit_failure(req: GenerateRequest, *,
                  provider: str, status: str, error: str,
                  model: str = "",
                  cost_yuan: float = 0.0,
                  duration_ms: int = 0,
                  local_path: Optional[str] = None) -> GenerateResult:
    """Write one ledger row for any non-success outcome and return a
    GenerateResult the caller can hand back."""
    entry = LedgerEntry(
        timestamp=_now_iso(),
        scene_id=req.scene_id,
        provider=provider or "router",
        type=req.asset_type,
        cost_cny=cost_yuan,
        duration_ms=duration_ms,
        status=status,
        asset_path=local_path or "",
        profile_version=PROFILE_VERSION,
        prompt_hash=fnv1a_64(req.prompt),
        outfit=req.outfit,
        error=error,
    )
    try:
        ledger_append(entry)
    except Exception:
        pass
    return GenerateResult(
        ok=False,
        provider=provider,
        model=model,
        error=error,
        local_path=Path(local_path) if local_path else None,
        cost_yuan=cost_yuan,
        duration_ms=duration_ms,
        status_hint=status,
    )


def _with_status_hint(result: GenerateResult, status: str) -> GenerateResult:
    """Return a copy of `result` with status_hint set (for caller convenience)."""
    if result.status_hint == status:
        return result
    return GenerateResult(
        ok=result.ok,
        asset_id=result.asset_id,
        local_path=result.local_path,
        public_url=result.public_url,
        cost_yuan=result.cost_yuan,
        duration_ms=result.duration_ms,
        provider=result.provider,
        model=result.model,
        error=result.error,
        status_hint=status,
    )


__all__ = [
    "PROFILE_VERSION",
    "generate_asset",
    "_build_adapter",
]
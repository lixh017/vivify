"""vivify.retry — error classification + backoff for video gen.

Different failures deserve different retry behavior:

  permanent       Don't retry. Surface to user immediately.
                  - quota_exceeded: MiniMax Token Plan 上限
                  - auth_failed: API key invalid / 403 not_model_activated
                  - bad_request: prompt rejected, schema invalid

  transient       Retry with exponential backoff.
                  - timeout: model took >10min, may succeed on retry
                  - network: curl exit nonzero, DNS, etc.
                  - server_error: 5xx from upstream
                  - rate_limit: 429

  unknown          Retry once, then surface if still fails.

Backoff schedule (configurable):
  attempt 1: 5s
  attempt 2: 15s   (×3)
  attempt 3: 45s   (×3)
  attempt 4: 135s  (×3)

Max retries defaults to 3. Override with --max-retries.
"""

from __future__ import annotations

import re
import time
from dataclasses import dataclass
from enum import Enum


class FailureClass(str, Enum):
    PERMANENT = "permanent"
    TRANSIENT = "transient"
    UNKNOWN = "unknown"


@dataclass
class RetryPolicy:
    max_retries: int = 3
    base_delay_sec: float = 5.0
    backoff_factor: float = 3.0  # 5s, 15s, 45s, 135s

    def delay_for_attempt(self, attempt: int) -> float:
        """attempt = 1 for first retry (after initial failure)."""
        return self.base_delay_sec * (self.backoff_factor ** (attempt - 1))


# Patterns that indicate permanent failures (don't retry)
_PERMANENT_PATTERNS = [
    (r"已达到.*用量上限", "quota_exceeded"),
    (r"Token Plan", "quota_exceeded"),
    (r"insufficient.*quota", "quota_exceeded"),
    (r"http\s*403", "permission_denied"),
    (r"\b403\b", "permission_denied"),
    (r"not.*activated", "model_not_activated"),
    (r"model.*not.*exist", "model_not_exist"),
    (r"invalid.*api.*key", "auth_failed"),
    (r"\b401\b", "auth_failed"),
    (r"unauthorized", "auth_failed"),
    (r"\b400\b", "bad_request"),
    (r"400.*invalid", "bad_request"),
    (r"prompt.*rejected", "bad_request"),
    (r"content.*policy", "content_policy"),
    (r"safe.*filter", "content_policy"),
]

# Patterns that indicate transient failures (retry with backoff)
_TRANSIENT_PATTERNS = [
    (r"timed?\s*out", "timeout"),
    (r"timeout", "timeout"),
    (r"\b5\d\d\b", "server_error"),
    (r"internal.*server.*error", "server_error"),
    (r"server\s*error", "server_error"),
    (r"bad\s*gateway", "server_error"),
    (r"\b429\b", "rate_limit"),
    (r"too.*many.*requests", "rate_limit"),
    (r"rate.*limit", "rate_limit"),
    (r"network", "network"),
    (r"connection.*reset", "network"),
    (r"connection.*refused", "network"),
    (r"could.*not.*resolve", "network"),
    (r"curl.*failed", "network"),
]


def classify_error(message: str) -> tuple[FailureClass, str]:
    """Classify an error message. Returns (FailureClass, reason).

    Examples:
      classify_error("已达到 Token Plan 用量上限")
        → (PERMANENT, "quota_exceeded")
      classify_error("video gen curl failed (HTTP 403):")
        → (PERMANENT, "model_not_activated")
      classify_error("mmx failed (rc=5): Request timed out")
        → (TRANSIENT, "timeout")
      classify_error("HTTP 500 server error")
        → (TRANSIENT, "server_error")
    """
    msg_lower = message.lower()

    # Check permanent first (more specific)
    for pattern, reason in _PERMANENT_PATTERNS:
        if re.search(pattern, message, re.IGNORECASE):
            return FailureClass.PERMANENT, reason

    for pattern, reason in _TRANSIENT_PATTERNS:
        if re.search(pattern, message, re.IGNORECASE):
            return FailureClass.TRANSIENT, reason

    return FailureClass.UNKNOWN, "uncategorized"


def should_retry(message: str, attempt: int, policy: RetryPolicy) -> tuple[bool, str]:
    """Decide whether to retry a failure.

    Returns (should_retry, reason).
    """
    if attempt > policy.max_retries:
        return False, f"max retries ({policy.max_retries}) exhausted"

    failure_class, reason = classify_error(message)
    if failure_class == FailureClass.PERMANENT:
        return False, f"permanent failure: {reason}"
    return True, f"{failure_class.value}: {reason}"


def sleep_for_retry(attempt: int, policy: RetryPolicy) -> float:
    """Sleep for the appropriate backoff. Returns seconds slept."""
    delay = policy.delay_for_attempt(attempt)
    time.sleep(delay)
    return delay


__all__ = [
    "FailureClass",
    "RetryPolicy",
    "classify_error",
    "should_retry",
    "sleep_for_retry",
]


if __name__ == "__main__":
    # Quick self-test
    samples = [
        "已达到 Token Plan 用量上限",
        "video gen curl failed (HTTP 403):",
        "mmx failed (rc=5): Request timed out",
        "HTTP 500 server error",
        "HTTP 429 too many requests",
        "curl: (6) Could not resolve host",
        "Connection reset by peer",
        "Some weird error we haven't seen",
    ]
    print(f"{'Message':<55} {'Class':<12} Reason")
    print("─" * 80)
    for m in samples:
        cls, reason = classify_error(m)
        print(f"{m[:55]:<55} {cls.value:<12} {reason}")

"""vivify.cost_cap — enforce per-video + monthly cost limits.

Wraps the opc-cost-cap skill (which defines the rules) into a runtime
check that vivify episode render calls BEFORE submitting API requests.

Default caps (from opc-cost-cap skill, Phase 1):
  - per_video_soft: ¥50  (warn, ask to confirm)
  - per_video_hard: ¥100 (abort)
  - monthly_hard:   ¥60,000 (Phase 1 budget ¥40-70k; abort at 60k for headroom)

Source of truth for monthly running total:
  SUM(cost_yuan) FROM episodes WHERE render_completed_at LIKE 'YYYY-MM%'

If the per-render estimate would breach any cap, abort with a clear
error explaining which cap and how to override.
"""

from __future__ import annotations

import sqlite3
from datetime import datetime, timezone
from pathlib import Path

# Caps (in ¥). These mirror ~/.claude/skills/opc-cost-cap/SKILL.md.
DEFAULT_PER_VIDEO_SOFT = 50.0
DEFAULT_PER_VIDEO_HARD = 100.0
DEFAULT_MONTHLY_HARD = 60_000.0


class CostCapExceeded(Exception):
    """Raised when an estimate exceeds a configured cap."""

    def __init__(self, cap_name: str, cap_value: float, estimate: float, suggestion: str):
        self.cap_name = cap_name
        self.cap_value = cap_value
        self.estimate = estimate
        super().__init__(
            f"[cost-cap] ABORT — {cap_name} exceeded: "
            f"estimate ¥{estimate:.2f} > cap ¥{cap_value:.2f}\n"
            f"  {suggestion}"
        )


def current_month() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m")


def monthly_spend_yuan(conn: sqlite3.Connection, month: str | None = None) -> float:
    """Sum cost_yuan from all episodes completed in the given month."""
    month = month or current_month()
    row = conn.execute(
        """SELECT COALESCE(SUM(cost_yuan), 0.0) AS total
           FROM episodes
           WHERE render_completed_at LIKE ?""",
        (f"{month}%",),
    ).fetchone()
    # row may be sqlite3.Row (dict-like) or plain tuple (positional)
    try:
        return float(row["total"])
    except (TypeError, KeyError):
        return float(row[0])


def check_cost_caps(conn: sqlite3.Connection, estimate_yuan: float,
                    per_video_hard: float = DEFAULT_PER_VIDEO_HARD,
                    monthly_hard: float = DEFAULT_MONTHLY_HARD,
                    per_video_soft: float = DEFAULT_PER_VIDEO_SOFT,
                    month: str | None = None,
                    force: bool = False) -> dict:
    """Check whether an estimated cost would breach any cap.

    Returns a dict with:
      - allowed: bool (True if all caps pass or force=True)
      - per_video_would_exceed: bool (estimate > per_video_hard)
      - monthly_would_exceed: bool (estimate + monthly_so_far > monthly_hard)
      - monthly_so_far: float
      - monthly_after: float
      - estimate: float
      - caps: dict of effective caps

    If not allowed and not force, raises CostCapExceeded with the
    most-binding cap.
    """
    month = month or current_month()
    spent = monthly_spend_yuan(conn, month)
    monthly_after = spent + estimate_yuan

    result = {
        "estimate": round(estimate_yuan, 4),
        "monthly_so_far": round(spent, 4),
        "monthly_after": round(monthly_after, 4),
        "per_video_would_exceed": estimate_yuan > per_video_hard,
        "monthly_would_exceed": monthly_after > monthly_hard,
        "per_video_soft_warn": estimate_yuan > per_video_soft,
        "caps": {
            "per_video_soft": per_video_soft,
            "per_video_hard": per_video_hard,
            "monthly_hard": monthly_hard,
            "month": month,
        },
        "force": force,
        "allowed": True,  # default; flipped to False only when caps block
    }

    # Most-binding cap wins
    if result["monthly_would_exceed"]:
        if not force:
            raise CostCapExceeded(
                "monthly_hard",
                monthly_hard,
                monthly_after,
                f"already spent ¥{spent:.2f} in {month}. "
                f"--force to override, or wait until {month} ends.",
            )
        # force=True: keep allowed=True (set above), but caller sees
        # monthly_would_exceed=True for the "OVERRIDDEN" banner.
    elif result["per_video_would_exceed"]:
        if not force:
            raise CostCapExceeded(
                "per_video_hard",
                per_video_hard,
                estimate_yuan,
                "Reduce --target-dur / --quality-tier, or pass --force to override.",
            )
        # force=True: see above
    return result


def render_cost_summary(result: dict) -> str:
    """Format a cost-cap check result for human display."""
    caps = result["caps"]
    # Priority: BLOCK > OVERRIDDEN > PASS
    if not result["allowed"] and not result["force"]:
        status = "❌ BLOCK"
    elif result["force"] and (result["per_video_would_exceed"] or result["monthly_would_exceed"]):
        status = "⚠ OVERRIDDEN (--force)"
    else:
        status = "✅ PASS"
    lines = [
        f"[cost-cap] {status}",
        f"  this render estimate:  ¥{result['estimate']:.2f}",
        f"  spent this month ({caps['month']}):     ¥{result['monthly_so_far']:.2f}",
        f"  after this render:     ¥{result['monthly_after']:.2f}",
        f"  caps: per_video_soft=¥{caps['per_video_soft']:.0f}  "
        f"per_video_hard=¥{caps['per_video_hard']:.0f}  "
        f"monthly_hard=¥{caps['monthly_hard']:.0f}",
    ]
    if result["per_video_soft_warn"] and result["allowed"]:
        lines.append(f"  ⚠ above per-video soft cap (¥{caps['per_video_soft']:.0f}) — within hard limit")
    return "\n".join(lines)


__all__ = [
    "DEFAULT_PER_VIDEO_SOFT",
    "DEFAULT_PER_VIDEO_HARD",
    "DEFAULT_MONTHLY_HARD",
    "CostCapExceeded",
    "current_month",
    "monthly_spend_yuan",
    "check_cost_caps",
    "render_cost_summary",
]
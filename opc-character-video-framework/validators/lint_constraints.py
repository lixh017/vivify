#!/usr/bin/env python3
"""
lint_constraints.py — Check that outfit anti_patterns and style_anchor
constraints are STRONG enough.

Usage:
    python3 validators/lint_constraints.py characters/fengge

Checks:
  - Count NEVER / no / 不要 in each outfit's anti_patterns (warn if < 2)
  - Count NEVER clauses in style_anchor (warn if < 5)
  - Check for common AI tells in outfit descriptions
    ("modern", "realistic", "human" — too vague)

Exit codes:
  0 = all checks pass (warnings allowed)
  1 = one or more hard checks failed
  2 = invalid args / load error
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

import yaml

NEGATIVE_PATTERNS = ["NEVER", "no ", "不要", "NOT", "avoid"]
MIN_ANTI_PATTERNS_PER_OUTFIT = 2
MIN_NEVER_IN_STYLE_ANCHOR = 5
AI_TELLS = ["modern", "realistic", "human", "generic", "ordinary", "normal"]


def _check(label: str, ok: bool, detail: str = "") -> bool:
    status = "PASS" if ok else "FAIL"
    line = f"  [{status}] {label}"
    if detail:
        line += f" — {detail}"
    print(line)
    return ok


def _warn(label: str, detail: str = "") -> None:
    print(f"  [WARN] {label} — {detail}")


def count_negatives(text: str) -> int:
    text_lower = text.lower()
    count = 0
    for p in NEGATIVE_PATTERNS:
        if p.lower() in text_lower:
            # count occurrences (NEVER might appear many times)
            count += text_lower.count(p.lower())
    return count


def find_ai_tells(text: str) -> list[str]:
    text_lower = text.lower()
    return [t for t in AI_TELLS if t in text_lower]


def load_character(character_dir: Path) -> dict:
    yaml_path = character_dir / "character.yaml"
    if not yaml_path.exists():
        print(f"[ERROR] character.yaml not found at {yaml_path}", file=sys.stderr)
        sys.exit(2)
    with yaml_path.open("r", encoding="utf-8") as f:
        data = yaml.safe_load(f)
    if not isinstance(data, dict) or "character" not in data:
        print(f"[ERROR] character.yaml is missing top-level 'character' key", file=sys.stderr)
        sys.exit(2)
    return data["character"]


def lint(character_dir: Path) -> int:
    print(f"\n=== Constraint-strength lint on {character_dir} ===")
    char = load_character(character_dir)

    failures = 0

    # --- Per-outfit anti_patterns strength ---
    print("\n[outfit anti_patterns]")
    outfits = char.get("outfits") or []
    weak_outfits = []
    for outfit in outfits:
        oid = outfit.get("id", "<no id>")
        anti = (outfit.get("anti_patterns") or "").strip()
        if not anti:
            _check(f"outfit '{oid}' has anti_patterns", False, "(empty)")
            failures += 1
            continue
        count = count_negatives(anti)
        ok = count >= MIN_ANTI_PATTERNS_PER_OUTFIT
        marker = "PASS" if ok else "WARN"
        print(f"  [{marker}] outfit '{oid}' anti_patterns negative tokens = {count} "
              f"(threshold {MIN_ANTI_PATTERNS_PER_OUTFIT})")
        if not ok:
            weak_outfits.append(oid)

        # AI tells in the anti_patterns string itself
        tells = find_ai_tells(anti)
        if tells:
            _warn(f"outfit '{oid}' anti_patterns contains AI-tell words", ", ".join(tells))

        # Also check the description field
        desc = (outfit.get("description") or "").strip()
        desc_tells = find_ai_tells(desc)
        if desc_tells:
            _warn(f"outfit '{oid}' description contains vague AI-tell words",
                  ", ".join(desc_tells))

    if weak_outfits:
        _warn("outfits with weak anti_patterns (AI will leak modern/generic cues)",
              ", ".join(weak_outfits))

    # --- Style anchor strength ---
    print("\n[style_anchor]")
    anchor = (char.get("style_anchor") or "").strip()
    if not anchor:
        _check("style_anchor present", False, "(empty)")
        failures += 1
    else:
        # Count NEVER specifically
        never_count = len(re.findall(r"\bNEVER\b", anchor))
        _check(f"style_anchor has >= {MIN_NEVER_IN_STYLE_ANCHOR} 'NEVER' clauses",
               never_count >= MIN_NEVER_IN_STYLE_ANCHOR,
               f"found {never_count}")
        if never_count < MIN_NEVER_IN_STYLE_ANCHOR:
            failures += 1

        total_neg = count_negatives(anchor)
        print(f"  [INFO] total negative tokens in style_anchor: {total_neg}")

    # --- Summary ---
    print()
    if failures == 0:
        print(f"=== RESULT: CONSTRAINT STRENGTH OK ({character_dir}) ===")
        return 0
    print(f"=== RESULT: {failures} HARD FAIL(S) ({character_dir}) ===")
    return 1


def main() -> int:
    if len(sys.argv) != 2:
        print("Usage: python3 lint_constraints.py <character_dir>", file=sys.stderr)
        return 2
    character_dir = Path(sys.argv[1]).resolve()
    if not character_dir.is_dir():
        print(f"[ERROR] not a directory: {character_dir}", file=sys.stderr)
        return 2
    return lint(character_dir)


if __name__ == "__main__":
    sys.exit(main())
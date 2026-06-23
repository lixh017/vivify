#!/usr/bin/env python3
"""
lint_character.py — Lint a character config directory.

Usage:
    python3 validators/lint_character.py characters/fengge

Checks:
  - Required fields present (name, species, canonical, outfits, scenes, voice_profiles)
  - Each outfit has anti_patterns (critical — without it, AI generates wrong outfits)
  - style_anchor contains strong negative anchors (NEVER / no / 不要 / NOT)
  - style_anchor is at least 30 chars
  - At least 3 outfits, 5 scenes
  - voice_profiles has at least 1 tone
  - canonical.primary file exists and is > 50KB
  - canonical.alternates are jpg files

Exit codes:
  0 = all checks pass
  1 = one or more checks failed
  2 = invalid args / load error
"""
from __future__ import annotations

import os
import sys
from pathlib import Path

import yaml

REQUIRED_TOP_LEVEL = ["name", "species", "canonical", "outfits", "scenes", "voice_profiles"]
NEGATIVE_ANCHOR_PATTERNS = ["NEVER", "no ", "不要", "NOT"]
MIN_OUTFITS = 3
MIN_SCENES = 5
MIN_STYLE_ANCHOR_CHARS = 30
CANONICAL_MIN_BYTES = 50 * 1024  # 50 KB


def _check(label: str, ok: bool, detail: str = "") -> bool:
    status = "PASS" if ok else "FAIL"
    line = f"  [{status}] {label}"
    if detail:
        line += f" — {detail}"
    print(line)
    return ok


def _warn(label: str, detail: str = "") -> None:
    print(f"  [WARN] {label} — {detail}")


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
    print(f"\n=== Linting {character_dir} ===")
    char = load_character(character_dir)

    failures = 0

    # --- Required fields ---
    print("\n[required fields]")
    for field in REQUIRED_TOP_LEVEL:
        ok = field in char and char[field] is not None
        if not _check(f"character.{field} present", ok):
            failures += 1

    # --- Outfits ---
    print("\n[outfits]")
    outfits = char.get("outfits") or []
    if not _check(f"outfits has >= {MIN_OUTFITS} entries", len(outfits) >= MIN_OUTFITS,
                  f"found {len(outfits)}"):
        failures += 1

    outfits_missing_anti = []
    for outfit in outfits:
        oid = outfit.get("id", "<no id>")
        anti = (outfit.get("anti_patterns") or "").strip()
        if not anti:
            outfits_missing_anti.append(oid)
        if not _check(f"outfit '{oid}' has anti_patterns", bool(anti),
                      "" if anti else "(empty)"):
            failures += 1
    if outfits_missing_anti:
        _warn("outfits without anti_patterns (AI will generate wrong outfits)",
              ", ".join(outfits_missing_anti))

    # --- Scenes ---
    print("\n[scenes]")
    scenes = char.get("scenes") or []
    if not _check(f"scenes has >= {MIN_SCENES} entries", len(scenes) >= MIN_SCENES,
                  f"found {len(scenes)}"):
        failures += 1

    # --- Voice profiles ---
    print("\n[voice_profiles]")
    voices = char.get("voice_profiles") or {}
    if not _check("voice_profiles has >= 1 tone", len(voices) >= 1,
                  f"found {len(voices)}"):
        failures += 1

    # --- Style anchor ---
    print("\n[style_anchor]")
    anchor = (char.get("style_anchor") or "").strip()
    if not _check(f"style_anchor length >= {MIN_STYLE_ANCHOR_CHARS} chars",
                  len(anchor) >= MIN_STYLE_ANCHOR_CHARS,
                  f"len={len(anchor)}"):
        failures += 1

    found_anchors = [p for p in NEGATIVE_ANCHOR_PATTERNS if p in anchor]
    if not _check("style_anchor contains negative anchors",
                  bool(found_anchors),
                  f"matched: {found_anchors or 'none'}"):
        failures += 1

    # --- Canonical images ---
    print("\n[canonical]")
    canonical = char.get("canonical") or {}
    primary_rel = canonical.get("primary")
    if not primary_rel:
        _check("canonical.primary set", False, "missing")
        failures += 1
    else:
        primary_path = character_dir / primary_rel
        exists = primary_path.exists()
        if not _check(f"canonical.primary exists ({primary_rel})", exists):
            failures += 1
        else:
            size = primary_path.stat().st_size
            if not _check("canonical.primary size >= 50 KB",
                          size >= CANONICAL_MIN_BYTES,
                          f"{size} bytes ({size/1024:.1f} KB)"):
                failures += 1

    alternates = canonical.get("alternates") or []
    bad_alts = []
    for alt in alternates:
        alt_path = character_dir / alt
        if not alt_path.exists():
            bad_alts.append(f"{alt} (missing)")
        elif not alt.lower().endswith((".jpg", ".jpeg")):
            bad_alts.append(f"{alt} (not .jpg)")
    if not _check("canonical.alternates are jpg files and exist",
                  not bad_alts,
                  ", ".join(bad_alts) if bad_alts else f"{len(alternates)} ok"):
        failures += 1

    # --- Summary ---
    print()
    if failures == 0:
        print(f"=== RESULT: ALL CHECKS PASSED ({character_dir}) ===")
        return 0
    print(f"=== RESULT: {failures} CHECK(S) FAILED ({character_dir}) ===")
    return 1


def main() -> int:
    if len(sys.argv) != 2:
        print("Usage: python3 lint_character.py <character_dir>", file=sys.stderr)
        return 2
    character_dir = Path(sys.argv[1]).resolve()
    if not character_dir.is_dir():
        print(f"[ERROR] not a directory: {character_dir}", file=sys.stderr)
        return 2
    return lint(character_dir)


if __name__ == "__main__":
    sys.exit(main())
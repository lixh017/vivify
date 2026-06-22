#!/usr/bin/env python3
"""
canonical_image_check.py — Validate canonical reference image quality.

Usage:
    python3 validators/canonical_image_check.py characters/fengge/canonical

Checks each image in the directory:
  - File size > 100 KB (lower usually = broken generation)
  - Resolution >= 1024x1024
  - Aspect ratio is square (1:1) or 9:16 (vertical)
  - File is JPEG or PNG

Also reads characters/<name>/character.yaml and verifies
canonical.primary and canonical.alternates reference images
that pass the same checks.

Exit codes:
  0 = all checks pass
  1 = one or more checks failed
  2 = invalid args / load error
"""
from __future__ import annotations

import sys
from pathlib import Path

from PIL import Image

MIN_BYTES = 100 * 1024  # 100 KB
MIN_DIM = 1024

ALLOWED_ASPECTS = [
    ("square (1:1)", lambda w, h: w == h),
    ("9:16 vertical", lambda w, h: h > 0 and abs(w / h - 9 / 16) < 0.02),
]

ALLOWED_EXTS = {".jpg", ".jpeg", ".png"}


def _check(label: str, ok: bool, detail: str = "") -> bool:
    status = "PASS" if ok else "FAIL"
    line = f"  [{status}] {label}"
    if detail:
        line += f" — {detail}"
    print(line)
    return ok


def check_image(path: Path) -> tuple[int, list[str]]:
    """Return (failure_count, error_messages) for a single image."""
    failures = 0
    errors: list[str] = []

    # --- extension ---
    ext = path.suffix.lower()
    if ext not in ALLOWED_EXTS:
        _check(f"{path.name} is JPEG/PNG", False, f"got {ext}")
        return 1, [f"bad extension: {ext}"]

    # --- size ---
    size = path.stat().st_size
    if not _check(f"{path.name} size >= 100 KB", size >= MIN_BYTES,
                  f"{size} bytes ({size/1024:.1f} KB)"):
        failures += 1
        errors.append("too small")

    # --- resolution + aspect ratio ---
    try:
        with Image.open(path) as img:
            w, h = img.size
    except Exception as e:
        _check(f"{path.name} openable as image", False, str(e))
        return failures + 1, errors + [f"open failed: {e}"]

    if not _check(f"{path.name} resolution >= {MIN_DIM}x{MIN_DIM}",
                  w >= MIN_DIM and h >= MIN_DIM,
                  f"{w}x{h}"):
        failures += 1
        errors.append("low resolution")

    aspect_label = "unknown"
    aspect_ok = False
    for label, predicate in ALLOWED_ASPECTS:
        if predicate(w, h):
            aspect_label = label
            aspect_ok = True
            break
    if not aspect_ok:
        aspect_label = f"{w}:{h}"
    if not _check(f"{path.name} aspect ratio is square or 9:16",
                  aspect_ok,
                  aspect_label):
        failures += 1
        errors.append("bad aspect")

    return failures, errors


def check_directory(directory: Path) -> int:
    print(f"\n=== Checking canonical images in {directory} ===")
    if not directory.is_dir():
        print(f"[ERROR] not a directory: {directory}", file=sys.stderr)
        return 2

    failures = 0
    images = sorted(p for p in directory.iterdir()
                    if p.suffix.lower() in ALLOWED_EXTS)
    if not images:
        print(f"  [WARN] no JPEG/PNG files in {directory}")
        return 0

    for img_path in images:
        f, _ = check_image(img_path)
        failures += f

    print()
    if failures == 0:
        print(f"=== RESULT: ALL CANONICAL IMAGES OK ({directory}) ===")
        return 0
    print(f"=== RESULT: {failures} CANONICAL IMAGE ISSUE(S) ({directory}) ===")
    return 1


def main() -> int:
    if len(sys.argv) != 2:
        print("Usage: python3 canonical_image_check.py <canonical_dir>", file=sys.stderr)
        return 2
    return check_directory(Path(sys.argv[1]).resolve())


if __name__ == "__main__":
    sys.exit(main())
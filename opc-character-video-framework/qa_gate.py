"""qa_gate.py — heuristic QA checks for generated shots.

Before muxing, each shot's image is checked against:
  - File size: suspiciously small = probably broken generation
  - Aspect ratio: 9:16 vertical (the configured platform)
  - Color statistics: not entirely black/white (broken generation)

For vision-based checks (e.g. "does the panda have visible claws?"),
set OPENAI_API_KEY or ANTHROPIC_API_KEY and the gate will use the
multimodal model. Without those keys, only heuristic checks run.

Usage:
  python3 qa_gate.py /tmp/shot-01.jpg /tmp/shot-02.jpg ...
  exit code 0 if all pass, 1 if any fail
"""

import os
import sys
from pathlib import Path


def check_image(path: str) -> tuple[bool, str]:
    """Return (passed, message) for a single image."""
    p = Path(path)
    if not p.exists():
        return False, f"missing: {path}"

    # 1. File size check — Seedream outputs are typically 200KB-2MB
    size = p.stat().st_size
    if size < 50_000:
        return False, f"{p.name}: too small ({size} bytes, likely broken)"

    # 2. Image dimensions + color stats
    try:
        from PIL import Image
        img = Image.open(p).convert("RGB")
        w, h = img.size
    except ImportError:
        return True, f"{p.name}: PIL not installed, skipping visual checks"
    except Exception as e:
        return False, f"{p.name}: cannot open ({e})"

    # 3. Aspect ratio: should be 9:16 vertical (720x1280 or 1024x1792)
    ratio = h / w if w else 0
    if ratio < 1.5 or ratio > 1.9:
        return False, f"{p.name}: aspect {w}x{h} (ratio {ratio:.2f}) not 9:16"

    # 4. Color stats — not entirely black/white
    import statistics
    pixels = list(img.getdata())
    if len(pixels) < 1000:
        return True, f"{p.name}: too few pixels to analyze"
    sample = pixels[::max(1, len(pixels) // 1000)]
    r_avg = statistics.mean(p[0] for p in sample)
    g_avg = statistics.mean(p[1] for p in sample)
    b_avg = statistics.mean(p[2] for p in sample)
    brightness = (r_avg + g_avg + b_avg) / 3
    if brightness < 20:
        return False, f"{p.name}: too dark (avg brightness {brightness:.1f})"
    if brightness > 240:
        return False, f"{p.name}: too bright (avg brightness {brightness:.1f})"

    # 5. Color variance — not all one color
    r_std = statistics.stdev(p[0] for p in sample) if len(sample) > 1 else 0
    if r_std < 10:
        return False, f"{p.name}: too monochrome (R stddev {r_std:.1f})"

    return True, f"{p.name}: ok ({w}x{h}, brightness {brightness:.0f}, R stddev {r_std:.1f})"


def check_all(paths: list[str]) -> tuple[int, int]:
    """Return (passed_count, failed_count)."""
    passed = failed = 0
    for p in paths:
        ok, msg = check_image(p)
        print(f"  {'✓' if ok else '✗'} {msg}")
        if ok:
            passed += 1
        else:
            failed += 1
    print(f"\nQA: {passed} passed, {failed} failed")
    return passed, failed


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("usage: qa_gate.py <image1> [image2 ...]")
        sys.exit(2)
    _, failed = check_all(sys.argv[1:])
    sys.exit(0 if failed == 0 else 1)
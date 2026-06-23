#!/usr/bin/env python3
"""
lint_prompt.py — Lint a single shot prompt against Seedance 2.0 best practices.

Usage:
    python3 validators/lint_prompt.py "镜头 1: 中景, 固定机位. 峰哥抱膝坐于炉旁..."

Checks:
  - Must use 分镜 structure ("镜头 N:" markers)
  - Must specify only ONE camera movement (warn if multiple)
  - Should use special character syntax: () <> {} 【】
  - Must include constraint words (no watermark, no logo, no text/subtitles)
  - Should include emotion-to-action mapping (not just abstract emotions like "happy" or "sad")
  - Should specify duration (4-15s for Seedance 2.0)
  - Should specify resolution (480p/720p/1080p)

Exit codes:
  0 = all checks pass
  1 = one or more checks failed
  2 = invalid args
"""
from __future__ import annotations

import re
import sys

CAMERA_MOVEMENT_PATTERNS = [
    "推镜头", "拉镜头", "横移", "跟拍", "摇镜头", "升降", "固定机位", "环绕",
    "zoom in", "zoom out", "pan left", "pan right", "tilt up", "tilt down",
    "dolly in", "dolly out", "tracking shot", "orbit", "static", "fixed",
    "slow push", "slow pull",
]

CONSTRAINT_PATTERNS = {
    "watermark": ["no watermark", "no watermarks", "无水印", "不要水印"],
    "logo":      ["no logo", "no logos", "无 logo", "无logo", "不要 logo"],
    "text":      ["no text", "no subtitles", "no caption", "无字幕", "不要文字", "无文字"],
}

ABSTRACT_EMOTIONS = [
    "happy", "sad", "angry", "fearful", "surprised", "disgust",
    "开心", "伤心", "难过", "愤怒", "高兴", "忧郁", "兴奋",
]

DURATION_RE = re.compile(r"\b(\d+(?:\.\d+)?)\s*s(?:ec)?(?:ond)?s?\b|时长\s*(\d+)\s*秒|duration[: ]+\d+s?",
                         re.IGNORECASE)
RESOLUTION_RE = re.compile(r"\b(480p|720p|1080p|4k|2160p)\b", re.IGNORECASE)
SHOT_MARKER_RE = re.compile(r"镜头\s*\d+|分镜\s*\d+|shot\s*\d+|scene\s*\d+", re.IGNORECASE)
SPECIAL_CHAR_PATTERNS = ["()", "<>", "{}", "【】"]


def _check(label: str, ok: bool, detail: str = "") -> bool:
    status = "PASS" if ok else "FAIL"
    line = f"  [{status}] {label}"
    if detail:
        line += f" — {detail}"
    print(line)
    return ok


def _warn(label: str, detail: str = "") -> None:
    print(f"  [WARN] {label} — {detail}")


def lint(prompt: str) -> int:
    print("\n=== Linting prompt ===")
    if not prompt or not prompt.strip():
        print("[ERROR] prompt is empty")
        return 1

    failures = 0
    lower = prompt.lower()

    # --- 分镜 structure ---
    print("\n[分镜 structure]")
    shot_markers = SHOT_MARKER_RE.findall(prompt)
    if not _check("prompt uses 镜头 N: / shot N markers", bool(shot_markers),
                  f"found {len(shot_markers)} marker(s)"):
        failures += 1

    # --- Camera movement count ---
    print("\n[camera movement]")
    found_movements = []
    for pat in CAMERA_MOVEMENT_PATTERNS:
        if pat.lower() in lower:
            found_movements.append(pat)
    if len(found_movements) > 1:
        _warn("multiple camera movements detected", ", ".join(found_movements))
        _check("only one camera movement specified", False,
               f"found {len(found_movements)}: {', '.join(found_movements)}")
        failures += 1
    elif len(found_movements) == 1:
        _check("exactly one camera movement", True, found_movements[0])
    else:
        _warn("no explicit camera movement", "Seedance 2.0 performs better with explicit movement")

    # --- Special character syntax ---
    print("\n[special character syntax]")
    found_specials = []
    for pat in SPECIAL_CHAR_PATTERNS:
        if pat[0] in prompt and pat[1] in prompt:
            found_specials.append(pat)
    if not _check("uses special character syntax ()/<>/{}/【】",
                  bool(found_specials),
                  f"matched: {', '.join(found_specials) or 'none'}"):
        failures += 1

    # --- Constraint words ---
    print("\n[constraints]")
    missing_constraints = []
    for category, pats in CONSTRAINT_PATTERNS.items():
        if not any(p.lower() in lower for p in pats):
            missing_constraints.append(category)
    if not _check("includes 'no watermark / logo / text' constraints",
                  not missing_constraints,
                  f"missing: {', '.join(missing_constraints) or 'none'}"):
        failures += 1

    # --- Emotion-to-action mapping ---
    print("\n[emotion-to-action mapping]")
    has_action_verbs = bool(re.search(
        r"\b(grabs|holds|leans|smiles|laughs|sighs|clutches|raises|lowers|opens|closes|"
        r"pushes|pulls|kicks|throws|catches|sits|stands|walks|runs|turns|tilts|nods|shakes|"
        r"拿起|握住|放下|推开|拉近|起身|坐下|转头|低头|抬头|点头|摇头|微笑|叹息)",
        lower,
    ))
    abstract_present = [e for e in ABSTRACT_EMOTIONS if e.lower() in lower]
    if abstract_present and not has_action_verbs:
        _check("emotion-to-action mapping present",
               False,
               f"only abstract emotions found ({', '.join(abstract_present)}); pair with physical actions")
        failures += 1
    else:
        _check("emotion-to-action mapping present",
               has_action_verbs or not abstract_present,
               "actions detected" if has_action_verbs else "no emotion words at all")

    # --- Duration ---
    print("\n[duration]")
    duration_match = DURATION_RE.search(prompt)
    if duration_match:
        raw = next(g for g in duration_match.groups() if g)
        seconds = float(raw)
        if 4 <= seconds <= 15:
            _check("duration is 4-15s", True, f"{seconds}s")
        else:
            _check("duration is 4-15s", False, f"{seconds}s (out of range)")
            failures += 1
    else:
        _check("duration specified", False, "no 4-15s duration found")
        failures += 1

    # --- Resolution ---
    print("\n[resolution]")
    res_match = RESOLUTION_RE.search(prompt)
    if not _check("resolution specified (480p/720p/1080p)",
                  bool(res_match),
                  res_match.group(0) if res_match else "not found"):
        failures += 1

    # --- Summary ---
    print()
    if failures == 0:
        print("=== RESULT: PROMPT PASSED ALL CHECKS ===")
        return 0
    print(f"=== RESULT: PROMPT FAILED {failures} CHECK(S) ===")
    return 1


def main() -> int:
    if len(sys.argv) < 2:
        print("Usage: python3 lint_prompt.py <prompt string>", file=sys.stderr)
        return 2
    prompt = " ".join(sys.argv[1:])
    return lint(prompt)


if __name__ == "__main__":
    sys.exit(main())
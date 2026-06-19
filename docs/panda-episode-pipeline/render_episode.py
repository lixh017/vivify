#!/usr/bin/env python3
"""panda-episode-pipeline — render a panda IP episode end-to-end.

Takes a STORYBOARD.md + SCRIPT-douyin.md pair and produces a finished
L3 MP4: video clips (Seedance 1.5-pro) + TTS voiceover (海螺
speech-02-hd) + ffmpeg-synthesized BGM + phone comments overlay,
all muxed into a single 720p 9:16 file.

Proven by panda EP#001 (June 2026): 58s episode in ~12 minutes,
~¥11-19 per episode.

Usage:
    python3 render_episode.py \
      --storyboard docs/showcase/panda-episode-001/STORYBOARD.md \
      --script     docs/showcase/panda-episode-001/SCRIPT-douyin.md \
      --voice      治愈 \
      --platform   抖音 \
      --target-dur 58 \
      --out        /tmp/episode.mp4

Required env (see INSTALL.md):
    ARK_API_KEY       火山引擎 Ark
    ARK_BASE_URL      default: https://ark.cn-beijing.volces.com/api/v3
    MINIMAX_API_KEY   海螺 MiniMax
    FFMPEG            path to ffmpeg binary (auto-detected if unset)
    OPC_RENDER_DIR    default: /tmp/opc-render
"""

import argparse
import base64
import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path

# ---- config ---------------------------------------------------------------

DEFAULT_ARK_BASE = "https://ark.cn-beijing.volces.com/api/v3"
DEFAULT_RENDER_DIR = "/tmp/opc-render"
DEFAULT_IMAGE_MODEL = "doubao-seedream-4-0-250828"
DEFAULT_VIDEO_MODEL = "doubao-seedance-1-5-pro-251215"
DEFAULT_TTS_MODEL = "speech-02-hd"
DEFAULT_TTS_VOICE = "male-qn-jingying"
DEFAULT_TTS_SPEED = 0.82
DEFAULT_TTS_PITCH = -1

# ---- tiny logger ----------------------------------------------------------

def info(msg: str) -> None:
    print(f"[pipeline] {msg}", flush=True)

def warn(msg: str) -> None:
    print(f"[pipeline] WARN: {msg}", file=sys.stderr, flush=True)

def fatal(msg: str) -> "None":
    print(f"[pipeline] FATAL: {msg}", file=sys.stderr, flush=True)
    sys.exit(1)

# ---- env ------------------------------------------------------------------

def require_env() -> dict:
    env = {
        "ARK_API_KEY":     os.environ.get("ARK_API_KEY"),
        "ARK_BASE_URL":    os.environ.get("ARK_BASE_URL", DEFAULT_ARK_BASE),
        "MINIMAX_API_KEY": os.environ.get("MINIMAX_API_KEY"),
        "FFMPEG":          os.environ.get("FFMPEG") or find_ffmpeg(),
        "RENDER_DIR":      os.environ.get("OPC_RENDER_DIR", DEFAULT_RENDER_DIR),
    }
    if not env["ARK_API_KEY"]:
        fatal("ARK_API_KEY not set (火山引擎 Ark key — see INSTALL.md)")
    if not env["MINIMAX_API_KEY"]:
        fatal("MINIMAX_API_KEY not set (海螺 TTS key — see INSTALL.md)")
    if not env["FFMPEG"] or not Path(env["FFMPEG"]).exists():
        fatal(f"ffmpeg not found at {env['FFMPEG']} (set FFMPEG env var or install via apt)")
    Path(env["RENDER_DIR"]).mkdir(parents=True, exist_ok=True)
    return env

def find_ffmpeg() -> str:
    candidates = [
        "/root/.openclaw/extensions/dingtalk-connector/node_modules/@ffmpeg-installer/linux-x64/ffmpeg",
        "/usr/bin/ffmpeg",
        "/usr/local/bin/ffmpeg",
        "ffmpeg",  # PATH
    ]
    for c in candidates:
        if Path(c).exists() and os.access(c, os.X_OK):
            return c
        if c == "ffmpeg":
            # try via shell
            r = subprocess.run(["which", "ffmpeg"], capture_output=True, text=True)
            if r.returncode == 0:
                return r.stdout.strip()
    return ""

# ---- curl wrapper ---------------------------------------------------------

def curl(method: str, url: str, headers: dict, body: dict = None,
         out_file: str = None) -> tuple[int, str, bytes]:
    """Returns (http_code, stderr, response_body). http_code is the actual
    HTTP status (200, 401, 404, ...), not curl's exit code."""
    cmd = ["curl", "-sS", "-X", method, url,
           "-w", "\n__HTTP_CODE__%{http_code}__"]
    for k, v in headers.items():
        cmd += ["-H", f"{k}: {v}"]
    body_tmp = None
    if body is not None:
        body_str = json.dumps(body)
        # If payload is large (e.g. base64 reference_image ≈1MB),
        # pass via --data-binary @tmpfile to avoid E2BIG on argv.
        if len(body_str) > 64_000:
            body_tmp = Path("/tmp") / f"opc-render-body-{os.getpid()}-{id(body)}.json"
            body_tmp.write_text(body_str, encoding="utf-8")
            cmd += ["-H", "Content-Type: application/json",
                    "--data-binary", f"@{body_tmp}"]
        else:
            cmd += ["-H", "Content-Type: application/json", "-d", body_str]
    if out_file:
        cmd += ["-o", out_file]

    try:
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=300)
    finally:
        if body_tmp and body_tmp.exists():
            body_tmp.unlink()
    if r.returncode != 0:
        # curl itself failed (network, DNS, etc.) — not an HTTP error
        return 0, r.stderr or "curl failed", b""

    # Parse the trailing __HTTP_CODE__NNN__ marker from stdout
    out = r.stdout
    http_code = 0
    marker = "__HTTP_CODE__"
    if marker in out:
        idx = out.rfind(marker)
        code_str = out[idx + len(marker):].split("__")[0]
        try:
            http_code = int(code_str)
        except ValueError:
            http_code = 0
        # Remove the marker + the body before it stays intact
        out = out[:idx].rstrip("\n")
    if out_file:
        with open(out_file, "rb") as f:
            return http_code, r.stderr or "", f.read()
    return http_code, r.stderr or "", out.encode() if isinstance(out, str) else out

# ---- STORYBOARD parsing ---------------------------------------------------

# Each shot starts with a Markdown heading like "## 镜头 1 — 黑屏白字 (钩子) [0:00-0:03]"
SHOT_RE = re.compile(
    r"^##\s+镜头\s+(\d+)\s+—\s+(.+?)\s+\[(\d+):(\d+)-(\d+):(\d+)\]\s*$",
    re.MULTILINE,
)

# 可灵 prompt block (``` ... ``` immediately after 视觉/构图/灯光/时长/镜头运动)
KELING_PROMPT_RE = re.compile(
    r"可灵\s*prompt(?:\s*\([^)]+\))?\s*[:：]\s*```([\s\S]+?)```", re.IGNORECASE
)

def parse_storyboard(path: str) -> list[dict]:
    """Returns a list of shots:
        {n, title, start_sec, end_sec, duration_sec, kling_prompt, has_video}
    """
    text = Path(path).read_text(encoding="utf-8")
    shots = []
    for m in SHOT_RE.finditer(text):
        n, title, sm, ss, em, es = m.groups()
        start = int(sm) * 60 + int(ss)
        end = int(em) * 60 + int(es)
        dur = end - start
        # find the shot's code block (``` ... ```) that follows this heading
        rest = text[m.end():]
        next_heading = re.search(r"^##\s+", rest, re.MULTILINE)
        section = rest[: next_heading.start()] if next_heading else rest
        # Extract the first code block in the section
        cb_m = re.search(r"```([\s\S]+?)```", section)
        block = cb_m.group(1) if cb_m else section
        # The kling prompt is the lines after "可灵 prompt" inside the block
        kp = ""
        if "可灵 prompt" in block:
            # Strip everything up to and including "可灵 prompt" line
            after = re.split(r"可灵\s*prompt[^\n]*\n", block, maxsplit=1)
            if len(after) == 2:
                kp = after[1].strip()
        # has_video: false if the shot is marked 静态 / 不需要图生视频
        # (this marker is on the 可灵 prompt line itself, which we stripped)
        is_static = ("不需要图生视频" in block or
                     "(静态," in block or
                     "静态, 不需要" in block or
                     "剪映模板更稳" in block)
        has_video = bool(kp) and not is_static
        shots.append({
            "n": int(n),
            "title": title.strip(),
            "start_sec": start,
            "end_sec": end,
            "duration_sec": dur,
            "kling_prompt": kp,
            "has_video": has_video,
        })
    if not shots:
        fatal(f"no shots parsed from {path} — check the '## 镜头 N — ... [m1:s1-m2:s2]' format")
    return shots

# ---- SCRIPT parsing -------------------------------------------------------

# Each 旁白 block starts with [mm:ss-mm:ss]  followed by 画面/旁白 lines
SCRIPT_BLOCK_RE = re.compile(
    r"\[(\d+):(\d+)-(\d+):(\d+)\][\s\S]+?(?=[\[【]\d+:\d+|\Z)",
    re.MULTILINE,
)
VOICEOVER_RE = re.compile(r"旁白\s*[:：]\s*([^\n]+)")

def parse_script(path: str) -> list[dict]:
    """Returns a list of voiceover entries: {start_sec, end_sec, text}"""
    text = Path(path).read_text(encoding="utf-8")
    out = []
    # Each voiceover block: [mm:ss-mm:ss] ... 旁白: <text> (may span newlines,
    # ends at the next "旁白:" or next timestamp block)
    for m in SCRIPT_BLOCK_RE.finditer(text):
        sm, ss, em, es = m.groups()
        start = int(sm) * 60 + int(ss)
        end = int(em) * 60 + int(es)
        block = m.group(0)
        # Skip meta / reference blocks (the "## 关键情绪锚点" section uses
        # bold **旁白**: as a column header, not a real voiceover line).
        # Real voiceover lines are inside a [mm:ss-mm:ss] block in the
        # "## 抖音 60 秒脚本" section, before the "---" or "## 关键情绪锚点"
        # divider.
        # Quick filter: skip if the block has too many "旁白" mentions
        # (signals a meta block, not a real voiceover).
        if block.count("旁白") > 1:
            continue
        # Find the line that contains "旁白" and extract the text after the
        # first colon (the line may have parenthetical notes before the
        # colon, e.g. "旁白 (语速略放慢): ...").
        lines = block.split("\n")
        vo = ""
        for i, line in enumerate(lines):
            if re.search(r"旁白", line):
                parts = re.split(r"[：:]", line, maxsplit=1)
                if len(parts) == 2 and "旁白" in parts[0]:
                    vo_lines = [parts[1].strip()]
                    for next_line in lines[i+1:]:
                        nl = next_line.strip()
                        if not nl:
                            break
                        if any(prefix in nl for prefix in ["旁白", "画面", "字幕", "BGM", "[", "【", "end"]):
                            break
                        vo_lines.append(nl)
                    vo = " ".join(vo_lines).strip()
                    break
        # Strip trailing punctuation/whitespace and outer quotes
        vo = re.sub(r"[，,。.\s]+$", "", vo)
        vo = vo.strip("\"'「」『』")
        if vo:
            out.append({
                "start_sec": start,
                "end_sec": end,
                "text": vo,
            })
    return out

def find_voiceover_for_shot(shot: dict, voiceovers: list[dict]) -> dict | None:
    """Return the voiceover that overlaps the shot's start_sec, or None."""
    for vo in voiceovers:
        if vo["start_sec"] <= shot["start_sec"] + 0.5 and vo["end_sec"] >= shot["start_sec"] - 0.5:
            return vo
    return None

# ---- L2a: image generation -----------------------------------------------

def _resolve_reference(ref: str) -> str:
    """Resolve a reference_image argument to what Seedream wants.

    Accepts:
      - http(s)://...   → returned as-is (public URL)
      - data:image/...;base64,...  → returned as-is
      - local path to a .jpg/.png → base64-encoded inline data URI

    Inline base64 is the preferred form: it eliminates the 24h signed-URL
    TTL problem, works fully offline, and is reproducible (the jpg is
    committed in the plugin repo, so renders are deterministic).
    """
    if ref.startswith(("http://", "https://", "data:")):
        return ref
    p = Path(ref)
    if not p.exists():
        return None
    ext = p.suffix.lstrip(".").lower() or "jpeg"
    if ext == "jpg":
        ext = "jpeg"
    mime = f"image/{ext}"
    b64 = base64.b64encode(p.read_bytes()).decode("ascii")
    return f"data:{mime};base64,{b64}"


def gen_image(env: dict, prompt: str, model: str, out_path: str,
              reference_image: str = None) -> tuple[str, str]:
    """Returns (local_path, public_url_with_signature). The public URL has
    a 24h X-Tos-Expires signature from 火山 Ark and can be fed directly to
    Seedance as the i2v image input. Don't keep the URL for >24h.

    If reference_image is provided (URL, local path, or base64 data URI),
    Seedream preserves the panda's identity across renders. This is the
    panda IP's "character anchor" — the same panda looks the same across
    every episode.

    Recommended form: local path to a committed canonical jpg. The pipeline
    base64-encodes it inline so no upload, no signing, no expiry.
    """
    body = {
        "model": model,
        "prompt": prompt,
        "size": "1024x1792",
        "response_format": "url",
        "watermark": False,
    }
    resolved_ref = None
    if reference_image:
        resolved_ref = _resolve_reference(reference_image)
        if resolved_ref:
            body["reference_image"] = resolved_ref
    info(f"image: generating ({len(prompt)} chars prompt"
         f"{', ref=YES' if resolved_ref else ''})")
    code, err, body_b = curl("POST", f"{env['ARK_BASE_URL']}/images/generations",
                              {"Authorization": f"Bearer {env['ARK_API_KEY']}"},
                              body=body)
    if code != 200:
        fatal(f"image gen curl failed (HTTP {code}): {err}")
    d = json.loads(body_b)
    url = d["data"][0]["url"]
    info(f"image: downloading to {out_path}")
    code, err, _ = curl("GET", url, {}, out_file=out_path)
    if code != 200:
        fatal(f"image download failed (HTTP {code}): {err}")
    return out_path, url


def get_canonical_reference_path(env: dict) -> str | None:
    """Return the local path to the canonical panda image (preferred form:
    zh-red / 朱红汉服). Returns None if PLUGIN_DIR is unset or the
    reference dir doesn't ship with the plugin.

    The returned path is fed to gen_image which base64-encodes it inline.
    No upload, no signed URL, no 24h TTL.
    """
    ref_dir = Path(env["PLUGIN_DIR"]) / "reference" if env.get("PLUGIN_DIR") else None
    if not ref_dir or not ref_dir.exists():
        return None
    # Prefer 朱红汉服 (zh-red) as the default anchor; fall back to whatever
    # canonical image exists.
    preferred = ref_dir / "panda-canonical-zh-red.jpg"
    if preferred.exists():
        return str(preferred)
    for jpg in ref_dir.glob("panda-canonical-*.jpg"):
        return str(jpg)
    return None

# ---- L2b: image-to-video --------------------------------------------------

def gen_video(env: dict, image_url: str, action: str, duration_sec: int,
              model: str, out_path: str) -> str:
    """Returns the local path of the downloaded MP4."""
    body = {
        "model": model,
        "content": [
            {"type": "text",
             "text": f"{action}  --duration {duration_sec} --resolution 720p --camerafixed false --watermark false"},
            {"type": "image_url", "image_url": {"url": image_url}},
        ],
    }
    info(f"video: submitting task ({duration_sec}s)")
    code, err, body_b = curl("POST",
                              f"{env['ARK_BASE_URL']}/contents/generations/tasks",
                              {"Authorization": f"Bearer {env['ARK_API_KEY']}"},
                              body=body)
    if code != 200:
        fatal(f"video gen curl failed (HTTP {code}): {err}")
    task_id = json.loads(body_b)["id"]
    info(f"video: task_id = {task_id}, polling...")
    # Poll
    start = time.time()
    while time.time() - start < 600:
        time.sleep(5)
        code, err, body_b = curl("GET",
                                  f"{env['ARK_BASE_URL']}/contents/generations/tasks/{task_id}",
                                  {"Authorization": f"Bearer {env['ARK_API_KEY']}"})
        d = json.loads(body_b)
        status = d.get("status", "?")
        if status == "succeeded":
            url = d["content"]["video_url"]
            info(f"video: succeeded, downloading")
            curl("GET", url, {}, out_file=out_path)
            return out_path
        if status in ("failed", "cancelled"):
            fatal(f"video task {task_id} {status}: {d}")
        info(f"video: status={status}")
    fatal(f"video task {task_id} timed out")

# ---- L3a: TTS voiceover ---------------------------------------------------

def gen_tts(env: dict, text: str, out_path: str) -> str:
    body = {
        "model": DEFAULT_TTS_MODEL,
        "text": text,
        "voice_setting": {
            "voice_id": DEFAULT_TTS_VOICE,
            "speed": DEFAULT_TTS_SPEED,
            "vol": 1.0,
            "pitch": DEFAULT_TTS_PITCH,
        },
        "audio_setting": {
            "sample_rate": 24000,
            "bitrate": 128000,
            "format": "mp3",
        },
    }
    code, err, body_b = curl("POST", "https://api.minimaxi.com/v1/t2a_v2",
                              {"Authorization": f"Bearer {env['MINIMAX_API_KEY']}"},
                              body=body)
    if code != 200:
        fatal(f"TTS curl failed (HTTP {code}): {err}")
    d = json.loads(body_b)
    # CRITICAL: 海螺 returns audio as HEX, not base64
    audio_bytes = bytes.fromhex(d["data"]["audio"])
    Path(out_path).write_bytes(audio_bytes)
    return out_path

# ---- ffmpeg helpers -------------------------------------------------------

def ff_text_card(env: dict, text: str, dur: int, out_path: str,
                 fade_in: bool = False) -> str:
    """Generate a black 720x1280 video with the given text centered.

    Uses WenQuanYi Zen Hei (中文字体, wqy-zenhei.ttc) so Chinese
    characters render correctly. Without `fontfile=`, ffmpeg falls
    back to DejaVu Sans (no CJK glyphs) and Chinese shows as tofu
    boxes (□□□).
    """
    ff = env["FFMPEG"]
    fade = "fade=t=in:st=1:d=2," if fade_in else ""
    font = "fontfile=/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc"
    cmd = [ff, "-y", "-f", "lavfi", "-i", f"color=c=black:s=720x1280:d={dur}",
           "-vf", f"drawtext=text='{text}':{font}:fontsize=64:fontcolor=white:x=(w-text_w)/2:y=(h-text_h)/2,{fade}format=yuv420p",
           "-c:v", "libx264", "-preset", "fast", "-crf", "26",
           "-pix_fmt", "yuv420p", "-r", "24", out_path]
    r = subprocess.run(cmd, capture_output=True, text=True)
    if r.returncode != 0:
        fatal(f"ffmpeg text card failed: {r.stderr[-500:]}")
    return out_path

def ff_concat(env: dict, list_file: str, out_path: str) -> str:
    ff = env["FFMPEG"]
    cmd = [ff, "-y", "-f", "concat", "-safe", "0", "-i", list_file,
           "-c:v", "libx264", "-preset", "medium", "-crf", "23",
           "-pix_fmt", "yuv420p", "-r", "24", "-an", out_path]
    r = subprocess.run(cmd, capture_output=True, text=True)
    if r.returncode != 0:
        fatal(f"ffmpeg concat failed: {r.stderr[-500:]}")
    return out_path

def ff_stretch_last_frame(env: dict, in_path: str, target_dur: int, out_path: str) -> str:
    """Pad with cloned last frame if shorter than target_dur."""
    ff = env["FFMPEG"]
    extra = max(0, target_dur)  # will be added by tpad
    cmd = [ff, "-y", "-i", in_path,
           "-vf", f"tpad=stop_mode=clone:stop_duration={extra}",
           "-c:v", "libx264", "-preset", "fast", "-crf", "23",
           "-pix_fmt", "yuv420p", "-r", "24", "-t", str(target_dur),
           out_path]
    r = subprocess.run(cmd, capture_output=True, text=True)
    if r.returncode != 0:
        fatal(f"ffmpeg stretch failed: {r.stderr[-500:]}")
    return out_path

def ff_trim(env: dict, in_path: str, target_dur: int, out_path: str,
            text_overlay: str = None) -> str:
    """Trim to target_dur seconds, optionally with text overlay."""
    ff = env["FFMPEG"]
    vf = f"format=yuv420p"
    if text_overlay:
        # Same CJK font as the text cards (WenQuanYi Zen Hei)
        font = "fontfile=/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc"
        vf = f"drawtext=text='{text_overlay}':{font}:fontsize=36:fontcolor=white:x=(w-text_w)/2:y=h-th-100,{vf}"
    cmd = [ff, "-y", "-i", in_path,
           "-vf", vf,
           "-c:v", "libx264", "-preset", "fast", "-crf", "23",
           "-pix_fmt", "yuv420p", "-r", "24", "-t", str(target_dur),
           out_path]
    r = subprocess.run(cmd, capture_output=True, text=True)
    if r.returncode != 0:
        fatal(f"ffmpeg trim failed: {r.stderr[-500:]}")
    return out_path

def ff_synth_bgm(env: dict, out_path: str, dur_sec: int) -> str:
    """Synthesize contemplative BGM as a raw 44.1kHz stereo WAV.

    Recipe (cleaner, less noisy than v1):
      - 110 Hz sine drone (A2 pedal tone) at -30 dB
      - 60 BPM pentatonic arpeggio (A3 / C4 / D4 / E4 / G4) at -8 dB,
        with longer attack/release to avoid clicky transients
      - Pink-ish rain (filtered white noise, 1 kHz LPF) at -22 dB
        (was 0.12 amp → 0.04 amp to remove the 杂音)
      - 4 s fade in + 4 s fade out (was 3 s)
    """
    import wave
    import struct
    import random
    import math

    sr = 44100
    n_samples = sr * dur_sec
    rng = random.Random(42)  # deterministic noise

    # White noise → 1 kHz lowpass (pink-ish, much softer than brown noise)
    white = [rng.uniform(-1.0, 1.0) for _ in range(n_samples)]
    rain_lp = [0.0] * n_samples
    rc = 1.0 / (2 * math.pi * 1000)
    dt = 1.0 / sr
    alpha = dt / (rc + dt)
    prev = 0.0
    for i in range(n_samples):
        prev = prev + alpha * (white[i] - prev)
        rain_lp[i] = prev

    # Build per-sample audio: drone + arpeggio + rain (much quieter rain)
    notes = [(0.0, 220.00), (1.0, 261.63), (2.0, 329.63), (3.0, 392.00), (4.0, 440.00)]
    audio_l = [0.0] * n_samples
    audio_r = [0.0] * n_samples
    fade_dur = 4.0  # was 3, longer for smoothness
    for i in range(n_samples):
        t = i / sr
        # 110 Hz drone (sine, very quiet)
        drone = 0.025 * math.sin(2 * math.pi * 110 * t)
        # Arpeggio: 1 note per beat at 60 BPM, with attack + release envelope
        arp = 0.0
        for start, freq in notes:
            beat_t = t - start
            if 0 <= beat_t < 0.95:
                # ADSR: 50ms attack, 850ms release
                if beat_t < 0.05:
                    env = beat_t / 0.05  # attack
                else:
                    env = math.exp(-1.5 * (beat_t - 0.05))  # release
                arp = max(arp, 0.12 * env * math.sin(2 * math.pi * freq * beat_t))
        # Rain (much softer than v1: 0.04 vs 0.12)
        rain = 0.04 * rain_lp[i]
        sample = drone + arp + rain
        # Fade in / out
        if t < fade_dur:
            sample *= t / fade_dur
        if t > dur_sec - fade_dur:
            sample *= (dur_sec - t) / fade_dur
        # Soft clip
        sample = math.tanh(sample * 1.5) * 0.7
        audio_l[i] = sample
        audio_r[i] = math.tanh((sample + 0.02 * rain_lp[i]) * 1.5) * 0.7

    # Write 16-bit stereo WAV
    with wave.open(out_path, "wb") as wf:
        wf.setnchannels(2)
        wf.setsampwidth(2)
        wf.setframerate(sr)
        interleaved = b"".join(
            struct.pack("<hh", int(audio_l[i] * 32767), int(audio_r[i] * 32767))
            for i in range(n_samples)
        )
        wf.writeframes(interleaved)
    return out_path

def ff_phone_comments(env: dict, out_path: str, dur_sec: int) -> str:
    """Render a 抖音 phone-screen mockup with scrolling comments.

    Emoji (❤ 💬 😭 😢 💗) are not in the WQY Zen Hei font (and
    NotoColorEmoji/CBDT is not supported by the 2018 ffmpeg in this
    env), so we substitute WQY-supported symbols that read the same:
      ❤ → ♥    (U+2665 BLACK HEART SUIT, in WQY)
      💬 → ★   (U+2605 BLACK STAR, used as a "comment mark")
      😭 😢 💗 → ★ (same, for emoji-laden comments)
    Visually clean and stays on-抖音 phone-comment feel.
    """
    ff = env["FFMPEG"]
    font = "fontfile=/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc"
    texts = [
        f"drawtext=text='熊猫 OPC 凌晨 3 点':{font}:fontsize=36:fontcolor=white:x=60:y=80",
        f"drawtext=text='♥ 12.4w   ★ 3,892':{font}:fontsize=24:fontcolor=0xaaaaaa:x=60:y=140",
        f"drawtext=text='—— 评论区 ——':{font}:fontsize=28:fontcolor=0x666666:x=60:y=220",
        f"drawtext=text='小熊软糖_66: 加油! ★':{font}:fontsize=30:fontcolor=white:x=80:y=300",
        f"drawtext=text='深夜发疯人: 太治愈了... ♥':{font}:fontsize=30:fontcolor=white:x=80:y=400",
        f"drawtext=text='emo战士: emo了 emo了 ★':{font}:fontsize=30:fontcolor=white:x=80:y=500",
        f"drawtext=text='失眠专业户: 凌晨3点不只我一个':{font}:fontsize=28:fontcolor=white:x=80:y=600",
        f"drawtext=text='猫头鹰本鹰: 打卡! ★':{font}:fontsize=30:fontcolor=white:x=80:y=700",
        f"drawtext=text='—— 熊猫轻轻划过,不点赞 ——':{font}:fontsize=24:fontcolor=0x666666:x=60:y=1200",
    ]
    vf = ",".join([
        "drawbox=x=20:y=20:w=680:h=1240:color=0x1a1a1a@1:t=fill",
        *texts,
        "format=yuv420p",
    ])
    cmd = [ff, "-y", "-f", "lavfi", "-i", f"color=c=0x0d0d0d:s=720x1280:d={dur_sec}",
           "-vf", vf, "-c:v", "libx264", "-preset", "fast", "-crf", "26",
           "-pix_fmt", "yuv420p", "-r", "24", out_path]
    r = subprocess.run(cmd, capture_output=True, text=True)
    if r.returncode != 0:
        fatal(f"ffmpeg phone comments failed: {r.stderr[-500:]}")
    return out_path

def ff_mix_audio(env: dict, bgm_path: str, voiceover_paths: list[tuple[int, str]],
                 out_path: str, dur_sec: int) -> str:
    """Mix BGM (quieter) with voiceovers placed at given timestamps (ms)."""
    ff = env["FFMPEG"]
    inputs = ["-i", bgm_path]
    for _delay, p in voiceover_paths:
        inputs += ["-i", p]
    # Build adelay + amix chain
    n = len(voiceover_paths)
    parts = []
    for i, (delay, _p) in enumerate(voiceover_paths, start=1):
        parts.append(f"[{i}:a]adelay={delay}|{delay}[v{i}]")
    voice_labels = "".join(f"[v{i}]" for i in range(1, n + 1))
    parts.append(f"{voice_labels}amix=inputs={n}[voices]")
    parts.append(f"[0:a]volume=0.4[bgm]")
    parts.append("[bgm][voices]amix=inputs=2[mix]")
    fc = ";".join(parts)
    cmd = [ff, "-y"] + inputs + [
        "-filter_complex", fc,
        "-map", "[mix]",
        "-t", str(dur_sec),
        "-ar", "44100", "-ac", "2", "-c:a", "pcm_s16le",
        out_path,
    ]
    r = subprocess.run(cmd, capture_output=True, text=True)
    if r.returncode != 0:
        fatal(f"ffmpeg audio mix failed: {r.stderr[-500:]}")
    return out_path

def ff_mux_final(env: dict, video_path: str, audio_path: str, out_path: str) -> str:
    """Mux the final video with mixed audio, AAC encode, faststart."""
    ff = env["FFMPEG"]
    cmd = [ff, "-y",
           "-i", video_path, "-i", audio_path,
           "-c:v", "copy", "-c:a", "aac", "-b:a", "128k",
           "-shortest", "-movflags", "+faststart",
           out_path]
    r = subprocess.run(cmd, capture_output=True, text=True)
    if r.returncode != 0:
        fatal(f"ffmpeg mux failed: {r.stderr[-500:]}")
    return out_path

# ---- main pipeline --------------------------------------------------------

def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0] if __doc__ else "")
    ap.add_argument("--storyboard", required=True)
    ap.add_argument("--script", required=True)
    ap.add_argument("--voice", required=True, choices=["治愈", "御宅", "哲学", "国潮"])
    ap.add_argument("--platform", required=True, choices=["抖音", "哔哩哔哩", "小红书"])
    ap.add_argument("--target-dur", type=int, default=58)
    ap.add_argument("--out", required=True)
    ap.add_argument("--image-model", default=DEFAULT_IMAGE_MODEL)
    ap.add_argument("--video-model", default=DEFAULT_VIDEO_MODEL)
    ap.add_argument("--reference-image", default=None,
                    help="URL of a canonical panda image. Seedream will "
                         "preserve the panda's identity across shots.")
    args = ap.parse_args()

    env = require_env()
    info(f"voice={args.voice} platform={args.platform} target={args.target_dur}s")
    info(f"image-model={args.image_model} video-model={args.video_model}")

    render_dir = Path(env["RENDER_DIR"])
    work = render_dir / "work"
    img_dir = work / "01-image"
    vid_dir = work / "02-video"
    seg_dir = work / "03-segments"
    vo_dir = work / "05-voiceover"
    final_dir = work / "08-final"
    for d in [img_dir, vid_dir, seg_dir, vo_dir, final_dir]:
        d.mkdir(parents=True, exist_ok=True)

    # 1. Parse inputs
    info("parsing storyboard")
    shots = parse_storyboard(args.storyboard)
    info(f"  found {len(shots)} shots")
    info("parsing script")
    voiceovers = parse_script(args.script)
    info(f"  found {len(voiceovers)} voiceover lines")

    # 2. L2a: image for each video shot (parallel-friendly, but serial for simplicity)
    image_urls = {}  # shot_n -> (local_path, public_url)
    # Resolve reference image for character consistency
    ref = args.reference_image
    if not ref:
        ref = get_canonical_reference_path(env)
        if ref:
            info(f"using canonical reference (inline base64): {Path(ref).name}")
    for shot in shots:
        if not shot["has_video"]:
            continue
        n = shot["n"]
        img_path = img_dir / f"shot-{n:02d}.jpg"
        url_cache = img_dir / f"shot-{n:02d}.url"
        if img_path.exists() and img_path.stat().st_size > 50_000 and url_cache.exists():
            info(f"image {n}: cached at {img_path}")
            image_urls[n] = (str(img_path), url_cache.read_text().strip())
        else:
            # Augment the kling prompt with panda IP anchor
            # (no "panda_visual_anchor: fengge_v1" — Seedream renders that as text)
            prompt = (shot["kling_prompt"]
                      + ", 9:16 vertical composition")
            local, url = gen_image(env, prompt, args.image_model, str(img_path),
                                   reference_image=ref)
            url_cache.write_text(url)
            image_urls[n] = (local, url)

    # 3. L2b: i2v for each image
    video_paths = {}
    for shot in shots:
        if not shot["has_video"]:
            continue
        n = shot["n"]
        vid_path = vid_dir / f"shot-{n:02d}.mp4"
        if vid_path.exists() and vid_path.stat().st_size > 100_000:
            info(f"video {n}: cached at {vid_path}")
            video_paths[n] = str(vid_path)
            continue
        # The action prompt = first line of kling_prompt (the actual
        # kling/Vidu prompt the operator wrote — not the 视觉/构图 metadata)
        kp = shot["kling_prompt"]
        action = kp.split("\n")[0][:200]
        # Pass the public URL from the image gen step (24h signature)
        if n not in image_urls:
            fatal(f"no image URL for shot {n}")
        _local, public_url = image_urls[n]
        gen_video(env, public_url, action, shot["duration_sec"],
                  args.video_model, str(vid_path))
        video_paths[n] = str(vid_path)

    # 4. L2c: build segments
    info("assembling segments")
    segment_paths = []
    for shot in shots:
        n = shot["n"]
        seg_path = seg_dir / f"{n:02d}.mp4"
        if shot["has_video"]:
            # Stretch / trim to match storyboard duration
            ff_trim(env, video_paths[n], shot["duration_sec"], str(seg_path))
        else:
            # Static text card
            # Heuristic: extract the on-screen text from the title
            text = shot["title"]
            ff_text_card(env, text, shot["duration_sec"], str(seg_path),
                         fade_in=("共鸣" in text or "文字" in shot["title"]))
        segment_paths.append(str(seg_path))

    # Build concat list
    concat_list = render_dir / "concat.txt"
    concat_list.write_text("\n".join(f"file '{p}'" for p in segment_paths) + "\n")
    concat_video = str(final_dir / "video.mp4")
    ff_concat(env, str(concat_list), concat_video)

    # 5. L3a: TTS voiceover for each shot that has 旁白
    info("generating voiceovers")
    voiceover_delays = []
    for shot in shots:
        vo = find_voiceover_for_shot(shot, voiceovers)
        if not vo:
            continue
        n = shot["n"]
        mp3_path = vo_dir / f"vo-{n:02d}.mp3"
        if not mp3_path.exists():
            gen_tts(env, vo["text"], str(mp3_path))
        # Convert to WAV for cleaner mixing
        wav_path = vo_dir / f"vo-{n:02d}.wav"
        if not wav_path.exists():
            ff = env["FFMPEG"]
            subprocess.run([ff, "-y", "-i", str(mp3_path), "-ar", "44100",
                            "-ac", "2", "-acodec", "pcm_s16le", str(wav_path)],
                           check=True, capture_output=True)
        voiceover_delays.append((vo["start_sec"] * 1000, str(wav_path)))

    # 6. L3b: BGM
    info("synthesizing BGM")
    bgm_path = str(final_dir / "bgm.wav")
    ff_synth_bgm(env, bgm_path, args.target_dur)

    # 7. L3c: phone comments overlay (replace any 'phone' shot)
    # Find the shot whose title contains '评论'
    for shot in shots:
        if "评论" in shot["title"] and shot["has_video"]:
            n = shot["n"]
            phone_path = seg_dir / f"{n:02d}-phone.mp4"
            ff_phone_comments(env, str(phone_path), shot["duration_sec"])
            # Replace this segment in the concat list
            # (we re-build the list and re-concat)
            info("phone comments overlay: re-concat")
            for i, sp in enumerate(segment_paths):
                if sp.endswith(f"{n:02d}.mp4"):
                    segment_paths[i] = str(phone_path)
            concat_list.write_text("\n".join(f"file '{p}'" for p in segment_paths) + "\n")
            ff_concat(env, str(concat_list), concat_video)
            break

    # 8. L3d: final mix
    info("final mix: audio + video")
    mixed_audio = str(final_dir / "audio.wav")
    if voiceover_delays:
        ff_mix_audio(env, bgm_path, voiceover_delays, mixed_audio, args.target_dur)
    else:
        mixed_audio = bgm_path  # BGM only
    ff_mux_final(env, concat_video, mixed_audio, args.out)

    info(f"✅ DONE: {args.out}")


if __name__ == "__main__":
    main()

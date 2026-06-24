#!/usr/bin/env python3
"""panda-episode-pipeline — render a panda IP episode end-to-end.

⚠️  DEPRECATED (June 2026) — kept as the manual-escape path.

The `vivify` CLI is the new entry point:
    ./scripts/vivify episode render fengge EP005 \
      --storyboard characters/fengge/examples/panda-episode-005/STORYBOARD.md \
      --script     characters/fengge/examples/panda-episode-005/SCRIPT-douyin.md \
      --voice 治愈 --platform 抖音 --target-dur 58 --parallel 4 \
      --use-driver        # opt into the new in-process orchestrator path

Why deprecated:
  - Hardcoded provider calls (no fallback chain, no cost-cap gate)
  - No per-shot DB row (just episode-level cost_yuan estimate)
  - No retry classifier (raw curl on HTTP 500)
  - One provider per asset_type (no multi-provider fallback)

When to keep using this file:
  - You need the proven 6-episode pipeline behavior exactly
  - You're debugging and want to bypass the new `--use-driver` path
  - You're contributing a regression test for the legacy behavior

New work should land in `vivify/episode_driver.py` + `vivify/asset_orchestrator.py`.

---

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
    VIVIFY_RENDER_DIR default: /tmp/vivify-render
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
DEFAULT_RENDER_DIR = "/tmp/vivify-render"
DEFAULT_IMAGE_MODEL = "doubao-seedream-4-0-250828"
DEFAULT_VIDEO_MODEL = "doubao-seedance-2-0-260128"  # Seedance 2.0: audio-video sync, better motion
DEFAULT_TTS_MODEL = "speech-02-hd"
DEFAULT_TTS_VOICE = "male-qn-jingying"
DEFAULT_TTS_SPEED = 0.82
DEFAULT_TTS_PITCH = -1

# Per-tone TTS voice profile. Each tone gets a distinct audible register so
# the audio matches the visual register. See docs/panda-episode-pipeline/README.md.
# emotion values per MiniMax speech-02-hd: neutral | happy | sad | angry | fearful | disgust | surprised
# Voice profiles and identity anchor are loaded from character.yaml
# at startup (see main()). The hardcoded TONE_VOICE_PROFILE dict below
# is kept only as a fallback for legacy invocations without
# --character-dir.
TONE_VOICE_PROFILE = {}

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
        "RENDER_DIR":      os.environ.get("VIVIFY_RENDER_DIR", DEFAULT_RENDER_DIR),
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
            body_tmp = Path("/tmp") / f"vivify-render-body-{os.getpid()}-{id(body)}.json"
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
        # Strip everything from "---" onward (meta / footer sections
        # that come after the voiceover blocks). The footer mentions
        # "旁白" again in summary stats, which would otherwise pollute
        # the count filter below.
        footer_idx = block.find("\n---\n")
        if footer_idx >= 0:
            block = block[:footer_idx]
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
        vo = vo.strip("\"'「」『`````").strip()
        if vo:
            out.append({
                "start_sec": start,
                "end_sec": end,
                "text": vo,
            })
    return out

def find_voiceover_for_shot(shot: dict, voiceovers: list[dict]) -> dict | None:
    """Return the voiceover that best matches the shot.

    A shot's voiceover is the one whose start_sec is closest to the
    shot's start_sec. Earlier versions returned the first overlap,
    which gave wrong matches when multiple voiceovers overlapped (e.g.
    shot 3 at start=8 wrongly picked vo at start=3 instead of vo at
    start=8 because both overlapped shot 3's window).
    """
    if not voiceovers:
        return None
    # Closest start_sec within a 1s tolerance
    best = None
    best_delta = 999.0
    for vo in voiceovers:
        delta = abs(vo["start_sec"] - shot["start_sec"])
        if delta <= 1.0 and delta < best_delta:
            best = vo
            best_delta = delta
    return best

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
    """Return the local path to the canonical reference image.

    Priority:
      1. env["CHARACTER"]["canonical"]["primary"]  (set by main() from character.yaml)
      2. PLUGIN_DIR / reference / panda-canonical-*.jpg  (legacy fallback)

    The returned path is fed to gen_image which base64-encodes it inline.
    No upload, no signed URL, no 24h TTL.
    """
    char = env.get("CHARACTER")
    if char and "canonical" in char:
        primary = char["canonical"].get("primary")
        if primary and Path(primary).exists():
            return primary
    ref_dir = Path(env["PLUGIN_DIR"]) / "reference" if env.get("PLUGIN_DIR") else None
    if not ref_dir or not ref_dir.exists():
        return None
    for jpg in ref_dir.glob("*-canonical-*.jpg"):
        return str(jpg)
    return None

# ---- L2b: image-to-video --------------------------------------------------

def gen_video(env: dict, image_url: str, action: str, duration_sec: int,
              model: str, out_path: str, audio_url: str = None,
              character_ref: str = None) -> str:
    """Returns the local path of the downloaded MP4.

    Provider dispatch (per MODEL_CATALOG[model]['provider']):
      - "minimax" → shells out to `mmx video generate` (T2V/I2V/SEF/S2V modes)
      - "火山方舟" (ark) → curl POST to /contents/generations/tasks

    Seedance 2.0: if audio_url is provided AND the model supports it,
    the model will sync mouth movement / scene audio to the audio.
    Older models (1.5-pro, 1.0-pro) reject the audio_url field —
    we check MODEL_CATALOG before passing it.

    MiniMax (Hailuo) does NOT support audio_url — voice is generated
    separately via TTS and muxed in ffmpeg, not lip-synced in-video.

    `character_ref` (optional local path): the canonical panda reference
    image. Only used by MiniMax S2V (--subject-image). Ignored by ark.
    """
    from model_router import MODEL_CATALOG
    catalog = MODEL_CATALOG.get(model, {})
    provider = catalog.get("provider", "火山方舟")

    if provider == "minimax":
        return _gen_video_minimax(env, model, catalog, image_url, action,
                                   duration_sec, out_path, character_ref)

    # === Ark / Seedance path (unchanged) ===
    model_supports_audio = "audio_input" in catalog.get("features", [])

    content = [
        {"type": "text",
         "text": f"{action}  --duration {duration_sec} --resolution 720p --camerafixed false --watermark false"},
        {"type": "image_url", "image_url": {"url": image_url}},
    ]
    if audio_url and model_supports_audio:
        content.append({"type": "audio_url", "audio_url": {"url": audio_url}})
    body = {
        "model": model,
        "content": content,
    }
    info(f"video[ark]: submitting task ({duration_sec}s"
         f"{', audio=YES' if audio_url and model_supports_audio else ''}, model={model})")
    code, err, body_b = curl("POST",
                              f"{env['ARK_BASE_URL']}/contents/generations/tasks",
                              {"Authorization": f"Bearer {env['ARK_API_KEY']}"},
                              body=body)
    if code != 200:
        fatal(f"video gen curl failed (HTTP {code}): {err}")
    task_id = json.loads(body_b)["id"]
    info(f"video[ark]: task_id = {task_id}, polling...")
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
            info(f"video[ark]: succeeded, downloading")
            curl("GET", url, {}, out_file=out_path)
            return out_path
        if status in ("failed", "cancelled"):
            fatal(f"video[ark] task {task_id} {status}: {d}")
        info(f"video[ark]: status={status}")
    fatal(f"video[ark] task {task_id} timed out")


def _gen_video_minimax(env: dict, model: str, catalog: dict,
                       image_url: str, action: str, duration_sec: int,
                       out_path: str, character_ref: str = None) -> str:
    """Video gen via `mmx video generate`. Dispatches by mmx_mode:

      - i2v:  --first-frame (most common, image → video motion)
      - sef:  --first-frame + --last-frame (start-end interpolation)
      - s2v:  --subject-image (character-locked via canonical ref)
    """
    import subprocess
    mmx_mode = catalog.get("mmx_mode", "i2v")

    # image_url may be:
    #   - http(s)://...           (passed through)
    #   - data:image/...;base64,... (extract + write temp file, mmx wants a path)
    #   - local path              (passed through)
    from pathlib import Path
    first_frame_path = None
    if image_url.startswith(("http://", "https://")):
        # mmx CLI accepts URLs directly for --first-frame
        first_frame_path = image_url
    elif image_url.startswith("data:image"):
        # data URI — extract base64 and write to temp file
        import base64
        m = re.match(r"data:image/(\w+);base64,(.+)", image_url)
        if not m:
            fatal(f"video[minimax]: unparseable data URI for first frame")
        ext = m.group(1)
        b64 = m.group(2)
        tmp = Path("/tmp/vivify-render") / f"mmx-first-frame-{os.getpid()}.{ext}"
        tmp.parent.mkdir(parents=True, exist_ok=True)
        tmp.write_bytes(base64.b64decode(b64))
        first_frame_path = str(tmp)
    else:
        first_frame_path = image_url  # local path

    cmd = ["mmx", "video", "generate",
           "--model", model,
           "--prompt", action,
           "--download", out_path,
           "--timeout", "900"]  # mmx default 300s is too short for back-to-back calls

    if mmx_mode == "s2v" and character_ref:
        # S2V: character-locked video
        cmd += ["--subject-image", character_ref,
                "--first-frame", first_frame_path]
    elif mmx_mode in ("i2v", "sef"):
        cmd += ["--first-frame", first_frame_path]
        # SEF: last frame = first frame of next shot (best-effort default to first)
        if mmx_mode == "sef":
            cmd += ["--last-frame", first_frame_path]
    else:
        # Plain T2V — no first frame
        pass

    info(f"video[minimax/{mmx_mode}]: {model} → {Path(out_path).name}")
    # mmx downloads to out_path when --download is given (waits for completion)
    r = subprocess.run(cmd, capture_output=True, text=True, timeout=1200)
    if r.returncode != 0:
        fatal(f"video[minimax] mmx failed (rc={r.returncode}): {r.stderr or r.stdout}")
    if not Path(out_path).exists():
        fatal(f"video[minimax] mmx returned 0 but {out_path} not found")
    return out_path


def upload_to_tos(env: dict, local_path: str, ttl_sec: int = 86400) -> str:
    """Upload a local file to 火山 TOS and return a public URL.

    Used for voiceover audio files that Seedance 2.0 needs as
    audio_url input. The returned URL has TTL hours; we re-upload
    each render so URLs are always fresh.
    """
    import subprocess
    import time
    from pathlib import Path

    p = Path(local_path)
    if not p.exists():
        raise FileNotFoundError(local_path)

    # Use 火山 Ark's image API to upload and return URL (same as
    # gen_image's behavior). For audio files, we wrap in base64 inside
    # a data URI as a fallback if TOS direct upload isn't available.
    # Simpler: use the gen_image trick — submit through images API.
    # But audio isn't accepted there. So use base64 data URI directly.
    # Seedance 2.0 accepts data URIs.

    suffix = p.suffix.lstrip(".").lower() or "mp3"
    mime = f"audio/{'mpeg' if suffix == 'mp3' else suffix}"
    data = p.read_bytes()
    b64 = __import__('base64').b64encode(data).decode("ascii")
    return f"data:{mime};base64,{b64}"


# ---- L3a: TTS voiceover ---------------------------------------------------

def gen_tts(env: dict, text: str, out_path: str, tone: str = None) -> str:
    """Generate TTS audio. If tone is provided, use the per-tone voice
    profile (different voice_id / speed / pitch / emotion per tone
    matrix) so the audio matches the visual register.

    Profile source priority:
      1. character.yaml voice_profiles[tone]  (loaded at startup)
      2. legacy TONE_VOICE_PROFILE dict
      3. hardcoded defaults
    """
    char = env.get("CHARACTER")
    profile = None
    if char and tone and tone in char.get("voice_profiles", {}):
        profile = char["voice_profiles"][tone]
    if not profile:
        profile = TONE_VOICE_PROFILE.get(tone or "", {
            "voice_id": DEFAULT_TTS_VOICE,
            "speed": DEFAULT_TTS_SPEED,
            "pitch": DEFAULT_TTS_PITCH,
            "emotion": "neutral",
            "vol": 1.0,
        })
    body = {
        "model": DEFAULT_TTS_MODEL,
        "text": text,
        "voice_setting": {
            "voice_id": profile["voice_id"],
            "speed": profile["speed"],
            "vol": profile["vol"],
            "pitch": profile["pitch"],
            "emotion": profile["emotion"],
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


def ff_apply_subtitles(env: dict, in_path: str, out_path: str,
                       subtitle_specs: list[tuple[float, float, str]]) -> str:
    """Overlay CJK subtitles on a video using ffmpeg drawtext.

    Each spec is (start_sec, end_sec, text). The text is shown only
    between start and end. Multiple drawtext filters are chained.

    Uses WQY Zen Hei (the only CJK font available in the openclaw ffmpeg
    environment) — without it ffmpeg falls back to DejaVu Sans and
    Chinese shows as tofu boxes (□□□).
    """
    if not subtitle_specs:
        # No subtitles, just re-encode in place to keep format consistent
        ff = env["FFMPEG"]
        subprocess.run([ff, "-y", "-i", in_path, "-c:v", "libx264",
                        "-preset", "medium", "-crf", "23",
                        "-pix_fmt", "yuv420p", "-r", "24", "-an", out_path],
                       capture_output=True, text=True)
        return out_path

    ff = env["FFMPEG"]
    font = "fontfile=/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc"
    vf_parts = []
    for start, end, text in subtitle_specs:
        # Escape single quotes, colons, percent signs for ffmpeg drawtext
        safe = text.replace("\\", "\\\\").replace("'", "\\'").replace(":", "\\:").replace("%", "\\%")
        # Wrap to 2 lines by inserting a newline at the midpoint if long
        if len(safe) > 14:
            mid = len(safe) // 2
            # Find a natural break (space, comma)
            for j in range(mid - 2, mid + 4):
                if j < len(safe) and safe[j] in " ,。":
                    safe = safe[:j + 1] + "\\n" + safe[j + 1:]
                    break
            else:
                safe = safe[:mid] + "\\n" + safe[mid:]
        vf_parts.append(
            f"drawtext=text='{safe}':{font}:"
            f"fontsize=44:fontcolor=white:borderw=4:bordercolor=black:"
            f"x=(w-text_w)/2:y=h-th-180:"
            f"enable='between(t,{start:.2f},{end:.2f})'"
        )
    vf = ",".join(vf_parts)
    cmd = [ff, "-y", "-i", in_path, "-vf", vf,
           "-c:v", "libx264", "-preset", "medium", "-crf", "23",
           "-pix_fmt", "yuv420p", "-r", "24", "-an", out_path]
    r = subprocess.run(cmd, capture_output=True, text=True)
    if r.returncode != 0:
        fatal(f"subtitle overlay failed: {r.stderr[-500:]}")
    return out_path
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

def ff_synth_bgm(env: dict, out_path: str, dur_sec: int, tone: str = None) -> str:
    """Synthesize per-tone BGM as a 44.1kHz stereo WAV.

    v2 BGM: per-tone profiles mimic Chinese instrument textures using
    pure numpy synthesis. Each tone gets a distinct musical identity:

      - 治愈: guqin-like plucking, low register, sparse, long release
      - 御宅: single sustained drone + soft bell-like plucks
      - 哲学: deep drone + sparse high sparse notes
      - 国潮: dizi-like flute melody + plucked accompaniment

    Recipe per tone:
      - Drone: low sine (root + fifth), slight detuning for warmth
      - Melody: pentatonic scale (do re mi sol la), ADSR with long release
      - Texture: pink noise LPF (rain / breath), much softer than v1
    """
    import wave
    import numpy as np

    sr = 44100
    if tone is None:
        tone = env.get("TONE", "治愈")

    rng = np.random.default_rng(42)
    n = sr * dur_sec
    t = np.arange(n) / sr

    # Per-tone profile
    PROFILES = {
        "治愈": {
            "drone": [(110.0, 0.05), (164.81, 0.03)],   # A2 + E3 (perfect fifth)
            "bpm": 50,
            "pluck_pattern": [(0.0, 220.00), (1.2, 261.63), (2.4, 293.66), (3.6, 329.63),
                              (4.8, 392.00), (6.0, 329.63), (7.2, 293.66), (8.4, 261.63)],
            "pluck_amp": 0.10,
            "pluck_release_ms": 1800,
            "pluck_harmonics": [1.0, 0.4, 0.15, 0.05],   # Plucked string harmonics
            "texture_amp": 0.025,
            "texture_lpf_hz": 600,
            "bell_layer": False,
        },
        "御宅": {
            "drone": [(98.0, 0.06), (130.81, 0.025)],    # G2 + C3
            "bpm": 40,
            "pluck_pattern": [(0.0, 196.00), (3.0, 220.00), (6.0, 261.63),
                              (9.0, 196.00)],
            "pluck_amp": 0.07,
            "pluck_release_ms": 2500,
            "pluck_harmonics": [1.0, 0.3, 0.1],
            "texture_amp": 0.02,
            "texture_lpf_hz": 400,
            "bell_layer": True,
            "bell_freqs": [880.0, 1108.7, 1318.5],        # High harmonics for bell
        },
        "哲学": {
            "drone": [(73.42, 0.06), (110.0, 0.04)],     # D2 + A2 (low fifth)
            "bpm": 30,
            "pluck_pattern": [(0.0, 146.83), (4.0, 174.61), (8.0, 220.00),
                              (12.0, 174.61), (16.0, 146.83)],
            "pluck_amp": 0.05,
            "pluck_release_ms": 3500,    # very long release
            "pluck_harmonics": [1.0, 0.2, 0.05],
            "texture_amp": 0.015,
            "texture_lpf_hz": 300,
            "bell_layer": False,
        },
        "国潮": {
            "drone": [(146.83, 0.04), (220.00, 0.025)],  # D3 + A3
            "bpm": 70,
            "pluck_pattern": [(0.0, 293.66), (0.86, 329.63), (1.71, 392.00),
                              (2.57, 440.00), (3.43, 392.00), (4.29, 329.63),
                              (5.14, 293.66), (6.0, 329.63), (6.86, 261.63),
                              (7.71, 293.66), (8.57, 329.63), (9.43, 392.00)],
            "pluck_amp": 0.08,
            "pluck_release_ms": 1200,
            "pluck_harmonics": [1.0, 0.5, 0.2, 0.08, 0.03],   # flute-like richer harmonics
            "texture_amp": 0.02,
            "texture_lpf_hz": 800,
            "bell_layer": False,
        },
    }
    profile = PROFILES.get(tone, PROFILES["治愈"])

    # === Drone ===
    drone = np.zeros(n)
    for freq, amp in profile["drone"]:
        drone += amp * np.sin(2 * np.pi * freq * t)
        # Slight detuning chorus for warmth
        drone += amp * 0.15 * np.sin(2 * np.pi * freq * 1.003 * t)

    # === Plucked melody (pentatonic) with harmonic stack ===
    pluck = np.zeros(n)
    beat_dur = 60.0 / profile["bpm"]
    for cycle in range(int(dur_sec / beat_dur) + 2):
        for offset, freq in profile["pluck_pattern"]:
            t_start = cycle * beat_dur * len(profile["pluck_pattern"]) + offset
            t_end = t_start + profile["pluck_release_ms"] / 1000.0
            mask = (t >= t_start) & (t < t_end)
            if not mask.any():
                continue
            t_note = t[mask] - t_start
            # ADSR with long release
            attack = 0.015  # 15ms attack (pluck)
            release = profile["pluck_release_ms"] / 1000.0
            env = np.where(t_note < attack,
                           t_note / attack,
                           np.exp(-3.5 * (t_note - attack) / release))
            # Sum harmonics for plucked-string / flute-like timbre
            sample = np.zeros_like(t_note)
            for i, h_amp in enumerate(profile["pluck_harmonics"]):
                sample += h_amp * np.sin(2 * np.pi * freq * (i + 1) * t_note)
            pluck[mask] += profile["pluck_amp"] * env * sample

    # === Bell layer (御宅 only) ===
    bell = np.zeros(n)
    if profile.get("bell_layer"):
        # Random sparse high bell hits
        bell_times = rng.choice(n, size=int(dur_sec / 3), replace=False)
        for bt in bell_times:
            for f in profile["bell_freqs"]:
                t_bell_end = min(bt + sr * 2, n)
                t_note = np.arange(t_bell_end - bt) / sr
                env = np.exp(-1.5 * t_note)
                bell[bt:t_bell_end] += 0.02 * env * np.sin(2 * np.pi * f * t_note)

    # === Texture (filtered pink noise) ===
    white = rng.uniform(-1.0, 1.0, n)
    # Simple 1st-order LPF
    rc = 1.0 / (2 * np.pi * profile["texture_lpf_hz"])
    alpha = (1.0 / sr) / (rc + (1.0 / sr))
    texture = np.zeros(n)
    prev = 0.0
    for i in range(n):
        prev = prev + alpha * (white[i] - prev)
        texture[i] = prev
    texture *= profile["texture_amp"]

    # === Sum and fade ===
    audio_l = drone + pluck + bell + texture
    audio_r = drone * 0.95 + pluck * 0.95 + bell + texture  # slight stereo offset

    # 4s fade in / out
    fade = int(sr * 4)
    audio_l[:fade] *= np.linspace(0, 1, fade)
    audio_l[-fade:] *= np.linspace(1, 0, fade)
    audio_r[:fade] *= np.linspace(0, 1, fade)
    audio_r[-fade:] *= np.linspace(1, 0, fade)

    # Normalize to -6 dBFS
    peak = max(np.abs(audio_l).max(), np.abs(audio_r).max())
    if peak > 0:
        audio_l *= 0.5 / peak
        audio_r *= 0.5 / peak

    # Write WAV (16-bit PCM stereo)
    with wave.open(out_path, "wb") as w:
        w.setnchannels(2)
        w.setsampwidth(2)
        w.setframerate(sr)
        # interleave L/R as int16
        interleaved = np.empty(n * 2, dtype=np.int16)
        interleaved[0::2] = (audio_l * 32767).astype(np.int16)
        interleaved[1::2] = (audio_r * 32767).astype(np.int16)
        w.writeframes(interleaved.tobytes())
    info(f"BGM: {tone} profile, {dur_sec}s, 16-bit stereo")
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
                    help="URL of a canonical reference image. Seedream will "
                         "preserve the character's identity across shots.")
    ap.add_argument("--character-dir", default=None,
                    help="Path to a character directory (containing character.yaml "
                         "and canonical/ reference images). Defaults to characters/fengge/ "
                         "relative to the script, or $PLUGIN_DIR/characters/fengge.")
    ap.add_argument("--title", default=None,
                    help="Episode title shown on the opening card. "
                         "Defaults to the slug.")
    ap.add_argument("--next-episode", default="下集预告",
                    help="End-card text shown at the closing (e.g. '下集:冬至')")
    ap.add_argument("--qa-skip", action="store_true",
                    help="Skip QA gate even if shots fail checks")
    ap.add_argument("--quality-tier", default="standard",
                    choices=["draft", "standard", "premium"],
                    help="Video model tier: draft (cheap/fast), standard (default), "
                         "premium (lip sync + best quality). Overrides --video-model.")
    ap.add_argument("--require-lip-sync", action="store_true",
                    help="Force lip sync (only available with premium tier)")
    ap.add_argument("--video-provider", default="auto",
                    choices=["auto", "ark", "minimax"],
                    help="Which provider to use for video gen: "
                         "auto (router decides), ark (Seedance), minimax (Hailuo). "
                         "When set, model_router only considers that provider's models.")
    ap.add_argument("--parallel", "-j", type=int, default=1,
                    help="Number of shots to render in parallel using a thread pool "
                         "(default 1 = serial). Backed by vivify.retry for "
                         "smart transient/permanent error classification.")
    ap.add_argument("--max-retries", type=int, default=3,
                    help="Max retries per shot on transient failures "
                         "(default 3). Permanent failures are not retried.")
    ap.add_argument("--retry-only", action="store_true",
                    help="Only re-run shots that failed in a previous render. "
                         "Used by `vivify workflow retry` → `vivify episode render --retry-only`.")
    args = ap.parse_args()
    if not args.title:
        slug = Path(args.storyboard).stem.replace("STORYBOARD", "").strip("-_") or "ep"
        args.title = slug

    env = require_env()

    # Load character profile (canonical images, voice profiles, style anchor)
    char_dir = args.character_dir
    if not char_dir:
        # Auto-detect: characters/fengge/ relative to script dir
        script_dir = Path(__file__).parent.resolve()
        default_char = script_dir / "characters" / "fengge"
        if default_char.exists():
            char_dir = str(default_char)
        elif env.get("PLUGIN_DIR"):
            default_char = Path(env["PLUGIN_DIR"]) / "characters" / "fengge"
            if default_char.exists():
                char_dir = str(default_char)
    if char_dir:
        try:
            from character_loader import load_character
            env["CHARACTER"] = load_character(char_dir)
            char = env["CHARACTER"]
            info(f"character: {char['name']} ({char.get('english_name', '')}) "
                 f"from {char_dir}")
            info(f"  outfits: {len(char['outfits'])}  scenes: {len(char['scenes'])}  "
                 f"tones: {', '.join(char['voice_profiles'].keys())}")
            if not args.title or args.title == "ep":
                args.title = f"{char['name']}短剧"
        except SystemExit as e:
            warn(f"character load failed: {e}")
            env["CHARACTER"] = None
    else:
        env["CHARACTER"] = None
        warn("no --character-dir given; using defaults (panda-style hardcoded)")

    info(f"voice={args.voice} platform={args.platform} target={args.target_dur}s")
    info(f"image-model={args.image_model}")

    # Model router: smart selection with auto-fallback
    from model_router import ModelRouter, parse_tier
    router = ModelRouter(env)
    tier = parse_tier(args.quality_tier)
    requirements = {
        "needs_lip_sync": args.require_lip_sync or (tier == "premium"),
    }
    resolved_model, reason = router.route_video_model(
        tier=tier,
        requirements=requirements,
        explicit_model=args.video_model if args.video_model != DEFAULT_VIDEO_MODEL else None,
        provider=args.video_provider if args.video_provider != "auto" else None,
    )
    info(f"video router: tier={tier} provider={args.video_provider} → {resolved_model} ({reason})")
    args.video_model = resolved_model

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
            # Augment the kling prompt with character anchor + locked visual style.
            # (no "panda_visual_anchor: fengge_v1" — Seedream renders that as text)
            # Style anchor is loaded from character.yaml (or fallback default).
            style_anchor = env.get("CHARACTER", {}).get("style_anchor", "")
            prompt = (shot["kling_prompt"]
                      + ", 9:16 vertical composition"
                      + (", " + style_anchor if style_anchor else ""))
            local, url = gen_image(env, prompt, args.image_model, str(img_path),
                                   reference_image=ref)
            url_cache.write_text(url)
            image_urls[n] = (local, url)

    # 2.5. L2-TTS: pre-generate voiceover audio (Seedance 2.0 uses these
    # as audio_url inputs for native lip sync). We store the public URLs
    # in voiceover_urls so L2b can pass them along.
    info("generating voiceovers for lip sync (Seedance 2.0)")
    voiceover_urls = {}
    for shot in shots:
        vo = find_voiceover_for_shot(shot, voiceovers)
        if not vo:
            continue
        n = shot["n"]
        mp3_path = vo_dir / f"vo-{n:02d}.mp3"
        if not mp3_path.exists():
            gen_tts(env, vo["text"], str(mp3_path), tone=args.voice)
        # Upload the audio to TOS so Seedance can fetch it
        try:
            audio_public_url = upload_to_tos(env, str(mp3_path))
            voiceover_urls[n] = audio_public_url
        except Exception as e:
            warn(f"audio upload failed for shot {n}: {e} — lip sync disabled for this shot")

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
        # Seedance 2.0 lip-sync: pass voiceover audio URL if available
        audio_url = voiceover_urls.get(n)
        # Character ref (only for MiniMax S2V — passed via character_ref arg)
        char_ref = None
        from model_router import MODEL_CATALOG as _MC
        if _MC.get(args.video_model, {}).get("provider") == "minimax":
            char_ref = ref  # the canonical reference image path
        # Try primary model, fall back via router if it fails
        current_model = args.video_model
        video_ok = False
        for attempt in range(3):  # max 3 fallbacks
            try:
                gen_video(env, public_url, action, shot["duration_sec"],
                          current_model, str(vid_path), audio_url=audio_url,
                          character_ref=char_ref)
                video_ok = True
                if current_model != args.video_model:
                    info(f"video {n}: using fallback {current_model} (after {args.video_model} failed)")
                router.record_call(current_model, shot["duration_sec"], success=True)
                break
            except Exception as e:
                warn(f"video {n}: {current_model} failed — {e}")
                router.record_call(current_model, shot["duration_sec"], success=False)
                next_model, fallback_reason = router.fallback_from(current_model)
                if next_model == current_model:
                    fatal(f"video {n}: no fallback available after {current_model} failed")
                warn(f"video {n}: {fallback_reason}")
                current_model = next_model
        if not video_ok:
            fatal(f"video {n}: all fallback attempts exhausted")
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
            gen_tts(env, vo["text"], str(mp3_path), tone=args.voice)
        # Convert to WAV for cleaner mixing
        wav_path = vo_dir / f"vo-{n:02d}.wav"
        if not wav_path.exists():
            ff = env["FFMPEG"]
            subprocess.run([ff, "-y", "-i", str(mp3_path), "-ar", "44100",
                            "-ac", "2", "-acodec", "pcm_s16le", str(wav_path)],
                           check=True, capture_output=True)
        voiceover_delays.append((vo["start_sec"] * 1000, str(wav_path)))

    # 6. L3b: BGM (per-tone)
    info(f"synthesizing BGM (tone={args.voice})")
    bgm_path = str(final_dir / "bgm.wav")
    env["TONE"] = args.voice
    ff_synth_bgm(env, bgm_path, args.target_dur, tone=args.voice)

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

    # 9. L3e: content craft layer — title card + subtitle overlay + end card
    # This is what turns a "render" into a "usable 抖音 short video".
    title_dur = 1.5
    end_dur = 1.5
    title_text = f"{args.title}\n\n峰哥 · 短剧"
    end_text = f"{args.next_episode}\n\n关注峰哥 看下一集"

    title_seg = seg_dir / "00-title.mp4"
    ff_text_card(env, title_text, title_dur, str(title_seg))
    end_seg = seg_dir / "99-end.mp4"
    ff_text_card(env, end_text, end_dur, str(end_seg))

    # Re-concat with title + end wrapping the shots
    full_segments = [str(title_seg)] + segment_paths + [str(end_seg)]
    full_concat_list = render_dir / "concat-full.txt"
    full_concat_list.write_text("\n".join(f"file '{p}'" for p in full_segments) + "\n")
    full_concat_video = str(final_dir / "video-full.mp4")
    ff_concat(env, str(full_concat_list), full_concat_video)

    # Apply subtitle overlay synced to each voiceover (offset by title card)
    subtitle_specs = []
    for shot in shots:
        vo = find_voiceover_for_shot(shot, voiceovers)
        if not vo:
            continue
        sub_start = title_dur + shot["start_sec"]
        sub_end = title_dur + shot["end_sec"]
        subtitle_specs.append((sub_start, sub_end, vo["text"]))
    sub_video = str(final_dir / "video-sub.mp4")
    info(f"applying {len(subtitle_specs)} subtitle overlays")
    ff_apply_subtitles(env, full_concat_video, sub_video, subtitle_specs)

    # 9.5. QA gate — heuristic checks on each generated shot image
    info("QA gate: checking generated shots")
    try:
        from qa_gate import check_image
        qa_passed = 0
        qa_failed = 0
        for shot in shots:
            n = shot["n"]
            shot_img = img_dir / f"shot-{n:02d}.jpg"
            if shot_img.exists():
                ok, msg = check_image(str(shot_img))
                if ok:
                    qa_passed += 1
                else:
                    qa_failed += 1
                    warn(f"QA: {msg}")
        info(f"QA: {qa_passed} passed, {qa_failed} failed")
        if qa_failed > 0 and not args.qa_skip:
            warn(f"{qa_failed} shot(s) failed QA — review before publishing")
    except ImportError:
        warn("qa_gate module not available, skipping")

    # 10. L3f: mux final video + audio
    out_arg = Path(args.out)
    if out_arg.suffix.lower() != ".mp4":
        out_arg.mkdir(parents=True, exist_ok=True)
        slug = Path(args.storyboard).stem.replace("STORYBOARD", "").strip("-_") or "ep"
        out_path = str(out_arg / f"{slug}-L3.mp4")
    else:
        out_path = str(out_arg)
    ff_mux_final(env, sub_video, mixed_audio, out_path)

    # Final model usage report
    router.print_report()

    info(f"✅ DONE: {out_path}")


if __name__ == "__main__":
    main()

"""vivify.providers.minimax — 海螺 (Hailuo / MiniMax) adapter via `mmx` CLI.

Shells out to `mmx video generate` for I2V / SEF / S2V modes. The mmx CLI
handles polling + download itself; this adapter just builds the right
argv and waits for exit.

The mmx binary is expected on PATH. If it's missing, subprocess raises
FileNotFoundError and the adapter returns ok=False with a clear error.

Modes (driven by MODEL_CATALOG[model]['mmx_mode']):
  - i2v:  --first-frame (image → video, most common)
  - sef:  --first-frame + --last-frame (start-end frame interpolation)
  - s2v:  --subject-image (character-locked via canonical ref)

S2V is the ID-drift fix: feed a canonical face shot to --subject-image
and Hailuo locks onto that character across the video.

Auth: requires `MINIMAX_API_KEY` in the `env` dict.
"""

from __future__ import annotations

import base64
import os
import re
import subprocess
import time
from pathlib import Path
from typing import Optional

from .base import AssetType, GenerateRequest, GenerateResult, ProviderAdapter


# Defaults — match render_episode.py
DEFAULT_VIDEO_MODEL = "MiniMax-Hailuo-2.3"
DEFAULT_MMX_TIMEOUT_SEC = 1200  # mmx default 300s is too short for back-to-back calls


class MiniMaxProvider(ProviderAdapter):
    """海螺 (Hailuo) provider — video generation via `mmx` CLI."""

    name = "minimax"
    asset_type: AssetType = "video"

    def __init__(self, env: dict, *, mmx_timeout_sec: int = DEFAULT_MMX_TIMEOUT_SEC):
        self.env = env
        self._mmx_timeout_sec = mmx_timeout_sec
        self._supported = {"video"}

    # ---- public API ------------------------------------------------------

    def supports(self, asset_type: AssetType) -> bool:
        return asset_type in self._supported

    def generate(self, req: GenerateRequest, *, out_path: Optional[Path] = None,
                 model: Optional[str] = None,
                 character_ref: Optional[str] = None) -> GenerateResult:
        if req.asset_type != "video":
            return GenerateResult(ok=False, provider=self.name,
                                   error=f"minimax only supports video, got {req.asset_type!r}")
        if out_path is None:
            return GenerateResult(ok=False, provider=self.name,
                                   error="out_path required for video generation")
        return self.generate_video(req, out_path=out_path, model=model,
                                     character_ref=character_ref)

    def cost_estimate(self, req: GenerateRequest) -> float:
        if req.asset_type != "video" or not req.duration_sec:
            return 0.0
        model = req.options.get("model") or DEFAULT_VIDEO_MODEL
        return _video_cost_yuan(model, req.duration_sec)

    # ---- video -----------------------------------------------------------

    def generate_video(self, req: GenerateRequest, *,
                       out_path: Path,
                       model: Optional[str] = None,
                       character_ref: Optional[str] = None) -> GenerateResult:
        model = model or req.options.get("model") or DEFAULT_VIDEO_MODEL
        if not req.reference_image:
            return GenerateResult(ok=False, provider=self.name, model=model,
                                   error="minimax video gen requires reference_image (first frame)")

        mmx_mode = _model_mmx_mode(model)
        first_frame = self._resolve_first_frame(req.reference_image)
        char_ref = character_ref or req.options.get("character_ref")

        cmd = ["mmx", "video", "generate",
               "--model", model,
               "--prompt", req.prompt,
               "--download", str(out_path),
               "--timeout", "900"]

        if mmx_mode == "s2v" and char_ref:
            cmd += ["--subject-image", char_ref, "--first-frame", first_frame]
        elif mmx_mode in ("i2v", "sef"):
            cmd += ["--first-frame", first_frame]
            if mmx_mode == "sef":
                # SEF: last frame defaults to first frame (best-effort)
                cmd += ["--last-frame", first_frame]
        # t2v (no first frame) — fall through with bare cmd

        t0 = time.time()
        try:
            r = subprocess.run(cmd, capture_output=True, text=True,
                                 timeout=self._mmx_timeout_sec)
        except subprocess.TimeoutExpired:
            return GenerateResult(ok=False, provider=self.name, model=model,
                                   error=f"mmx timed out after {self._mmx_timeout_sec}s")
        except FileNotFoundError:
            return GenerateResult(ok=False, provider=self.name, model=model,
                                   error="`mmx` CLI not found on PATH — install with: pip install minimax-mmx")

        if r.returncode != 0:
            err = r.stderr or r.stdout
            return GenerateResult(ok=False, provider=self.name, model=model,
                                   error=f"mmx rc={r.returncode}: {err[:300]}")

        if not out_path.exists():
            return GenerateResult(ok=False, provider=self.name, model=model,
                                   error=f"mmx returned 0 but {out_path} not found")

        duration = req.duration_sec or 6
        cost = _video_cost_yuan(model, duration)
        return GenerateResult(
            ok=True, local_path=out_path,
            cost_yuan=cost,
            duration_ms=int((time.time() - t0) * 1000),
            provider=self.name, model=model,
        )

    # ---- internals -------------------------------------------------------

    @staticmethod
    def _resolve_first_frame(image_url: str) -> str:
        """Translate an image ref into a path/URL mmx accepts.

        - http(s)://... → passed through (mmx downloads)
        - data:image/...;base64,... → decoded to temp file
        - local path → passed through
        """
        if image_url.startswith(("http://", "https://")):
            return image_url
        if image_url.startswith("data:image"):
            m = re.match(r"data:image/(\w+);base64,(.+)", image_url)
            if not m:
                raise ValueError(f"unparseable data URI for first frame: {image_url[:50]}…")
            ext = m.group(1)
            b64 = m.group(2)
            tmp_dir = Path(os.environ.get("VIVIFY_RENDER_DIR", "/tmp/vivify-render"))
            tmp = tmp_dir / f"mmx-first-frame-{os.getpid()}.{ext}"
            tmp.parent.mkdir(parents=True, exist_ok=True)
            tmp.write_bytes(base64.b64decode(b64))
            return str(tmp)
        # Assume local path
        return image_url


# ---- helpers ---------------------------------------------------------------

def _model_mmx_mode(model: str) -> str:
    """Look up mmx_mode in model_router.MODEL_CATALOG. Default: 'i2v'."""
    try:
        from model_router import MODEL_CATALOG
        return MODEL_CATALOG.get(model, {}).get("mmx_mode", "i2v")
    except Exception:
        return "i2v"


def _video_cost_yuan(model: str, duration_sec: int) -> float:
    """Look up cost per second in model_router.MODEL_CATALOG."""
    try:
        from model_router import MODEL_CATALOG
        rate = MODEL_CATALOG.get(model, {}).get("cost_per_sec_yuan", 0.0)
    except Exception:
        rate = 0.0
    return rate * duration_sec


__all__ = ["MiniMaxProvider", "DEFAULT_VIDEO_MODEL"]

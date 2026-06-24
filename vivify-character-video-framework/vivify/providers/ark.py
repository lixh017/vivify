"""vivify.providers.ark — 火山方舟 (Volcengine Ark) adapter.

Handles two asset types:
  - image: synchronous POST /images/generations → download URL
  - video: asynchronous POST /contents/generations/tasks → poll → download

The curl helper, _resolve_reference, and polling loop are ported from
the working `render_episode.py` (lines 109-466) and adapted to the
ProviderAdapter interface. The original `render_episode.py` is left
untouched so the existing `vivify episode render` pipeline keeps
working — this module is a separate, lower-level entry point that
the L2 orchestrator skill can drive.

Auth: requires `ARK_API_KEY` and `ARK_BASE_URL` in the `env` dict.
Defaults match `render_episode.DEFAULT_ARK_BASE`.

Cost model:
  - image: flat ¥0.20 per call (matches vivify.pricing.COST_IMAGE_YUAN)
  - video: ¥ per second, looked up in MODEL_CATALOG from model_router.py
"""

from __future__ import annotations

import base64
import json
import os
import subprocess
import time
from pathlib import Path
from typing import Optional

from .base import AssetType, GenerateRequest, GenerateResult, ProviderAdapter


# Defaults — keep in sync with render_episode.py
DEFAULT_ARK_BASE = "https://ark.cn-beijing.volces.com/api/v3"
DEFAULT_IMAGE_MODEL = "doubao-seedream-4-0-250828"
DEFAULT_VIDEO_MODEL = "doubao-seedance-1-5-pro-251215"
DEFAULT_IMAGE_SIZE = "1024x1792"
DEFAULT_IMAGE_COST_YUAN = 0.20  # matches vivify.pricing.COST_IMAGE_YUAN

# Polling — 5s interval, 10 min hard cap
DEFAULT_POLL_INTERVAL_SEC = 5
DEFAULT_POLL_TIMEOUT_SEC = 600
CURL_TIMEOUT_SEC = 300


# ---- HTTP helper ----------------------------------------------------------

def curl(method: str, url: str, headers: dict, body: dict = None,
         out_file: str = None, timeout: int = CURL_TIMEOUT_SEC) -> tuple[int, str, bytes]:
    """Returns (http_code, stderr, response_body). http_code is the actual
    HTTP status (200, 401, 404, ...), not curl's exit code.

    Identical contract to render_episode.curl(); kept duplicated here
    so this module is self-contained and import-side-effect-free."""
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
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
    finally:
        if body_tmp and body_tmp.exists():
            body_tmp.unlink()
    if r.returncode != 0:
        return 0, r.stderr or "curl failed", b""

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
        out = out[:idx].rstrip("\n")
    if out_file:
        with open(out_file, "rb") as f:
            return http_code, r.stderr or "", f.read()
    return http_code, r.stderr or "", out.encode() if isinstance(out, str) else out


def _resolve_reference(ref: str) -> Optional[str]:
    """Resolve a reference_image argument to what Seedream wants.

    Accepts:
      - http(s)://...   → returned as-is (public URL)
      - data:image/...;base64,...  → returned as-is
      - local path to a .jpg/.png → base64-encoded inline data URI

    Inline base64 eliminates the 24h signed-URL TTL problem.
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


# ---- adapter ---------------------------------------------------------------

class ArkProvider(ProviderAdapter):
    """火山方舟 (Ark) provider — image + video generation."""

    name = "ark"
    asset_type: AssetType = "image"  # default; per-call methods may differ

    def __init__(self, env: dict, *,
                 poll_interval_sec: int = DEFAULT_POLL_INTERVAL_SEC,
                 poll_timeout_sec: int = DEFAULT_POLL_TIMEOUT_SEC,
                 image_model: str = DEFAULT_IMAGE_MODEL):
        self.env = env
        self._poll_interval_sec = poll_interval_sec
        self._poll_timeout_sec = poll_timeout_sec
        self._image_model = image_model
        # Ark image-gen is one provider method, video another
        # (no per-asset-type subclasses — adapter handles both)
        self._supported = {"image", "video"}

    # ---- public API ------------------------------------------------------

    def supports(self, asset_type: AssetType) -> bool:
        return asset_type in self._supported

    def generate(self, req: GenerateRequest, *, out_path: Optional[Path] = None,
                 model: Optional[str] = None) -> GenerateResult:
        if req.asset_type == "image":
            if out_path is None:
                return GenerateResult(ok=False, provider=self.name,
                                       error="out_path required for image generation")
            return self.generate_image(req, out_path=out_path)
        if req.asset_type == "video":
            if out_path is None:
                return GenerateResult(ok=False, provider=self.name,
                                       error="out_path required for video generation")
            return self.generate_video(req, out_path=out_path, model=model)
        return GenerateResult(ok=False, provider=self.name,
                              error=f"ark does not support asset_type={req.asset_type!r}")

    def cost_estimate(self, req: GenerateRequest) -> float:
        if req.asset_type == "image":
            return DEFAULT_IMAGE_COST_YUAN
        if req.asset_type == "video":
            if not req.duration_sec:
                return 0.0
            return _video_cost_yuan(req.options.get("model") or DEFAULT_VIDEO_MODEL,
                                     req.duration_sec)
        return 0.0

    # ---- image -----------------------------------------------------------

    def generate_image(self, req: GenerateRequest, *,
                       out_path: Path,
                       model: Optional[str] = None) -> GenerateResult:
        """Image gen: POST → URL → download."""
        model = model or self._image_model
        size = req.size or DEFAULT_IMAGE_SIZE
        body = {
            "model": model,
            "prompt": req.prompt,
            "size": size,
            "response_format": "url",
            "watermark": False,
        }
        resolved_ref = None
        if req.reference_image:
            resolved_ref = _resolve_reference(req.reference_image)
            if resolved_ref:
                body["reference_image"] = resolved_ref

        t0 = time.time()
        code, err, body_b = curl(
            "POST", f"{self.env['ARK_BASE_URL']}/images/generations",
            {"Authorization": f"Bearer {self.env['ARK_API_KEY']}"},
            body=body,
        )
        if code != 200:
            return GenerateResult(ok=False, provider=self.name, model=model,
                                   error=f"HTTP {code}: {err or body_b[:200]!r}")

        try:
            d = json.loads(body_b)
            url = d["data"][0]["url"]
        except (json.JSONDecodeError, KeyError, IndexError) as e:
            return GenerateResult(ok=False, provider=self.name, model=model,
                                   error=f"unparseable response: {e}")

        # Download to out_path
        out_path.parent.mkdir(parents=True, exist_ok=True)
        dl_code, dl_err, _ = curl("GET", url, {}, out_file=str(out_path))
        if dl_code != 200:
            return GenerateResult(ok=False, provider=self.name, model=model,
                                   error=f"download HTTP {dl_code}: {dl_err}")

        return GenerateResult(
            ok=True,
            local_path=out_path,
            public_url=url,
            cost_yuan=DEFAULT_IMAGE_COST_YUAN,
            duration_ms=int((time.time() - t0) * 1000),
            provider=self.name,
            model=model,
        )

    # ---- video -----------------------------------------------------------

    def generate_video(self, req: GenerateRequest, *,
                       out_path: Path,
                       model: Optional[str] = None,
                       ratio: str = "9:16",
                       resolution: str = "720p",
                       audio_url: Optional[str] = None) -> GenerateResult:
        """Video gen: POST task → poll → download."""
        model = model or req.options.get("model") or DEFAULT_VIDEO_MODEL
        duration = req.duration_sec or 5

        # Build content array
        content = [
            {"type": "text",
             "text": (f"{req.prompt}  --duration {duration} --resolution {resolution} "
                      f"--ratio {ratio} --camerafixed false --watermark false")},
        ]
        if req.reference_image:
            resolved = _resolve_reference(req.reference_image)
            if resolved:
                content.append({"type": "image_url", "image_url": {"url": resolved}})

        # Only add audio_url if model supports audio_input
        if audio_url and _model_supports_audio(model):
            content.append({"type": "audio_url", "audio_url": {"url": audio_url}})

        body = {"model": model, "content": content}
        t0 = time.time()
        code, err, body_b = curl(
            "POST", f"{self.env['ARK_BASE_URL']}/contents/generations/tasks",
            {"Authorization": f"Bearer {self.env['ARK_API_KEY']}"},
            body=body,
        )
        if code != 200:
            return GenerateResult(ok=False, provider=self.name, model=model,
                                   error=f"task create HTTP {code}: {err or body_b[:200]!r}")

        try:
            task_id = json.loads(body_b)["id"]
        except (json.JSONDecodeError, KeyError) as e:
            return GenerateResult(ok=False, provider=self.name, model=model,
                                   error=f"unparseable task create response: {e}")

        # Poll
        url, status, poll_err = self._poll_task(task_id)
        if status != "succeeded":
            return GenerateResult(
                ok=False, asset_id=task_id, provider=self.name, model=model,
                error=f"task {status}: {poll_err}" if poll_err else f"task {status}",
            )

        # Download (24h TTL URL — do this NOW)
        out_path.parent.mkdir(parents=True, exist_ok=True)
        dl_code, dl_err, _ = curl("GET", url, {}, out_file=str(out_path))
        if dl_code != 200:
            return GenerateResult(ok=False, asset_id=task_id, provider=self.name, model=model,
                                   error=f"download HTTP {dl_code}: {dl_err}")

        cost = _video_cost_yuan(model, duration)
        return GenerateResult(
            ok=True, asset_id=task_id, local_path=out_path, public_url=url,
            cost_yuan=cost,
            duration_ms=int((time.time() - t0) * 1000),
            provider=self.name, model=model,
        )

    # ---- internals -------------------------------------------------------

    def _poll_task(self, task_id: str) -> tuple[Optional[str], str, Optional[str]]:
        """Poll a task until succeeded/failed/cancelled/timeout.

        Returns (video_url, status, error_message). On success, video_url is set.
        """
        deadline = time.time() + self._poll_timeout_sec
        last_status = "unknown"
        while time.time() < deadline:
            time.sleep(self._poll_interval_sec)
            code, err, body_b = curl(
                "GET", f"{self.env['ARK_BASE_URL']}/contents/generations/tasks/{task_id}",
                {"Authorization": f"Bearer {self.env['ARK_API_KEY']}"},
            )
            if code != 200:
                # Transient — keep polling until deadline
                last_status = f"poll_http_{code}"
                continue
            try:
                d = json.loads(body_b)
            except json.JSONDecodeError:
                last_status = "poll_unparseable"
                continue
            status = d.get("status", "?")
            last_status = status
            if status == "succeeded":
                return d.get("content", {}).get("video_url"), status, None
            if status in ("failed", "cancelled"):
                return None, status, json.dumps(d)[:500]
        return None, "timeout", f"last status: {last_status}"


# ---- helpers ---------------------------------------------------------------

def _model_supports_audio(model: str) -> bool:
    """Check if a model advertises the 'audio_input' feature."""
    try:
        from model_router import MODEL_CATALOG
        return "audio_input" in MODEL_CATALOG.get(model, {}).get("features", [])
    except Exception:
        return False


def _video_cost_yuan(model: str, duration_sec: int) -> float:
    """Look up cost per second in model_router.MODEL_CATALOG."""
    try:
        from model_router import MODEL_CATALOG
        rate = MODEL_CATALOG.get(model, {}).get("cost_per_sec_yuan", 0.0)
    except Exception:
        rate = 0.0
    return rate * duration_sec


__all__ = [
    "ArkProvider",
    "DEFAULT_ARK_BASE",
    "DEFAULT_IMAGE_MODEL",
    "DEFAULT_VIDEO_MODEL",
    "DEFAULT_IMAGE_COST_YUAN",
    "curl",
]

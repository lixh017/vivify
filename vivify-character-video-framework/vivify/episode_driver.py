"""vivify.episode_driver — per-shot asset pipeline.

Replaces the render_episode.py subprocess call in commands/episode.py.

For each shot in a parsed storyboard, this driver:
  1. Calls generate_asset(asset_type="image") to render the seed frame
  2. Calls generate_asset(asset_type="video") with the image as
     reference_image and canonical_ref for s2v character-lock
  3. Calls _call_tts to render voiceover (uses MiniMaxProvider.generate_tts)
  4. Updates the shots row in the SQLite DB with real paths + real cost

Returns a list of ShotAssetResult, one per shot, with the real cost. The
caller (commands/episode.py) sums these to get the episode's real
cost_yuan (vs. the old estimate-only path).

Note: this module does NOT call ffmpeg. The muxer is a separate step
(see render_episode.ff_* helpers, imported in commands/episode.py).
"""
from __future__ import annotations

import sqlite3
from dataclasses import dataclass
from pathlib import Path
from typing import Optional

from .asset_orchestrator import generate_asset
from .providers.base import GenerateRequest, GenerateResult


# --- result dataclass -------------------------------------------------------

@dataclass
class ShotAssetResult:
    shot_number: int
    image_path: Optional[Path]
    video_path: Optional[Path]
    tts_path: Optional[Path]
    image_cost_yuan: float
    video_cost_yuan: float
    tts_cost_yuan: float

    @property
    def total_cost_yuan(self) -> float:
        return (
            (self.image_cost_yuan or 0.0)
            + (self.video_cost_yuan or 0.0)
            + (self.tts_cost_yuan or 0.0)
        )


# --- TTS adapter shim -------------------------------------------------------

def _call_tts(req: GenerateRequest, env: dict, voice_profile: dict,
              out_path: Path) -> GenerateResult:
    """TTS adapter shim — separate from generate_asset because TTS is not
    a video model with router-managed fallback.

    Implementation:
      1. Try the new MiniMaxProvider adapter (Task 2: not yet shipped).
      2. Fall back to the legacy render_episode.gen_tts (validated by
         6 production panda episodes) by injecting voice_profile into
         env['CHARACTER']['voice_profiles']['<tone-key>'] and passing
         the tone through.

    Returns GenerateResult so the driver can read cost/ok/local_path.
    """
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)

    # Path 1: new adapter (if Task 2 has shipped and generate_tts exists)
    try:
        from .providers.minimax import MiniMaxProvider
        p = MiniMaxProvider(env=env)
        if hasattr(p, "generate_tts"):
            return p.generate_tts(req, out_path=out_path,
                                   voice_profile=voice_profile)
    except (ImportError, Exception):
        pass

    # Path 2: legacy render_episode.gen_tts (the fallback that ships today)
    try:
        import sys as _sys
        from pathlib import Path as _P
        fw_dir = str(_P(__file__).resolve().parent.parent)
        if fw_dir not in _sys.path:
            _sys.path.insert(0, fw_dir)
        from render_episode import gen_tts  # type: ignore
    except Exception as e:
        return GenerateResult(
            ok=False, provider="tts", model="speech-02-hd",
            error=f"no TTS adapter available (MiniMaxProvider.generate_tts "
                  f"missing and render_episode.gen_tts not importable: {e})",
        )

    # Build a tone-key that gen_tts will recognise. We don't have a tone
    # string in the request, so derive a stable key from voice_profile
    # fields. gen_tts looks up env['CHARACTER']['voice_profiles'][tone]
    # first; we set it to a single key matching the voice_id.
    tone_key = voice_profile.get("voice_id") or "_default"
    shim_env = dict(env)  # shallow copy
    shim_env["CHARACTER"] = {
        "voice_profiles": {tone_key: voice_profile},
    }
    try:
        gen_tts(shim_env, req.prompt, str(out_path), tone=tone_key)
    except SystemExit:
        # gen_tts calls fatal() which sys.exit(1) on HTTP errors
        return GenerateResult(
            ok=False, provider="minimax", model="speech-02-hd",
            error="TTS call failed (gen_tts exited non-zero)",
        )
    except Exception as e:
        return GenerateResult(
            ok=False, provider="minimax", model="speech-02-hd",
            error=f"gen_tts raised: {type(e).__name__}: {e}",
        )
    return GenerateResult(
        ok=True, provider="minimax", model="speech-02-hd",
        local_path=out_path, cost_yuan=0.0,  # legacy doesn't track cost
    )


# --- character.yaml resolvers ----------------------------------------------

_DEFAULT_VOICE_PROFILE = {
    "voice_id": "male-qn-jingying",
    "speed": 0.85,
    "pitch": 0,
    "emotion": "neutral",
    "vol": 1.0,
}


def _resolve_canonical_ref(char_dir: Path,
                            character_yaml_path: Optional[str]) -> Optional[str]:
    """If a character.yaml exists with character.canonical.primary, return
    the resolved absolute path. Otherwise return None.

    Used as the character_ref for s2v (ID-drift fix on 海螺 video).
    """
    if not character_yaml_path:
        return None
    yaml_path = Path(character_yaml_path)
    if not yaml_path.exists():
        return None
    try:
        import yaml
        data = yaml.safe_load(yaml_path.read_text(encoding="utf-8")) or {}
    except Exception:
        return None
    char = (data.get("character") or {})
    primary = ((char.get("canonical") or {}).get("primary"))
    if not primary:
        return None
    resolved = (char_dir / primary).resolve()
    if not resolved.exists():
        return None
    return str(resolved)


def _resolve_voice_profile(character_yaml_path: Optional[str],
                            tone: Optional[str]) -> dict:
    """Resolve character.yaml's voice_profiles[tone] to a profile dict.
    Falls back to _DEFAULT_VOICE_PROFILE if yaml is missing or tone
    is not defined.
    """
    if not character_yaml_path or not Path(character_yaml_path).exists():
        return dict(_DEFAULT_VOICE_PROFILE)
    try:
        import yaml
        data = yaml.safe_load(Path(character_yaml_path).read_text(encoding="utf-8")) or {}
    except Exception:
        return dict(_DEFAULT_VOICE_PROFILE)
    char = (data.get("character") or {})
    profiles = (char.get("voice_profiles") or {})
    if tone and tone in profiles:
        return dict(profiles[tone])
    return dict(_DEFAULT_VOICE_PROFILE)


# --- main entry point -------------------------------------------------------

def render_episode_assets(*,
                          shots: list[dict],
                          voiceovers: list[dict],
                          episode_id: str,
                          character_id: str,
                          env: dict,
                          out_dir: Path,
                          db_path: str,
                          voice_profile: dict,
                          canonical_ref: Optional[str] = None,
                          episode_pk: Optional[int] = None,
                          config_path: Optional[Path] = None,
                          monthly_hard: float = 60_000.0,
                          video_provider_filter: str = "ark",
                          image_provider_filter: str = "ark",
                          ) -> list[ShotAssetResult]:
    """Run the per-shot asset pipeline. Returns one ShotAssetResult per shot.

    Args:
      shots:        parsed storyboard (from render_episode.parse_storyboard)
      voiceovers:   parsed script (from render_episode.parse_script)
      episode_id:   e.g. "EP001" — used in ledger scene_id
      character_id: e.g. "fengge" — used in ledger scene_id
      env:          vendor credentials (ARK_API_KEY, MINIMAX_API_KEY, etc.)
      out_dir:      where to write per-shot image/video/tts files
      db_path:      SQLite path for monthly-cap lookup
      voice_profile: resolved voice_profiles[tone] dict from character.yaml
      canonical_ref: path to canonical face/3-quarter reference (for s2v)
      episode_pk:   if provided, update shot rows by id
      config_path:  router YAML override
      monthly_hard: monthly cost cap (default ¥60k)
      video_provider_filter: provider for video gen (default 'ark')
      image_provider_filter: provider for image gen (default 'ark')

    Each shot writes 2 ledger rows (one image, one video) and at most
    1 TTS call. TTS does NOT go through the orchestrator — it calls the
    MiniMax adapter directly, so no TTS ledger row is auto-emitted.
    """
    out_dir = Path(out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    results: list[ShotAssetResult] = []

    # Map voiceovers by shot number for fast lookup
    vo_by_n = {v.get("n", i + 1): v for i, v in enumerate(voiceovers)}

    for shot in shots:
        n = shot["n"]
        scene_id = f"{character_id}-{episode_id}-shot{n}"
        dur = int(shot.get("duration_sec") or 5)
        prompt = shot.get("kling_prompt") or shot.get("prompt") or ""
        shot_work = out_dir / f"shot-{n:02d}"
        shot_work.mkdir(parents=True, exist_ok=True)

        # 1. Image
        img_out = shot_work / f"shot-{n:02d}.jpg"
        img_req = GenerateRequest(
            scene_id=scene_id, asset_type="image",
            prompt=prompt, size="1024x1024",
            options={"out_path": str(img_out),
                     "model": "doubao-seedream-4-0-250828"},
        )
        img_result = generate_asset(
            img_req, env=env, config_path=config_path,
            provider_filter=image_provider_filter,
            db_path=db_path, monthly_hard=monthly_hard,
        )

        # 2. Video (image → video)
        vid_out = shot_work / f"shot-{n:02d}.mp4"
        vid_options = {
            "out_path": str(vid_out),
            "model": "doubao-seedance-2-0-260128",
        }
        if canonical_ref:
            vid_options["character_ref"] = canonical_ref
        # If image failed, fall back to text-only video (no reference_image)
        ref_img = str(img_out) if img_result.ok else None
        vid_req = GenerateRequest(
            scene_id=scene_id, asset_type="video",
            prompt=prompt, reference_image=ref_img,
            duration_sec=dur, options=vid_options,
        )
        vid_result = generate_asset(
            vid_req, env=env, config_path=config_path,
            provider_filter=video_provider_filter,
            db_path=db_path, monthly_hard=monthly_hard,
        )

        # 3. TTS (direct adapter — no orchestrator, no auto ledger)
        tts_path = None
        tts_cost = 0.0
        vo = vo_by_n.get(n)
        if vo and vo.get("text"):
            tts_out = shot_work / f"voiceover-{n:02d}.mp3"
            tts_req = GenerateRequest(
                scene_id=scene_id, asset_type="tts",
                prompt=vo["text"],
            )
            tts_result = _call_tts(tts_req, env, voice_profile, tts_out)
            if tts_result.ok:
                tts_path = tts_result.local_path
                tts_cost = tts_result.cost_yuan

        result = ShotAssetResult(
            shot_number=n,
            image_path=img_result.local_path if img_result.ok else None,
            video_path=vid_result.local_path if vid_result.ok else None,
            tts_path=tts_path,
            image_cost_yuan=img_result.cost_yuan if img_result.ok else 0.0,
            video_cost_yuan=vid_result.cost_yuan if vid_result.ok else 0.0,
            tts_cost_yuan=tts_cost,
        )
        results.append(result)

        # 4. Update shot row with real paths + real cost
        if episode_pk is not None:
            _update_shot_row(
                str(db_path), episode_pk=episode_pk, shot_number=n,
                image_path=str(result.image_path) if result.image_path else None,
                video_path=str(result.video_path) if result.video_path else None,
                cost_yuan=(result.image_cost_yuan + result.video_cost_yuan),
            )

    return results


def _update_shot_row(db_path: str, *, episode_pk: int, shot_number: int,
                     image_path: Optional[str], video_path: Optional[str],
                     cost_yuan: float) -> None:
    """Update a single shot row with the real results from generate_asset.

    Uses a single SQL UPDATE keyed by (episode_id, shot_number) — this is
    the natural unique key, not the auto-increment id, so callers can pass
    a shot list and we can update without first querying for ids.
    """
    sets, vals = [], []
    if image_path is not None:
        sets.append("image_path = ?"); vals.append(image_path)
    if video_path is not None:
        sets.append("video_path = ?"); vals.append(video_path)
    if cost_yuan is not None:
        sets.append("cost_yuan = ?"); vals.append(cost_yuan)
    if not sets:
        return
    vals += [episode_pk, shot_number]
    with sqlite3.connect(db_path) as conn:
        conn.execute(
            f"UPDATE shots SET {', '.join(sets)} "
            f"WHERE episode_id = ? AND shot_number = ?",
            vals,
        )
        conn.commit()


__all__ = [
    "ShotAssetResult",
    "render_episode_assets",
    "_call_tts",
    "_resolve_canonical_ref",
    "_resolve_voice_profile",
]

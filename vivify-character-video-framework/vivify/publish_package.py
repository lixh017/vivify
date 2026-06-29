"""vivify.publish_package — generate a copy-paste-ready upload package.

We don't have platform-publish APIs (user chose "人工发布" — the human
uploads manually after we render). But the AI already knows the story,
the character, and the tone — so it can do 90% of the prep work:

  - Title (3 variants: emotional / comedic / mysterious tone)
  - Description (50-100 chars summarizing the episode)
  - Hashtag set (platform-tuned: 抖音 entertainment, 小红书 lifestyle,
    B站 otaku)
  - Cover timestamp candidates (based on emotional peaks in storyboard)
  - Best posting time (optional, based on character + platform)

Output: a markdown file the human can copy-paste into the platform's
upload UI.

This module is pure logic. NO Click dependency. The Click wrapper
lives in `vivify.commands.episode.prepare_publish_cmd`.

Workflow:

  vivify render EP001
  vivify episode prepare-publish fengge EP001 --platform 抖音 --voice 治愈
  # → writes characters/fengge/examples/panda-episode-001/PUBLISH_PACKAGE_抖音.md
  # → human opens file, copies each section, pastes into 抖音 backend
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any


# ---------------------------------------------------------------------------
# Public dataclasses
# ---------------------------------------------------------------------------

@dataclass
class TitleVariants:
    """Three tone variants of the episode title.

    The human picks ONE for the platform upload. The other two are kept
    as backups in case the first one feels off after seeing the cover.
    """
    emotional: str
    comedic: str
    mysterious: str


@dataclass
class PostingWindow:
    """Suggested posting time window for this platform.

    `start_hour` / `end_hour` are 24h local times. `rationale` explains
    why this window suits this character + platform (one sentence).
    """
    start_hour: int
    end_hour: int
    rationale: str


@dataclass
class PublishPackage:
    """The full generated package for one (episode, platform) pair."""
    titles: TitleVariants
    description: str
    hashtags: list[str]                # always begins with "#"
    cover_candidates: list[float]      # timestamps in seconds
    posting_window: PostingWindow | None
    platform: str
    character_id: str
    episode_id: str
    voice: str | None = None


# ---------------------------------------------------------------------------
# Platform tone guide
# ---------------------------------------------------------------------------

# The tone guide encodes platform-specific limits + style hints. These
# are derived from observed best-practice on each platform (2024-2025):
#   抖音: short punchy titles, ~5 hashtags
#   小红书: detailed lifestyle descriptions, ~10 tags
#   B站: long titles, niche anime/otaku tags
_PLATFORM_TONE: dict[str, dict[str, Any]] = {
    "抖音": {
        "hashtag_count": 5,
        "hashtag_style": "concise-entertainment",
        "description_max": 80,
        "title_max": 30,
        "default_hashtags": ["#熊猫", "#治愈", "#国潮", "#短剧", "#峰哥"],
    },
    "小红书": {
        "hashtag_count": 10,
        "hashtag_style": "lifestyle-detailed",
        "description_max": 200,
        "title_max": 20,
        "default_hashtags": [
            "#熊猫", "#治愈", "#国潮生活", "#传统文化", "#毛绒毯",
            "#深夜食堂", "#禅意生活", "#慢生活", "#东方美学", "#IP角色",
        ],
    },
    "B站": {
        "hashtag_count": 8,
        "hashtag_style": "anime-niche",
        "description_max": 500,
        "title_max": 80,
        "default_hashtags": [
            "#熊猫", "#治愈系", "#国创动画", "#短片动画", "#拟人化",
            "#峰哥", "#治愈向", "#手绘动画",
        ],
    },
}


def _platform_tone(platform: str) -> dict[str, Any]:
    """Return the tone guide for `platform`. Raises KeyError on unknown."""
    if platform not in _PLATFORM_TONE:
        raise KeyError(
            f"unknown platform '{platform}' "
            f"(expected one of: {', '.join(_PLATFORM_TONE)})"
        )
    return _PLATFORM_TONE[platform]


# ---------------------------------------------------------------------------
# Emotion peak extraction
# ---------------------------------------------------------------------------

# Keywords that signal an emotional high-point in the storyboard. We
# weight them and pick the shots with the highest score.
_EMOTION_KEYWORDS: dict[str, float] = {
    # strong peaks
    "满月": 3.0, "大雪": 3.0, "告别": 3.0, "重逢": 3.0,
    "泪": 2.5, "哭": 2.5, "震撼": 2.5, "惊艳": 2.5,
    "高潮": 2.5, "转折": 2.5, "最高": 2.5,
    # medium peaks
    "月光": 2.0, "远山": 2.0, "围炉": 2.0, "冬至": 2.0,
    "立春": 2.0, "立秋": 2.0, "夏日": 2.0, "深秋": 2.0,
    "古寺": 2.0, "夜话": 2.0, "窗棂": 2.0, "雪": 2.0,
    "月": 1.5, "灯": 1.5, "火": 1.5, "雪": 1.5,
    "笑": 1.5, "拥抱": 2.0, "独白": 1.5,
    # mild peaks
    "茶": 1.0, "书": 1.0, "风": 1.0, "雨": 1.0,
    "竹": 1.0, "山": 1.0, "水": 1.0, "花": 1.0,
}

# Words that signal a CLIMAX (the platform's "must-cover" shot).
_CLIMAX_WORDS = ["满月", "大雪", "高潮", "高潮", "重逢", "告别",
                 "转折", "最高", "震撼", "惊艳", "泪", "哭"]


def _extract_emotion_peaks(storyboard: list[dict]) -> list[float]:
    """Return timestamps (in seconds) of emotional high-points.

    Each shot is scored by counting emotion keywords in its title +
    kling_prompt (Chinese). The score = sum(weight * matches).
    Returns up to 3 highest-scoring timestamps, ordered by time.
    """
    if not storyboard:
        return []

    scored: list[tuple[float, float]] = []
    for shot in storyboard:
        title = shot.get("title") or ""
        prompt = shot.get("kling_prompt") or ""
        text = f"{title} {prompt}"
        score = 0.0
        for kw, weight in _EMOTION_KEYWORDS.items():
            if kw in text:
                score += weight
        # Climax shot: bonus if the title contains a climax word
        for kw in _CLIMAX_WORDS:
            if kw in title:
                score += 1.0
                break
        # Also: any shot at the midpoint or near end is inherently peak-worthy
        scored.append((float(shot.get("start_sec", 0)), score))

    # Pick top-3 with non-zero score, then sort by time. A shot with
    # score=0 has no emotion words — not a peak. (This matters for the
    # `no peaks → fall back` branch in _pick_cover_candidates.)
    nonzero = [(t, s) for t, s in scored if s > 0]
    if not nonzero:
        return []
    top = sorted(nonzero, key=lambda x: -x[1])[:3]
    return sorted(t for t, _ in top)


# ---------------------------------------------------------------------------
# Character keyword extraction
# ---------------------------------------------------------------------------

def _emotion_keyword(personality_tags: list[str], voice: str | None) -> str:
    """Pick the emotion keyword that best fits the character's voice.

    Maps the 4 voice tones to a Chinese emotion word for the title.
    Falls back to the first personality tag if voice is missing.
    """
    voice_emotion = {
        "治愈": "治愈",
        "御宅": "独处",
        "哲学": "哲思",
        "国潮": "国潮",
    }
    if voice and voice in voice_emotion:
        return voice_emotion[voice]
    if personality_tags:
        return str(personality_tags[0]).strip()
    return "日常"


def _intriguing_action(voice: str | None, personality_tags: list[str]) -> str:
    """Pick an intriguing verb phrase for the mysterious title.

    Voice-driven defaults; falls back to personality tag.
    """
    by_voice = {
        "治愈": "温一壶茶",
        "御宅": "窝在毯子里",
        "哲学": "望着月亮",
        "国潮": "翻开了那卷旧画",
    }
    if voice and voice in by_voice:
        return by_voice[voice]
    if personality_tags:
        tag = str(personality_tags[0])
        return f"想{tag}的事"
    return "在窗边发呆"


# ---------------------------------------------------------------------------
# Title generation (pure templates, no LLM call)
# ---------------------------------------------------------------------------

def _title_emotional(char_name: str, emotion_kw: str, episode_id: str) -> str:
    """Emotional-tone title: '{character_name}{emotion_keyword}第{N}集'"""
    # Try to pull the episode number from episode_id ("EP005" → "5").
    ep_str = str(episode_id or "")
    m = re.search(r"(\d+)", ep_str)
    n = str(int(m.group(1))) if m else (ep_str or "新")
    return f"{char_name}{emotion_kw}第{n}集"


def _title_comedic(first_prompt: str, char_name: str, episode_id: str) -> str:
    """Comedic title: '{short_setup} → {short_punchline}'.

    Setup = first 8 chars of first shot's prompt (parentheticals
    stripped, so we don't get '一只成年熊猫(峰...' for fengge).
    Punchline = a contrastive suffix based on the episode id.
    """
    # Strip parentheticals + collapse spaces before slicing
    cleaned = re.sub(r"\([^)]*\)", " ", first_prompt or "")
    cleaned = re.sub(r"[\s,，。:;；]+", " ", cleaned).strip()
    setup = cleaned[:8].strip(" ,，。")
    if not setup:
        setup = "今天的故事"
    # Punchline: derive from episode id; just reuse the char name + episode
    ep_str = str(episode_id or "")
    m = re.search(r"(\d+)", ep_str)
    n = str(int(m.group(1))) if m else "?"
    punchline = f"{char_name}的第{n}集反转"
    return f"{setup} → {punchline}"


def _title_mysterious(intriguing: str, char_name: str) -> str:
    """Mysterious title: '凌晨3点,{character_name}{intriguing_action}'"""
    return f"凌晨3点,{char_name}{intriguing}"


def _generate_titles(character: dict, episode_data: dict,
                     storyboard: list[dict], voice: str | None) -> TitleVariants:
    """Generate 3 title variants for one episode."""
    char_name = character.get("name") or character.get("id") or "角色"
    # Prefer `episode_id` (e.g. "EP005"); fall back to `id` (int PK)
    episode_id = (episode_data or {}).get("episode_id") or \
                 str((episode_data or {}).get("id") or "EP?")
    personality_tags = (
        character.get("personality_tags")
        or character.get("personality")
        or []
    )
    if isinstance(personality_tags, str):
        personality_tags = [personality_tags]

    emotion_kw = _emotion_keyword(personality_tags, voice)
    intriguing = _intriguing_action(voice, personality_tags)
    first_prompt = storyboard[0]["kling_prompt"] if storyboard else ""

    return TitleVariants(
        emotional=_title_emotional(char_name, emotion_kw, episode_id),
        comedic=_title_comedic(first_prompt, char_name, episode_id),
        mysterious=_title_mysterious(intriguing, char_name),
    )


# ---------------------------------------------------------------------------
# Description generation
# ---------------------------------------------------------------------------

def _generate_description(storyboard: list[dict], episode_data: dict,
                          character: dict, platform: str) -> str:
    """Build a 2-sentence description from the first shot's prompt +
    episode duration. Clamped to platform's `description_max`.

    The first sentence paints the visual opening (from shot 1's prompt,
    truncated to ~40 chars). The second sentence anchors with the
    duration + platform tone.
    """
    first_prompt = ""
    if storyboard:
        first_prompt = (storyboard[0].get("kling_prompt") or "").strip()

    # Compress the prompt for human readability:
    # 1. Strip parenthetical content (e.g. "(峰哥)" → "")
    # 2. Strip rgb(...) technical tokens (just noise for humans)
    # 3. Strip the bare "rgb" word leftover
    # 4. Collapse whitespace + punctuation to single spaces
    # 5. Take the first ~40 chars at a word boundary
    cleaned = re.sub(r"\([^)]*\)", " ", first_prompt)
    cleaned = re.sub(r"rgb\(\s*\d+\s*,\s*\d+\s*,\s*\d+\s*\)", " ", cleaned)
    cleaned = re.sub(r"\brgb\b", " ", cleaned)
    cleaned = re.sub(r"[\s,，。:;；]+", " ", cleaned).strip()
    opening = cleaned[:40].strip()
    if not opening:
        opening = f"{(character.get('name') or '主角')}的日常片段"

    duration_sec = int((episode_data or {}).get("actual_dur_sec")
                       or (episode_data or {}).get("target_dur_sec")
                       or 0)
    duration_phrase = f"{duration_sec}秒" if duration_sec else "短片"

    platform_tail = {
        "抖音": f"{duration_phrase}治愈片段",
        "小红书": f"{duration_phrase}慢生活记录",
        "B站": f"{duration_phrase}国创短片动画",
    }.get(platform, duration_phrase)

    desc = f"{opening}。{platform_tail}。"
    max_chars = _platform_tone(platform)["description_max"]
    if len(desc) > max_chars:
        desc = desc[: max_chars - 1] + "…"
    return desc


# ---------------------------------------------------------------------------
# Hashtag generation
# ---------------------------------------------------------------------------

def _generate_hashtags(character: dict, platform: str,
                       voice: str | None) -> list[str]:
    """Build the platform-tuned hashtag set.

    1. Read `character.hashtag_seed` if present (optional list of strings).
    2. Append platform-default tags until we hit `hashtag_count`.
    3. Always begin with '#'. De-duplicate, preserving order.
    """
    tone = _platform_tone(platform)
    target = tone["hashtag_count"]
    defaults = list(tone["default_hashtags"])

    # Read seed from character.yaml (optional, may be missing)
    seed = character.get("hashtag_seed") or []
    if isinstance(seed, str):
        seed = [seed]
    # Normalize seed: prepend '#' if missing
    seed_norm = []
    for tag in seed:
        tag = tag.strip()
        if not tag:
            continue
        if not tag.startswith("#"):
            tag = "#" + tag
        seed_norm.append(tag)

    # If a voice is given, prepend it as a tag (so it always shows up)
    if voice:
        v_tag = "#" + voice
        if v_tag not in seed_norm:
            seed_norm.insert(0, v_tag)

    # Merge, dedupe, pad
    seen = set()
    out: list[str] = []
    for tag in seed_norm + defaults:
        if tag in seen:
            continue
        seen.add(tag)
        out.append(tag)
        if len(out) >= target:
            break
    return out[:target]


# ---------------------------------------------------------------------------
# Cover timestamp picking
# ---------------------------------------------------------------------------

def _pick_cover_candidates(storyboard: list[dict],
                           emotion_peaks: list[float]) -> list[float]:
    """Pick 3 cover timestamps:
       1. The first emotional peak (or shot 1's start if no peaks)
       2. The midpoint of the storyboard
       3. The peak closest to 80% through the duration

    If the emotion peaks don't extend that late, use 80% of total
    duration as a fallback (it's better to suggest a late timestamp
    than to dedupe with the midpoint).
    """
    if not storyboard:
        return []

    total_dur = max((s.get("end_sec", 0) for s in storyboard), default=0)
    if total_dur <= 0:
        return []

    # 1. First emotional peak — fall back to first shot's start
    first_peak = emotion_peaks[0] if emotion_peaks else float(
        storyboard[0].get("start_sec", 0))

    # 2. Midpoint of duration
    midpoint = total_dur / 2.0

    # 3. Pick the LATEST emotion peak (closest to 80% if peaks exist
    # that far in, else use 80% of duration as a fallback).
    target_late = 0.8 * total_dur
    if emotion_peaks:
        # Prefer a peak that's at or after the midpoint. If none, pick
        # the latest available peak (could be early, but better than
        # colliding with the midpoint).
        late_candidates = [p for p in emotion_peaks if p >= midpoint]
        if late_candidates:
            late_peak = min(late_candidates, key=lambda t: abs(t - target_late))
        else:
            # All peaks are before the midpoint — pick the latest one
            late_peak = max(emotion_peaks)
    else:
        late_peak = target_late

    # Round to 1 decimal place (covers don't need sub-second precision)
    candidates = sorted({
        round(first_peak, 1),
        round(midpoint, 1),
        round(late_peak, 1),
    })
    return candidates


# ---------------------------------------------------------------------------
# Posting window
# ---------------------------------------------------------------------------

def _generate_posting_window(character: dict, platform: str,
                             voice: str | None) -> PostingWindow | None:
    """Suggest a posting window based on platform + character persona.

    Heuristics (derived from observed traffic patterns, 2024-2025):
      抖音:    19:00-22:00 (peak entertainment time)
      小红书:  20:00-23:00 (lifestyle scroll before bed)
      B站:     21:00-24:00 (otaku late-night)
    Voice tweaks:
      哲学:    earlier (18:00-21:00) — contemplation at dusk
    """
    base = {
        "抖音": (19, 22, "抖音晚间娱乐高峰 (19-22 点)"),
        "小红书": (20, 23, "小红书睡前种草时段 (20-23 点)"),
        "B站": (21, 24, "B站深夜御宅时段 (21-24 点)"),
    }.get(platform)
    if not base:
        return None
    start, end, rationale = base
    if voice == "哲学":
        start, end = 18, 21
        rationale = "哲学调黄昏时段 (18-21 点) — 适合独思"
    return PostingWindow(start_hour=start, end_hour=end, rationale=rationale)


# ---------------------------------------------------------------------------
# Main entry: build_publish_package
# ---------------------------------------------------------------------------

def build_publish_package(
    episode_data: dict,
    character_yaml: dict,
    storyboard: list[dict],
    script: list[dict] | None,
    platform: str,
    voice: str | None,
) -> PublishPackage:
    """Compose a full PublishPackage from the inputs.

    Args:
        episode_data:    row from `episodes` table (dict).
        character_yaml:  the loaded `character` block of character.yaml.
        storyboard:      parsed storyboard (list of shot dicts).
        script:          parsed script voiceovers (list, may be empty/None).
        platform:        one of 抖音 / 小红书 / B站.
        voice:           one of 治愈/御宅/哲学/国潮 (or None).

    Returns:
        PublishPackage with all 5 sections populated.
    """
    if platform not in _PLATFORM_TONE:
        raise ValueError(
            f"unknown platform '{platform}' "
            f"(expected one of: {', '.join(_PLATFORM_TONE)})"
        )

    emotion_peaks = _extract_emotion_peaks(storyboard)
    titles = _generate_titles(character_yaml, episode_data, storyboard, voice)
    description = _generate_description(
        storyboard, episode_data, character_yaml, platform)
    hashtags = _generate_hashtags(character_yaml, platform, voice)
    cover = _pick_cover_candidates(storyboard, emotion_peaks)
    posting = _generate_posting_window(character_yaml, platform, voice)

    return PublishPackage(
        titles=titles,
        description=description,
        hashtags=hashtags,
        cover_candidates=cover,
        posting_window=posting,
        platform=platform,
        character_id=(episode_data or {}).get("character_id", "?"),
        episode_id=(episode_data or {}).get("episode_id", "?"),
        voice=voice,
    )


# ---------------------------------------------------------------------------
# Markdown formatter
# ---------------------------------------------------------------------------

_PLATFORM_DISPLAY = {"抖音": "抖音", "小红书": "小红书", "B站": "B站"}


def _fmt_titles(pkg: PublishPackage) -> str:
    return (
        "## 1. 标题 (3 选 1)\n\n"
        f"- 情感向: {pkg.titles.emotional}\n"
        f"- 搞笑向: {pkg.titles.comedic}\n"
        f"- 悬疑向: {pkg.titles.mysterious}\n"
    )


def _fmt_description(pkg: PublishPackage) -> str:
    return (
        "## 2. 简介\n\n"
        f"{pkg.description}\n"
    )


def _fmt_hashtags(pkg: PublishPackage) -> str:
    # Space-separated, all on one line — easy to copy-paste into a
    # platform's hashtag input field.
    one_line = " ".join(pkg.hashtags)
    return (
        "## 3. 话题标签 (复制整行)\n\n"
        f"{one_line}\n"
    )


def _fmt_cover(pkg: PublishPackage) -> str:
    lines = ["## 4. 封面候选 (从这 3 个时间点截屏)\n"]
    for ts in pkg.cover_candidates:
        lines.append(f"- {ts:g}s")
    lines.append("")
    lines.append(
        "操作: 打开渲染好的 MP4 → 拖到候选时间点 → 截屏 → 上传作为封面")
    lines.append("")
    return "\n".join(lines)


def _fmt_posting_window(pkg: PublishPackage) -> str:
    if not pkg.posting_window:
        return "## 5. 建议发布时段\n\n(暂无建议)\n"
    pw = pkg.posting_window
    return (
        "## 5. 建议发布时段\n\n"
        f"{pw.start_hour:02d}:00 - {pw.end_hour:02d}:00\n\n"
        f"({pw.rationale})\n"
    )


def format_publish_package_md(pkg: PublishPackage, episode_data: dict,
                              character_yaml: dict) -> str:
    """Render the package as a single markdown document for human copy-paste.

    Sections (in order):
      1. 标题 (3 variants)
      2. 简介 (1 line, ≤ platform description_max)
      3. 话题标签 (1 line, space-separated)
      4. 封面候选 (3 timestamps)
      5. 建议发布时段
    """
    platform_display = _PLATFORM_DISPLAY.get(pkg.platform, pkg.platform)
    char_name = (character_yaml or {}).get("name") or pkg.character_id
    ep_id = (episode_data or {}).get("episode_id") or pkg.episode_id
    dur = (episode_data or {}).get("actual_dur_sec") or \
          (episode_data or {}).get("target_dur_sec") or "?"

    header = (
        f"# {char_name} {ep_id} — {platform_display} 发布素材\n\n"
        f"> 时长: {dur}s · 平台: {platform_display} · "
        f"语气: {pkg.voice or 'default'}\n\n"
        "---\n\n"
    )

    body = (
        _fmt_titles(pkg)
        + "\n---\n\n"
        + _fmt_description(pkg)
        + "\n---\n\n"
        + _fmt_hashtags(pkg)
        + "\n---\n\n"
        + _fmt_cover(pkg)
        + "\n---\n\n"
        + _fmt_posting_window(pkg)
    )
    return header + body


# ---------------------------------------------------------------------------
# Path helpers (used by the Click command)
# ---------------------------------------------------------------------------

def default_output_path(character_id: str, episode_id: str,
                        platform: str, characters_root: Path | None = None
                        ) -> Path:
    """Return the default output path: characters/<ip>/examples/<EP>/PUBLISH_PACKAGE_<platform>.md

    `characters_root` defaults to <repo-root>/characters (i.e. one
    level up from the vivify package). The episode directory is
    assumed to be named `panda-episode-<N>` when episode_id is
    `EP<N>`, but falls back to just the episode_id if that pattern
    doesn't match.
    """
    root = characters_root or (Path(__file__).resolve().parent.parent.parent / "characters")
    # Try to map EP005 → panda-episode-005 (fengge's naming convention)
    m = re.search(r"(\d+)", episode_id)
    if m:
        ep_dir = f"panda-episode-{int(m.group(1)):03d}"
    else:
        ep_dir = episode_id
    return root / character_id / "examples" / ep_dir / \
        f"PUBLISH_PACKAGE_{platform}.md"


__all__ = [
    "TitleVariants",
    "PostingWindow",
    "PublishPackage",
    "build_publish_package",
    "format_publish_package_md",
    "default_output_path",
]
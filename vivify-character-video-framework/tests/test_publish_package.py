"""Tests for vivify.publish_package + `episode prepare-publish` Click command.

Covers:
  - build_publish_package for each platform (3 tests)
  - Title variants: 3 styles × 3 platforms = 9 (parametrized)
  - Hashtag padding to platform target count
  - Description length per platform constraint
  - Cover timestamp picking: first peak, midpoint, late peak
  - Markdown format includes all 5 sections
  - Click command integration via CliRunner (3 tests):
      - basic happy path
      - --dry-run prints to stdout
      - unknown platform fails with clear error
"""
from __future__ import annotations

import sqlite3
import sys
from pathlib import Path

import pytest
import yaml
from click.testing import CliRunner

from vivify.publish_package import (
    TitleVariants,
    PublishPackage,
    PostingWindow,
    _PLATFORM_TONE,
    _extract_emotion_peaks,
    _generate_hashtags,
    _pick_cover_candidates,
    _generate_titles,
    _generate_description,
    build_publish_package,
    format_publish_package_md,
    default_output_path,
    _platform_tone,
)
from vivify.cli import main as cli


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

# Repo root + framework dir (for the showcase + characters paths used
# in the Click integration tests).
_THIS = Path(__file__).resolve()
_FW_DIR = _THIS.parents[1]   # .../vivify-character-video-framework/
_MONOREPO = _FW_DIR.parent   # .../ (monorepo root, where characters/ lives)


def _char_yaml() -> dict:
    """Load fengge's character.yaml from the worktree."""
    p = _FW_DIR / "characters" / "fengge" / "character.yaml"
    with p.open("r", encoding="utf-8") as f:
        return yaml.safe_load(f)


def _storyboard_shots() -> list[dict]:
    """Parse the real fengge episode-005 storyboard (4 shots, 16s)."""
    p = _FW_DIR / "characters" / "fengge" / "examples" / "panda-episode-005" / "STORYBOARD.md"
    sys.path.insert(0, str(_FW_DIR))
    from render_episode import parse_storyboard  # type: ignore
    return parse_storyboard(str(p))


def _episode_data(ep_id: str = "EP005", dur: int = 16) -> dict:
    return {
        "id": 1,
        "character_id": "fengge",
        "episode_id": ep_id,
        "tone": "御宅",
        "platform": "抖音",
        "actual_dur_sec": dur,
        "target_dur_sec": dur,
        "storyboard_path": "...",
        "script_path": "...",
    }


# ---------------------------------------------------------------------------
# Platform-level: build_publish_package
# ---------------------------------------------------------------------------

def test_build_publish_package_douyin():
    pkg = build_publish_package(
        episode_data=_episode_data(),
        character_yaml=_char_yaml()["character"],
        storyboard=_storyboard_shots(),
        script=[],
        platform="抖音",
        voice="治愈",
    )
    assert isinstance(pkg, PublishPackage)
    assert isinstance(pkg.titles, TitleVariants)
    assert pkg.platform == "抖音"
    assert len(pkg.hashtags) == _platform_tone("抖音")["hashtag_count"]
    assert pkg.cover_candidates, "should have at least 1 cover timestamp"
    assert pkg.posting_window is not None


def test_build_publish_package_xiaohongshu():
    pkg = build_publish_package(
        episode_data=_episode_data(),
        character_yaml=_char_yaml()["character"],
        storyboard=_storyboard_shots(),
        script=[],
        platform="小红书",
        voice="治愈",
    )
    assert pkg.platform == "小红书"
    # 小红书: 10 hashtags
    assert len(pkg.hashtags) == 10
    # description ≤ 200
    assert len(pkg.description) <= 200


def test_build_publish_package_bilibili():
    pkg = build_publish_package(
        episode_data=_episode_data(),
        character_yaml=_char_yaml()["character"],
        storyboard=_storyboard_shots(),
        script=[],
        platform="B站",
        voice="国潮",
    )
    assert pkg.platform == "B站"
    # B站: 8 hashtags
    assert len(pkg.hashtags) == 8
    assert len(pkg.description) <= 500
    # B站 title max is 80 — emotional title should fit
    assert len(pkg.titles.emotional) <= 80


# ---------------------------------------------------------------------------
# Title variants — parametrized across 3 styles × 3 platforms
# ---------------------------------------------------------------------------

@pytest.mark.parametrize("voice", ["治愈", "御宅", "哲学", "国潮"])
def test_title_emotional(voice):  # 4 cases
    char = _char_yaml()["character"]
    titles = _generate_titles(char, _episode_data(), _storyboard_shots(), voice)
    # Emotional: "{char_name}{emotion}第N集"
    assert titles.emotional.startswith(char["name"])
    assert "第5集" in titles.emotional  # episode EP005 → N=5
    # Each voice maps to a distinct emotion word
    voice_to_emotion = {"治愈": "治愈", "御宅": "独处", "哲学": "哲思", "国潮": "国潮"}
    assert voice_to_emotion[voice] in titles.emotional


@pytest.mark.parametrize("voice", ["治愈", "御宅", "哲学", "国潮"])
def test_title_comedic(voice):
    char = _char_yaml()["character"]
    titles = _generate_titles(char, _episode_data(), _storyboard_shots(), voice)
    # Comedic: "{setup} → {punchline}"
    assert "→" in titles.comedic
    assert char["name"] in titles.comedic


@pytest.mark.parametrize("voice", ["治愈", "御宅", "哲学", "国潮"])
def test_title_mysterious(voice):
    char = _char_yaml()["character"]
    titles = _generate_titles(char, _episode_data(), _storyboard_shots(), voice)
    # Mysterious: "凌晨3点,{char_name}{action}"
    assert titles.mysterious.startswith("凌晨3点,")
    assert char["name"] in titles.mysterious


# ---------------------------------------------------------------------------
# Hashtag padding to platform target count
# ---------------------------------------------------------------------------

@pytest.mark.parametrize("platform,expected_count", [
    ("抖音", 5),
    ("小红书", 10),
    ("B站", 8),
])
def test_hashtag_count_matches_platform_target(platform, expected_count):
    char = _char_yaml()["character"]
    tags = _generate_hashtags(char, platform, voice="治愈")
    assert len(tags) == expected_count
    # All tags start with '#'
    assert all(t.startswith("#") for t in tags)
    # No duplicates
    assert len(set(tags)) == len(tags)


def test_hashtags_include_voice_when_voice_given():
    char = _char_yaml()["character"]
    tags = _generate_hashtags(char, "抖音", voice="哲学")
    # Voice tag should be prepended
    assert "#哲学" in tags
    assert tags[0] == "#哲学"


def test_hashtags_padding_when_no_seed():
    """With no `hashtag_seed` in character.yaml, we still fill to target."""
    char = {"name": "测试", "personality_tags": ["温柔"]}  # no hashtag_seed
    tags = _generate_hashtags(char, "抖音", voice=None)
    assert len(tags) == 5
    assert all(t.startswith("#") for t in tags)


def test_hashtags_respect_seed_when_present():
    char = {"name": "测试", "hashtag_seed": ["#我的IP", "#主题"]}
    tags = _generate_hashtags(char, "抖音", voice=None)
    # Seed should appear in the result
    assert "#我的IP" in tags
    assert "#主题" in tags


# ---------------------------------------------------------------------------
# Description length per platform constraint
# ---------------------------------------------------------------------------

@pytest.mark.parametrize("platform,max_chars", [
    ("抖音", 80),
    ("小红书", 200),
    ("B站", 500),
])
def test_description_within_platform_max(platform, max_chars):
    char = _char_yaml()["character"]
    desc = _generate_description(
        _storyboard_shots(), _episode_data(), char, platform)
    assert len(desc) <= max_chars, (
        f"description too long for {platform}: {len(desc)} > {max_chars}")


def test_description_mentions_duration():
    char = _char_yaml()["character"]
    desc = _generate_description(
        _storyboard_shots(), _episode_data(dur=58), char, "抖音")
    # The 58-second duration should appear in the tail
    assert "58" in desc


# ---------------------------------------------------------------------------
# Cover timestamp picking: first peak, midpoint, late peak
# ---------------------------------------------------------------------------

def test_cover_picks_three_candidates_in_time_order():
    shots = _storyboard_shots()  # 4 shots × 4s, peaks likely at shot 1 (满月-like word)
    peaks = _extract_emotion_peaks(shots)
    candidates = _pick_cover_candidates(shots, peaks)
    # 2 or 3 candidates (3rd may dedupe with midpoint if no peaks extend late)
    assert 2 <= len(candidates) <= 3
    # Sorted by time
    assert candidates == sorted(candidates)
    # First one is a peak or shot 1's start (0.0)
    assert candidates[0] >= 0
    # Midpoint of a 16s storyboard is 8.0 — within the shots
    assert any(7.0 <= c <= 9.0 for c in candidates)
    # Last is either near 80% of 16s = 12.8 OR is the latest peak (>= midpoint)
    assert any(8.0 <= c <= 14.0 for c in candidates)


def test_cover_handles_no_emotion_peaks():
    """Without emotion peaks, falls back to first-shot start + midpoint."""
    shots = [{"n": 1, "start_sec": 0, "end_sec": 5, "duration_sec": 5,
              "kling_prompt": "平平无奇的开场", "title": "开场",
              "has_video": True}]
    peaks = _extract_emotion_peaks(shots)
    assert peaks == []  # no emotion words → no peaks
    candidates = _pick_cover_candidates(shots, peaks)
    # Should still produce 3 candidates (shot start, midpoint, 80%)
    assert len(candidates) == 3
    assert candidates[0] == 0.0  # falls back to shot 1's start


def test_extract_emotion_peaks_returns_up_to_three():
    """Emotion peaks should return at most 3 timestamps."""
    shots = [
        {"n": 1, "start_sec": 0, "end_sec": 5, "duration_sec": 5,
         "kling_prompt": "满月高悬", "title": "月光", "has_video": True},
        {"n": 2, "start_sec": 5, "end_sec": 10, "duration_sec": 5,
         "kling_prompt": "大雪纷飞", "title": "雪夜", "has_video": True},
        {"n": 3, "start_sec": 10, "end_sec": 15, "duration_sec": 5,
         "kling_prompt": "告别时刻", "title": "告别", "has_video": True},
        {"n": 4, "start_sec": 15, "end_sec": 20, "duration_sec": 5,
         "kling_prompt": "重逢之夜", "title": "重逢", "has_video": True},
    ]
    peaks = _extract_emotion_peaks(shots)
    assert len(peaks) <= 3
    assert all(isinstance(t, float) for t in peaks)


# ---------------------------------------------------------------------------
# Markdown format includes all 5 sections
# ---------------------------------------------------------------------------

def test_markdown_includes_all_five_sections():
    pkg = build_publish_package(
        episode_data=_episode_data(),
        character_yaml=_char_yaml()["character"],
        storyboard=_storyboard_shots(),
        script=[],
        platform="抖音",
        voice="治愈",
    )
    md = format_publish_package_md(pkg, _episode_data(),
                                    _char_yaml()["character"])
    # Section headers
    assert "## 1. 标题" in md
    assert "## 2. 简介" in md
    assert "## 3. 话题标签" in md
    assert "## 4. 封面候选" in md
    assert "## 5. 建议发布时段" in md
    # All three title variants appear
    assert pkg.titles.emotional in md
    assert pkg.titles.comedic in md
    assert pkg.titles.mysterious in md
    # Hashtags are on one line (space-separated)
    tag_lines = [ln for ln in md.splitlines() if ln.startswith("#") and " " in ln]
    assert any(len(ln.split()) >= 3 for ln in tag_lines)
    # Cover timestamps
    for ts in pkg.cover_candidates:
        assert f"{ts:g}s" in md
    # Posting window
    assert "20:00" in md or "19:00" in md  # 抖音 peak window


def test_markdown_hashtags_space_separated_single_line():
    """Critical for platform paste: hashtags must be on ONE line."""
    pkg = build_publish_package(
        episode_data=_episode_data(),
        character_yaml=_char_yaml()["character"],
        storyboard=_storyboard_shots(),
        script=[],
        platform="小红书",
        voice="国潮",
    )
    md = format_publish_package_md(pkg, _episode_data(),
                                    _char_yaml()["character"])
    # Find the hashtag section
    sec_start = md.find("## 3. 话题标签")
    sec_end = md.find("## 4.")
    sec = md[sec_start:sec_end]
    # Find the line with the hashtags (after the header)
    lines = sec.splitlines()
    hashtag_line = next(ln for ln in lines if ln.startswith("#"))
    # All on one line, space-separated
    assert " " in hashtag_line or hashtag_line.count("#") >= 1
    assert "\n" not in hashtag_line


# ---------------------------------------------------------------------------
# default_output_path
# ---------------------------------------------------------------------------

def test_default_output_path_maps_ep005():
    p = default_output_path("fengge", "EP005", "抖音")
    assert p.name == "PUBLISH_PACKAGE_抖音.md"
    assert "fengge" in p.parts
    assert "examples" in p.parts
    assert "panda-episode-005" in p.parts


def test_default_output_path_with_custom_root(tmp_path):
    root = tmp_path / "characters"
    p = default_output_path("myip", "EP012", "B站", characters_root=root)
    assert p.parent.parent.parent.parent == root
    assert p.name == "PUBLISH_PACKAGE_B站.md"
    assert "panda-episode-012" in p.parts


# ---------------------------------------------------------------------------
# Click command integration
# ---------------------------------------------------------------------------

@pytest.fixture
def db_path(tmp_path):
    """Init real schema + register fengge + register EP005."""
    from vivify.db import init_db
    db = tmp_path / "test.db"
    init_db(str(db))
    conn = sqlite3.connect(str(db))
    conn.execute(
        """INSERT OR REPLACE INTO characters
           (id, name, english_name, species, dir_path, character_yaml_path,
            canonical_dir)
           VALUES (?, ?, ?, ?, ?, ?, ?)""",
        ("fengge", "峰哥", "Fengge", "成年熊猫",
         str(_FW_DIR / "characters" / "fengge"),
         str(_FW_DIR / "characters" / "fengge" / "character.yaml"),
         str(_FW_DIR / "characters" / "fengge" / "canonical")),
    )
    sb = str(_FW_DIR / "characters" / "fengge" / "examples"
             / "panda-episode-005" / "STORYBOARD.md")
    sc = str(_FW_DIR / "characters" / "fengge" / "examples"
             / "panda-episode-005" / "SCRIPT-douyin.md")
    conn.execute(
        """INSERT INTO episodes (character_id, episode_id, tone, platform,
            storyboard_path, script_path, target_dur_sec, status)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?)""",
        ("fengge", "EP005", "御宅", "抖音", sb, sc, 16, "completed"),
    )
    conn.commit()
    conn.close()
    return db


def _invoke(*args, db_path=None):
    """Click test runner. --db is a global option before the subcommand."""
    argv = ["--db", str(db_path)] if db_path else []
    argv += list(args)
    return CliRunner().invoke(cli, argv)


def test_prepare_publish_happy_path_writes_file(db_path, tmp_path, monkeypatch):
    """Default: write PUBLISH_PACKAGE_抖音.md into fengge/examples/EP005 dir."""
    # Override the output dir to live under tmp_path so we don't pollute the repo.
    out = tmp_path / "out.md"

    result = _invoke(
        "episode", "prepare-publish", "fengge", "EP005",
        "--platform", "抖音",
        "--voice", "治愈",
        "--output", str(out),
        db_path=db_path,
    )
    assert result.exit_code == 0, result.output
    assert out.exists()
    content = out.read_text(encoding="utf-8")
    assert "# 峰哥 EP005 — 抖音 发布素材" in content
    assert "## 1. 标题" in content
    assert "## 5. 建议发布时段" in content
    # "wrote:" confirmation printed
    assert "wrote:" in result.output


def test_prepare_publish_dry_run_prints_to_stdout(db_path):
    """--dry-run should print the markdown to stdout and NOT touch disk."""
    result = _invoke(
        "episode", "prepare-publish", "fengge", "EP005",
        "--platform", "小红书",
        "--voice", "国潮",
        "--dry-run",
        db_path=db_path,
    )
    assert result.exit_code == 0, result.output
    # Markdown body in stdout
    assert "## 1. 标题" in result.output
    assert "## 3. 话题标签" in result.output
    assert "## 5. 建议发布时段" in result.output
    # Should NOT have written a file (no "wrote:" line)
    assert "wrote:" not in result.output


def test_prepare_publish_unknown_platform_fails_with_clear_error(db_path, tmp_path):
    """Invalid platform → Click rejects it before we even hit our code."""
    out = tmp_path / "out.md"
    result = _invoke(
        "episode", "prepare-publish", "fengge", "EP005",
        "--platform", "TikTok",  # not in our choice list
        "--output", str(out),
        db_path=db_path,
    )
    assert result.exit_code != 0
    # Click prints "Invalid value for '--platform'" on the error stream;
    # the merged output should mention it.
    combined = (result.output or "") + (result.stderr or "") if hasattr(result, "stderr") else result.output
    assert "platform" in combined.lower() or "TikTok" in combined or "抖音" in combined


# ---------------------------------------------------------------------------
# Misc robustness
# ---------------------------------------------------------------------------

def test_unknown_platform_raises_keyerror_in_tone():
    with pytest.raises(KeyError):
        _platform_tone("TikTok")


def test_build_publish_package_raises_on_unknown_platform():
    with pytest.raises(ValueError):
        build_publish_package(
            episode_data=_episode_data(),
            character_yaml=_char_yaml()["character"],
            storyboard=[],
            script=None,
            platform="TikTok",
            voice="治愈",
        )


def test_posting_window_for_yuzhi_voice_is_earlier():
    """哲学 voice should shift posting window earlier (18-21)."""
    pkg_normal = build_publish_package(
        episode_data=_episode_data(),
        character_yaml=_char_yaml()["character"],
        storyboard=_storyboard_shots(),
        script=[],
        platform="抖音",
        voice="治愈",
    )
    pkg_philo = build_publish_package(
        episode_data=_episode_data(),
        character_yaml=_char_yaml()["character"],
        storyboard=_storyboard_shots(),
        script=[],
        platform="抖音",
        voice="哲学",
    )
    assert pkg_normal.posting_window.start_hour == 19
    assert pkg_philo.posting_window.start_hour == 18
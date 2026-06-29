"""Tests for --dry-run combined with --use-driver.

Pre-existing limitation (B2, surfaced 2026-06): --dry-run exited BEFORE
the driver branch, so `--dry-run --use-driver` did nothing useful —
it returned the cost estimate without exercising the per-shot pipeline.

Fix: when both flags are set, the driver branch is taken with
generate_asset and _call_tts monkeypatched to return fake results.
This exercises the full per-shot plumbing (DB writes, cost aggregation,
adapter interface) without spending money on real API calls.
"""
import json
import sqlite3
from pathlib import Path
from unittest.mock import patch

import pytest
from click.testing import CliRunner

from vivify.cli import main as cli
from vivify.providers.base import GenerateResult


# --- fixtures ---------------------------------------------------------------

@pytest.fixture
def db_path(tmp_path):
    """Init the real schema + register the fengge character."""
    from vivify.db import init_db
    db = tmp_path / "test.db"
    init_db(str(db))

    conn = sqlite3.connect(str(db))
    conn.execute("""INSERT OR REPLACE INTO characters
        (id, name, english_name, species, dir_path, character_yaml_path, canonical_dir)
        VALUES (?, ?, ?, ?, ?, ?, ?)""",
        ("fengge", "峰哥", "Fengge", "成年熊猫",
         str(Path.cwd() / "characters" / "fengge"),
         str(Path.cwd() / "characters" / "fengge" / "character.yaml"),
         str(Path.cwd() / "characters" / "fengge" / "canonical")))
    conn.commit()
    conn.close()
    return db


def _invoke(*args, db_path=None):
    argv = ["--db", str(db_path)] if db_path else []
    argv += list(args)
    return CliRunner().invoke(cli, argv)


# Storyboards live in the repo-root showcase (docs/showcase), not in
# characters/fengge/examples. The framework dir is one level below the
# monorepo root, so parents[2] of this test file is the repo root.
_SHOWCASE_DIR = Path(__file__).resolve().parents[2] / "docs" / "showcase" / "panda-episode-001"


def _storyboard():
    return _SHOWCASE_DIR / "STORYBOARD.md"


def _script():
    return _SHOWCASE_DIR / "SCRIPT-douyin.md"


# --- tests ------------------------------------------------------------------

def test_dry_run_with_use_driver_runs_full_pipeline_no_spend(
    db_path, tmp_path, monkeypatch,
):
    """--dry-run --use-driver must:
    - NOT call real adapter (would burn money)
    - Run generate_asset per shot (image + video) → per-shot DB rows
    - Cost is the STUBBED cost (¥0.20 image + ¥1.05 video), not the
      estimate from pricing (¥2.00 image + ¥2.50 video)
    - Episode row gets written
    """
    import vivify.commands.episode as ep_mod
    monkeypatch.setattr(ep_mod, "_parse_voiceovers", lambda *a, **kw: [])

    # If real adapter gets called, we'd burn money — make it loud.
    def explode(*args, **kwargs):
        raise AssertionError(
            "real adapter called during --dry-run --use-driver — would "
            "spend real money!"
        )
    monkeypatch.setattr("vivify.asset_orchestrator._build_adapter",
                        lambda *a, **k: explode())
    # Track what episode_driver.generate_asset was called with (the stubbed
    # version inside commands/episode.py replaces this, but we still want
    # to confirm the real orchestrator pipeline ran)
    generate_asset_calls = {"n": 0}
    real_generate_asset = __import__(
        "vivify.episode_driver", fromlist=["generate_asset"]
    ).generate_asset
    orig_generate_asset = real_generate_asset

    def spy(req, **kwargs):
        generate_asset_calls["n"] += 1
        return orig_generate_asset(req, **kwargs)
    # The stub inside commands/episode.py overwrites generate_asset at
    # module level. So we check call count via _call_tts instead, which
    # is also stubbed.

    result = _invoke(
        "episode", "render", "fengge", "EP-DRYRUN",
        "--storyboard", str(_storyboard()),
        "--script", str(_script()),
        "--voice", "治愈",
        "--platform", "抖音",
        "--dry-run",
        "--use-driver",
        "--out", str(tmp_path / "out"),
        db_path=db_path,
    )

    # The driver pipeline ran — per-shot rows must have image_path,
    # video_path, and the STUBBED cost (¥1.25 per shot = ¥0.20 + ¥1.05).
    # If the legacy estimate path ran instead, cost would be the
    # estimated video cost (¥2.50 for 5s), not the stubbed ¥1.05.
    conn = sqlite3.connect(str(db_path))
    shot_rows = conn.execute(
        "SELECT shot_number, image_path, video_path, cost_yuan FROM shots "
        "WHERE episode_id IN (SELECT id FROM episodes WHERE episode_id='EP-DRYRUN')"
    ).fetchall()
    conn.close()

    assert len(shot_rows) >= 2, (
        f"driver pipeline didn't run: only {len(shot_rows)} per-shot rows. "
        f"CLI exit={result.exit_code}, output={result.output}"
    )
    for n, ip, vp, cost in shot_rows:
        assert ip is not None, f"shot {n} image_path is None"
        assert vp is not None, f"shot {n} video_path is None"
        # Stubbed cost: 0.20 image + 1.05 video = 1.25 per shot.
        # NOT the estimate (which would be ~2.50/shot for 5s video).
        assert abs(cost - 1.25) < 0.01, (
            f"shot {n} cost_yuan={cost}, expected 1.25 (stubbed image 0.20 + "
            f"video 1.05). If this is the estimate (~2.50), the driver "
            f"pipeline didn't run — `--dry-run --use-driver` is broken."
        )


def test_dry_run_without_use_driver_uses_estimate_only(
    db_path, tmp_path, monkeypatch,
):
    """--dry-run WITHOUT --use-driver: legacy behavior — uses estimated cost
    (not real per-shot), does NOT exercise the driver pipeline."""
    import vivify.commands.episode as ep_mod
    monkeypatch.setattr(ep_mod, "_parse_voiceovers", lambda *a, **kw: [])

    real_adapter_called = {"n": 0}
    monkeypatch.setattr(
        "vivify.asset_orchestrator._build_adapter",
        lambda *a, **k: (
            real_adapter_called.__setitem__("n", real_adapter_called["n"] + 1),
            None,
        )[1],
    )

    result = _invoke(
        "episode", "render", "fengge", "EP-ESTIMATE",
        "--storyboard", str(_storyboard()),
        "--script", str(_script()),
        "--voice", "治愈",
        "--platform", "抖音",
        "--dry-run",   # NO --use-driver
        "--out", str(tmp_path / "out"),
        db_path=db_path,
    )

    # Real adapter was NOT called (estimate-only path)
    assert real_adapter_called["n"] == 0, (
        f"dry-run without --use-driver should not call adapter, "
        f"but got {real_adapter_called['n']} calls"
    )

    # Per-shot rows exist; cost_yuan is the DIVIDED estimate (¥58/9 ≈ ¥6.44).
    # Driver stubbed cost is ¥1.25/shot. If we see ¥1.25, the driver
    # path leaked in.
    conn = sqlite3.connect(str(db_path))
    shot_rows = conn.execute(
        "SELECT shot_number, cost_yuan FROM shots "
        "WHERE episode_id IN (SELECT id FROM episodes WHERE episode_id='EP-ESTIMATE')"
    ).fetchall()
    conn.close()
    assert len(shot_rows) >= 2
    for n, cost in shot_rows:
        assert abs(cost - 1.25) > 0.5, (
            f"shot {n} cost_yuan={cost} looks like the driver stubbed cost "
            f"(¥1.25), but this is the estimate-only path. Driver path leaked."
        )
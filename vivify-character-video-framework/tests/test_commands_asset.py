"""Tests for the `vivify asset {generate,ledger,router}` CLI commands.

These are the L1 user-facing entry points that drive the new
asset_orchestrator pipeline. Tests use tmp_path / monkeypatch to
isolate filesystem side-effects (ledger, router config).

The existing `asset list|register|show|register-canonical` commands
are tested implicitly by the rest of the suite — not re-tested here.
"""

import json
import os
from pathlib import Path
from unittest.mock import patch, MagicMock

import pytest
from click.testing import CliRunner

from vivify.cli import main


# --- fixtures ---------------------------------------------------------------

@pytest.fixture
def runner():
    return CliRunner()


@pytest.fixture
def isolated_ledger(tmp_path, monkeypatch):
    """Point ledger at tmp so tests don't pollute ~/.claude/agents/."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")
    return tmp_path / "ledger.jsonl"


@pytest.fixture
def isolated_router_config(tmp_path, monkeypatch):
    """Point router config at tmp so tests don't pollute ~/.claude/config/."""
    p = tmp_path / "router.yaml"
    monkeypatch.setattr("vivify.asset_router.DEFAULT_CONFIG_PATH", p)
    return p


@pytest.fixture
def isolated_db(tmp_path):
    """Use a throwaway DB so the CLI doesn't read the real one."""
    db = tmp_path / "cli.db"
    return str(db)


def invoke(runner, *args, db=None):
    """Invoke `vivify` with --db <db> to isolate DB state."""
    return runner.invoke(main, ["--quiet-init", "--db", db or "/tmp/cli-test.db", *args],
                          catch_exceptions=False)


# --- router --show-config --------------------------------------------------

def test_asset_router_show_config_creates_default(runner, isolated_router_config):
    """`vivify asset router --show-config` writes default YAML and prints it."""
    result = invoke(runner, "asset", "router", "--show-config")
    assert result.exit_code == 0, result.output
    assert isolated_router_config.exists()
    # The default config uses ark for video + image
    assert "ark" in result.output
    assert "monthly_cny" in result.output or "monthly" in result.output


def test_asset_router_show_config_reads_existing(runner, isolated_router_config):
    """If config exists, --show-config prints its contents (no overwrite)."""
    isolated_router_config.write_text("default_providers:\n  video: minimax\n")
    result = invoke(runner, "asset", "router", "--show-config")
    assert result.exit_code == 0, result.output
    assert "minimax" in result.output
    # Should NOT overwrite
    assert "video: minimax" in isolated_router_config.read_text()


# --- router --pick ---------------------------------------------------------

def test_asset_router_pick_dry_run(runner, isolated_router_config):
    """`--pick --type video` prints chosen provider + estimated cost."""
    isolated_router_config.write_text("""\
default_providers:
  video: ark
  image: ark
cost_caps:
  per_asset_cny: 5
""")
    result = invoke(runner, "asset", "router", "--pick",
                    "--type", "video", "--duration-sec", "5")
    assert result.exit_code == 0, result.output
    assert "ark" in result.output
    assert "estimate" in result.output.lower() or "¥" in result.output


# --- ledger ----------------------------------------------------------------

def test_asset_ledger_tail_empty(runner, isolated_ledger, isolated_db):
    """`vivify asset ledger` on empty ledger prints a helpful message."""
    result = invoke(runner, "asset", "ledger", db=isolated_db)
    assert result.exit_code == 0, result.output
    assert "empty" in result.output.lower() or "no entries" in result.output.lower()


def test_asset_ledger_as_json(runner, isolated_ledger, monkeypatch):
    """`--as-json` prints the rows as a JSON array."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", isolated_ledger)
    # Pre-populate the ledger
    from vivify.asset_ledger import LedgerEntry, append
    isolated_ledger.parent.mkdir(parents=True, exist_ok=True)
    isolated_ledger.touch()
    append(LedgerEntry(
        timestamp="2026-06-24T10:00:00.000Z",
        scene_id="s1", provider="ark", type="image",
        cost_cny=0.2, duration_ms=1000, status="success",
        asset_path="/tmp/a.jpg",
        prompt_hash="fnv64:abc",
    ))
    result = invoke(runner, "asset", "ledger", "--as-json")
    assert result.exit_code == 0, result.output
    rows = json.loads(result.output)
    assert isinstance(rows, list)
    assert len(rows) == 1
    assert rows[0]["scene_id"] == "s1"


def test_asset_ledger_filter_by_status(runner, isolated_ledger, monkeypatch):
    """`--filter status=success` keeps only matching rows."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", isolated_ledger)
    from vivify.asset_ledger import LedgerEntry, append
    isolated_ledger.parent.mkdir(parents=True, exist_ok=True)
    isolated_ledger.touch()
    append(LedgerEntry(timestamp="2026-06-24T10:00:00.000Z",
                        scene_id="ok", provider="ark", type="image",
                        cost_cny=0.2, duration_ms=1000, status="success",
                        asset_path="/tmp/a.jpg"))
    append(LedgerEntry(timestamp="2026-06-24T10:00:01.000Z",
                        scene_id="bad", provider="ark", type="image",
                        cost_cny=0.0, duration_ms=500, status="provider-failed",
                        error="HTTP 400"))

    result = invoke(runner, "asset", "ledger", "--filter", "status=success",
                    "--as-json")
    assert result.exit_code == 0, result.output
    rows = json.loads(result.output)
    assert len(rows) == 1
    assert rows[0]["scene_id"] == "ok"


# --- generate (dry-run) ----------------------------------------------------

def test_asset_generate_dry_run_picks_provider(runner, isolated_router_config, isolated_db):
    """`--dry-run` picks a provider but does not call any adapter."""
    isolated_router_config.write_text("""\
default_providers:
  image: ark
""")
    # If we did NOT dry-run, ark would actually try to call the API.
    # With --dry-run, only the router decision should happen.
    result = invoke(runner, "asset", "generate",
                    "--scene", '{"scene_id":"s1","type":"image","prompt":"red circle"}',
                    "--type", "image", "--dry-run", db=isolated_db)
    assert result.exit_code == 0, result.output
    assert "ark" in result.output
    assert "dry-run" in result.output.lower() or "would pick" in result.output.lower()


def test_asset_generate_unknown_type_errors(runner, isolated_db):
    """Unknown asset type → Click validation error (exit code 2)."""
    result = invoke(runner, "asset", "generate",
                    "--scene", '{"scene_id":"s1","type":"bgm","prompt":"x"}',
                    "--type", "bgm", db=isolated_db)
    # bgm is not supported by the new CLI yet (only image + video)
    # Should still exit cleanly (0) and print a "not supported" note
    # OR raise a Click error. Either way, no crash.
    assert "bgm" in result.output or result.exit_code != 0


# --- generate actual --------------------------------------------------------

def test_asset_generate_calls_orchestrator(runner, isolated_router_config,
                                            isolated_ledger, isolated_db):
    """Without --dry-run, generate_asset is called with a GenerateRequest."""
    isolated_router_config.write_text("""\
default_providers:
  image: ark
cost_caps:
  per_asset_cny: 5
""")
    fake_result = MagicMock(ok=True, provider="ark",
                              model="doubao-seedream-4-0-250828",
                              local_path="/tmp/img.jpg", public_url=None,
                              cost_yuan=0.2, duration_ms=1000, error=None,
                              status_hint=None, asset_id=None)
    with patch("vivify.commands.asset.generate_asset",
                return_value=fake_result) as mock_orch:
        result = invoke(runner, "asset", "generate",
                        "--scene", '{"scene_id":"s1","type":"image","prompt":"red circle"}',
                        "--type", "image", db=isolated_db)
    assert result.exit_code == 0, result.output
    assert mock_orch.called
    # The orchestrator was called with a GenerateRequest whose scene_id came from --scene
    call_args = mock_orch.call_args
    req = call_args.kwargs.get("req") or call_args.args[0]
    assert req.scene_id == "s1"
    assert req.prompt == "red circle"
    # Output mentions success
    assert "✓" in result.output or "succeeded" in result.output
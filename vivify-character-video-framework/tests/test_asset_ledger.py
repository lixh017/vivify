"""Tests for vivify.asset_ledger — JSONL append-only ledger.

Schema fields match the existing data in
~/.claude/agents/opc-asset-ledger.jsonl (which we auto-rename to
vivify-asset-ledger.jsonl on first run):
  timestamp, scene_id, provider, type, cost_cny, duration_ms, status,
  asset_path, profile_version, prompt_hash, consistency_score, outfit
"""

import json
from datetime import datetime
from pathlib import Path

import pytest

from vivify.asset_ledger import (
    LedgerEntry,
    LEGACY_LEDGER_PATHS,
    LEDGER_PATH,
    append,
    ensure_ledger,
    fnv1a_64,
    read_recent,
    stream,
)


# --- hash -------------------------------------------------------------------

def test_fnv1a_64_deterministic():
    """Same input → same hash, every time."""
    h1 = fnv1a_64("hello world")
    h2 = fnv1a_64("hello world")
    assert h1 == h2
    assert h1.startswith("fnv64:")
    # 16 hex chars after the prefix
    assert len(h1) == len("fnv64:") + 16


def test_fnv1a_64_distinct_inputs_distinct_hashes():
    """Different inputs → different hashes."""
    assert fnv1a_64("a") != fnv1a_64("b")


def test_fnv1a_64_matches_legacy_value():
    """Hash of an empty string should be the documented FNV-1a-64 baseline."""
    # FNV-1a-64 of empty input is 0xcbf29ce484222325 (well-known)
    # Our impl emits lowercase hex, no 0x prefix
    h = fnv1a_64("")
    assert h == "fnv64:cbf29ce484222325"


# --- ledger file I/O --------------------------------------------------------

def test_ensure_ledger_creates_file_if_missing(tmp_path, monkeypatch):
    """ensure_ledger() creates the parent dir + empty file at LEDGER_PATH."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")
    p = ensure_ledger()
    assert p == tmp_path / "ledger.jsonl"
    assert p.exists()
    assert p.read_text() == ""


def test_ensure_ledger_renames_legacy_file(tmp_path, monkeypatch):
    """If a legacy opc-asset-ledger.jsonl exists, rename to vivify-asset-ledger.jsonl."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "vivify-asset-ledger.jsonl")
    legacy = tmp_path / "opc-asset-ledger.jsonl"
    legacy.write_text('{"timestamp":"2026-01-01T00:00:00Z","scene_id":"x","provider":"y","type":"image","cost_cny":0.2,"duration_ms":1000,"status":"success","asset_path":"/tmp/a.jpg","profile_version":"fengge_v1","prompt_hash":"fnv64:abc"}\n')
    p = ensure_ledger()
    assert p.exists()
    assert not legacy.exists()
    # Legacy data is preserved
    lines = p.read_text().strip().split("\n")
    assert len(lines) == 1
    assert json.loads(lines[0])["scene_id"] == "x"


# --- append / read ---------------------------------------------------------

def test_append_writes_valid_jsonl(tmp_path, monkeypatch):
    """Each entry is one JSON object on its own line, no trailing newline issues."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")
    ensure_ledger()
    e = LedgerEntry(
        timestamp="2026-06-24T10:00:00.000Z",
        scene_id="s1",
        provider="ark",
        type="image",
        cost_cny=0.20,
        duration_ms=2500,
        status="success",
        asset_path="/tmp/img.jpg",
        profile_version="fengge_v1",
        prompt_hash="fnv64:deadbeef00000000",
    )
    append(e)
    text = (tmp_path / "ledger.jsonl").read_text()
    lines = text.strip().split("\n")
    assert len(lines) == 1
    d = json.loads(lines[0])
    assert d["scene_id"] == "s1"
    assert d["provider"] == "ark"
    assert d["cost_cny"] == 0.20


def test_append_then_read_recent_round_trip(tmp_path, monkeypatch):
    """Append 3 entries, read_recent(10) returns all 3 in order."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")
    ensure_ledger()
    for i in range(3):
        append(LedgerEntry(
            timestamp=f"2026-06-24T10:00:0{i}.000Z",
            scene_id=f"s{i}",
            provider="ark", type="image",
            cost_cny=0.1, duration_ms=100, status="success",
            asset_path=None,
        ))
    entries = read_recent(10)
    assert len(entries) == 3
    assert [e.scene_id for e in entries] == ["s0", "s1", "s2"]


def test_read_recent_respects_n(tmp_path, monkeypatch):
    """read_recent(2) returns only the last 2 entries."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")
    ensure_ledger()
    for i in range(5):
        append(LedgerEntry(
            timestamp=f"2026-06-24T10:00:0{i}.000Z",
            scene_id=f"s{i}", provider="ark", type="image",
            cost_cny=0.1, duration_ms=100, status="success", asset_path=None,
        ))
    entries = read_recent(2)
    assert [e.scene_id for e in entries] == ["s3", "s4"]


def test_stream_yields_entries(tmp_path, monkeypatch):
    """stream() is a generator over the ledger."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")
    ensure_ledger()
    append(LedgerEntry(
        timestamp="2026-06-24T10:00:00.000Z",
        scene_id="x", provider="ark", type="image",
        cost_cny=0.1, duration_ms=100, status="success", asset_path=None,
    ))
    entries = list(stream())
    assert len(entries) == 1
    assert entries[0].scene_id == "x"


def test_ledger_entry_to_dict_includes_all_fields():
    """to_dict round-trips all fields, including defaults."""
    e = LedgerEntry(
        timestamp="2026-06-24T10:00:00.000Z",
        scene_id="s1", provider="ark", type="image",
        cost_cny=0.20, duration_ms=2500, status="success", asset_path=None,
    )
    d = e.to_dict()
    assert d["scene_id"] == "s1"
    assert d["profile_version"] == "fengge_v1"  # default
    assert d["prompt_hash"] == ""                # default
    assert d["outfit"] is None                  # default
    assert d["error"] is None                   # default
    # Round-trip via from_dict
    e2 = LedgerEntry.from_dict(d)
    assert e2.scene_id == e.scene_id
    assert e2.status == e.status


def test_ledger_entry_handles_malformed_jsonl(tmp_path, monkeypatch):
    """Malformed lines are skipped, not crashed on."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")
    ensure_ledger()
    p = tmp_path / "ledger.jsonl"
    p.write_text(
        '{"timestamp":"2026-06-24T10:00:00.000Z","scene_id":"good1","provider":"ark","type":"image","cost_cny":0.1,"duration_ms":100,"status":"success","asset_path":null}\n'
        'this is not json\n'
        '{"timestamp":"2026-06-24T10:00:01.000Z","scene_id":"good2","provider":"ark","type":"image","cost_cny":0.1,"duration_ms":100,"status":"success","asset_path":null}\n'
    )
    entries = read_recent(10)
    assert [e.scene_id for e in entries] == ["good1", "good2"]

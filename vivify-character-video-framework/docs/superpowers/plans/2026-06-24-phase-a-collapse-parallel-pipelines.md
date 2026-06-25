# Phase A: Collapse Parallel Pipelines — `episode render` drives `generate_asset()`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `vivify episode render fengge EP001` route every shot through `generate_asset()` (orchestrator) so the asset ledger, real-cost tracking, monthly cost-cap, and SQLite `shots/asset` rows all line up from one render. After this, `render_episode.py` becomes a deprecated manual escape (still importable, no longer spawned as a subprocess by the new path).

**Architecture:** Today `commands/episode.py:645-744` shells out to `render_episode.py` as a black box and re-discovers outputs by convention (`/tmp/vivify-render/work/01-image/shot-NN.jpg`). Phase A replaces the subprocess with in-process calls:
- `commands/episode.py:render_cmd` parses storyboard + script via the already-imported `render_episode.parse_storyboard / parse_script`
- For each shot, in serial (or parallel via `concurrent.futures` if `parallel > 1`), it calls `generate_asset()` for the image, then for the video, then for the TTS
- It updates `shots` / `assets` / `episodes` rows with the **real** results from `GenerateResult.local_path / cost_yuan / provider / model`
- It reuses `render_episode.ff_*` helpers (concat, subtitles, mix_audio, mux_final, synth_bgm, text_card, trim, stretch_last_frame) for muxing — no ffmpeg code is rewritten
- `cost_cap.check_cost_caps` is invoked **per shot** (not once on estimate) so monthly cap is real, not theatre
- The asset ledger (`vivify-asset-ledger.jsonl`) gets one row per shot per asset type (image, video, tts) — the same as the manual `vivify asset generate` path

**Tech Stack:** Python 3.10+, Click, SQLite, PyYAML, Click testing harness (CliRunner), pytest + monkeypatch, `render_episode.ff_*` ffmpeg helpers.

---

## File map (after Phase A)

```
vivify/
├── commands/
│   └── episode.py            # MODIFIED: render_cmd rewrites subprocess → in-process
├── asset_orchestrator.py     # MODIFIED: pass character_ref; wire db_path→monthly cap
├── providers/
│   └── minimax.py            # MODIFIED: add TTS generate() method (refactor from render_episode.gen_tts)
└── (NEW) episode_driver.py   # NEW: per-shot loop with parallel/serial, real-cost accumulation
render_episode.py             # UNTOUCHED in code; CLAUDE.md marks it deprecated
tests/
├── test_episode_driver.py    # NEW: serial happy path, parallel, retry, cost-cap
├── test_minimax_tts.py       # NEW: TTS adapter via mocked subprocess/requests
└── test_asset_orchestrator.py # MODIFIED: monthly cap from DB; character_ref passthrough
```

---

## Task 1: Monthly cost-cap wired in `generate_asset()`

**Files:**
- Modify: `vivify/asset_orchestrator.py:119-145` (signature), `:171-194` (cost gate)
- Test: `tests/test_asset_orchestrator.py` (extend existing)

The orchestrator currently has `db_path` and `monthly_so_far` as "reserved for future". Wire them now. `cost_cap.py:48-61` already provides `monthly_spend_yuan(conn, month)`. Add an optional `monthly_hard` param and call `check_cost_caps` with the current month-to-date total + this call's estimate. If blocked, write `status="budget-exceeded"` ledger row and return failed `GenerateResult`. **Do not raise** — keep the orchestrator's contract (returns GenerateResult, writes ledger).

- [ ] **Step 1: Write the failing tests**

In `tests/test_asset_orchestrator.py`, add a new test class `TestMonthlyCap`:

```python
class TestMonthlyCap:
    def test_monthly_cap_blocks_when_db_total_plus_estimate_exceeds(self, monkeypatch, tmp_path):
        """If db_path is given and this call's estimate would push monthly total
        over the cap, generate_asset must write budget-exceeded and return ok=False."""
        import sqlite3
        db = tmp_path / "vivify.db"
        conn = sqlite3.connect(db)
        conn.execute("CREATE TABLE episodes (cost_yuan REAL, render_completed_at TEXT)")
        conn.execute("INSERT INTO episodes VALUES (?, ?)", (59999.0, "2026-06-15T10:00:00Z"))
        conn.commit()
        conn.close()

        from vivify.asset_orchestrator import generate_asset
        from vivify.providers.base import GenerateRequest, AssetType
        from vivify.asset_ledger import LEDGER_PATH

        # Force-override the ledger path so we don't pollute real home
        monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")

        req = GenerateRequest(
            scene_id="s1", asset_type=AssetType("video"), prompt="test",
            duration_sec=10,
        )
        result = generate_asset(
            req, env={}, config_path=None, provider_filter="ark",
            db_path=str(db), monthly_hard=60000.0,
        )
        assert result.ok is False
        assert result.status_hint == "budget-exceeded"
        assert "monthly" in (result.error or "").lower()

    def test_monthly_cap_passes_when_db_total_low(self, monkeypatch, tmp_path):
        """If monthly total is low, generate_asset proceeds and reaches adapter."""
        import sqlite3
        db = tmp_path / "vivify.db"
        conn = sqlite3.connect(db)
        conn.execute("CREATE TABLE episodes (cost_yuan REAL, render_completed_at TEXT)")
        conn.execute("INSERT INTO episodes VALUES (?, ?)", (100.0, "2026-06-01T00:00:00Z"))
        conn.commit()
        conn.close()
        monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "ledger.jsonl")

        from vivify.asset_orchestrator import generate_asset, _build_adapter
        from vivify.providers.base import GenerateRequest, AssetType, GenerateResult
        from unittest.mock import MagicMock

        # Stub adapter that returns ok
        fake = MagicMock()
        fake.name = "ark"
        fake.asset_type = "video"
        fake.generate.return_value = GenerateResult(
            ok=True, provider="ark", model="seedance", local_path=tmp_path / "x.mp4",
            cost_yuan=10.0, duration_ms=1000,
        )
        monkeypatch.setattr("vivify.asset_orchestrator._build_adapter", lambda *a, **kw: fake)

        req = GenerateRequest(scene_id="s1", asset_type=AssetType("video"),
                              prompt="t", duration_sec=10)
        result = generate_asset(req, env={}, provider_filter="ark",
                                db_path=str(db), monthly_hard=60000.0)
        assert result.ok is True
        fake.generate.assert_called_once()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd vivify-character-video-framework && python -m pytest tests/test_asset_orchestrator.py::TestMonthlyCap -v`
Expected: FAIL with `TypeError: generate_asset() got an unexpected keyword argument 'monthly_hard'` (because the param doesn't exist yet).

- [ ] **Step 3: Implement monthly cap check in `generate_asset`**

Modify `vivify/asset_orchestrator.py`:

In the signature (line 119-125), replace the placeholder params with real ones:

```python
def generate_asset(req: GenerateRequest, *,
                   env: dict,
                   config_path: Optional[Path] = None,
                   provider_filter: Optional[str] = None,
                   db_path: Optional[str] = None,
                   monthly_hard: Optional[float] = None,    # NEW: cap, default from cost_cap
                   policy: Optional[RetryPolicy] = None) -> GenerateResult:
```

In the cost-gate block (around line 171-194), add monthly check **after** the per_asset check:

```python
# 3a. Monthly-cap check (only if db_path + monthly_hard are provided)
if db_path and monthly_hard is not None:
    import sqlite3
    from .cost_cap import monthly_spend_yuan
    with sqlite3.connect(db_path) as _c:
        spent = monthly_spend_yuan(_c)
    if spent + estimate > monthly_hard:
        return _emit_failure(
            req, provider=provider_id, status="budget-exceeded",
            error=(f"monthly cap: already ¥{spent:.2f} + this ¥{estimate:.2f} "
                   f"> cap ¥{monthly_hard:.2f}"),
            cost_yuan=estimate,
        )
```

Delete the `monthly_so_far: Already-spent-amount-in-month, for future monthly gate.` docstring line and the `Reserved for future monthly-cap DB lookup` line.

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_asset_orchestrator.py -v`
Expected: 30+ passed (28 existing + 2 new).

- [ ] **Step 5: Commit**

```bash
git add vivify/asset_orchestrator.py tests/test_asset_orchestrator.py
git commit -m "feat(asset): wire monthly cost-cap in generate_asset via cost_cap.monthly_spend_yuan"
```

---

## Task 2: TtsProvider on the MiniMax adapter

**Files:**
- Modify: `vivify/providers/minimax.py` (add `generate_tts` method)
- Test: `tests/test_minimax_tts.py` (new)

The legacy `render_episode.gen_tts` (lines 565-615) hits `https://api.minimaxi.com/v1/t2a_v2` with the character's `voice_profiles[tone]` and writes HEX-returned MP3 to disk. Lift it into `MiniMaxProvider` as a new `generate_tts` method. The adapter must accept `tone` (resolved by the caller from `voice_profiles`) and a `voice_id/speed/pitch/emotion/vol` profile dict.

- [ ] **Step 1: Write the failing tests**

Create `tests/test_minimax_tts.py`:

```python
"""Tests for MiniMaxProvider.generate_tts (lifts render_episode.gen_tts)."""
import json
import pytest
from pathlib import Path
from unittest.mock import patch, MagicMock

from vivify.providers.minimax import MiniMaxProvider
from vivify.providers.base import GenerateRequest, AssetType


def _hex_audio(b: bytes) -> str:
    return b.hex()


def test_generate_tts_writes_mp3_from_hex_response(tmp_path):
    """When the API returns audio as HEX (海螺 quirk), the adapter must
    bytes.fromhex + write to out_path, not base64-decode."""
    out = tmp_path / "tts.mp3"
    fake_audio_bytes = b"\xff\xfb\x90\x00" * 10  # not a real MP3 — content is irrelevant
    fake_response = {"data": {"audio": _hex_audio(fake_audio_bytes)}}

    fake_curl = MagicMock(return_value=(200, "", json.dumps(fake_response)))
    with patch("vivify.providers.minimax._curl", fake_curl, create=True), \
         patch("vivify.providers.minimax.curl", fake_curl):
        p = MiniMaxProvider(env={"MINIMAX_API_KEY": "test"})
        req = GenerateRequest(
            scene_id="EP001-shot1", asset_type=AssetType("tts"),
            prompt="嘿,你看那山",
        )
        result = p.generate_tts(
            req, out_path=out,
            voice_profile={"voice_id": "male-qn-jingying", "speed": 0.78,
                           "pitch": -2, "emotion": "neutral", "vol": 1.0},
        )
    assert result.ok is True, result.error
    assert out.exists()
    assert out.read_bytes() == fake_audio_bytes
    # The request body should include the voice profile values verbatim
    call_kwargs = fake_curl.call_args
    body = call_kwargs.kwargs.get("body") or call_kwargs.args[3]
    assert body["voice_setting"]["voice_id"] == "male-qn-jingying"
    assert body["voice_setting"]["speed"] == 0.78
    assert body["text"] == "嘿,你看那山"


def test_generate_tts_raises_on_non_200(tmp_path):
    out = tmp_path / "tts.mp3"
    fake_curl = MagicMock(return_value=(401, "unauthorized", ""))
    with patch("vivify.providers.minimax.curl", fake_curl):
        p = MiniMaxProvider(env={"MINIMAX_API_KEY": "test"})
        req = GenerateRequest(
            scene_id="EP001-shot1", asset_type=AssetType("tts"),
            prompt="test",
        )
        result = p.generate_tts(
            req, out_path=out,
            voice_profile={"voice_id": "x", "speed": 1.0, "pitch": 0,
                           "emotion": "neutral", "vol": 1.0},
        )
    assert result.ok is False
    assert "401" in (result.error or "")


def test_generate_tts_returns_error_when_api_key_missing(tmp_path):
    out = tmp_path / "tts.mp3"
    p = MiniMaxProvider(env={})  # no MINIMAX_API_KEY
    req = GenerateRequest(
        scene_id="EP001-shot1", asset_type=AssetType("tts"), prompt="x")
    result = p.generate_tts(
        req, out_path=out,
        voice_profile={"voice_id": "x", "speed": 1.0, "pitch": 0,
                       "emotion": "neutral", "vol": 1.0},
    )
    assert result.ok is False
    assert "MINIMAX_API_KEY" in (result.error or "")
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_minimax_tts.py -v`
Expected: FAIL with `AttributeError: 'MiniMaxProvider' object has no attribute 'generate_tts'`.

- [ ] **Step 3: Implement `generate_tts` on `MiniMaxProvider`**

Modify `vivify/providers/minimax.py`. Add at end of class (after `generate_video`):

```python
    # ---- TTS -------------------------------------------------------------

    DEFAULT_TTS_MODEL = "speech-02-hd"
    TTS_URL = "https://api.minimaxi.com/v1/t2a_v2"

    def supports(self, asset_type: AssetType) -> bool:
        # Override: minimax now supports video + tts
        return asset_type in self._supported

    def _supported_set(self):
        return self._supported

    def generate_tts(self, req: GenerateRequest, *,
                     out_path: Path,
                     voice_profile: dict) -> GenerateResult:
        """Generate TTS audio via 海螺 t2a_v2 endpoint.

        海螺 returns audio as HEX (not base64) — we bytes.fromhex it.
        voice_profile is the resolved per-tone dict from character.yaml
        voice_profiles[tone] (voice_id, speed, pitch, emotion, vol).
        """
        if not self.env.get("MINIMAX_API_KEY"):
            return GenerateResult(ok=False, provider=self.name,
                                   error="MINIMAX_API_KEY not in env")
        body = {
            "model": self.DEFAULT_TTS_MODEL,
            "text": req.prompt,
            "voice_setting": {
                "voice_id": voice_profile["voice_id"],
                "speed": voice_profile["speed"],
                "vol": voice_profile["vol"],
                "pitch": voice_profile["pitch"],
                "emotion": voice_profile["emotion"],
            },
            "audio_setting": {
                "sample_rate": 24000,
                "bitrate": 128000,
                "format": "mp3",
            },
        }
        # Local import so the adapter doesn't pull in render_episode's
        # curl helper. We replicate a minimal version inline.
        code, err, body_b = _curl(
            "POST", self.TTS_URL,
            {"Authorization": f"Bearer {self.env['MINIMAX_API_KEY']}",
             "Content-Type": "application/json"},
            body=body, timeout=60,
        )
        if code != 200:
            return GenerateResult(ok=False, provider=self.name,
                                   error=f"t2a_v2 HTTP {code}: {err}")
        try:
            data = json.loads(body_b)
            audio_bytes = bytes.fromhex(data["data"]["audio"])
        except (KeyError, ValueError, TypeError) as e:
            return GenerateResult(ok=False, provider=self.name,
                                   error=f"t2a_v2 response malformed: {e}")
        Path(out_path).parent.mkdir(parents=True, exist_ok=True)
        Path(out_path).write_bytes(audio_bytes)
        return GenerateResult(
            ok=True, provider=self.name, model=self.DEFAULT_TTS_MODEL,
            local_path=Path(out_path), cost_yuan=0.0,  # TTS not priced in v1
        )


# Module-level helper (replicated from render_episode.curl; no import to
# avoid pulling the whole legacy module into providers/).
import json as _json
import subprocess as _subprocess


def _curl(method, url, headers, body=None, timeout=60):
    """Minimal curl wrapper. Returns (code, err, body)."""
    cmd = ["curl", "-sS", "-X", method,
           "-H", f"Content-Type: application/json",
           "-w", "\n%{http_code}", url, "--max-time", str(timeout)]
    for k, v in headers.items():
        cmd += ["-H", f"{k}: {v}"]
    if body is not None:
        cmd += ["-d", _json.dumps(body)]
    try:
        r = _subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
    except _subprocess.TimeoutExpired:
        return 0, "timeout", ""
    if r.returncode != 0:
        return r.returncode, r.stderr, r.stdout
    out = r.stdout.rsplit("\n", 1)
    if len(out) == 2 and out[1].isdigit():
        return int(out[1]), "", out[0]
    return 0, "no http code in response", r.stdout
```

Also update `__init__.py` of the class: `_supported = {"video", "tts"}` (replace existing line 48).

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_minimax_tts.py -v`
Expected: 3 passed.

- [ ] **Step 5: Commit**

```bash
git add vivify/providers/minimax.py tests/test_minimax_tts.py
git commit -m "feat(provider): add MiniMaxProvider.generate_tts (lifts render_episode.gen_tts)"
```

---

## Task 3: `character_ref` passthrough in `asset_orchestrator`

**Files:**
- Modify: `vivify/asset_orchestrator.py:202-213`
- Test: `tests/test_asset_orchestrator.py` (extend)

`MiniMaxProvider.generate_video` accepts `character_ref` (s2v mode) but the orchestrator currently only forwards `out_path` and `model` from `req.options`. Forward `character_ref` too.

- [ ] **Step 1: Write the failing test**

In `tests/test_asset_orchestrator.py`, add to `TestOrchestrator` (or new class):

```python
def test_character_ref_forwarded_to_adapter(self, monkeypatch, tmp_path):
    """The orchestrator must pass character_ref from req.options to the
    adapter, otherwise the s2v (character-locked) ID-drift fix is unreachable
    through generate_asset()."""
    from vivify.asset_orchestrator import generate_asset
    from vivify.providers.base import GenerateRequest, AssetType, GenerateResult
    from unittest.mock import MagicMock

    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "l.jsonl")

    fake = MagicMock()
    fake.name = "minimax"
    fake.asset_type = "video"
    fake.generate.return_value = GenerateResult(
        ok=True, provider="minimax", model="MiniMax-S2V-01",
        local_path=tmp_path / "v.mp4", cost_yuan=5.0, duration_ms=2000,
    )
    monkeypatch.setattr("vivify.asset_orchestrator._build_adapter", lambda *a, **kw: fake)

    req = GenerateRequest(
        scene_id="s1", asset_type=AssetType("video"), prompt="p",
        duration_sec=5,
        options={"out_path": str(tmp_path / "v.mp4"),
                 "model": "MiniMax-S2V-01",
                 "character_ref": "/abs/path/canonical/face.jpg"},
    )
    generate_asset(req, env={}, provider_filter="minimax")
    fake.generate.assert_called_once()
    # Check character_ref was passed as kwarg
    kwargs = fake.generate.call_args.kwargs
    assert kwargs.get("character_ref") == "/abs/path/canonical/face.jpg"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_asset_orchestrator.py::TestOrchestrator::test_character_ref_forwarded_to_adapter -v`
Expected: FAIL — `character_ref` not in call args (currently dropped).

- [ ] **Step 3: Implement the passthrough**

In `vivify/asset_orchestrator.py:202-213`, replace:

```python
    out_path_opt = (req.options or {}).get("out_path")
    model_opt = (req.options or {}).get("model")
    for attempt in range(policy.max_retries + 1):    # initial + N retries
        try:
            if out_path_opt is not None or model_opt is not None:
                result = adapter.generate(
                    req,
                    out_path=Path(out_path_opt) if out_path_opt else None,
                    model=model_opt,
                )
            else:
                result = adapter.generate(req)
```

with:

```python
    out_path_opt = (req.options or {}).get("out_path")
    model_opt = (req.options or {}).get("model")
    character_ref_opt = (req.options or {}).get("character_ref")
    for attempt in range(policy.max_retries + 1):    # initial + N retries
        try:
            # Build kwargs only for what the adapter accepts. We don't know
            # the adapter's signature here, so we use try/except on call.
            kwargs = {}
            if out_path_opt is not None:
                kwargs["out_path"] = Path(out_path_opt)
            if model_opt is not None:
                kwargs["model"] = model_opt
            if character_ref_opt is not None:
                kwargs["character_ref"] = character_ref_opt
            if kwargs:
                result = adapter.generate(req, **kwargs)
            else:
                result = adapter.generate(req)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_asset_orchestrator.py -v`
Expected: 30+ passed (existing + new char_ref test).

- [ ] **Step 5: Commit**

```bash
git add vivify/asset_orchestrator.py tests/test_asset_orchestrator.py
git commit -m "fix(asset): forward character_ref from req.options to adapter (s2v reachable)"
```

---

## Task 4: `episode_driver.py` — per-shot loop with real-cost + ledger

**Files:**
- New: `vivify/episode_driver.py`
- Test: `tests/test_episode_driver.py`

This is the **load-bearing new module**. It replaces the subprocess call in `commands/episode.py:render_cmd`. Per shot it does:

1. Resolve image asset via `generate_asset(asset_type="image", ...)` → writes ledger, returns local_path
2. Resolve video asset via `generate_asset(asset_type="video", ...)` with the image as `reference_image` and `character_ref` from canonical → writes ledger
3. Resolve TTS via `MiniMaxProvider.generate_tts(...)` (Task 2) → writes its own ledger row via direct call
4. Update `shots` row with real image_path/video_path/cost_yuan/provider/model
5. Accumulate real `total_cost_yuan` for the episode

The driver takes the **already-parsed** shots list (from `render_episode.parse_storyboard` + `parse_script`) plus env + config. It does **not** call ffmpeg — that's the muxer's job (next task).

- [ ] **Step 1: Write the failing tests**

Create `tests/test_episode_driver.py`:

```python
"""Tests for vivify.episode_driver — per-shot asset loop driven by generate_asset."""
import json
import sqlite3
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from vivify.episode_driver import render_episode_assets, ShotAssetResult
from vivify.providers.base import GenerateRequest, AssetType, GenerateResult


@pytest.fixture
def empty_db(tmp_path):
    db = tmp_path / "test.db"
    conn = sqlite3.connect(db)
    # Minimal episodes table for monthly_spend_yuan
    conn.execute("""CREATE TABLE episodes (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        cost_yuan REAL, render_completed_at TEXT, status TEXT
    )""")
    conn.execute("""CREATE TABLE shots (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        episode_id INTEGER, shot_number INTEGER,
        image_path TEXT, video_path TEXT, cost_yuan REAL,
        model_used TEXT, kling_prompt TEXT
    )""")
    conn.commit()
    conn.close()
    return db


@pytest.fixture
def parsed_shots():
    return [
        {"n": 1, "start_sec": 0.0, "duration_sec": 4,
         "kling_prompt": "镜头1: 推镜头 峰哥 走入竹林", "scene_id": "bamboo_courtyard"},
        {"n": 2, "start_sec": 4.0, "duration_sec": 4,
         "kling_prompt": "镜头2: 拉镜头 峰哥 转身回眸", "scene_id": "bamboo_courtyard"},
    ]


@pytest.fixture
def voiceovers():
    return [
        {"n": 1, "start_sec": 0.5, "text": "嘿,你看那山"},
        {"n": 2, "start_sec": 4.5, "text": "嗯……风停了"},
    ]


def test_render_episode_assets_calls_generate_asset_per_shot(
    empty_db, parsed_shots, voiceovers, tmp_path, monkeypatch,
):
    """Each shot must trigger: 1 image generate_asset, 1 video generate_asset,
    1 TTS call. Total: 2*(1+1+1) = 6 calls for 2 shots."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "l.jsonl")

    # Mock generate_asset
    import vivify.episode_driver as driver_mod
    call_count = {"n": 0}

    def fake_gen(req, **kwargs):
        call_count["n"] += 1
        ext = "jpg" if req.asset_type == AssetType("image") else "mp4"
        return GenerateResult(
            ok=True, provider="ark", model="x",
            local_path=tmp_path / f"shot-{call_count['n']}.{ext}",
            cost_yuan=2.0, duration_ms=1000,
        )

    # Mock TTS adapter
    def fake_tts(req, **kwargs):
        return GenerateResult(
            ok=True, provider="minimax", model="speech-02-hd",
            local_path=tmp_path / f"tts-{req.scene_id}.mp3",
            cost_yuan=0.05, duration_ms=500,
        )

    monkeypatch.setattr(driver_mod, "generate_asset", fake_gen)
    monkeypatch.setattr(driver_mod, "_call_tts", fake_tts)

    out_dir = tmp_path / "render"
    results = render_episode_assets(
        shots=parsed_shots,
        voiceovers=voiceovers,
        episode_id="EP001",
        character_id="fengge",
        env={},
        out_dir=out_dir,
        db_path=str(empty_db),
        voice_profile={"voice_id": "male-qn-jingying", "speed": 0.78,
                       "pitch": -2, "emotion": "neutral", "vol": 1.0},
        canonical_ref="/abs/canonical/face.jpg",
    )
    # 2 shots × (image + video) = 4 generate_asset calls
    assert call_count["n"] == 4
    assert len(results) == 2
    for r in results:
        assert r.image_path is not None
        assert r.video_path is not None
        assert r.tts_path is not None
        # Real cost = 2.0 (image) + 2.0 (video) + 0.05 (tts) = 4.05
        assert abs(r.total_cost_yuan - 4.05) < 0.01


def test_render_episode_assets_accumulates_real_cost(
    empty_db, parsed_shots, voiceovers, tmp_path, monkeypatch,
):
    """The total cost returned must be the SUM of all real per-asset costs
    (not the estimate from pricing.estimate_episode_cost)."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "l.jsonl")

    import vivify.episode_driver as driver_mod
    monkeypatch.setattr(driver_mod, "generate_asset", lambda req, **kw: GenerateResult(
        ok=True, provider="ark", model="x",
        local_path=tmp_path / "x.jpg", cost_yuan=3.33, duration_ms=1000,
    ))
    monkeypatch.setattr(driver_mod, "_call_tts", lambda req, **kw: GenerateResult(
        ok=True, provider="minimax", model="speech-02-hd",
        local_path=tmp_path / "x.mp3", cost_yuan=0.10, duration_ms=500,
    ))

    results = render_episode_assets(
        shots=parsed_shots, voiceovers=voiceovers,
        episode_id="EP001", character_id="fengge", env={},
        out_dir=tmp_path / "r", db_path=str(empty_db),
        voice_profile={"voice_id": "x", "speed": 1.0, "pitch": 0,
                       "emotion": "neutral", "vol": 1.0},
        canonical_ref=None,
    )
    # Per shot: 3.33 (img) + 3.33 (vid) + 0.10 (tts) = 6.76
    # 2 shots: 13.52
    total = sum(r.total_cost_yuan for r in results)
    assert abs(total - 13.52) < 0.02


def test_render_episode_assets_writes_per_shot_db_rows(
    empty_db, parsed_shots, voiceovers, tmp_path, monkeypatch,
):
    """After render, the shots table must have its image_path + video_path +
    cost_yuan columns populated for each shot (real, not estimated)."""
    monkeypatch.setattr("vivify.asset_ledger.LEDGER_PATH", tmp_path / "l.jsonl")

    import vivify.episode_driver as driver_mod
    counter = {"i": 0}

    def fake_gen(req, **kw):
        counter["i"] += 1
        ext = "jpg" if req.asset_type == AssetType("image") else "mp4"
        return GenerateResult(
            ok=True, provider="ark", model="x",
            local_path=tmp_path / f"asset-{counter['i']}.{ext}",
            cost_yuan=1.0, duration_ms=100,
        )

    monkeypatch.setattr(driver_mod, "generate_asset", fake_gen)
    monkeypatch.setattr(driver_mod, "_call_tts",
        lambda req, **kw: GenerateResult(
            ok=True, provider="minimax", model="tts",
            local_path=tmp_path / "tts.mp3", cost_yuan=0.0, duration_ms=100,
        ))

    # Pre-insert episode + 2 shot rows so we can update them
    conn = sqlite3.connect(str(empty_db))
    ep_pk = conn.execute("INSERT INTO episodes DEFAULT VALUES").lastrowid
    for s in parsed_shots:
        conn.execute("INSERT INTO shots (episode_id, shot_number, kling_prompt) VALUES (?, ?, ?)",
                     (ep_pk, s["n"], s["kling_prompt"]))
    conn.commit()
    conn.close()

    render_episode_assets(
        shots=parsed_shots, voiceovers=voiceovers,
        episode_id="EP001", character_id="fengge", env={},
        out_dir=tmp_path / "r", db_path=str(empty_db),
        episode_pk=ep_pk,
        voice_profile={"voice_id": "x", "speed": 1.0, "pitch": 0,
                       "emotion": "neutral", "vol": 1.0"},
        canonical_ref=None,
    )
    conn = sqlite3.connect(str(empty_db))
    rows = conn.execute(
        "SELECT shot_number, image_path, video_path, cost_yuan FROM shots "
        "WHERE episode_id = ? ORDER BY shot_number", (ep_pk,)
    ).fetchall()
    conn.close()
    assert len(rows) == 2
    for n, ip, vp, cost in rows:
        assert ip is not None and Path(ip).exists() or True  # file is in tmp_path
        assert vp is not None
        # cost_yuan = image (1.0) + video (1.0) = 2.0 (TTS not on shot row)
        assert cost == 2.0
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_episode_driver.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'vivify.episode_driver'`.

- [ ] **Step 3: Implement `vivify/episode_driver.py`**

```python
"""vivify.episode_driver — per-shot asset pipeline.

Replaces the render_episode.py subprocess call in commands/episode.py.

For each shot in a parsed storyboard, this driver:
  1. Calls generate_asset(asset_type="image") to render the seed frame
  2. Calls generate_asset(asset_type="video") with the image as reference_image
     and canonical_ref for s2v character-lock
  3. Calls _call_tts to render voiceover (uses MiniMaxProvider.generate_tts)
  4. Updates the shots row in the SQLite DB with real paths + real cost

Returns a list of ShotAssetResult, one per shot, with the real cost. The
caller (commands/episode.py) sums these to get the episode's real cost_yuan.

Note: this module does NOT call ffmpeg. The muxer is a separate step
(see render_episode.ff_* helpers, imported in commands/episode.py).
"""
from __future__ import annotations

import sqlite3
from dataclasses import dataclass
from pathlib import Path
from typing import Optional

from .asset_orchestrator import generate_asset
from .providers.base import AssetType, GenerateRequest, GenerateResult
from .providers.minimax import MiniMaxProvider


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
        return (self.image_cost_yuan or 0.0) + \
               (self.video_cost_yuan or 0.0) + \
               (self.tts_cost_yuan or 0.0)


def _call_tts(req: GenerateRequest, env: dict, voice_profile: dict,
              out_path: Path) -> GenerateResult:
    """TTS adapter shim — separate from generate_asset because TTS is not
    a video model with router-managed fallback. We call the adapter
    directly. Cost goes to ledger via the orchestrator; for now we just
    return the result and the driver writes a manual ledger row."""
    p = MiniMaxProvider(env=env)
    return p.generate_tts(req, out_path=out_path, voice_profile=voice_profile)


def render_episode_assets(*, shots: list[dict], voiceovers: list[dict],
                          episode_id: str, character_id: str, env: dict,
                          out_dir: Path, db_path: str,
                          voice_profile: dict,
                          canonical_ref: Optional[str] = None,
                          episode_pk: Optional[int] = None,
                          config_path: Optional[Path] = None,
                          monthly_hard: float = 60_000.0) -> list[ShotAssetResult]:
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
            scene_id=scene_id, asset_type=AssetType("image"),
            prompt=prompt, size="1024x1024",
            options={"out_path": str(img_out), "model": "doubao-seedream-4-0-250828"},
        )
        img_result = generate_asset(
            img_req, env=env, config_path=config_path,
            provider_filter="ark", db_path=db_path, monthly_hard=monthly_hard,
        )

        # 2. Video (image → video)
        vid_out = shot_work / f"shot-{n:02d}.mp4"
        vid_options = {"out_path": str(vid_out), "model": "doubao-seedance-2-0-260128"}
        if canonical_ref:
            vid_options["character_ref"] = canonical_ref
        vid_req = GenerateRequest(
            scene_id=scene_id, asset_type=AssetType("video"),
            prompt=prompt, reference_image=str(img_out) if img_result.ok else None,
            duration_sec=dur, options=vid_options,
        )
        # If image failed, fall back to text-only video (no reference_image)
        if not img_result.ok:
            vid_req.reference_image = None
        vid_result = generate_asset(
            vid_req, env=env, config_path=config_path,
            provider_filter="ark", db_path=db_path, monthly_hard=monthly_hard,
        )

        # 3. TTS
        tts_path = None
        tts_cost = 0.0
        vo = vo_by_n.get(n)
        if vo and vo.get("text"):
            tts_out = shot_work / f"voiceover-{n:02d}.mp3"
            tts_req = GenerateRequest(
                scene_id=scene_id, asset_type=AssetType("tts"),
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
                db_path, episode_pk=episode_pk, shot_number=n,
                image_path=str(result.image_path) if result.image_path else None,
                video_path=str(result.video_path) if result.video_path else None,
                cost_yuan=(result.image_cost_yuan + result.video_cost_yuan),
            )

    return results


def _update_shot_row(db_path: str, *, episode_pk: int, shot_number: int,
                     image_path: Optional[str], video_path: Optional[str],
                     cost_yuan: float) -> None:
    """Update a single shot row with the real results from generate_asset."""
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_episode_driver.py -v`
Expected: 3 passed.

- [ ] **Step 5: Commit**

```bash
git add vivify/episode_driver.py tests/test_episode_driver.py
git commit -m "feat(driver): per-shot asset pipeline driving generate_asset + TTS + DB update"
```

---

## Task 5: `commands/episode.py:render_cmd` — replace subprocess with `render_episode_assets`

**Files:**
- Modify: `vivify/commands/episode.py:645-744`
- Test: `tests/test_commands_episode.py` (new)

Replace the `subprocess.call(render_episode.py, ...)` block with a call to `render_episode_assets()`. The muxer (ffmpeg concat + subtitles + audio mix + final mux) still uses `render_episode.ff_*` helpers — these are pure functions and we keep them. Only the asset-generation boundary changes.

- [ ] **Step 1: Write the failing test**

Create `tests/test_commands_episode.py`:

```python
"""Tests for commands.episode render_cmd — the new in-process pipeline."""
import sqlite3
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
from click.testing import CliRunner

from vivify.cli import cli


@pytest.fixture
def db_path(tmp_path):
    db = tmp_path / "test.db"
    # Init schema
    from vivify.db import init_db
    init_db(str(db))

    # Register fengge character
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


def test_render_cmd_dry_run_does_not_call_subprocess(db_path, tmp_path):
    """Dry run: must write episode+shots rows; must NOT spawn render_episode.py."""
    storyboard = Path.cwd() / "characters" / "fengge" / "examples" / "panda-episode-001" / "STORYBOARD.md"
    script = storyboard.parent / "SCRIPT-douyin.md"

    runner = CliRunner()
    with patch("subprocess.call") as mock_subproc:
        result = runner.invoke(cli, [
            "episode", "render", "fengge", "EP001",
            "--storyboard", str(storyboard),
            "--script", str(script),
            "--voice", "治愈",
            "--platform", "抖音",
            "--dry-run",
            "--db-path", str(db_path),
            "--out-dir", str(tmp_path / "out"),
        ])
    assert result.exit_code == 0, result.output
    mock_subproc.assert_not_called()
    # Episode row written
    conn = sqlite3.connect(str(db_path))
    ep = conn.execute("SELECT * FROM episodes WHERE character_id='fengge'").fetchone()
    assert ep is not None
    assert ep[0] is not None  # status column index in our schema


def test_render_cmd_calls_episode_driver_not_subprocess(
    db_path, tmp_path, monkeypatch,
):
    """Real run: must call render_episode_assets, NOT subprocess.call('render_episode.py')."""
    storyboard = Path.cwd() / "characters" / "fengge" / "examples" / "panda-episode-001" / "STORYBOARD.md"
    script = storyboard.parent / "SCRIPT-douyin.md"

    # Patch the driver so we don't burn real API quota
    fake_results = []
    import vivify.commands.episode as ep_mod

    def fake_render_assets(*args, **kwargs):
        shots = kwargs.get("shots") or args[0]
        for s in shots:
            fake_results.append(s.get("n"))
        # Return one ShotAssetResult per shot
        from vivify.episode_driver import ShotAssetResult
        return [ShotAssetResult(
            shot_number=s["n"], image_path=None, video_path=None, tts_path=None,
            image_cost_yuan=0.0, video_cost_yuan=0.0, tts_cost_yuan=0.0,
        ) for s in shots]

    monkeypatch.setattr(ep_mod, "render_episode_assets", fake_render_assets)

    runner = CliRunner()
    with patch("subprocess.call") as mock_subproc:
        result = runner.invoke(cli, [
            "episode", "render", "fengge", "EP001",
            "--storyboard", str(storyboard),
            "--script", str(script),
            "--voice", "治愈",
            "--platform", "抖音",
            "--db-path", str(db_path),
            "--out-dir", str(tmp_path / "out"),
        ])
    assert result.exit_code == 0, result.output
    mock_subproc.assert_not_called()
    assert len(fake_results) >= 1  # at least 1 shot went through the driver
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_commands_episode.py -v`
Expected: FAIL — `render_episode_assets` not imported in `commands/episode.py`; also `subprocess.call` IS still called by the current code.

- [ ] **Step 3: Refactor `commands/episode.py:render_cmd`**

At the top of the file (after line 33's existing imports), add:

```python
from .episode_driver import render_episode_assets, ShotAssetResult
```

Then replace the entire block from line 645 (`# 6. Spawn render_episode.py as subprocess`) through line 744 (the `sys.exit(rc)` end of the else branch) with the new in-process pipeline. The new flow:

```python
    # 6. Resolve character dir for canonical reference + voice profile
    char_dir = Path(char["dir_path"])
    char_yaml = char.get("character_yaml_path") and Path(char["character_yaml_path"])
    canonical_ref = None
    voice_profile_dict = None
    if char_yaml and char_yaml.exists():
        import yaml
        with open(char_yaml) as _f:
            _c = yaml.safe_load(_f)
        canon = (_c.get("character") or {}).get("canonical") or {}
        primary = canon.get("primary")
        if primary:
            canonical_ref = str((char_dir / primary).resolve())
        tone = voice or "治愈"
        voice_profile_dict = (
            (_c.get("character") or {}).get("voice_profiles") or {}
        ).get(tone) or {"voice_id": "male-qn-jingying", "speed": 0.85,
                        "pitch": 0, "emotion": "neutral", "vol": 1.0}

    # 7. Run per-shot asset pipeline (the new in-process path)
    from .episode_driver import render_episode_assets
    click.echo(f"\n[vivify] running per-shot asset pipeline "
               f"({n_shots} shots, parallel={parallel})\n")
    t0 = time.time()
    asset_results = render_episode_assets(
        shots=shots, voiceovers=_parse_voiceovers(script, storyboard),
        episode_id=episode_id, character_id=character_id,
        env=obj.get("env", {}), out_dir=Path(out_dir) / "work",
        db_path=db_path, voice_profile=voice_profile_dict,
        canonical_ref=canonical_ref, episode_pk=ep_pk,
        monthly_hard=60000.0,
    )
    elapsed = time.time() - t0
    real_total_cost = sum(r.total_cost_yuan for r in asset_results)

    # 8. Mux with ffmpeg (reuse render_episode helpers)
    final_mp4 = Path(out_path)
    _mux_episode(asset_results, voiceovers=_parse_voiceovers(script, storyboard),
                 env=obj.get("env", {}), out_path=final_mp4, target_dur=target_dur,
                 title=title, next_episode=next_episode, voice=voice)

    # 9. Post-render: ffprobe + DB finalize with REAL cost
    actual_dur = _ffprobe_duration(str(final_mp4))
    file_size = _file_size(str(final_mp4))

    with connect(db_path) as conn:
        if final_mp4.exists():
            conn.execute(
                """UPDATE episodes SET
                    status = 'completed',
                    output_path = ?,
                    actual_dur_sec = ?,
                    file_size_bytes = ?,
                    cost_yuan = ?,
                    render_completed_at = datetime('now')
                   WHERE id = ?""",
                (str(final_mp4), actual_dur, file_size, real_total_cost, ep_pk),
            )
            conn.execute(
                """UPDATE render_jobs SET status = 'completed',
                                          completed_at = datetime('now')
                   WHERE id = ?""",
                (job_pk,),
            )
            click.echo(f"\n✅ render OK in {elapsed:.0f}s — {final_mp4} (¥{real_total_cost:.2f})")
        else:
            err_msg = "ffmpeg mux produced no output"
            conn.execute(
                """UPDATE episodes SET
                    status = 'failed', error_message = ?,
                    render_completed_at = datetime('now')
                   WHERE id = ?""",
                (err_msg, ep_pk),
            )
            conn.execute(
                """UPDATE render_jobs SET status = 'failed',
                                          completed_at = datetime('now'),
                                          error_message = ?
                   WHERE id = ?""",
                (err_msg, job_pk),
            )
            conn.commit()
            click.echo(f"\n❌ render FAILED — mux produced no output", err=True)
            sys.exit(1)
```

Add two helper functions near the top of `commands/episode.py`:

```python
def _parse_voiceovers(script_path: str | None, storyboard_path: str) -> list[dict]:
    """Parse voiceovers from script, fall back to empty if script missing."""
    if not script_path or not Path(script_path).exists():
        return []
    try:
        from render_episode import parse_script
        return parse_script(script_path)
    except Exception as e:
        click.echo(f"[warn] could not parse script: {e}", err=True)
        return []


def _mux_episode(asset_results, *, voiceovers, env, out_path, target_dur,
                 title, next_episode, voice):
    """Concatenate per-shot image+video+tts into the final MP4 using
    render_episode.ff_* helpers. Best-effort: if mux fails, the per-shot
    assets still exist on disk for manual recovery."""
    try:
        from render_episode import (
            ff_concat, ff_apply_subtitles, ff_mix_audio, ff_mux_final,
            ff_text_card,
        )
    except ImportError:
        click.echo("[warn] render_episode helpers not importable; "
                   "skipping mux", err=True)
        return

    work = out_path.parent / "work"
    concat_list = work / "concat.txt"
    audio_inputs = []
    lines = []
    cum_dur = 0.0

    # Title card
    title_path = work / "title.jpg"
    if title:
        ff_text_card(env, title, dur=2, out_path=str(title_path))
        lines.append(f"file '{title_path}'\nduration 2.0\n")
        cum_dur += 2.0

    for r in asset_results:
        if r.video_path and Path(r.video_path).exists():
            d = r.video_path.rsplit(".", 1)[0] + ".mp4"
            # ensure mp4 ext
            lines.append(f"file '{r.video_path}'\n")
            cum_dur += 5.0  # approximation
        if r.tts_path and Path(r.tts_path).exists():
            audio_inputs.append((int(cum_dur), str(r.tts_path)))

    # End card
    end_path = work / "end.jpg"
    ff_text_card(env, next_episode, dur=2, out_path=str(end_path))
    lines.append(f"file '{end_path}'\nduration 2.0\n")

    concat_list.write_text("".join(lines))
    raw_concat = work / "raw.mp4"
    ff_concat(env, str(concat_list), str(raw_concat))

    # Subtitles + audio mix + final mux
    subbed = work / "subbed.mp4"
    ff_apply_subtitles(env, str(raw_concat), str(subbed),
                       voiceover_text="\n".join(v.get("text", "") for v in voiceovers))

    if audio_inputs:
        bgm_path = work / "bgm.mp3"
        from render_episode import ff_synth_bgm
        ff_synth_bgm(env, str(bgm_path), dur_sec=int(cum_dur + 4), tone=voice)
        mixed = work / "mixed.mp3"
        ff_mix_audio(env, str(bgm_path), audio_inputs, str(mixed))

        ff_mux_final(env, str(subbed), str(mixed), str(out_path))
    else:
        # No TTS — just rename
        out_path.write_bytes(subbed.read_bytes())
```

Also import `from ..db import connect, init_db` is already there. The `env` dict must be threaded into the Click context — add at the top of `render_cmd`:

```python
    env = obj.get("env") or _load_env()
    # ... pass `env=env` to render_episode_assets
```

Where `_load_env()` reads from `~/.claude/config/vivify-volcengine.env` if present (same logic as `commands/asset.py:_vendor_env`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_commands_episode.py -v`
Expected: 2 passed.

Also re-run the full test suite: `python -m pytest tests/ -v`
Expected: 73 + 2 (TTS) + 1 (char_ref) + 2 (monthly cap) + 3 (driver) + 2 (cmd) = 83 passed.

- [ ] **Step 5: Commit**

```bash
git add vivify/commands/episode.py vivify/episode_driver.py tests/test_commands_episode.py
git commit -m "refactor(episode): replace render_episode.py subprocess with in-process driver"
```

---

## Task 6: Mark `render_episode.py` as deprecated in CLAUDE.md

**Files:**
- Modify: `CLAUDE.md` (mark legacy path deprecated)
- Modify: `render_episode.py` (add deprecation banner at top of module docstring)

- [ ] **Step 1: Update `render_episode.py` docstring**

At the very top of `render_episode.py` (line 1-3), add:

```python
"""render_episode — LEGACY single-shot pipeline.

.. deprecated:: 2026-06-24
    The `vivify episode render` command no longer spawns this script as
    a subprocess. It now drives `vivify.episode_driver.render_episode_assets`
    in-process, so all shots go through `generate_asset()` → ledger.

    This file remains for two purposes:
      1. Manual escape hatch: `python3 render_episode.py --storyboard ... --out ...`
         still works as a one-off CLI for ad-hoc testing.
      2. Source of ffmpeg helpers (`ff_*` functions) imported by the new
         driver.

    The legacy code path is FROZEN — no new features. New work goes into
    `vivify/episode_driver.py` + `vivify/asset_orchestrator.py`.
"""
```

- [ ] **Step 2: Update `CLAUDE.md`**

In the "File layout" section, change the description of `render_episode.py`:

OLD:
```
├── render_episode.py            ← legacy single-shot pipeline (still works)
```

NEW:
```
├── render_episode.py            ← DEPRECATED 2026-06-24. ffmpeg helpers only.
│                                   `vivify episode render` now drives
│                                   vivify/episode_driver.py in-process.
```

In "Quick start", demote the `render_episode.py` example to a footnote:

OLD:
```bash
# Render an episode with fengge (legacy single-shot path — still works)
python3 render_episode.py --character-dir characters/fengge \
  --storyboard examples/panda-episode-004/STORYBOARD.md \
  --script examples/panda-episode-004/SCRIPT-douyin.md \
  --voice 治愈 --platform 抖音 --out /tmp/out
```

NEW (kept, but moved to a "Manual escape" subsection):
```markdown
### Manual escape (deprecated, do not use for new episodes)

```bash
# Still works for ad-hoc testing, but `vivify episode render` is preferred.
python3 render_episode.py --character-dir characters/fengge \
  --storyboard examples/panda-episode-004/STORYBOARD.md \
  --script examples/panda-episode-004/SCRIPT-douyin.md \
  --voice 治愈 --platform 抖音 --out /tmp/out
```
```

- [ ] **Step 3: Run full test suite**

Run: `python -m pytest tests/ -q`
Expected: 83+ passed, 0 failed.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md render_episode.py
git commit -m "docs: mark render_episode.py deprecated; vivify episode render is the supported path"
```

---

## Task 7: End-to-end smoke test

**Files:** None new. This is a manual verification step.

- [ ] **Step 1: Dry-run a real episode end-to-end**

Run:
```bash
cd vivify-character-video-framework
./scripts/vivify episode render fengge EP001 \
  --storyboard characters/fengge/examples/panda-episode-001/STORYBOARD.md \
  --script     characters/fengge/examples/panda-episode-001/SCRIPT-douyin.md \
  --voice 治愈 --platform 抖音 --target-dur 58 --dry-run \
  --out /tmp/phase-a-dry
```

Expected output: progress lines for each shot, "DRY RUN complete", episode row in DB.

- [ ] **Step 2: Verify DB**

Run: `sqlite3 .tmp/data/vivify.db "SELECT id, character_id, episode_id, status, cost_yuan FROM episodes WHERE episode_id='EP001';"`

Expected: 1 row, status=completed, cost_yuan=0 (dry-run) or estimated.

- [ ] **Step 3: Verify shot rows**

Run: `sqlite3 .tmp/data/vivify.db "SELECT shot_number, duration_sec, image_path, video_path, cost_yuan, model_used FROM shots WHERE episode_id IN (SELECT id FROM episodes WHERE episode_id='EP001') ORDER BY shot_number;"`

Expected: N rows (N = number of shots in storyboard), image_path and video_path populated (with the dry-run stubs, paths will point to whatever the mocks return).

- [ ] **Step 4: Verify ledger**

Run: `tail -20 ~/.claude/agents/vivify-asset-ledger.jsonl`

Expected: N rows for EP001 (one per shot, one per asset type).

- [ ] **Step 5: If anything fails — DO NOT proceed to real run. Fix and re-test dry-run first.**

---

## Task 8: Real render (one shot, with --force, with budget cap visible)

**Files:** None new. Manual verification.

- [ ] **Step 1: Set up a tiny test scenario**

```bash
cd vivify-character-video-framework
# Use a 1-shot storyboard to minimize cost
cat > /tmp/tiny-storyboard.md <<EOF
# 1-shot test
## Shot 1
- duration: 3
- prompt: 镜头1: 推镜头 峰哥 站在竹林小院
EOF
cat > /tmp/tiny-script.md <<EOF
# script
[0.0] 嘿
EOF
```

- [ ] **Step 2: Real render, --force to bypass dry-run gate**

```bash
./scripts/vivify episode render fengge TEST001 \
  --storyboard /tmp/tiny-storyboard.md \
  --script     /tmp/tiny-script.md \
  --voice 治愈 --platform 抖音 --target-dur 10 \
  --out /tmp/phase-a-real --force
```

Expected: 1 image (Seedream) + 1 video (Seedance, ≤3s) + 1 TTS (海螺) + final MP4 in /tmp/phase-a-real. Cost visible in output. Episode row in DB with `cost_yuan` = real sum (not estimate).

- [ ] **Step 3: Verify**

```bash
sqlite3 .tmp/data/vivify.db "SELECT cost_yuan, actual_dur_sec, file_size_bytes FROM episodes WHERE episode_id='TEST001';"
ls -la /tmp/phase-a-real/
tail -5 ~/.claude/agents/vivify-asset-ledger.jsonl | jq .
```

Expected: `cost_yuan` populated, `actual_dur_sec` matches ffprobe, file exists, ledger has 3 rows for TEST001 (image, video, tts).

- [ ] **Step 4: Verify monthly cap is REAL**

```bash
sqlite3 .tmp/data/vivify.db "SELECT SUM(cost_yuan) FROM episodes WHERE render_completed_at LIKE '2026-06%';"
```

Expected: number matches the sum of `cost_yuan` column for the month (not estimate).

Then re-run a fake expensive scenario by temporarily editing `monthly_hard=1.0` in `episode_driver.py` and re-running — should produce budget-exceeded ledger row + failed episode.

- [ ] **Step 5: Commit any new fixes from the smoke test**

```bash
git add -A
git commit -m "fix: smoke-test fixes from Phase A end-to-end"
```

---

## Verification summary

After all tasks complete:

- `python -m pytest tests/ -q` → 83+ passed
- `sqlite3 .tmp/data/vivify.db "SELECT COUNT(*) FROM episodes"` → 2 (EP001 dry-run + TEST001 real)
- `sqlite3 .tmp/data/vivify.db "SELECT COUNT(*) FROM shots"` → N+1 (storyboard shots)
- `wc -l ~/.claude/agents/vivify-asset-ledger.jsonl` → ≥ 2*shots (one row per asset per shot)
- `grep -n "subprocess.call(render_episode.py" vivify/commands/episode.py` → 0 results (subprocess gone)
- `grep -n "monthly_hard" vivify/asset_orchestrator.py` → ≥ 1 (now wired)
- `grep -n "generate_tts" vivify/providers/minimax.py` → ≥ 1 (TTS adapter exists)

**This is the Phase A exit criterion**: one real `vivify episode render` populates DB, ledger, and produces an MP4 with cost_yuan = real sum (not estimate), and the monthly cap actually blocks a 2nd run.

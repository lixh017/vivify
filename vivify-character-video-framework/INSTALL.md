# Vivify Installation — 5 minutes to first render

This document walks you (or any AI agent) from a fresh Linux/macOS
machine to a working `vivify` CLI. Run each step. If anything fails,
`vivify doctor` will tell you which step missed.

## Prerequisites

- Python 3.10+
- pip (any recent version)
- ~500 MB free disk for ffmpeg + Python deps
- An ARK account at https://console.volcengine.com/ark (sign up, generate API key)
- Optional: mmx account at https://platform.minimaxi.com (TTS) — `mmx auth login` handles setup

## Step 1: Clone + install Python deps

```bash
git clone https://github.com/lixh017/vivify.git
cd vivify
pip install pyyaml click pytest
```

No `pip install vivify` — the CLI runs from source. `scripts/vivify` is the entry.

## Step 2: Set ARK credentials

Create `~/.claude/config/vivify-volcengine.env` (or the legacy `opc-volcengine.env`):

```bash
mkdir -p ~/.claude/config
cat > ~/.claude/config/vivify-volcengine.env <<'EOF'
ARK_API_KEY=your-ark-key-here
ARK_BASE_URL=https://ark.cn-beijing.volces.com/api/v3
EOF
```

Get the key from https://console.volcengine.com/ark → 开通管理 → API Key 管理.

## Step 3: Install mmx (海螺 TTS)

The 海螺 TTS uses the `mmx` CLI which manages its own auth.

```bash
pip install minimax-mmx
mmx auth login   # interactive; uses your MiniMax credentials
mmx --version    # should print 1.0.x
```

## Step 4: Install ffmpeg

Required for the final MP4 mux step.

```bash
# Ubuntu/Debian
sudo apt install ffmpeg

# macOS
brew install ffmpeg

# Windows: download from https://ffmpeg.org/download.html
# and put ffmpeg.exe in your PATH

ffmpeg -version    # should print version info
```

## Step 5: Verify with `vivify doctor`

```bash
./scripts/vivify doctor
```

Expected output:

```
═══ vivify doctor ═══
✓ python: Python 3.12.x
✓ ffmpeg: ffmpeg version ...
✓ mmx: 1.0.x
✓ ARK_API_KEY: loaded
✓ db: ready
✓ disk: free
✓ models[ark]: ready

Summary: 7/7 ok.
```

If any ✗ appears, the doctor prints the install hint. **Fix that one
step above, re-run doctor, repeat until all ✓.**

## First render

```bash
# 1. Register a character (5 questions via the CLI or just use fengge)
./scripts/vivify character list

# 2. Run the smoke test (one shot, ~¥2-5, ~3 min)
./scripts/vivify episode add fengge EP001 --storyboard ... --script ...
./scripts/vivify episode render fengge EP001 --use-driver --out /tmp/test.mp4 --force

# Or use the one-shot smoke script that auto-generates fixtures:
./scripts/render-smoke.sh fengge EP001 10
```

## Where things live

```
vivify-character-video-framework/
├── scripts/vivify          # CLI entry
├── scripts/render-smoke.sh  # One-command end-to-end smoke
├── vivify/                  # Python package (CLI source)
├── characters/fengge/       # Showcase IP data
├── skills/                  # Skills read by AI agents
├── tests/                   # pytest suite (186 tests)
├── INSTALL.md               # ← you are here
└── README.md                # Main project README
```

## Common gotchas

**`doctor` says `✗ ffmpeg: NOT FOUND` but `which ffmpeg` works**

Your ffmpeg is installed but not on the PATH the vivify CLI inherits.
Check `echo $PATH` in the same shell where you ran doctor. Solution:
add `export PATH="/usr/local/bin:$PATH"` to your `~/.bashrc`.

**`doctor` says `✗ ARK_API_KEY: missing`**

The env file isn't being loaded. Check:
- File exists at `~/.claude/config/vivify-volcengine.env` (or `opc-volcengine.env`)
- File is readable (`chmod 600 ~/.claude/config/vivify-volcengine.env`)
- Key has no trailing whitespace or quotes

**`mmx` works but `doctor` says ✗**

`mmx` lives at `~/.local/bin/mmx` (user pip) which isn't on the system
PATH. Solution: `export PATH="$HOME/.local/bin:$PATH"` in `~/.bashrc`.

**Render succeeds but final mp4 is missing**

ffmpeg mux step failed. The per-shot images + videos are saved
(episode status is `assets_only`). Install ffmpeg, then:

```bash
./scripts/vivify episode mux <character> <episode>
```

## What if `doctor` says everything is fine but rendering still fails?

Run the doctor with `--verbose` to see hidden details:

```bash
./scripts/vivify doctor --verbose
```

Then look at the recent ledger entries:

```bash
tail -5 ~/.claude/agents/vivify-asset-ledger.jsonl | python -m json.tool
```

Errors are recorded there with `error` field set. Common patterns:
- `ModelNotOpen` → your ARK account hasn't activated that model
- `quota_exceeded` → top up ARK balance
- `HTTP 401` → API key invalid

## Uninstallation

```bash
# Remove the project
rm -rf vivify-character-video-framework

# Remove credentials
rm ~/.claude/config/vivify-volcengine.env

# Remove installed tools (optional)
pip uninstall minimax-mmx
sudo apt remove ffmpeg    # or: brew uninstall ffmpeg
```

That's it. 5 minutes, 7 checks, and you're rendering.
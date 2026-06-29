#!/usr/bin/env bash
# render-smoke.sh — one-command real-render smoke for vivify
#
# Renders the smallest meaningful episode end-to-end through the new
# in-process driver path (--use-driver). Costs ~¥2-5 depending on model.
# Use to verify the pipeline works against real APIs before production.
#
# Usage:
#   ./scripts/render-smoke.sh fengge EP001                 # 10s target
#   ./scripts/render-smoke.sh fengge EP001 30              # 30s target
#   ./scripts/render-smoke.sh fengge EP001 10 --no-confirm # skip confirmation
#   FORCE=1 ./scripts/render-smoke.sh fengge EP001         # force even if cap low
#
# Required env (loaded automatically from ~/.claude/config/opc-volcengine.env
# or vivify-volcengine.env if present):
#   ARK_API_KEY    — 火山方舟 (Seedream image + Seedance video)
#   mmx CLI        — 海螺 TTS (self-authenticated, no API key needed)
#
# Optional:
#   VIVIFY_DB      — SQLite path (default: .tmp/data/vivify.db)

set -euo pipefail

# --- args -------------------------------------------------------------------

CHARACTER="${1:-fengge}"
EPISODE_ID="${2:-EP001}"
TARGET_DUR="${3:-10}"
NO_CONFIRM=0
for arg in "$@"; do
  case "$arg" in
    --no-confirm) NO_CONFIRM=1 ;;
  esac
done

# --- paths ------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
VIVIFY="$ROOT/scripts/vivify"
OUT_DIR="${VIVIFY_RENDER_DIR:-/tmp/vivify-smoke-renders}/$CHARACTER-$EPISODE_ID"

# --- env load ---------------------------------------------------------------

ENV_LOADED=0
for envfile in \
  "$HOME/.claude/config/vivify-volcengine.env" \
  "$HOME/.claude/config/opc-volcengine.env"; do
  if [ -f "$envfile" ]; then
    echo "[smoke] loading env from $envfile"
    set -a; . "$envfile"; set +a
    ENV_LOADED=1
    break
  fi
done

if [ -z "${ARK_API_KEY:-}" ]; then
  echo "❌ ARK_API_KEY not set"
  echo "   Expected at ~/.claude/config/opc-volcengine.env or vivify-volcengine.env"
  echo "   Get key: https://console.volcengine.com/ark/"
  exit 1
fi

if ! command -v mmx >/dev/null 2>&1; then
  echo "❌ mmx CLI not on PATH"
  echo "   Install: pip install minimax-mmx (then run: mmx auth login)"
  exit 1
fi

echo "[smoke] ARK_API_KEY: ${ARK_API_KEY:0:8}..."
echo "[smoke] mmx version: $(mmx --version 2>&1 | head -1)"
echo "[smoke] target: $CHARACTER / $EPISODE_ID / ${TARGET_DUR}s"
echo "[smoke] output: $OUT_DIR"

# --- pre-flight cost estimate ----------------------------------------------

# Smallest storyboard we can synthesize on-the-fly: 2 shots × 5s = 10s.
# For real showcase storyboards, use a longer --target-dur.
EST_IMAGE_YUAN=$((TARGET_DUR / 5 * 2 * 20))   # ¥0.20/image × ~2 images/shot
EST_VIDEO_YUAN=$((TARGET_DUR / 5 * 2 * 105)) # ¥1.05/5s Seedance Fast
EST_TTS_YUAN=$((TARGET_DUR / 5 * 1))         # ~¥0.01/shot TTS
EST_TOTAL=$((EST_IMAGE_YUAN + EST_VIDEO_YUAN + EST_TTS_YUAN))
EST_TOTAL_CNY=$(echo "scale=2; $EST_TOTAL / 100" | bc 2>/dev/null || echo "~$((EST_TOTAL / 100))")

echo ""
echo "╭─ estimated cost ─╮"
echo "│ images:  ¥$((EST_IMAGE_YUAN / 100)).$((EST_IMAGE_YUAN % 100))"
echo "│ videos:  ¥$((EST_VIDEO_YUAN / 100)).$((EST_VIDEO_YUAN % 100))"
echo "│ TTS:     ¥$((EST_TTS_YUAN / 100)).$((EST_TTS_YUAN % 100))"
echo "│ TOTAL:   ¥$EST_TOTAL_CNY (real money, charged to ARK_API_KEY account)"
echo "╰─────────────────────╯"
echo ""

if [ "$NO_CONFIRM" -eq 0 ] && [ -z "${FORCE:-}" ]; then
  echo "Press Enter to spend ~¥$EST_TOTAL_CNY on real APIs, or Ctrl-C to abort..."
  read -r _
fi

# --- synthesize a minimal storyboard + script on the fly --------------------

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

STORYBOARD="$WORK_DIR/STORYBOARD.md"
SCRIPT="$WORK_DIR/SCRIPT-douyin.md"

N_SHOTS=$((TARGET_DUR / 5))
if [ "$N_SHOTS" -lt 1 ]; then N_SHOTS=1; fi

{
  echo "# Smoke storyboard — $EPISODE_ID ($TARGET_DUR s, $N_SHOTS shots)"
  for i in $(seq 1 "$N_SHOTS"); do
    START=$(( (i - 1) * 5 ))
    END=$(( i * 5 ))
    echo ""
    echo "## 镜头 $i — 测试镜头 [$(printf '%d:%02d' $((START/60)) $((START%60)))-$(printf '%d:%02d' $((END/60)) $((END%60)))]"
    echo ""
    echo '```'
    echo "视觉: 测试场景 $i"
    echo "时长: 5 秒"
    echo "可灵 prompt:"
    echo "镜头$i: 测试 prompt for $CHARACTER, shot $i — short bamboo scene, keep clean"
    echo '```'
  done
} > "$STORYBOARD"

{
  echo "[0:00-0:0$((${#N_SHOTS}*5))]"
  for i in $(seq 1 "$N_SHOTS"); do
    echo "旁白: 测试旁白 $i"
  done
} > "$SCRIPT"

echo "[smoke] storyboard: $STORYBOARD ($N_SHOTS shots)"
echo "[smoke] script: $SCRIPT"

# --- run ---------------------------------------------------------------------

cd "$ROOT"

echo ""
echo "[smoke] running: vivify episode render $CHARACTER $EPISODE_ID --use-driver ..."
echo ""

# Always pass --force since this is a smoke + cost is already pre-estimated
# Note: --quiet-init is a GLOBAL option (must come before the subcommand)
"$VIVIFY" --quiet-init episode render "$CHARACTER" "$EPISODE_ID" \
  --storyboard "$STORYBOARD" \
  --script "$SCRIPT" \
  --voice 治愈 \
  --platform 抖音 \
  --target-dur "$TARGET_DUR" \
  --use-driver \
  --out "$OUT_DIR" \
  --force 2>&1 | tail -40

EXIT_CODE=${PIPESTATUS[0]}

echo ""
echo "═══════════════════════════════════════════════════════════════════"
if [ "$EXIT_CODE" -eq 0 ]; then
  echo "✅ smoke OK — output: $OUT_DIR"
  echo ""
  echo "Check DB state:"
  echo "  sqlite3 ${VIVIFY_DB:-.tmp/data/vivify.db} \\"
  echo "    \"SELECT episode_id, status, cost_yuan, output_path FROM episodes WHERE episode_id='$EPISODE_ID';\""
else
  echo "❌ smoke FAILED (exit=$EXIT_CODE)"
  echo ""
  echo "Common causes:"
  echo "  - Insufficient Ark balance: check https://console.volcengine.com/ark/"
  echo "  - mmx quota exceeded: mmx quota show"
  echo "  - Network: test curl https://ark.cn-beijing.volces.com/api/v3"
fi
echo "═══════════════════════════════════════════════════════════════════"
exit "$EXIT_CODE"
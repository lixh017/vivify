#!/usr/bin/env bash
# make_episode.sh — scaffold a new panda IP episode
#
# Generates STORYBOARD.md + SCRIPT-douyin.md from IP-bible vocabulary,
# pre-filled with the chosen tone's visual register and outfit/scene
# whitelists. Author fills in the voiceover lines + actions.
#
# Usage:
#   ./scripts/make_episode.sh \
#     --slug panda-episode-005 \
#     --tone 御宅 \
#     --season 冬至 \
#     --outfits "outfit_jacket_redwhite,outfit_changshan_blue" \
#     --scenes "竹林小院,围炉夜话,月下窗棂,雨后青苔" \
#     --duration 16
#
# After scaffolding, render with:
#   python3 docs/panda-episode-pipeline/render_episode.py \
#     --storyboard docs/showcase/panda-episode-005/STORYBOARD.md \
#     --script    docs/showcase/panda-episode-005/SCRIPT-douyin.md \
#     --voice     御宅 \
#     --platform  抖音 \
#     --out       docs/showcase/panda-episode-005/RENDER

set -euo pipefail

# --- arg parsing ---
SLUG=""
TONE=""
SEASON=""
OUTFITS=""
SCENES=""
DURATION=16
PLATFORM="抖音"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --slug)     SLUG="$2"; shift 2 ;;
    --tone)     TONE="$2"; shift 2 ;;
    --season)   SEASON="$2"; shift 2 ;;
    --outfits)  OUTFITS="$2"; shift 2 ;;
    --scenes)   SCENES="$2"; shift 2 ;;
    --duration) DURATION="$2"; shift 2 ;;
    --platform) PLATFORM="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

if [[ -z "$SLUG" || -z "$TONE" || -z "$SEASON" || -z "$OUTFITS" || -z "$SCENES" ]]; then
  echo "usage: $0 --slug <slug> --tone <治愈|御宅|哲学|国潮> --season <节气> --outfits <csv> --scenes <csv> [--duration 16] [--platform 抖音]" >&2
  exit 1
fi

case "$TONE" in
  治愈) TONE_GUIDE="治愈调: 慢、暖、留白。句子要短,带'嘿'/'啊'/'嗯...'的停顿。画面给暖光 + 茶烟 + 雨声。结尾允许悬而未决,但要给观众一个可以'靠'着的具体物(茶盏/毛毯/雨声)。" ;;
  御宅) TONE_GUIDE="御宅调: 内向、独处、书斋感。'我也不爱出门'/'那就一起待着'。画面给室内小景 + 毛绒毯 + 深夜台灯。少修饰,多停顿。" ;;
  哲学) TONE_GUIDE="哲学调: 反鸡汤、存在主义。'但真的是这样吗?' 提问不答。画面给月下 + 山水 + 远景。多用问句,少给结论。" ;;
  国潮) TONE_GUIDE="国潮调: 古诗、传统美学、节气时令。'古人云'/'你看那山'。画面给折扇 + 卷轴 + 红叶 + 樱花。可引《庄子》/《诗经》/唐诗,1 句即可。" ;;
  *) echo "invalid tone: $TONE (must be 治愈/御宅/哲学/国潮)" >&2; exit 1 ;;
esac

# --- compute number of shots ---
SHOT_COUNT=$((DURATION / 4))  # 4s per shot
[[ $SHOT_COUNT -lt 1 ]] && SHOT_COUNT=1

# --- split CSV into arrays ---
IFS=',' read -ra OUTFIT_ARR <<< "$OUTFITS"
IFS=',' read -ra SCENE_ARR <<< "$SCENES"

if [[ ${#OUTFIT_ARR[@]} -lt $SHOT_COUNT ]]; then
  echo "need at least $SHOT_COUNT outfits, got ${#OUTFIT_ARR[@]}" >&2
  exit 1
fi
if [[ ${#SCENE_ARR[@]} -lt $SHOT_COUNT ]]; then
  echo "need at least $SHOT_COUNT scenes, got ${#SCENE_ARR[@]}" >&2
  exit 1
fi

# --- outfit descriptions (from IP bible §2) ---
outfit_desc() {
  case "$1" in
    outfit_hufu_red)        echo "朱红汉服 (vermilion red hanfu, wide-sleeved round-collar robe, jade pendant)" ;;
    outfit_changshan_blue)  echo "宝蓝长衫 (royal blue changshan, mandarin collar, silver buttons)" ;;
    outfit_workwear_orange) echo "暖橙唐宋短打 (warm orange Tang-Song short jacket, mandarin collar, side-fastening buttons, rolled sleeves, 国潮 sash) — **NOT a modern cleaner uniform**; no reflective stripes / utility pockets / logos" ;;
    outfit_robe_green)      echo "翠绿僧袍 (jade-green monk robe, wide sleeves, prayer beads)" ;;
    outfit_jacket_redwhite) echo "红白运动夹克 (vermilion + cream sports jacket, white tee underneath)" ;;
    *) echo "[unknown outfit $1 — please replace with valid outfit ID]" ;;
  esac
}

# --- prop whitelist suggestion per shot (rotating) ---
PROPS=(折扇 茶盏 竹杖 铜钱 葫芦 古书 油纸伞 糖葫芦 围棋子 围炉)

# --- output dir ---
OUT_DIR="docs/showcase/$SLUG"
mkdir -p "$OUT_DIR"

# --- write STORYBOARD.md ---
{
  echo "# $SLUG — Storyboard ($TONE + $SEASON)"
  echo
  echo "> $PLATFORM ${DURATION}s 版本,**$SHOT_COUNT 个镜头**,每个镜头 4 秒。"
  echo "> $TONE_GUIDE"
  echo
  echo "## 镜头 1 — ${SCENE_ARR[0]} (${OUTFIT_ARR[0]}) [00:00-00:04]"
  echo
  echo
  echo '```'
  echo "视觉: [TODO: 用 $TONE 调性的具体画面描述峰哥在${SCENE_ARR[0]}做什么]"
  echo "构图: 中景/特写依内容定,峰哥占画面 60%,场景留 40% 给国潮元素"
  echo "灯光: ${SCENE_ARR[0]} 适配的暖/冷光"
  echo "时长: 4 秒"
  echo "镜头运动: 静态或慢推 (留白优先)"
  echo
  echo "可灵 prompt (图生视频):"
  echo "一只成年熊猫(峰哥),头身比 1:1.2 圆胖身材,面部白 rgb(245,240,225) 眼周黑 rgb(26,26,26),眼神半阖带笑意不直视镜头,身穿 ${OUTFIT_ARR[0]} ($(outfit_desc "${OUTFIT_ARR[0]}")),手持[折扇|茶盏|竹杖|铜钱|葫芦],站在 ${SCENE_ARR[0]},[$TONE 调性适配的光线]氛围,**国潮 + 色彩靓丽** (朱红/暖橙/翠绿/宝蓝撞色,非暗色调、非水墨),不露爪,不攻击性姿势,无文字,9:16 构图,4s"
  echo '```'
  echo
} > "$OUT_DIR/STORYBOARD.md"

# remaining shots
for i in $(seq 2 $SHOT_COUNT); do
  idx=$((i - 1))
  start=$(( (i - 1) * 4 ))
  end=$((i * 4))
  start_fmt=$(printf "%02d:%02d" $((start / 60)) $((start % 60)))
  end_fmt=$(printf "%02d:%02d" $((end / 60)) $((end % 60)))
  {
    echo "## 镜头 $i — ${SCENE_ARR[$idx]} (${OUTFIT_ARR[$idx]}) [$start_fmt-$end_fmt]"
    echo
    echo
    echo '```'
    echo "视觉: [TODO]"
    echo "构图: 中景/特写依内容定,峰哥占画面 60%,场景留 40% 给国潮元素"
    echo "灯光: ${SCENE_ARR[$idx]} 适配的暖/冷光"
    echo "时长: 4 秒"
    echo "镜头运动: 静态或慢推 (留白优先)"
    echo
    echo "可灵 prompt (图生视频):"
    echo "一只成年熊猫(峰哥),头身比 1:1.2 圆胖身材,面部白 rgb(245,240,225) 眼周黑 rgb(26,26,26),眼神半阖带笑意不直视镜头,身穿 ${OUTFIT_ARR[$idx]} ($(outfit_desc "${OUTFIT_ARR[$idx]}")),手持[折扇|茶盏|竹杖|铜钱|葫芦],站在 ${SCENE_ARR[$idx]},[$TONE 调性适配的光线]氛围,**国潮 + 色彩靓丽** (朱红/暖橙/翠绿/宝蓝撞色,非暗色调、非水墨),不露爪,不攻击性姿势,无文字,9:16 构图,4s"
    echo '```'
    echo
  } >> "$OUT_DIR/STORYBOARD.md"
done

{
  echo "---"
  echo
  echo "总时长: ${DURATION}s ($SHOT_COUNT shots × 4s)"
} >> "$OUT_DIR/STORYBOARD.md"

# --- write SCRIPT-douyin.md ---
WORD_BUDGET=$((DURATION * 3))
{
  echo "# $SLUG — Script ($PLATFORM ${DURATION}s, $TONE + $SEASON)"
  echo
  echo "> **Voice**: $TONE"
  echo "> **Hook**: TODO (从 7 个钩子公式里选一个:三问开场/反常识/留白/故事/画面/引用/数字)"
  echo "> **Pattern**: TODO"
  echo "> **Duration**: $DURATION 秒 ($SHOT_COUNT shots × 4s)"
  echo "> **目标字数**: ~$WORD_BUDGET 字 (3 字/秒 口语节奏)"
  echo
  echo "## $PLATFORM ${DURATION} 秒脚本 (主推版本)"
  echo
  echo '```'
} > "$OUT_DIR/SCRIPT-douyin.md"

for i in $(seq 1 $SHOT_COUNT); do
  idx=$((i - 1))
  start=$(( (i - 1) * 4 ))
  end=$((i * 4))
  start_fmt=$(printf "%02d:%02d" $((start / 60)) $((start % 60)))
  end_fmt=$(printf "%02d:%02d" $((end / 60)) $((end % 60)))
  {
    echo "[$start_fmt-$end_fmt]  画面: [TODO: 峰哥在${SCENE_ARR[$idx]}做什么]"
    echo "              旁白: [TODO: 写 8-15 字旁白,用 $TONE 调性的语气]"
    echo
  } >> "$OUT_DIR/SCRIPT-douyin.md"
done

{
  echo '```'
  echo
  echo "---"
  echo
  echo "**字数**: 待填充"
  echo
  echo "**IP 圣经提醒** ($TONE 调性):"
  echo "$TONE_GUIDE"
  echo
  echo "**禁用词**: ❌ '加油' / '失败是成功之母' / '你一定可以' / 强行说教 / 感叹号堆砌 / emoji 替代思考 / '唉' 开头"
  echo
  echo "**服饰轮换**: ${OUTFITS}"
  echo "**场景轮换**: ${SCENES}"
  echo "**节气**: $SEASON"
} >> "$OUT_DIR/SCRIPT-douyin.md"

echo "✅ scaffolded: $OUT_DIR/"
echo "   STORYBOARD.md — $(grep -c '^## 镜头' "$OUT_DIR/STORYBOARD.md") shots, IP-bible pre-filled"
echo "   SCRIPT-douyin.md — $(grep -c '^\[' "$OUT_DIR/SCRIPT-douyin.md") voiceover blocks (TODO)"
echo
echo "next:"
echo "  1. 编辑 $OUT_DIR/STORYBOARD.md 填每个 shot 的'视觉:'"
echo "  2. 编辑 $OUT_DIR/SCRIPT-douyin.md 填每个 [TODO] 旁白"
echo "  3. python3 docs/panda-episode-pipeline/render_episode.py \\"
echo "       --storyboard $OUT_DIR/STORYBOARD.md \\"
echo "       --script    $OUT_DIR/SCRIPT-douyin.md \\"
echo "       --voice     $TONE \\"
echo "       --platform  $PLATFORM \\"
echo "       --out       $OUT_DIR/RENDER"
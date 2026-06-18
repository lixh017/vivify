# Install: panda-episode-pipeline

> **Audience**: humans (content lead, AI 操作手). **Not** for Claude to read —
> Claude reads `SKILL.md` (the recipe), this is for "first time using the skill"
> one-time setup.

## 5-minute setup

### 1. Get the API keys (2 minutes)

```bash
# 火山引擎 Ark (image + video) — register at console.volcengine.com/ark
# OpenAPI 开通 → 创建 API Key → 复制
export ARK_API_KEY="your-volcengine-ark-key"

# 海螺 MiniMax (TTS) — register at api.minimaxi.com
# 接口密钥 → 创建新 key → 复制
export MINIMAX_API_KEY="your-minimax-key"
```

The 火山 Ark key gives access to Seedream 4.0 (image) + Seedance 1.5-pro (video). The MiniMax key gives access to speech-02-hd (TTS) with the `male-qn-jingying` voice.

### 2. Confirm ffmpeg (1 minute)

```bash
# The pre-built 2018 ffmpeg in openclaw is what the script uses
FFMPEG=/root/.openclaw/extensions/dingtalk-connector/node_modules/@ffmpeg-installer/linux-x64/ffmpeg
$FFMPEG -version 2>&1 | head -1
# Should print: ffmpeg version N-47683-...
```

If missing: `apt-get install -y ffmpeg` (or just `brew install ffmpeg` on macOS — the script auto-detects `/usr/bin/ffmpeg` if openclaw path is absent).

### 3. Verify the API keys work (2 minutes)

```bash
# Test 火山 Ark
curl -sS -X GET "$ARK_BASE_URL/models" \
  -H "Authorization: Bearer $ARK_API_KEY" | python3 -c "
import json, sys
d = json.load(sys.stdin)
live = [m for m in d['data'] if 'status' not in m]
seedance = [m['id'] for m in live if 'seedance' in m['id']]
seedream = [m['id'] for m in live if 'seedream' in m['id']]
print(f'Seedance: {seedance}')
print(f'Seedream: {seedream}')
"
# Should list 3+ Seedance models and 3+ Seedream models

# Test 海螺 MiniMax
curl -sS -X POST "https://api.minimaxi.com/v1/t2a_v2" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $MINIMAX_API_KEY" \
  -d '{"model":"speech-02-hd","text":"嘿","voice_setting":{"voice_id":"male-qn-jingying","speed":0.85,"vol":1,"pitch":0},"audio_setting":{"sample_rate":24000,"bitrate":128000,"format":"mp3"}}' \
  -o /tmp/tts-test.json
python3 -c "
import json
d = json.load(open('/tmp/tts-test.json'))
audio = d.get('data', {}).get('audio', '')
print(f'OK: {len(bytes.fromhex(audio))} bytes audio')
"
# Should print "OK: ~50000 bytes audio"
```

### 4. Run the reference episode (5-12 minutes)

```bash
# From the repo root
export FFMPEG=/root/.openclaw/extensions/dingtalk-connector/node_modules/@ffmpeg-installer/linux-x64/ffmpeg
export OPC_RENDER_DIR=/tmp/opc-render
python3 ~/.claude/skills/panda-episode-pipeline/render_episode.py \
  --storyboard docs/showcase/panda-episode-001/STORYBOARD.md \
  --script     docs/showcase/panda-episode-001/SCRIPT-douyin.md \
  --voice      治愈 \
  --platform   抖音 \
  --target-dur 58 \
  --out        /tmp/test-render.mp4
```

The script:
1. Calls 火山 Ark for 6 images (~30s)
2. Calls 火山 Ark for 7 i2v clips (~6 min)
3. Calls 海螺 MiniMax for 7 voiceover segments (~10s)
4. Runs ffmpeg for BGM synthesis + concat + final mix (~30s)

**Output**: `/tmp/test-render.mp4` (~10MB, 58s, 720p 9:16, with TTS voiceover + BGM).

## If something fails

### "ARK_API_KEY not set"

The script reads env at startup. Make sure the env var is in your shell. For persistent env, add to `~/.bashrc` or use a `.env` file loaded by the script.

### "MiniMax returned base64 garbage"

**This is the most common gotcha.** 海螺 returns audio as **HEX** (not base64) in the `data.audio` field. The script uses `bytes.fromhex()` correctly. If you see "Audio file with ID3" but no duration, the decode was wrong.

### "Seedance 1.5-pro not found"

The script defaults to `doubao-seedance-1-5-pro-251215`. If your account doesn't have access to that model, try `doubao-seedance-1-0-pro-250528` (older, more available). Pass via `--video-model`.

### "ffmpeg 2018 syntax error"

The openclaw ffmpeg is N-47683 from 2018. Some filter options are missing (`normalize=0`, `duration=first` on amix). The script avoids these — if you customize the ffmpeg calls, stick to documented 2018-era options.

## Next steps

- Render episode #001 from `docs/showcase/panda-episode-001/` (worked example)
- Run `panda_topic` / `panda_script` MCP tools to generate a new storyboard
- Pass the new storyboard through this skill
- Repeat for episode #002, #003, ...

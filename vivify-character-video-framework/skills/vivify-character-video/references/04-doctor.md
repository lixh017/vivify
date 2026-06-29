# `vivify doctor` — Environment Diagnostics

The fastest way to catch "why is the render broken?" **before** you spend
¥30 of API calls. Run it as the **first step** of every build workflow.

## When to run

- First time the user touches this repo (`apt install ffmpeg` not done, etc.)
- After any user reports "doesn't work" / "渲染失败"
- Before any expensive `vivify episode render` call
- After a host migration or new shell session

## How to invoke

```bash
vivify doctor
```

Exits 0 if all pass; non-zero if any ✗ appears. Output is human-readable,
no JSON to parse. ~1 second to run.

## What it checks

| Check | What it looks at | What ✗ means | Fix to tell the user |
|---|---|---|---|
| ffmpeg | `which ffmpeg` or `$FFMPEG` | mux step will fail | `apt install ffmpeg` (Ubuntu) / `brew install ffmpeg` (macOS) |
| ffmpeg font | WQY Zen Hei present (for 中文 mux) | 中文 rendered as □□□ | `apt install fonts-wqy-zenhei` |
| ARK_API_KEY | env var set + non-empty | image / Seedance video gen will 403 | `export ARK_API_KEY="..."` in `~/.bashrc` |
| ARK Model access | API probe for Seedance perm | 403 on live calls | open https://console.volcengine.com/ark |
| MiniMax API key | env var set | MiniMax video + TTS will fail | `export MiniMax_API_KEY="..."` |
| Cost ledger writable | DB exists + writable | asset ledger writes will crash | `vivify db migrate` or check perms |
| Disk free | >2GB on out dir | write errors mid-render | clean `.tmp/renders/` |
| Git LFS clean | no LFS files dirty | canonical images may load 0-byte | `git lfs pull` |

## Output shape

```
vivify doctor
✓ ffmpeg: /usr/bin/ffmpeg (6.1.1)
✓ ffmpeg font: WQY Zen Hei found
✓ ARK_API_KEY: set
✓ ARK Seedance access: ok (probed)
✗ MiniMax API key: NOT SET
✓ Cost ledger: /root/.../opc.db (writable)
✓ Disk: 38.4 GB free at .tmp/renders/
✓ Git LFS: clean

1 issue found. Render will fail with MiniMax video provider.
Fix: export MiniMax_API_KEY="..." in your shell.
Exit: 1
```

## If doctor passes but render still fails

That means the failure is **runtime**, not environment. Cross-reference
`02-recovery.md` for:

- 403 / permission_denied (account perm issue, not env)
- quota_exceeded (billing, not env)
- timeout (network, retry)
- storyboard parse failed (input format, not env)

## See also

- `references/02-recovery.md` — runtime error translations
- `vivify-panda-episode-build` Step 0 (preflight) — calls doctor first
- `vivify-character-video` decision tree — "broken / first time" branch

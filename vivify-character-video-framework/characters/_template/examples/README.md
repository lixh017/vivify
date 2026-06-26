# Examples — rendered episode directories

> Each rendered episode for this IP lives in its own subdirectory here.
> Use the `--out` flag on `vivify episode render` to point the output here,
> e.g. `--out characters/{{name}}/examples/EP001/`.

## Layout

```
characters/{{name}}/examples/
├── README.md          ← (this file)
├── EP001/             ← first rendered episode
│   ├── final.mp4
│   ├── STORYBOARD.md
│   ├── SCRIPT-douyin.md
│   ├── shots/         ← per-shot jpgs + mp4s
│   ├── voiceover/     ← per-shot wav files
│   └── manifest.json  ← what was generated, cost, time
├── EP002/
│   └── ...
└── ...
```

## Naming convention

`<episode_id>/` — use a 3-digit zero-padded ID, e.g. `EP001`, `EP002`, ...

If you render an A/B variant, append the variant tag:
- `EP001-a/` — variant a
- `EP001-b/` — variant b

## What to commit

✅ Commit:
- The final `final.mp4` (or a low-res preview if storage is a concern)
- `STORYBOARD.md`, `SCRIPT-*.md` (the inputs that produced the output)
- `manifest.json` (cost, time, model versions used)
- `shots/*.jpg` and `shots/*.mp4` (the per-shot outputs)

❌ Do NOT commit:
- `voiceover/*.wav` (regeneratable, large)
- Any large intermediate frames / temp files
- API keys, secrets, signed URLs
- `.bak-*` files (use `git clean` for those)

## .gitignore recommendations

For each `EPxxx/` subdir, you can add to the parent `.gitignore`:

```gitignore
characters/{{name}}/examples/EP*/voiceover/*.wav
characters/{{name}}/examples/EP*/shots/*-thumb.jpg
```

## Indexing

Update `characters/{{name}}/README.md`'s "Examples" section with a table:

```markdown
| EP | Title | Platform | Voice | Rendered | Notes |
|---|---|---|---|---|---|
| EP001 | _t.b.w._ | 抖音 | 治愈 | 2026-06-26 | first episode, baseline |
```

## See also

- `experiments/README.md` — A/B variant episodes
- `analytics/README.md` — performance data for these episodes
- `../README.md` — the IP-level index

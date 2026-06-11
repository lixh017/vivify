# Contributing to OPC

> AI 视频创作平台 (NemoVideo hybrid mode). This is the one-page runbook for
> new contributors. Read [`README.md`](README.md) first for the high-level
> pitch and build options, then come back here for the day-to-day loop.

## 1. Prerequisites

- **Go 1.25** (the `fts5` build tag is mandatory; see Makefile)
- **Node 20** + **pnpm 9** (the monorepo is wired with `pnpm-workspace.yaml`)
- **sqlite3** CLI (only needed for ad-hoc DB inspection)
- **ANTHROPIC_API_KEY** in your env if you intend to run integration tests
  (`make test-integration`) — the regular suite runs without it

## 2. Bootstrap (5 commands, in order)

```bash
./scripts/dev.sh setup          # install Go modules + pnpm workspace deps
./scripts/dev.sh up             # boot DB + API on :8080 + web on :3000
curl -fsS http://127.0.0.1:8080/readyz
make test                       # race + fts5 test suite (~30s)
make smoke-api                  # 5-handler E2E regression on :8081
```

If `make smoke-api` returns 200 on every handler, you are good to ship
incremental changes.

## 3. Day-to-day loop

```bash
make test           # unit + integration-tagged tests with -race
make build          # apps/bin/opc-api (FTS5 enabled)
make smoke-api      # build + scripts/api-smoke.sh
make vet            # go vet -tags fts5 ./...
make fmt            # gofmt -w apps/api/**/*.go
```

Run all four before opening a PR. CI runs them again on a clean checkout.

## 4. Where things live

```
apps/
  api/                # Go HTTP server (chi router, slog, SQLite + FTS5)
    cmd/server/       # main binary
    cmd/opc-asset/    # brand-on asset CLI
    internal/handlers/# one file per route family (ai.go, batch.go, ...)
    internal/assetgen/# Profile struct + prompt builders (no model calls)
    internal/auth/    # session auth + multi-tenant scoping (Phase 2)
    internal/db/      # SQLite migrations + FTS5 schema
  web/                # Next.js 14 app router (apps/web/app/...)
  shared/             # cross-package TS types
  console/            # admin/ops console
  loadtest/           # k6 / vegeta scripts
  assets/             # generated brand assets (gitignored except manual.jpg)
scripts/              # dev.sh, api-smoke.sh, deploy.sh, healthcheck.sh
docs/                 # PHASE2-AUTH.md and the superpowers specs/
```

## 5. How to add a new AI handler

Five steps, in this order:

1. **Handler** — drop a new file in `apps/api/internal/handlers/<name>.go`
   with the chi route registration, a request/response struct, and a test
   `<name>_test.go` next to it (table-driven, see `ai_test.go` for the
   shape).
2. **Route** — wire it into the router in `internal/handlers/...` (look
   for the `routes.go` or `router.go` next to the other handlers).
3. **Assetgen hook** — if the handler emits an image or video, route
   the prompt through `internal/assetgen.NewProfile(...)` /
   `BuildPrompt(...)` so the output is brand-on. Do **not** accept raw
   user prompts as the final prompt.
4. **Smoke** — add one curl line to `scripts/api-smoke.sh` exercising the
   happy path (200 + non-empty response). The script asserts the
   contract, not the model output.
5. **Verify** — run `make smoke-api` and the `verify-project-state`
   agent before declaring done (see §9).

## 6. How to add a new IP profile

Everything lives in `apps/api/internal/assetgen/profile.go`:

1. Add a new top-level `var NewProfileV1 = Profile{...}` (do **not**
   mutate `FenggeV1` — profiles are versioned so ledger entries stay
   interpretable).
2. Add a helper next to the existing ones, e.g. `BuildNewProfilePrompt()`,
   that composes `Body + Colors + Scene` into a model-ready string.
3. Extend the consistency check in `consistency.go` with any new
   required anchors (e.g. a new signature color).
4. Add a fixture under `assetgen_test.go` so `make test` covers it.

Profiles are pure data + pure functions; they never call a model.

## 7. Testing conventions

- **Table-driven** tests with `t.Run(name, ...)` subtests. See
  `handlers/ai_test.go` for the canonical shape.
- **`-race` is mandatory** in CI and in `make test` — do not submit
  tests that only pass without the race detector.
- **FTS5 build tag** — FTS-gated tests are tagged with `//go:build fts5`
  and skipped (not failed) when the tag is absent. `make test-plain`
  exercises that skip-not-fail path.
- **Integration tests** — tagged with `//go:build integration` AND
  guarded by `ANTHROPIC_API_KEY`. When the key is missing, the suite
  skips with a clear message; it never fails the run. Default model is
  Haiku 4.5 to keep cost under a cent per run.
- **Coverage target: 80%** for any new package (see
  `common/testing.md`).

## 8. Where logs go

- **Local dev / smoke runs** — the API writes to `/tmp/opc-api-<port>.log`
  (path is logged at startup). The smoke script tails this on failure.
- **Production** — structured `slog` JSON to stdout. Every request gets
  a `request_id=...` field; pass it in `X-Request-ID` to correlate.
  LLM calls additionally carry `model=`, `tokens_in=`, `tokens_out=`,
  `cost_usd=` so the cost handler (`handlers/cost.go`) can roll up
  totals per tenant.
- **Never** log full request bodies for `/api/auth/*` — session cookies
  and registration payloads are PII.

## 9. Verify agent

Before declaring any task done, run the independent verifier — it does
**not** trust the author's self-assessment:

```bash
claude-code --agent verify-project-state
```

It re-runs the evidence commands behind each `✅` / `❌` / `❓` in
`PROJECT-STATE.md` and posts a PASS/FAIL report under
`~/.claude/audit/verify-reports/`. The author and the reviewer are
different roles; do not skip this step.

## 10. Code of Conduct

Be kind, be specific, prefer face-to-face disagreement over a thread.
This is an internal MVP; we follow the same norms as the rest of the
monorepo and have not adopted a separate CoC document yet.

---

**See also**: [`README.md`](README.md) (build options), [`docs/PHASE2-AUTH.md`](docs/PHASE2-AUTH.md)
(auth + multi-tenancy), [`scripts/api-smoke.sh`](scripts/api-smoke.sh) (regression contract).

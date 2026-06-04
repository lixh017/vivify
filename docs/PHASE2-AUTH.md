# Phase 2 — Authentication & Multi-Tenancy

> Status: **in progress** (slice 1 / 5 — auth backend). See the
> Phase 2 task list in the project tracker for the full rollout.

This document covers the operator-facing pieces of Phase 2's session
auth and multi-tenant data model. It is the runbook for:

1. The first-time user bootstrap (the system ships with **no
   register endpoint open by default**).
2. The migration of existing single-tenant Phase 1 data into the
   multi-tenant shape (`UserID` columns on the business entities).
3. The cookie / SameSite / Secure choices and their operator
   consequences.

It deliberately sits next to the code so the choices stay in sync
with the implementation; if you change a default here, change it in
the same commit.

---

## 1. First-time user bootstrap

`POST /api/auth/register` is **off by default**. The first admin
must be created out-of-band. The supported out-of-band path is a
small one-shot CLI that lives in `apps/api/cmd/seeduser` (added in
the same slice as this runbook).

```text
# Build the API binary as usual, then:
./bin/opc-api seeduser \
    --email admin@example.com \
    --name "Admin" \
    --password '<initial password>'

# Or via the docker compose stack:
docker compose run --rm api seeduser \
    --email admin@example.com \
    --name "Admin" \
    --password '<initial password>'
```

The CLI is intentionally minimal: it opens the same SQLite DB the
server uses, calls `auth.HashPassword` exactly like the handler
would, and inserts a single `users` row. There is no flag to skip
the password prompt — passwords are not echoed.

After the first user exists you have two options:

- **Open registration** by setting `REGISTRATION_ENABLED=1` in the
  API container's environment. `/api/auth/register` becomes
  callable from the login page's "Sign up" link (a follow-up UI
  slice).
- **Keep registration closed** and create every additional user via
  the same `seeduser` CLI.

We default to closed because the spec calls out that the system
has no first-run user flow of its own; the CLI is the only safe
way to land an admin before the first login UI is built.

---

## 2. Data migration backfill (Phase 1 → Phase 2)

Phase 1 was single-tenant. Every business row in `topics`,
`scripts`, `content_items`, `knowledge_docs`, and `series` was
inserted without an owner. Phase 2 adds a `user_id` column to all
five tables (see `internal/models/*.go`).

`db.Migrate` runs `gorm.AutoMigrate` at boot, which adds the
column with a default of 0 (NOT NULL but zero-valued) on existing
SQLite databases. **This is intentionally lossy**: the GORM
migration does not know which user "owns" the pre-Phase-2 data,
because the answer is "none of them do — it was single-tenant".

You have to decide what to do with that data. The two supported
options are:

### 2a. Claim every pre-Phase-2 row under the first admin

```sql
-- Run once, after the first admin has been created via `seeduser`.
-- Replace <ADMIN_USER_ID> with the value of users.id for the admin.
UPDATE topics          SET user_id = <ADMIN_USER_ID> WHERE user_id = 0;
UPDATE scripts         SET user_id = <ADMIN_USER_ID> WHERE user_id = 0;
UPDATE content_items   SET user_id = <ADMIN_USER_ID> WHERE user_id = 0;
UPDATE knowledge_docs  SET user_id = <ADMIN_USER_ID> WHERE user_id = 0;
UPDATE series          SET user_id = <ADMIN_USER_ID> WHERE user_id = 0;
```

The handlers in `internal/handlers/*` scope reads and writes to the
caller's `user_id`. After the backfill every pre-Phase-2 row is
visible only to the admin and to anyone who later logs in as the
admin's account.

### 2b. Hide pre-Phase-2 rows from everyone

If you would rather not auto-assign the admin, leave the
`user_id = 0` rows in place and let the auth filter hide them from
every account. The current handlers do **not** add an
`OR user_id = 0` clause on reads, so those rows are simply
invisible to every caller — which is the safe default if the
backfill is unknown or contested.

A future migration can surface them with a "claim" UI; the
handler's "List" path is the natural place to add a `?include=legacy`
filter when that lands.

### 2c. Verify the migration

The integration test in `internal/db/migrate_test.go` asserts the
expected `idx_<table>_user_id` indexes after `Migrate` runs. Run
the full suite with `go test -race ./...` before and after the
production migration; if the index list is missing an entry,
`TestMigrateCreatesIndexes` will name it explicitly.

For a one-shot sanity check against a live database:

```text
sqlite3 opc.db "SELECT name FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%_user_id';"
```

You should see exactly five rows:

```text
idx_topics_user_id
idx_scripts_user_id
idx_content_items_user_id
idx_knowledge_docs_user_id
idx_series_user_id
```

---

## 3. Cookie & CSRF posture

| Knob | Default | Override | Notes |
|------|---------|----------|-------|
| `opc_session` cookie name | fixed | n/a | matches the Go constant in `internal/handlers/auth.go` |
| `HttpOnly` | on | n/a | JS in the browser cannot read the token |
| `Secure` | on (production) | `OPC_INSECURE_COOKIES=1` | flip for local HTTP dev only |
| `SameSite` | `Lax` | n/a | blocks cross-site `POST` from a third-party origin |
| Path | `/api` | n/a | the token never flows to the static frontend |
| Expiry | 7 days | n/a | mirrored from `auth.sessionTTL`; server row holds the source of truth |

`SameSite=Lax` is the right default for an internal app that is
reached through a same-site login form: the cookie is sent on
top-level navigations and on the JSON API the same browser tab
hits, but a third-party form post on a different origin cannot
ride the cookie. We are explicitly **not** configuring a
`/api` CORS allow-origin for any external site; the only client is
the OPC web app served from the same origin via the nginx reverse
proxy.

If a future slice wants a public embed or a third-party
integration that needs cross-site `POST`, the right move is a
dedicated `opc_session_csrf` token (or a Bearer-only
`/api/v2/auth/token` surface), not a relaxation of the
`SameSite` policy. The current spec does not require that.

---

## 4. Open follow-ups (out of scope for this slice)

These are the items the spec called out as "concerns" and "extra"
— they are not blocking the auth backend but should be tracked
before Phase 2 ships to other environments:

- "Log out all devices" / per-device session listing. Currently
  `auth.DeleteSession` only nukes the supplied token.
- CSRF protection for non-Lax clients (see §3).
- A reverse-proxy-aware `Secure` flag: today `OPC_INSECURE_COOKIES`
  is the only toggle. A future "trust the proxy" mode would inspect
  `X-Forwarded-Proto` and flip automatically.
- Cascading delete for `user_id` removal. Today the foreign key is
  logical (no `ON DELETE CASCADE`); a future "delete my account"
  endpoint will need a follow-up migration.

---

## 5. Quick-reference

- Backend: `apps/api/internal/handlers/auth.go` (handler),
  `apps/api/internal/handlers/middleware.go` (RequireAuth),
  `apps/api/internal/auth/` (password + session helpers).
- Web UI: `apps/web/app/login/page.tsx`, `apps/web/lib/api.ts`
  (auth namespace), `apps/web/app/NavBar.tsx` (logout button).
- Env vars: `REGISTRATION_ENABLED`, `OPC_INSECURE_COOKIES`
  (see `.env.example`).
- Tests: `apps/api/internal/handlers/auth_test.go`.

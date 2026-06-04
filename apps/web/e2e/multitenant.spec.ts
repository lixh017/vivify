import { test, expect } from "@playwright/test"
import { ApiClient, newAuthedContext } from "./_helpers"

// Two-browser cross-tenant isolation test (Phase 2 Task 3 QA fix).
//
// The store-layer tests in apps/api/internal/handlers/multitenant_test.go
// prove that the API correctly scopes reads and writes by userID when
// the request goes through RequireAuth. This spec proves the SAME
// invariant at the browser level: when two real browser contexts share
// the same origin and DB, userA's data must not leak into userB's
// view. The pre-Phase-2 codebase had no such isolation at all (no
// users, no auth), so the gap we are closing is the whole multi-tenant
// story — but the two-browser shape is the most realistic expression
// of the regression class we want to prevent.

test.describe("Multi-tenant browser isolation", () => {
  // unique-per-run users so the spec is order-independent with the
  // rest of the suite and survives local re-runs without manual
  // cleanup. The `+` syntax is rejected by email validators; we use
  // dashes instead.
  const ts = Date.now()
  const userA = { email: `mta-${ts}@example.com`, password: "hunter2A!", name: "MT A" }
  const userB = { email: `mtb-${ts}@example.com`, password: "hunter2B!", name: "MT B" }

  test.beforeAll(async () => {
    // Seed two users via the public register endpoint. This
    // exercises the production code path (no DB poke) and works
    // because the Playwright config sets REGISTRATION_ENABLED=1.
    // Idempotent: if a previous run already created the user
    // (e.g. a 409), we just continue — the login below will
    // succeed for the existing account.
    const api = await ApiClient.create()
    try {
      await api.register(userA.email, userA.password, userA.name)
      await api.register(userB.email, userB.password, userB.name)
    } finally {
      await api.dispose()
    }
  })

  test("userA's topics are not visible in userB's session", async ({ browser }) => {
    // Log both users in via the API and grab their session tokens.
    // We use the API (not the UI login page) so the assertion can
    // pin the data-isolation contract independently of the login
    // form's behavior.
    const api = await ApiClient.create()
    let tokenA: string
    let tokenB: string
    try {
      tokenA = await api.login(userA.email, userA.password)
      tokenB = await api.login(userB.email, userB.password)
    } finally {
      await api.dispose()
    }

    // Spin up two independent browser contexts, each carrying one
    // user's session cookie. This is the "two logged-in browsers"
    // shape the spec promises.
    const ctxA = await newAuthedContext(browser, tokenA)
    const ctxB = await newAuthedContext(browser, tokenB)
    const pageA = await ctxA.newPage()
    const pageB = await ctxB.newPage()

    try {
      // userA creates a topic. The UI create flow is the most
      // realistic surface; the API would also work, but driving
      // the page proves the cookie-→-context chain holds end to
      // end.
      const title = `MT isolation ${ts}`
      await pageA.goto("/topics")
      await pageA.getByRole("button", { name: "+ 新建" }).click()
      await pageA.locator('[data-testid="input-test-title"]').fill(title)
      await pageA.locator('[data-testid="input-test-angle"]').fill("MT test")
      await pageA.locator('[data-testid="input-test-platform"]').selectOption("抖音")
      await pageA.locator('[data-testid="input-test-status"]').selectOption("想法")
      await pageA.locator('[data-testid="btn-submit"]').click()

      // The new card shows up for userA.
      await expect(pageA.getByText(title)).toBeVisible()

      // userB navigates to /topics in their own session.
      await pageB.goto("/topics")
      // Give the page a moment to settle (the list refetches on
      // mount), then assert the topic is NOT visible. The
      // locator-scoped assertion has a built-in auto-retry so we
      // don't need a manual wait.
      await expect(pageB.getByText(title)).toHaveCount(0)

      // And the empty state should be visible — this is the
      // friendlier failure message than "topic not found", and
      // it also pins that userB sees an empty kanban rather
      // than a 500 / login redirect.
      await expect(
        pageB.getByText(/还没有选题|空|暂无/i).first(),
      ).toBeVisible({ timeout: 5_000 })
    } finally {
      await ctxA.close()
      await ctxB.close()
    }
  })

  test("userA's API list is empty for userB's session", async () => {
    // Companion test that asserts the same invariant one level
    // lower, at the API. This catches a class of regressions the
    // browser test would miss (e.g. the web client re-fetching
    // a stale cache). It also runs in a fraction of the time of
    // the browser spec, so we get the assertion twice for cheap.
    const apiA = await ApiClient.create()
    const apiB = await ApiClient.create()
    let tokenA: string
    let tokenB: string
    try {
      tokenA = await apiA.login(userA.email, userA.password)
      tokenB = await apiB.login(userB.email, userB.password)
    } finally {
      await apiA.dispose()
      await apiB.dispose()
    }

    // userA's request context for write, userB's for read.
    const writerCtx = await (await import("@playwright/test")).request.newContext({
      baseURL: "http://localhost:48080",
      extraHTTPHeaders: { Cookie: `opc_session=${tokenA}` },
    })
    const readerCtx = await (await import("@playwright/test")).request.newContext({
      baseURL: "http://localhost:48080",
      extraHTTPHeaders: { Cookie: `opc_session=${tokenB}` },
    })

    try {
      // userA creates a topic via the API.
      const createRes = await writerCtx.post("/topics", {
        data: {
          title: `api-mt ${ts}`,
          angle: "isolation test",
          platform: "抖音",
          status: "想法",
        },
      })
      expect(createRes.ok()).toBeTruthy()

      // userB lists — must be empty. We hit /topics explicitly with
      // their cookie so the assertion doesn't depend on the cookie
      // name being opaque or the redirect chain.
      const listRes = await readerCtx.get("/topics")
      expect(listRes.ok()).toBeTruthy()
      const body = (await listRes.json()) as {
        items: { title: string; user_id: number }[]
        total: number
      }
      const leaked = body.items.filter(
        (it) => it.title === `api-mt ${ts}`,
      )
      expect(leaked).toEqual([])
      expect(body.total).toBe(0)
    } finally {
      await writerCtx.dispose()
      await readerCtx.dispose()
    }
  })
})

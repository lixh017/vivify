import { test, expect } from '@playwright/test'

// Dashboard "✨ AI 选题" button E2E coverage:
//
//   1. register (or log in, if a prior run already created) a
//      fixed e2e user so the test is deterministic across runs
//   2. carry the session cookie into a fresh browser context
//   3. open /dashboard and confirm the "✨ AI 选题" button is
//      present
//   4. click the button and wait for the TopicsModal to render
//   5. assert the modal contains at least one real topic item
//      with a non-trivial title
//
// The button is wired to POST /api/ai/topics. The default seed
// produces 5 topics; the test only needs ≥ 1 to be considered a
// pass. The API call is allowed up to 60s because the underlying
// model can take ~25s on the first cold call.

test.describe('Dashboard topics button', () => {
  let sessionCookie: string
  let context: import('@playwright/test').BrowserContext
  let page: import('@playwright/test').Page

  test.beforeAll(async ({ request, browser }) => {
    // Register (or fall back to login if 409) — fixed email for determinism.
    const email = 'e2e-dashboard-button@e2e.local'
    const password = 'e2e-test-pw'
    let cookieHeader: string | null = null
    const regRes = await request.post('/api/auth/register', {
      data: { email, password, name: 'E2E' },
    })
    if (regRes.status() === 201) {
      cookieHeader = regRes.headers()['set-cookie'] ?? null
    } else if (regRes.status() === 409) {
      // Tolerate re-run: log in with the same credentials.
      const loginRes = await request.post('/api/auth/login', {
        data: { email, password },
      })
      expect(loginRes.status()).toBe(200)
      cookieHeader = loginRes.headers()['set-cookie'] ?? null
    } else {
      throw new Error(`register returned ${regRes.status()}`)
    }
    expect(cookieHeader).not.toBeNull()
    const m = cookieHeader!.match(/opc_session=([^;]+)/)
    expect(m).not.toBeNull()
    sessionCookie = m![1]

    // Use a fresh context so we control the cookies.
    context = await browser.newContext()
    await context.addCookies([
      {
        name: 'opc_session',
        value: sessionCookie,
        domain: 'localhost',
        path: '/',
        secure: false,
        httpOnly: true,
        sameSite: 'Lax',
      },
    ])
    page = await context.newPage()
  })

  test.afterAll(async () => {
    await context?.close()
  })

  test('clicking the AI 选题 button opens the topics modal with real topics', async () => {
    // 1) Navigate to /dashboard.
    await page.goto('/dashboard')

    // 2) Confirm the page loaded with the button.
    const button = page.getByTestId('btn-generate-topics')
    await expect(button).toBeVisible()

    // 3) Click the button. The default seed produces 5 topics in the modal.
    await button.click()

    // 4) Wait for the modal to appear. The TopicsModal is rendered with
    //    data-testid="topics-modal" and contains topic items with
    //    data-testid="topic-item-N".
    const modal = page.getByTestId('topics-modal')
    await expect(modal).toBeVisible({ timeout: 30_000 })

    // 5) Wait for at least one topic to appear (the API call can take ~25s;
    //    demo mode is faster).
    const firstTopic = page.getByTestId('topic-item-0')
    await expect(firstTopic).toBeVisible({ timeout: 60_000 })

    // 6) Assert the topic has a non-trivial title element.
    const title = firstTopic.locator('h3').first()
    await expect(title).toBeVisible()
    const titleText = (await title.textContent()) ?? ''
    expect(titleText.length).toBeGreaterThan(5)
  })
})

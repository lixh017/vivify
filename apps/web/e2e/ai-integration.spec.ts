import { test, expect } from '@playwright/test'
import { ApiClient } from './_helpers'

// AI integration E2E coverage:
//
// The three AI surfaces (生成选题, 拟人化脚本, AI 复盘) all hit the
// /api/ai/* endpoints. The backend returns 503 when the Anthropic
// API key is missing (the typical dev/test environment). The UI
// must surface the error gracefully — a red error banner that
// names the failure — instead of crashing or hanging.
//
// This spec uses Playwright's `page.route()` to intercept the
// /api/ai/* requests and serve a synthetic 503 response. We do this
// instead of relying on the actual backend error so the test is
// independent of the API's behavior under that key-not-configured
// condition. The router is reset between tests via afterEach so a
// later test in the file isn't poisoned by the intercept.

test.describe('AI integration — graceful 503', () => {
  test.afterEach(async ({ context }) => {
    // Drop any unfulfilled routes after the test runs. Doing this
    // here (rather than in beforeEach) means we don't have to
    // remember to re-register the intercept in every test.
    await context.unroute('**/api/ai/**')
  })

  test('AI 生成选题: 503 → red error banner', async ({ page }) => {
    // Intercept any /api/ai/* call and return a 503 with a
    // payload that mirrors the backend's actual response shape.
    await page.route('**/api/ai/**', async (route) => {
      await route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'anthropic API key not configured' }),
      })
    })

    await page.goto('/topics')
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()

    // Open the AI panel and the form. The topics page has a
    // "AI 生成" button that toggles the inline panel.
    await page.getByRole('button', { name: 'AI 生成' }).click()

    await expect(page.locator('[data-testid="ai-panel-topics"]')).toBeVisible()

    await page.locator('[data-testid="input-test-ai-seed"]').fill('深夜 emo')
    await page.locator('[data-testid="btn-ai-topics"]').click()

    // The page surfaces the error as a red banner inside the
    // AI panel. The error message text comes from the API's
    // response body — we assert the panel still rendered (no
    // crash) and the red banner is visible.
    const panel = page.locator('[data-testid="ai-panel-topics"]')
    await expect(panel.getByText(/anthropic API key not configured|API.*failed/)).toBeVisible()
  })

  test('AI 拟人化: 503 → toast banner', async ({ page }) => {
    // Seed a script so the page has the script form available.
    // The form is empty by default; we just need the humanize
    // button to be enabled (which requires non-empty content).
    await page.route('**/api/ai/**', async (route) => {
      await route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'anthropic API key not configured' }),
      })
    })

    await page.goto('/scripts')
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()

    await page.getByRole('button', { name: '+ 新建脚本' }).click()

    // The humanize button is disabled until the content textarea
    // has text. We pre-fill the content with a long-enough string
    // that the button activates.
    await page.locator('[data-testid="input-test-content"]').fill('这是一段需要拟人化的脚本正文，e2e 测试用。')

    // The page uses window.confirm before calling the AI endpoint
    // because the action overwrites the textarea. We accept the
    // dialog automatically.
    page.on('dialog', (dialog) => dialog.accept())

    await page.locator('[data-testid="btn-ai-humanize"]').click()

    // The error surfaces as a yellow toast with the API error
    // message. The exact text comes from the API response body
    // (passed through the api.ts client's ApiError).
    await expect(page.getByText(/anthropic API key not configured|AI 拟人化失败/)).toBeVisible()
  })

  test('AI 复盘: 503 → red error banner inside modal', async ({ page }) => {
    // Seed a content item so the postmortem button has something
    // to act on. The dashboard page renders one list item per
    // content item; the AI 复盘 button is per-item.
    const api = await ApiClient.create()
    try {
      await api.wipeAll()
      await api.createContentItem({
        script_id: 0,
        platform: '抖音',
        platform_url: 'https://example.com/e2e-ai',
        performance_metrics: '播放 100',
      })
    } finally {
      await api.dispose()
    }

    await page.route('**/api/ai/**', async (route) => {
      await route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'anthropic API key not configured' }),
      })
    })

    await page.goto('/dashboard')
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()

    // Click the per-item AI 复盘 button.
    await page.locator('[data-testid="btn-ai-postmortem"]').first().click()

    // The page opens a modal with the postmortem report. On
    // failure the modal stays open and shows a red error banner.
    // We assert the error text is present somewhere on the page
    // (the modal is a fixed overlay that may not have a stable
    // role selector).
    await expect(
      page.getByText(/anthropic API key not configured|AI 复盘失败/),
    ).toBeVisible()
  })
})

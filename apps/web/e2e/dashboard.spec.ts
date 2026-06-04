import { test, expect } from '@playwright/test'
import { ApiClient } from './_helpers'

// Dashboard / 表现 page E2E coverage:
//   1. open the page
//   2. create a new 表现 record via the inline form
//   3. verify the record appears in the list with the platform and
//      performance data
//   4. delete the record via the per-item delete button
//   5. verify the record disappears from the list
//
// The dashboard form lets the user log a content item with a
// platform, platform_url, and performance_metrics. The AI 复盘
// button is covered by ai-integration.spec.ts.

test.describe('Dashboard page', () => {
  test.beforeEach(async () => {
    const api = await ApiClient.create()
    try {
      await api.wipeAll()
    } finally {
      await api.dispose()
    }
  })

  test('create 表现 record → visible → delete', async ({ page }) => {
    await page.goto('/dashboard')
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()

    await page.getByRole('button', { name: '+ 新建记录' }).click()

    const url = 'https://example.com/e2e-test-' + Date.now()
    const metrics = '播放 1.2k, 点赞 88, 评论 12'
    await page.locator('[data-testid="input-test-platform"]').selectOption('抖音')
    await page.locator('[data-testid="input-test-platform_url"]').fill(url)
    await page.locator('[data-testid="input-test-performance_metrics"]').fill(metrics)

    await page.locator('[data-testid="btn-submit"]').click()

    // The new record shows up in the list with the platform URL as
    // a link. The link's href is the unique signature we use to
    // find the row.
    const item = page.locator('[data-testid="list-item"]').filter({ hasText: url })
    await expect(item).toBeVisible()

    // The list also shows the metrics text, so we can sanity-check
    // the body rendered too.
    await expect(item).toContainText(metrics)

    await item.locator('[data-testid^="btn-delete-"]').click()

    // After delete, the empty state should render again.
    await expect(page.getByText('还没有数据')).toBeVisible()
  })
})

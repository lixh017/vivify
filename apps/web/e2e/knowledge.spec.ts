import { test, expect } from '@playwright/test'
import { ApiClient } from './_helpers'

// Knowledge base page E2E coverage:
//   1. open the page
//   2. create a new knowledge doc via the inline form
//   3. verify the doc appears in the list under its doc_type group
//   4. delete the doc via the per-doc delete button
//   5. verify the doc disappears from the list
//
// The knowledge page also has an "IP 模板" tab; that flow is
// covered here as a smoke check (create an IP, verify it shows
// up). The full IP detail drill-down is intentionally left for
// a later spec — the page is dense and the create flow alone is
// enough to validate the wiring.

test.describe('Knowledge page', () => {
  test.beforeEach(async () => {
    const api = await ApiClient.create()
    try {
      await api.wipeAll()
    } finally {
      await api.dispose()
    }
  })

  test('create doc → visible in list → delete', async ({ page }) => {
    await page.goto('/knowledge')
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()

    await page.getByRole('button', { name: '+ 新建文档' }).click()

    const title = `E2E 文档 ${Date.now()}`
    const path = `e2e/test/${Date.now()}`
    const content = 'e2e 测试用的知识库文档正文，校验渲染与删除流程。'
    await page.locator('[data-testid="input-test-title"]').fill(title)
    await page.locator('[data-testid="input-test-path"]').fill(path)
    await page.locator('[data-testid="input-test-doc_type"]').selectOption('SOP')
    await page.locator('[data-testid="input-test-content"]').fill(content)

    await page.locator('[data-testid="btn-submit"]').click()

    // Docs are grouped by doc_type. The new doc should appear in
    // the list with the title text and the doc_type group header.
    const item = page.locator('[data-testid="list-item"]').filter({ hasText: title })
    await expect(item).toBeVisible()
    await expect(page.getByRole('heading', { name: 'SOP' })).toBeVisible()

    await item.locator('[data-testid^="btn-delete-"]').click()

    // After delete, the empty state should render.
    await expect(page.getByText('还没有数据')).toBeVisible()
  })

  test('IP templates tab → create IP → visible in grid', async ({ page }) => {
    await page.goto('/knowledge')
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()

    // Switch to the IP templates tab.
    await page.getByRole('button', { name: 'IP 模板' }).click()

    await page.getByRole('button', { name: '+ 新建 IP' }).click()

    const name = `E2E IP ${Date.now()}`
    const type = `e2e-type-${Date.now()}`
    await page.locator('input[placeholder*="数字人"]').fill(type)
    await page.locator('input[placeholder*="云岚"]').fill(name)
    await page.locator('textarea').fill('e2e 测试用的 IP 简介。')

    await page.getByRole('button', { name: '创建 IP 模板' }).click()

    // The new IP card shows up in the grid with the name and
    // type as labels.
    await expect(page.getByText(name).first()).toBeVisible()
    await expect(page.getByText(type).first()).toBeVisible()
  })
})

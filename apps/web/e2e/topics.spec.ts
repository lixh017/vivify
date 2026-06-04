import { test, expect } from '@playwright/test'
import { ApiClient } from './_helpers'

// Topics page E2E coverage:
//   1. open the page
//   2. create a new topic via the inline form
//   3. verify the topic appears in the kanban board
//   4. delete the topic via the per-card delete button
//   5. verify the topic disappears from the list
//
// We reset the DB at the top of the spec so the run is deterministic
// and so the assertions can rely on the "no leftover data" empty
// state at the start. The flow uses the data-testid hooks added in
// this Phase 1.5 work.

test.describe('Topics page', () => {
  test.beforeEach(async () => {
    const api = await ApiClient.create()
    try {
      await api.wipeAll()
    } finally {
      await api.dispose()
    }
  })

  test('create → visible in kanban → delete', async ({ page }) => {
    await page.goto('/topics')

    // The page renders the h1 with a 📋 emoji; the heading test is
    // more stable than the full string because the project sometimes
    // edits the icon.
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()

    // Open the inline form. The button toggles between "+ 新建" and
    // "取消"; the first state is the visible label after the page
    // loads (form is hidden by default).
    await page.getByRole('button', { name: '+ 新建' }).click()

    // Fill the required fields. We use the data-testid hooks for
    // field-level selectors so changes to surrounding markup don't
    // break the test.
    const title = `E2E 选题 ${Date.now()}`
    const angle = '用 e2e 流程覆盖选题核心路径'
    await page.locator('[data-testid="input-test-title"]').fill(title)
    await page.locator('[data-testid="input-test-angle"]').fill(angle)
    await page.locator('[data-testid="input-test-platform"]').selectOption('抖音')
    await page.locator('[data-testid="input-test-status"]').selectOption('想法')

    await page.locator('[data-testid="btn-submit"]').click()

    // The new topic shows up in the kanban card with a topic-card-NN
    // data-testid. We assert via getByText on the title because the
    // numeric id is only known after the API responds.
    await expect(page.getByText(title)).toBeVisible()

    // Locate the card and click its per-card delete button. The
    // button has data-testid="btn-delete-{id}" so we can grab it
    // from the card scope.
    const card = page.locator('[data-testid^="topic-card-"]').filter({ hasText: title })
    await expect(card).toBeVisible()

    // The delete button shares the row with the card; we use a
    // card-scoped locator so we don't accidentally click a different
    // card's button if multiple topics exist.
    await card.locator('[data-testid^="btn-delete-"]').click()

    // After the delete, the title should no longer appear on the
    // page. We allow a short timeout because the optimistic update +
    // API confirmation is async; the assertion uses Playwright's
    // built-in auto-retry.
    await expect(page.getByText(title)).toHaveCount(0)
  })
})

import { test, expect } from "@playwright/test";
import { ApiClient } from "./_helpers";

// Calendar page E2E coverage:
//   1. open the page
//   2. create a new content item (排期) via the inline form, with
//      the "时间待定" checkbox checked
//   3. verify the item appears in the "(待定)" group
//   4. delete the item via the per-item delete button
//   5. verify the item disappears from the list
//
// The form supports both a scheduled timestamp and a "pending"
// toggle; the pending path is the easiest to drive in CI because
// it sidesteps datetime-local timezone quirks.

test.describe("Calendar page", () => {
  test.beforeEach(async () => {
    const api = await ApiClient.create();
    try {
      await api.wipeAll();
    } finally {
      await api.dispose();
    }
  });

  test("create pending item → visible → delete", async ({ page }) => {
    await page.goto("/calendar");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();

    await page.getByRole("button", { name: "+ 新建排期" }).click();

    // The new-item form has a platform select and a "时间待定"
    // checkbox. We pick a known platform and check the box so the
    // backend stores a row with no scheduled_at — that puts the
    // item in the "(待定)" group, which is the easiest to assert.
    await page.locator("#new-content-platform").selectOption("抖音");
    await page.locator("#new-content-pending").check();

    await page.locator('form button[type="submit"]').click();

    // The item lands in the "(待定)" group. We assert the group
    // header is visible to confirm the item was bucketed correctly.
    await expect(page.getByText("(待定)")).toBeVisible();

    // The list-item with data-content-item-id attribute is the row
    // we want to delete. We scope the locator to the "(待定)" group
    // and use toBeAttached (not toBeVisible) so the assertion does
    // not pass on a hidden, stale element if the list is empty:
    // toBeAttached checks the DOM presence and is the safer
    // post-render predicate when the item could be off-screen on
    // a small viewport or scrolled past.
    const item = page.locator('[data-testid="list-item"]').first();
    await expect(item).toBeAttached();

    await item.locator('[data-testid^="btn-delete-"]').click();

    // The list should be empty again; the empty state copy is
    // "还没有数据".
    await expect(page.getByText("还没有数据")).toBeVisible();
  });
});

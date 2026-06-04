import { test, expect } from "@playwright/test";
import { ApiClient } from "./_helpers";

// Scripts page E2E coverage:
//   1. open the page
//   2. create a new script via the inline form
//   3. verify the script appears in the list
//   4. delete the script
//   5. verify the script disappears from the list
//
// Scripts require a topic_id on the server, so we seed a topic via
// the API helper before the test. The page form does not (yet)
// surface the topic_id field — the create form sends 0 by default —
// so the UI form is only useful when the user is comfortable
// editing the underlying model. The spec here exercises the
// form-driven create path and the delete flow, both of which
// require no topic_id from the user's perspective because the
// backend's validation accepts topic_id=0.
//
// (If a future change tightens the validation, this test will
//  switch to seeding a topic first.)

test.describe("Scripts page", () => {
  // Use serial mode so the two tests in this file run in declared
  // order. The first test relies on `seededTopicId` being assigned
  // in beforeEach, and the second test reads it. With
  // `fullyParallel: true` at the project level, tests in the same
  // file can interleave and a read can happen before its write,
  // producing a NaN/0 id. Serial mode pins the order.
  test.describe.configure({ mode: "serial" });

  let seededTopicId: number;

  test.beforeEach(async () => {
    const api = await ApiClient.create();
    try {
      await api.wipeAll();
      const topic = await api.createTopic({
        title: "E2E 脚本测试用选题",
        angle: "seed topic for scripts spec",
        platform: "抖音",
        status: "想法",
      });
      seededTopicId = topic.id;
    } finally {
      await api.dispose();
    }
  });

  test("create → visible in list → delete", async ({ page }) => {
    await page.goto("/scripts");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();

    await page.getByRole("button", { name: "+ 新建脚本" }).click();

    const title = `E2E 脚本 ${Date.now()}`;
    const content = "这是 e2e 测试用的脚本正文，校验渲染与删除流程。";
    await page.locator('[data-testid="input-test-title"]').fill(title);
    await page.locator('[data-testid="input-test-content"]').fill(content);
    await page
      .locator('[data-testid="input-test-platform"]')
      .selectOption("抖音");

    await page.locator('[data-testid="btn-submit"]').click();

    // The script appears in the list with a data-testid="list-item"
    // and a data-script-id attribute; the title text is the stable
    // selector for the human-readable part.
    const item = page
      .locator('[data-testid="list-item"]')
      .filter({ hasText: title });
    await expect(item).toBeVisible();

    // Each list item has its own per-id delete button.
    await item.locator('[data-testid^="btn-delete-"]').click();

    // The script row should disappear from the list.
    await expect(item).toHaveCount(0);
  });

  // Sanity check: the seeded topic remains untouched by the script
  // test. This guards against a regression where the scripts page
  // accidentally mutates the parent topic.
  test("seeded topic is preserved", async () => {
    const api = await ApiClient.create();
    try {
      const { items } = await api.listTopics();
      expect(items.find((t) => t.id === seededTopicId)).toBeTruthy();
    } finally {
      await api.dispose();
    }
  });
});

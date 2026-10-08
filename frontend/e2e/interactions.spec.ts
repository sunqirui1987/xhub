import { expect, test } from "@playwright/test";
import { loginAdmin, sessionBearer, stableGoto, t, GATEWAY, watchGateway } from "./helpers";
import { recordChain } from "./report";

// Enumerate current controls, but attach assertions to each transition. Mutating
// business actions remain explicit scenarios with their own resource fixtures.
test("click every internal sidebar destination", async ({ page }) => {
  test.setTimeout(180_000);
  const guard = watchGateway(page);
  await loginAdmin(page);
  const sidebar = page.locator('[data-slot="sidebar"]');
  const hrefs = await sidebar.getByRole("link").evaluateAll((links) =>
    [...new Set(links.map((link) => link.getAttribute("href") || ""))]
      .filter((href) => href.startsWith("/") && href !== "/ui/"),
  );
  expect(hrefs.length).toBeGreaterThan(10);
  for (const href of hrefs) {
    await test.step("sidebar " + href, async () => {
      await sidebar.locator('a[href="' + href + '"]').first().click();
      await expect.poll(() => new URL(page.url()).pathname.replace(/\/$/, "")).toBe(href.replace(/\/$/, ""));
      await expect(page.getByTestId("admin-shell")).toBeVisible();
      await expect(page.getByText(/Dashboard error:|This page couldn’t load/)).toHaveCount(0);
      recordChain("sidebar pass " + href);
    });
  }
  guard.assertOk();
});

for (const route of ["/models-and-endpoints", "/guardrails", "/usage", "/logs", "/price-data", "/route-templates", "/admin-panel"]) {
  test("click each top level tab on " + route, async ({ page }) => {
    const guard = watchGateway(page);
    await loginAdmin(page);
    await stableGoto(page, route);
    const tabs = page.getByRole("tablist").first().getByRole("tab");
    const names = await tabs.allTextContents();
    for (const name of names) {
      const tab = page.getByRole("tablist").first().getByRole("tab", { name: name.trim(), exact: true });
      await test.step(name, async () => {
        await tab.click();
        await expect(tab).toHaveAttribute("aria-selected", "true");
        await expect(page.getByText(/Dashboard error:|This page couldn’t load/)).toHaveCount(0);
        recordChain("tab pass " + route + " " + name.trim());
      });
    }
    guard.assertOk();
  });
}

test("model search, pagination boundaries and cancelled create", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  await stableGoto(page, "/models-and-endpoints");
  await expect(page.getByRole("button", { name: "上一页", exact: true })).toBeDisabled();
  await page.getByRole("textbox", { name: "搜索模型" }).fill("no-such-e2e-model");
  await expect(page.getByText("没有找到匹配的模型", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "下一页", exact: true })).toBeDisabled();
  await page.getByRole("textbox", { name: "搜索模型" }).fill("gpt-4o-mini");
  await expect(page.getByRole("button", { name: "gpt-4o-mini", exact: true })).toBeVisible();
  await page.getByRole("button", { name: t("pages.models.add"), exact: true }).click();
  const form = page.locator("form").filter({ has: page.getByLabel("模型提供商 *") });
  await expect(form.getByRole("button", { name: "添加模型", exact: true })).toBeDisabled();
  await expect(form.getByLabel("上游模型 *")).toBeDisabled();
  await form.getByRole("button", { name: "取消", exact: true }).click();
  await expect(page.getByRole("textbox", { name: "搜索模型" })).toBeVisible();
  guard.assertOk();
});

test("key create cancellation leaves no saved resource", async ({ page }) => {
  await loginAdmin(page);
  const alias = "e2e-cancelled-key";
  await page.getByTestId("create-key-button").click();
  await page.getByLabel(t("Key Name")).fill(alias);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const response = await page.request.get(GATEWAY + "/key/list", {
    headers: { Authorization: "Bearer " + await sessionBearer(page) },
  });
  expect(response.ok()).toBeTruthy();
  expect(await response.text()).not.toContain(alias);
});

test("account menu opens and logout invalidates the browser session", async ({ page }) => {
  await loginAdmin(page);
  await page.getByRole("button", { name: /^Account menu/ }).click();
  await expect(page.getByTestId("sidebar-account-menu-panel")).toBeVisible();
  await page.getByRole("button", { name: t("Logout"), exact: true }).click();
  await expect(page).toHaveURL(/\/login/);
  await stableGoto(page, "/api-keys");
  await expect(page).toHaveURL(/\/login/);
});

test("retired API routes reject writes and remain absent from navigation", async ({ page }) => {
  await loginAdmin(page);
  const session = await sessionBearer(page);
  for (const route of ["/v1/agents", "/v1/memory", "/v1/workflows/runs"]) {
    const response = await page.request.post(GATEWAY + route, {
      headers: { Authorization: "Bearer " + session }, data: { name: "retired-e2e" },
    });
    expect(response.status(), route).toBe(410);
  }
  for (const route of ["agents", "skills", "memory", "workflows", "mcp-servers", "access-groups"]) {
    await expect(page.locator('a[href^="/ui/' + route + '"]')).toHaveCount(0);
  }
});

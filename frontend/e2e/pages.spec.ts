import { expect, test } from "@playwright/test";
import { DASHBOARD_PAGES, NAV_GROUPS, loginAdmin, t, uiPath, watchGateway } from "./helpers";

test.describe("console pages", () => {
  test("nav groups and Virtual Keys home", async ({ page }) => {
    await loginAdmin(page);
    for (const g of NAV_GROUPS) {
      await expect(page.getByText(g, { exact: true }).first()).toBeVisible();
    }
    await expect(page.getByText(t("nav.apiKeys")).first()).toBeVisible();
    await expect(page.getByText(t("nav.playground")).first()).toBeVisible();
  });

  test("all dashboard pages stay in the shell", async ({ page }) => {
    test.setTimeout(180_000);
    const guard = watchGateway(page);
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(`${page.url()}: ${err.message}`));
    await loginAdmin(page);
    for (const path of DASHBOARD_PAGES) {
      await page.goto(uiPath(path));
      await expect(page, path).not.toHaveURL(/\/login/);
      await expect(page.getByText("This page couldn’t load")).toHaveCount(0);
      await expect(page.getByText(/Dashboard error:/), path).toHaveCount(0, { timeout: 3_000 });
      await expect(page.getByText(t("nav.groups.gateway")).first()).toBeVisible();
    }
    expect(errors, errors.join("\n")).toEqual([]);
    guard.assertOk();
  });

  test("chat shell is not admin sidebar", async ({ page }) => {
    await loginAdmin(page);
    await page.goto(uiPath("/chat"));
    await expect(page).toHaveURL(/\/chat/, { timeout: 15_000 });
    await expect(page.locator("[data-slot=sidebar-group-label]", { hasText: t("nav.groups.gateway") })).toHaveCount(0);
  });
});

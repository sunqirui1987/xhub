import { expect, test } from "@playwright/test";
import { gotoLogin, loginAdmin, uiPath, watchGateway } from "./helpers";
import { translate } from "../src/i18n/translate";

const t = (key: string) => translate("zh-CN", key);

test.describe("login", () => {
  test("empty password shows validation", async ({ page }) => {
    await gotoLogin(page);
    const dir = process.env.E2E_SHOTS;
    if (dir) await page.screenshot({ path: `${dir}/ui-login.png` });
    await page.getByPlaceholder(t("login.usernamePlaceholder")).fill("admin");
    await page.getByRole("button", { name: t("login.submit"), exact: true }).click();
    await expect(page.getByText(t("login.passwordRequired"))).toBeVisible();
    await expect(page).toHaveURL(/\/login/);
  });

  test("wrong password shows error", async ({ page }) => {
    const guard = watchGateway(page);
    await gotoLogin(page);
    await page.getByPlaceholder(t("login.usernamePlaceholder")).fill("admin");
    await page.getByPlaceholder(t("login.passwordPlaceholder")).fill("not-the-master");
    await page.getByRole("button", { name: t("login.submit"), exact: true }).click();
    await expect(page.getByText(/Invalid credentials|login failed|UI_USERNAME/i)).toBeVisible();
    await expect(page).toHaveURL(/\/login/);
    guard.assertOk();
  });

  test("admin + master key lands on Virtual Keys", async ({ page }) => {
    await loginAdmin(page);
    const dir = process.env.E2E_SHOTS;
    if (dir) await page.screenshot({ path: `${dir}/ui-keys.png` });
    await expect(page.getByText(t("nav.groups.gateway"), { exact: true }).first()).toBeVisible();
    await expect(page.getByRole("button", { name: t("pages.apiKeys.create") })).toBeVisible();
  });

  test("unauthenticated dashboard redirects to login", async ({ page }) => {
    await page.goto(uiPath("/api-keys"));
    await expect(page).toHaveURL(/\/login/, { timeout: 15_000 });
    await expect(page.getByRole("heading", { name: t("login.title") })).toBeVisible();
  });

  test("SSO button is present", async ({ page }) => {
    await gotoLogin(page);
    await expect(page.getByRole("button", { name: t("login.sso") })).toBeVisible();
  });
});

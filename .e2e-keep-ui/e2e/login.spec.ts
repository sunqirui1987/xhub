import { expect, test } from "@playwright/test";
import { gotoLogin, loginAdmin, watchGateway } from "./helpers";

test.describe("login", () => {
  test("empty password shows validation", async ({ page }) => {
    await gotoLogin(page);
    const dir = process.env.E2E_SHOTS;
    if (dir) await page.screenshot({ path: `${dir}/ui-login.png` });
    await page.getByLabel("Password").fill("");
    await page.getByRole("button", { name: "Login", exact: true }).click();
    await expect(page.locator(".err")).toContainText("Please enter your username / password");
    await expect(page).toHaveURL(/\/login/);
  });

  test("wrong password shows error envelope", async ({ page }) => {
    const guard = watchGateway(page);
    await gotoLogin(page);
    await page.getByLabel("Password").fill("not-the-master");
    await page.getByRole("button", { name: "Login", exact: true }).click();
    await expect(page.locator(".err")).toContainText(/Invalid credentials used to access UI/i);
    await expect(page).toHaveURL(/\/login/);
    guard.assertOk();
  });

  test("admin + master key lands on Virtual Keys", async ({ page }) => {
    await loginAdmin(page);
    const dir = process.env.E2E_SHOTS;
    if (dir) await page.screenshot({ path: `${dir}/ui-keys.png` });
    await expect(page.getByText("AI GATEWAY")).toBeVisible();
    await expect(page.getByRole("link", { name: "Virtual Keys" })).toBeVisible();
    await expect(page.getByRole("button", { name: "+ Create New Key" })).toBeVisible();
  });

  test("unauthenticated dashboard redirects to login", async ({ page }) => {
    await page.goto("/api-keys");
    await expect(page).toHaveURL(/\/login/, { timeout: 15_000 });
    await expect(page.getByRole("heading", { name: "XHub" })).toBeVisible();
  });

  test("ui-config is fetched and SSO button is present", async ({ page }) => {
    const cfg = page.waitForResponse((r) => r.url().includes("/.well-known/litellm-ui-config") && r.status() === 200);
    await gotoLogin(page);
    await cfg;
    await expect(page.getByRole("button", { name: "Login with SSO" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Login with SSO" })).toBeEnabled();
    await expect(page.getByText(/Username is/)).toBeVisible();
    await expect(page.getByText(/MASTER_KEY/)).toBeVisible();
    await expect(page.locator(".hint")).not.toHaveText(/Password = master key/);
  });
});

import { expect, test } from "@playwright/test";
import { watchGateway } from "./helpers";

test("SSO button exchanges ?code= and reaches Virtual Keys", async ({ page }) => {
  const guard = watchGateway(page);
  await page.goto("/login");
  await expect(page.getByRole("button", { name: "Login with SSO" })).toBeEnabled();
  await page.getByRole("button", { name: "Login with SSO" }).click();
  await expect(page.getByRole("heading", { name: "Virtual Keys" })).toBeVisible();
  const role = await page.evaluate(() => localStorage.getItem("xhub_role"));
  expect(role).toBe("proxy_admin");
  const token = await page.evaluate(() => localStorage.getItem("xhub_token"));
  expect(token || "").toMatch(/^sess-/);
  guard.assertOk();
});

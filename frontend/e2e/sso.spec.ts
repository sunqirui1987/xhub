import { expect, test } from "@playwright/test";
import { gotoLogin, t } from "./helpers";

test("SSO button is enabled when configured", async ({ page }) => {
  await gotoLogin(page);
  const btn = page.getByRole("button", { name: t("login.sso") });
  await expect(btn).toBeVisible();
});

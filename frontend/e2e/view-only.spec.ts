import { expect, test } from "@playwright/test";
import { MASTER, login, loginAdmin, t } from "./helpers";

test.describe("view-only", () => {
  test("hides Playground and Create New Key", async ({ page }) => {
    await loginAdmin(page);
    await page.waitForLoadState("domcontentloaded");
    const cookies = await page.context().cookies();
    const token = cookies.find((c) => c.name === "token")?.value;
    // JWT key claim is the sess- used as Bearer.
    const payload = JSON.parse(Buffer.from(token!.split(".")[1], "base64url").toString());
    const sess = payload.key as string;
    const res = await page.request.post("http://127.0.0.1:4000/user/new", {
      headers: { Authorization: `Bearer ${sess}`, "Content-Type": "application/json" },
      data: {
        user_email: "viewer@xhub.local",
        password: "viewer-pass",
        user_role: "proxy_admin_viewer",
      },
    });
    expect(res.ok()).toBeTruthy();
    await page.evaluate(() => {
      document.cookie = "token=; path=/; max-age=0";
      document.cookie = "token=; path=/ui; max-age=0";
      sessionStorage.removeItem("token");
    });
    await login(page, "viewer@xhub.local", "viewer-pass");
    await expect(page.getByText(t("nav.apiKeys")).first()).toBeVisible({ timeout: 20_000 });
    await expect(page.getByRole("link", { name: t("nav.playground"), exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: t("pages.apiKeys.create") })).toHaveCount(0);
  });
});

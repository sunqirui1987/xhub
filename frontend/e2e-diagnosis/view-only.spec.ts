import { expect, test } from "@playwright/test";
import { GATEWAY, login, loginAdmin, sessionBearer, t, watchGateway } from "./helpers";

test("ordinary member cannot perform platform writes", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + await sessionBearer(page) };
  const created = await page.request.post(GATEWAY + "/user/new", {
    headers, data: { user_email: "member-permissions@xhub.local", password: "member-pass", user_role: "user" },
  });
  expect(created.ok(), await created.text()).toBeTruthy();
  await login(page, "member-permissions@xhub.local", "member-pass");
  await expect(page.getByText(t("nav.apiKeys")).first()).toBeVisible();
  await expect(page.locator('a[href="/ui/admin-panel"]')).toHaveCount(0);
  await expect(page.locator('a[href="/ui/models-and-endpoints"]')).toHaveCount(0);
  const memberHeaders = { Authorization: "Bearer " + await sessionBearer(page) };
  for (const [path, data] of [
    ["/model/new", { model_name: "forbidden-model", litellm_params: { model: "openai/gpt-4o-mini" } }],
    ["/organization/new", { organization_alias: "forbidden-org" }],
    ["/user/new", { user_email: "forbidden@example.com" }],
  ] as const) {
    const denied = await page.request.post(GATEWAY + path, { headers: memberHeaders, data });
    expect(denied.status(), path).toBe(403);
  }
  guard.assertOk();
});

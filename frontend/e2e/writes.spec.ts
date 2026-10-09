import { chooseKeyTeam, chooseOrganization } from "./helpers";
import { GATEWAY, UPSTREAM } from "./helpers";
import { expect, test, type Page } from "@playwright/test";
import { loginAdmin, stableGoto, t, uiPath, watchGateway } from "./helpers";

async function sessionFrom(page: Page): Promise<string> {
  const cookies = await page.context().cookies();
  const token = cookies.find((c) => c.name === "token")?.value;
  return JSON.parse(Buffer.from(token!.split(".")[1], "base64url").toString()).key as string;
}

async function pickOption(page: Page, option: string | RegExp) {
  const loc = page.getByRole("option", { name: option });
  if (await loc.count()) {
    await loc.first().click();
    return;
  }
  await page.getByText(option).first().click();
}

test("virtual key update, regenerate, block, and delete", async ({ page }) => {
  test.setTimeout(120_000);
  const guard = watchGateway(page);
  await loginAdmin(page);
  const alias = "e2e-key-lifecycle";
  const renamed = "e2e-key-renamed";
  await page.getByTestId("create-key-button").click();
  await chooseKeyTeam(page);
  await page.getByLabel(t("Key Name")).fill(alias);
  const generated = page.waitForResponse((res) => new URL(res.url()).pathname === "/key/generate" && res.request().method() === "POST");
  await page.getByRole("button", { name: t("pages.apiKeys.createSubmit"), exact: true }).click();
  const createdKey = await generated;
  expect(createdKey.status(), "UI key generation accepts the selected team").toBe(200);
  expect(createdKey.request().postDataJSON().team_id, "UI submits a team for the new key").toBeTruthy();
  await expect(page.getByRole("dialog").locator("pre").filter({ hasText: /sk-/ })).toBeVisible({ timeout: 15_000 });
  const oldSecret = (await page.getByRole("dialog").locator("pre").filter({ hasText: /sk-/ }).innerText()).trim();
  const invoke = (key: string) => page.request.post(GATEWAY + "/v1/chat/completions", {
    headers: { Authorization: "Bearer " + key },
    data: { model: "gpt-4o-mini", messages: [{ role: "user", content: "key lifecycle" }] },
  });
  expect((await invoke(oldSecret)).status()).toBe(200);
  await page.keyboard.press("Escape");
  await page.getByText(alias, { exact: true }).first().click();
  await expect(page.getByRole("heading", { name: alias })).toBeVisible({ timeout: 15_000 });

  await page.getByRole("tab", { name: t("Settings") }).click();
  await page.getByRole("button", { name: t("Edit Settings") }).click();
  await page.getByRole("textbox", { name: t("Key Alias") }).fill(renamed);
  const saved = page.waitForResponse((res) => res.url().includes("/key/update") && res.request().method() === "POST");
  await page
    .getByRole("button", { name: t("Save Changes") })
    .last()
    .click();
  expect((await saved).ok()).toBeTruthy();
  await expect(page.getByText(renamed).first()).toBeVisible({ timeout: 15_000 });

  await page.getByRole("button", { name: t("Regenerate Key") }).click();
  const rotated = page.waitForResponse(res => res.url().includes("/regenerate") && res.request().method() === "POST");
  await page.getByRole("button", { name: t("Regenerate"), exact: true }).click();
  const freshSecret = (await (await rotated).json()).key as string;
  expect(freshSecret).not.toBe(oldSecret);
  expect((await invoke(oldSecret)).status()).toBe(401);
  expect((await invoke(freshSecret)).status()).toBe(200);
  await expect(page.getByRole("dialog").getByText(/sk-/).first()).toBeVisible({ timeout: 15_000 });
  await page
    .getByRole("dialog")
    .getByRole("button", { name: t("Close"), exact: true })
    .first()
    .click();

  await page.getByRole("button", { name: t("More key actions") }).click();
  await page.getByRole("menuitem", { name: t("Block Key") }).click();
  await page.getByRole("button", { name: t("Block"), exact: true }).click();
  await expect(page.getByText(t("Blocked")).first()).toBeVisible({ timeout: 15_000 });
  expect([401, 403]).toContain((await invoke(freshSecret)).status());
  await page.getByRole("button", { name: t("More key actions") }).click();
  await page.getByRole("menuitem", { name: t("Unblock Key") }).click();
  await page.getByRole("button", { name: t("Unblock"), exact: true }).click();
  await expect.poll(async () => (await invoke(freshSecret)).status()).toBe(200);

  await page.getByRole("button", { name: t("More key actions") }).click();
  await page.getByRole("menuitem", { name: t("Delete Key") }).click();
  await page.getByPlaceholder(renamed).fill(renamed);
  await page.getByRole("button", { name: t("common.delete") }).click();
  await expect(page.getByText(renamed)).toHaveCount(0, { timeout: 15_000 });
  expect((await invoke(freshSecret)).status()).toBe(401);
  guard.assertOk();
});

/**
 * 用途：验证显式 OpenAI 协议模型可测试连接、改名、停启和删除。
 * 前置条件：通过后台创建指向本地 fake upstream 的模型；验证结果：每次页面操作均落到真实后台。
 * 清理方式：流程末尾删除模型，E2E 隔离数据库也会在进程退出时移除。
 */
test("model update, test connection, and delete", async ({ page }) => {
  test.setTimeout(120_000);
  const guard = watchGateway(page);
  await loginAdmin(page);
  const bearer = await (async () => {
    const cookies = await page.context().cookies();
    const token = cookies.find((c) => c.name === "token")?.value;
    return JSON.parse(Buffer.from(token!.split(".")[1], "base64url").toString()).key as string;
  })();
  const created = await page.request.post(`${GATEWAY}/model/new`, {
    headers: { Authorization: `Bearer ${bearer}`, "Content-Type": "application/json" },
    data: {
      model_name: "e2e-model-ops",
      model_info: { transport: "bypass_openai_chat", endpoint_types: ["chat"] },
      litellm_params: { model: "openai/gpt-4o-mini", custom_llm_provider: "openai", api_key: "sk-fake", api_base: UPSTREAM, input_cost_per_token: 0.00000015, output_cost_per_token: 0.0000006 },
    },
  });
  expect(created.ok(), await created.text()).toBeTruthy();
  await stableGoto(page, "/models-and-endpoints");
  if (page.url().includes("/login")) {
    await loginAdmin(page);
    await stableGoto(page, "/models-and-endpoints");
  }
  await page.getByRole("tab", { name: t("pages.models.all") }).click();
  await expect(page.getByText("e2e-model-ops").first()).toBeVisible({ timeout: 15_000 });
  await page.getByRole("button", { name: "e2e-model-ops", exact: true }).click();
  await page.getByRole("button", { name: "测试连接", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "连接正常，模型已响应。" })).toBeVisible();
  await page.getByRole("button", { name: "编辑模型", exact: true }).click();
  await page.getByLabel("对外模型名称 *").fill("e2e-model-renamed");
  const updated = page.waitForResponse(
    (response) =>
      response.request().method() === "PATCH" &&
      new URL(response.url()).pathname.startsWith("/model/") &&
      new URL(response.url()).pathname.endsWith("/update"),
  );
  await page.getByRole("button", { name: "保存修改", exact: true }).click();
  expect((await updated).ok()).toBeTruthy();
  await expect(page.getByRole("heading", { name: "e2e-model-renamed", exact: true })).toBeVisible();
  await page.getByRole("switch", { name: "e2e-model-renamed 启用模型" }).click();
  await expect(page.getByRole("switch", { name: "e2e-model-renamed 启用模型" })).not.toBeChecked();
  await expect(page.getByRole("button", { name: "测试连接", exact: true })).toBeDisabled();
  await page.getByRole("switch", { name: "e2e-model-renamed 启用模型" }).click();
  await expect(page.getByRole("switch", { name: "e2e-model-renamed 启用模型" })).toBeChecked();
  await page.getByRole("button", { name: "删除", exact: true }).click();
  await page.getByRole("dialog").getByPlaceholder("e2e-model-renamed").fill("e2e-model-renamed");
  await page.getByRole("dialog").getByRole("button", { name: "删除", exact: true }).click();
  await expect(page.getByRole("button", { name: "e2e-model-renamed", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "e2e-model-ops", exact: true })).toHaveCount(0);
  guard.assertOk();
});

test("team member add is listed", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  await page.goto(uiPath("/teams"));
  await page.getByTestId("create-team-button").click();
  await chooseOrganization(page);
  await page.getByTestId("team-name-input").fill("e2e-member-team");
  await page.getByTestId("create-team-submit").click();
  await expect(page.getByText("e2e-member-team").first()).toBeVisible({ timeout: 15_000 });
  await page.getByText("e2e-member-team", { exact: true }).first().click();
  await page.getByRole("tab", { name: t("Members") }).click();
  const member = await page.request.post(`${GATEWAY}/user/new`, {
    headers: { Authorization: `Bearer ${await sessionFrom(page)}`, "Content-Type": "application/json" },
    data: { user_id: "e2e-member", user_email: "e2e-member@example.com", user_role: "user", password: "e2e-member-password" },
  });
  expect(member.ok(), await member.text()).toBeTruthy();
  await page.getByRole("button", { name: t("Add Member") }).click();
  const memberDialog = page.getByRole("dialog");
  await expect(memberDialog.getByRole("heading", { name: /成员/ })).toBeVisible();
  const emailSearch = memberDialog.getByRole("combobox", { name: t("Email") });
  await emailSearch.click();
  await emailSearch.fill("e2e-member@example.com");
  await page.getByRole("option", { name: "e2e-member@example.com" }).click();
  await memberDialog.getByRole("button", { name: t("Add Member") }).click();
  await expect(page.getByText("e2e-member@example.com").first()).toBeVisible({ timeout: 15_000 });
  guard.assertOk();
});

test("router fallback update lists the mapping", async ({ page }) => {
  test.setTimeout(90_000);
  const guard = watchGateway(page);
  await loginAdmin(page);
  const bearer = await (async () => {
    const cookies = await page.context().cookies();
    const token = cookies.find((c) => c.name === "token")?.value;
    return JSON.parse(Buffer.from(token!.split(".")[1], "base64url").toString()).key as string;
  })();
  const created = await page.request.post(`${GATEWAY}/model/new`, {
    headers: { Authorization: `Bearer ${bearer}`, "Content-Type": "application/json" },
    data: {
      model_name: "e2e-fallback-model",
      model_info: { transport: "bypass_openai_chat", endpoint_types: ["chat"] },
      litellm_params: { model: "openai/gpt-4o-mini", api_key: "sk-fake", api_base: UPSTREAM, input_cost_per_token: 0.00000015, output_cost_per_token: 0.0000006 },
    },
  });
  expect(created.ok(), await created.text()).toBeTruthy();
  await stableGoto(page, "/route-templates");
  if (page.url().includes("/login")) {
    await loginAdmin(page);
    await stableGoto(page, "/route-templates");
  }
  const platformRow = page.locator('[data-slot="card"]').filter({ has: page.getByRole("heading", { name: t("pages.routeTemplates.platformDefault"), exact: true }) });
  await platformRow.getByRole("button", { name: t("pages.routeTemplates.edit") }).click();
  await page.getByRole("tab", { name: t("pages.routeTemplates.advancedSettings") }).click();
  await page.getByRole("combobox", { name: t("pages.routeTemplates.advanced.addConfiguration") }).selectOption("failureFallbacks");
  await page.getByRole("button", { name: t("pages.routeTemplates.advanced.addConfiguration") }).click();
  await page.getByPlaceholder(t("Select primary model")).first().click();
  await pickOption(page, "gpt-4o-mini");
  await page.getByPlaceholder(t("Select fallback models to add...")).first().click();
  await page.getByRole("option", { name: "e2e-fallback-model" }).click();
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: t("Save"), exact: true }).click();
  await expect(page.getByText(t("pages.routeTemplates.platformSaved"))).toBeVisible();
  await page.reload();
  await platformRow.getByRole("button", { name: t("pages.routeTemplates.edit") }).click();
  await page.getByRole("tab", { name: t("pages.routeTemplates.advancedSettings") }).click();
  await expect(page.getByRole("list").filter({ has: page.getByText("e2e-fallback-model", { exact: true }) }).getByText("e2e-fallback-model", { exact: true })).toBeVisible();
  guard.assertOk();
});

test("admin panel saves prompt storage and hides the unused settings", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  await page.goto(uiPath("/admin-panel"));
  await expect(page.getByText(t("Logging Settings"))).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("tab", { name: t("SSO Settings") })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: t("Security Settings") })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: "SCIM" })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: t("Hashicorp Vault") })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: t("CyberArk Conjur") })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: t("Plugins") })).toHaveCount(0);
  const promptSwitch = page.getByRole("switch");
  await promptSwitch.click();
  await page.getByRole("button", { name: t("Save Settings") }).click();
  await expect(page.getByText(t("Spend logs settings updated successfully"))).toBeVisible({ timeout: 15_000 });
  guard.assertOk();
});

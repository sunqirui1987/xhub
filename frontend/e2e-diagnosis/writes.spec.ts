import { chooseKeyTeam, chooseOrganization } from "./helpers";
import { GATEWAY, UPSTREAM } from "./helpers";
import { expect, test, type Page } from "@playwright/test";
import { loginAdmin, stableGoto, t, uiPath, watchGateway } from "./helpers";

async function sessionFrom(page: Page): Promise<string> {
  const cookies = await page.context().cookies();
  const token = cookies.find((c) => c.name === "token")?.value;
  return JSON.parse(Buffer.from(token!.split(".")[1], "base64url").toString()).key as string;
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
  const generated = page.waitForResponse(
    (res) => new URL(res.url()).pathname === "/key/generate" && res.request().method() === "POST",
  );
  await page.getByRole("button", { name: t("pages.apiKeys.createSubmit"), exact: true }).click();
  const createdKey = await generated;
  expect(createdKey.status(), "UI key generation accepts the selected team").toBe(200);
  expect(createdKey.request().postDataJSON().team_id, "UI submits a team for the new key").toBeTruthy();
  await expect(page.getByRole("dialog").locator("pre").filter({ hasText: /sk-/ })).toBeVisible({ timeout: 15_000 });
  const oldSecret = (await page.getByRole("dialog").locator("pre").filter({ hasText: /sk-/ }).innerText()).trim();
  const invoke = (key: string) =>
    page.request.post(GATEWAY + "/v1/chat/completions", {
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
  const rotated = page.waitForResponse((res) => res.url().includes("/regenerate") && res.request().method() === "POST");
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
      litellm_params: {
        model: "openai/gpt-4o-mini",
        custom_llm_provider: "openai",
        api_key: "sk-fake",
        api_base: UPSTREAM,
        input_cost_per_token: 0.00000015,
        output_cost_per_token: 0.0000006,
      },
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
    data: {
      user_id: "e2e-member",
      user_email: "e2e-member@example.com",
      user_role: "user",
      password: "e2e-member-password",
    },
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

/** 验证模板故障转移弹窗的保存、刷新和真实绑定执行；前置隔离网关及本地上游。
 * 自建主模型与备用模型，主模型返回500；断言模板正文、备用响应和账单，finally解除绑定并清除资源。 */
test("router fallback update lists the mapping", async ({ page }) => {
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + (await sessionFrom(page)) };
  const suffix = Date.now().toString();
  const primary = "template-primary-" + suffix,
    backup = "template-backup-" + suffix;
  const templateName = "template-fallback-" + suffix;
  let templateID = "",
    key = "";
  try {
    for (const [name, model] of [
      [primary, "e2e-fallback-500-" + suffix],
      [backup, "custom/template-backup"],
    ]) {
      const result = await page.request.post(GATEWAY + "/model/new", {
        headers,
        data: {
          model_name: name,
          model_info: {
            id: "dep-" + name,
            transport: "bypass_openai_chat",
            endpoint_types: ["chat", "bypass:openai-chat"],
            pricing_source: "manual",
          },
          litellm_params: {
            model,
            custom_llm_provider: "custom",
            api_key: "sk-fake",
            api_base: UPSTREAM,
            input_cost_per_token: 0.000001,
            output_cost_per_token: 0.000002,
          },
        },
      });
      expect(result.status(), await result.text()).toBe(200);
    }
    await stableGoto(page, "/route-templates");
    await page
      .getByRole("button", { name: t("pages.routeTemplates.create"), exact: true })
      .first()
      .click();
    await page.getByLabel(t("pages.routeTemplates.name"), { exact: true }).fill(templateName);
    await page.getByRole("tab", { name: "故障转移", exact: true }).click();
    await page.getByRole("button", { name: "添加故障转移", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "配置模板故障转移", exact: true });
    await dialog.getByLabel("故障转移主模型", { exact: true }).selectOption(primary);
    await dialog.getByRole("button", { name: "添加回退目标", exact: true }).click();
    await dialog.getByLabel("回退目标 1", { exact: true }).selectOption(backup);
    await dialog.getByRole("button", { name: "保存到模板", exact: true }).click();
    const saving = page.waitForResponse(
      (r) => new URL(r.url()).pathname === "/route_template/new" && r.request().method() === "POST",
    );
    await page.getByRole("button", { name: "保存模板", exact: true }).click();
    const saved = await saving;
    expect(saved.status(), await saved.text()).toBe(200);
    templateID = (await saved.json()).id;
    expect(templateID).toBeTruthy();
    const stored = await page.request.get(GATEWAY + "/route_template/" + templateID, { headers });
    expect(stored.status()).toBe(200);
    expect((await stored.json()).body.fallbacks).toEqual([{ [primary]: [backup] }]);
    await page.reload();
    const row = page.getByRole("row").filter({ has: page.getByText(templateName, { exact: true }) });
    await row.getByRole("button", { name: t("pages.routeTemplates.edit"), exact: true }).click();
    await page.getByRole("tab", { name: "故障转移", exact: true }).click();
    await expect(page.getByRole("region", { name: "模板故障转移", exact: true })).toContainText("1. " + backup);
    const teams = await (await page.request.get(GATEWAY + "/v2/team/list?page=1&page_size=500", { headers })).json();
    const teamID = teams.teams.find((row: any) => row.team_alias === "e2e-fixture-team").team_id;
    const jwt = (await page.context().cookies()).find((c) => c.name === "token")!.value;
    const userID = JSON.parse(Buffer.from(jwt.split(".")[1], "base64url").toString()).user_id;
    const minted = await page.request.post(GATEWAY + "/key/generate", {
      headers,
      data: {
        key_alias: templateName,
        key_type: "llm_api",
        team_id: teamID,
        user_id: userID,
        route_template_id: templateID,
      },
    });
    expect(minted.status(), await minted.text()).toBe(200);
    key = (await minted.json()).key;
    const response = await page.request.post(GATEWAY + "/bypass/openai/v1/chat/completions", {
      headers: { Authorization: "Bearer " + key },
      data: { model: primary, messages: [{ role: "user", content: "template fallback" }] },
    });
    expect(response.status(), await response.text()).toBe(200);
    expect((await response.json()).model).toBe("custom/template-backup");
    const callID = response.headers()["x-litellm-call-id"];
    await expect
      .poll(async () => {
        const log = await page.request.get(GATEWAY + "/spend/logs/ui/" + callID, { headers });
        return log.ok() ? Number((await log.json()).spend) : 0;
      })
      .toBeGreaterThan(0);
  } finally {
    if (key) {
      expect(
        (await page.request.post(GATEWAY + "/key/update", { headers, data: { key, route_template_id: "" } })).status(),
      ).toBe(200);
      expect((await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys: [key] } })).status()).toBe(200);
    }
    if (templateID)
      expect(
        (await page.request.post(GATEWAY + "/route_template/" + templateID + "/delete", { headers })).status(),
      ).toBe(200);
    for (const name of [primary, backup]) {
      const deleted = await page.request.post(GATEWAY + "/model/delete", { headers, data: { id: "dep-" + name } });
      expect([200, 404]).toContain(deleted.status());
    }
  }
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

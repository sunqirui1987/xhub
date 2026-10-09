import { expect, test, type Locator, type Page } from "@playwright/test";
import { GATEWAY, MASTER, loginAdmin, stableGoto, t, watchGateway } from "./helpers";

/** 用途：打开护栏管理标签；参数：page 页面、name 标签名称；返回：标签内容完成切换。 */
async function openTab(page: Page, name: string) {
  await stableGoto(page, "/guardrails");
  await expect(page.getByRole("heading", { name: t("护栏管理"), exact: true })).toBeVisible();
  await page.getByRole("tab", { name, exact: true }).click();
}

/** 用途：打开本地规则编辑器；参数：page 管理员页面；返回：编辑弹窗定位器。 */
async function openLocal(page: Page) {
  await openTab(page, t("pages.guardrails.title"));
  await page.getByRole("button", { name: t("pages.guardrails.create"), exact: true }).click();
  await page.getByRole("menuitem", { name: "关键词 / 正则护栏", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("heading", { name: "创建关键词 / 正则护栏", exact: true })).toBeVisible();
  return dialog;
}

/** 用途：验证下拉可展开且只有 pre_call 可用；参数：page 页面、dialog 编辑弹窗；返回：完成断言。 */
async function assertMode(page: Page, dialog: Locator) {
  await dialog.getByRole("combobox", { name: "执行模式", exact: true }).click();
  const list = page.getByRole("listbox");
  await expect(list).toBeVisible();
  await expect(list.getByRole("option", { name: "调用前（请求） · pre_call", exact: true })).toBeEnabled();
  for (const name of [
    "调用中 · during_call（暂不支持）",
    "调用后（响应） · post_call（暂不支持）",
    "MCP 工具调用前 · pre_mcp_call（暂不支持）",
  ]) {
    await expect(list.getByRole("option", { name, exact: true })).toHaveAttribute("aria-disabled", "true");
  }
  await page.keyboard.press("Escape");
}

/** 用途：调用真实网关聊天数据面；参数：page 登录页面、body 请求正文；返回：响应和 JSON。 */
async function chat(page: Page, body: Record<string, unknown>) {
  const response = await page.request.post(GATEWAY + "/v1/chat/completions", {
    headers: { Authorization: "Bearer " + MASTER, "Content-Type": "application/json" },
    data: body,
  });
  return { response, json: await response.json() };
}

/** 用途：清理本测试产生的规则；参数：page 页面、names 精确名称列表；返回：清理完成。 */
async function cleanup(page: Page, names: string[]) {
  const response = await page.request.get(GATEWAY + "/guardrails/list", {
    headers: { Authorization: "Bearer " + MASTER },
  });
  if (!response.ok()) return;
  const body = await response.json();
  for (const row of Array.isArray(body.guardrails) ? body.guardrails : []) {
    if (names.includes(row.guardrail_name) && row.guardrail_id) {
      await page.request.delete(GATEWAY + "/guardrails/" + encodeURIComponent(row.guardrail_id), {
        headers: { Authorization: "Bearer " + MASTER },
      });
    }
  }
}

test.describe("新护栏逻辑完整 E2E", () => {
  test.beforeEach(async ({ page }) => {
    await loginAdmin(page);
  });
  test.afterEach(async ({ page }) => {
    await cleanup(page, [
      "e2e-guardrails-local",
      "e2e-guardrails-default",
      "e2e-guardrails-xgo",
      "e2e-guardrails-invalid",
    ]);
  });

  test("花园展示创建入口、支持状态和提交护栏空态", async ({ page }) => {
    const guard = watchGateway(page);
    await openTab(page, t("pages.guardrails.garden"));
    await expect(page.getByRole("heading", { name: "选择护栏实现", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: /关键词 \/ 正则护栏/ })).toBeVisible();
    await expect(page.getByRole("button", { name: /XGo 自定义护栏/ })).toBeVisible();
    await expect(page.getByText("外部服务目录", { exact: true })).toBeVisible();
    await page.getByLabel("搜索护栏服务", { exact: true }).fill("CrowdStrike");
    await expect(page.getByRole("button", { name: /CrowdStrike/ })).toContainText("尚未移植");
    await page.getByRole("tab", { name: t("pages.guardrails.submitted"), exact: true }).click();
    await expect(page.getByText("Cannot read properties of undefined", { exact: false })).toHaveCount(0);
    guard.assertOk();
  });

  test("关键词正则完成草稿调试、保存、编辑、真实拦截与删除", async ({ page }) => {
    const guard = watchGateway(page);
    const dialog = await openLocal(page);
    await assertMode(page, dialog);
    await dialog.getByLabel("护栏名称", { exact: true }).fill("e2e-guardrails-local");
    await dialog.getByLabel("正则表达式（每行一个，可选）", { exact: true }).fill("1[3-9][0-9]{9}");
    await dialog.getByRole("combobox", { name: "命中后的动作", exact: true }).selectOption("redact");
    await dialog.getByLabel("替换文本", { exact: true }).fill("[手机号已隐藏]");
    await dialog.getByLabel("测试文本", { exact: true }).fill("请联系 13800138000");
    const trial = page.waitForResponse((r) => new URL(r.url()).pathname === "/guardrails/apply_guardrail");
    await dialog.getByRole("button", { name: "测试当前规则", exact: true }).click();
    expect((await (await trial).json()).response_text).toBe("请联系 [手机号已隐藏]");
    const saved = page.waitForResponse(
      (r) => new URL(r.url()).pathname === "/guardrails" && r.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: "保存护栏", exact: true }).click();
    expect((await saved).ok()).toBeTruthy();
    await expect(dialog).not.toBeVisible();
    const redacted = await chat(page, {
      model: "gpt-4o-mini",
      guardrails: ["e2e-guardrails-local"],
      messages: [{ role: "user", content: "我的电话是 13800138000" }],
    });
    expect(redacted.response.status(), JSON.stringify(redacted.json)).toBe(200);
    expect(redacted.json.choices[0].message.content).toBe("upstream-saw-redacted");
    await openTab(page, t("pages.guardrails.title"));
    await page.getByLabel("搜索护栏", { exact: true }).fill("e2e-guardrails-local");
    await page.getByRole("row").filter({ hasText: "e2e-guardrails-local" }).getByRole("button").first().click();
    await expect(page.getByLabel("正则表达式（每行一个，可选）", { exact: true })).toHaveValue("1[3-9][0-9]{9}");
    await page.getByRole("combobox", { name: "命中后的动作", exact: true }).selectOption("block");
    const updated = page.waitForResponse(
      (r) => r.request().method() === "PATCH" && new URL(r.url()).pathname.startsWith("/guardrails/"),
    );
    await page.getByRole("button", { name: "保存护栏", exact: true }).click();
    expect((await updated).ok()).toBeTruthy();
    const blocked = await chat(page, {
      model: "gpt-4o-mini",
      guardrails: ["e2e-guardrails-local"],
      messages: [{ role: "user", content: "我的电话是 13800138000" }],
    });
    expect(blocked.response.status()).toBe(400);
    expect(JSON.stringify(blocked.json)).toContain("guardrail_failed");
    await openTab(page, t("pages.guardrails.title"));
    await page.getByLabel("搜索护栏", { exact: true }).fill("e2e-guardrails-local");
    await page.getByRole("row").filter({ hasText: "e2e-guardrails-local" }).getByRole("button").nth(1).click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: t("common.delete"), exact: true })
      .click();
    await expect(page.getByRole("row").filter({ hasText: "e2e-guardrails-local" })).toHaveCount(0);
    guard.assertOk();
  });

  test("默认启用规则拦截请求，并拒绝非法 RE2 保存", async ({ page }) => {
    const guard = watchGateway(page);
    const dialog = await openLocal(page);
    await dialog.getByLabel("护栏名称", { exact: true }).fill("e2e-guardrails-invalid");
    await dialog.getByLabel("正则表达式（每行一个，可选）", { exact: true }).fill("[");
    const invalid = page.waitForResponse(
      (r) => new URL(r.url()).pathname === "/guardrails" && r.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: "保存护栏", exact: true }).click();
    expect((await invalid).status()).toBe(400);
    await expect(dialog.getByRole("alert")).toContainText("regular expression");
    await dialog.getByLabel("护栏名称", { exact: true }).fill("e2e-guardrails-default");
    await dialog.getByLabel("正则表达式（每行一个，可选）", { exact: true }).fill("");
    await dialog.getByLabel("关键词（每行一个）", { exact: true }).fill("e2e-default-block");
    await dialog.getByRole("switch", { name: "默认启用（所有请求）", exact: true }).click();
    const created = page.waitForResponse(
      (r) => new URL(r.url()).pathname === "/guardrails" && r.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: "保存护栏", exact: true }).click();
    expect((await created).ok()).toBeTruthy();
    expect(
      (
        await chat(page, { model: "gpt-4o-mini", messages: [{ role: "user", content: "e2e-default-block" }] })
      ).response.status(),
    ).toBe(400);
    expect(
      (await chat(page, { model: "gpt-4o-mini", messages: [{ role: "user", content: "安全内容" }] })).response.status(),
    ).toBe(200);
    guard.assertOk();
  });

  test("XGo 编辑器展示 primitives，并完成脚本调试、保存和真实转发", async ({ page }) => {
    const guard = watchGateway(page);
    await openTab(page, t("pages.guardrails.title"));
    await page.getByRole("button", { name: t("pages.guardrails.create"), exact: true }).click();
    await page.getByRole("menuitem", { name: "XGo 自定义脚本护栏", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("heading", { name: "创建 XGo 护栏", exact: true })).toBeVisible();
    for (const primitive of ["Return Values", "HTTP Requests", "Regex Functions", "JSON Functions", "LLM Chat"])
      await expect(dialog).toContainText(primitive);
    await assertMode(page, dialog);
    await dialog.getByLabel("护栏名称", { exact: true }).fill("e2e-guardrails-xgo");
    const xgo = [
      'import . "xhub/guardrail"',
      "func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {",
      "  for i, text := range texts {",
      '    if Contains(text, "xgo-block") { return Block("XGo blocked") }',
      '    texts[i] = RegexReplace(text, "1[3-9][0-9]{9}", "[手机号已隐藏]")',
      "  }",
      "  return Modify(texts)",
      "}",
    ].join("\n");
    await dialog.getByLabel("XGo 代码", { exact: true }).fill(xgo);
    await dialog.getByLabel("测试文本", { exact: true }).fill("xgo-block");
    const trial = page.waitForResponse((r) => new URL(r.url()).pathname === "/guardrails/test_custom_code");
    await dialog.getByRole("button", { name: "测试当前代码", exact: true }).click();
    expect((await (await trial).json()).result.action).toBe("block");
    const saved = page.waitForResponse(
      (r) => new URL(r.url()).pathname === "/guardrails" && r.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: "保存护栏", exact: true }).click();
    expect((await saved).ok()).toBeTruthy();
    const redacted = await chat(page, {
      model: "gpt-4o-mini",
      guardrails: ["e2e-guardrails-xgo"],
      messages: [{ role: "user", content: "phone 13800138000" }],
    });
    expect(redacted.response.status()).toBe(200);
    expect(redacted.json.choices[0].message.content).toBe("upstream-saw-redacted");
    guard.assertOk();
  });
});

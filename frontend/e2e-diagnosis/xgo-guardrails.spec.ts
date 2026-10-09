import { expect, test, type Page } from "@playwright/test";
import { loginAdmin, stableGoto, t, watchGateway } from "./helpers";

/** openEditor 通过 page 登录并点击打开 XGo 编辑器，返回对话框定位器；断言入口及语言显示。
 * 调用：本文件各 E2E；登录与数据写入均使用隔离 schema，结束后统一清理。 */
async function openEditor(page: Page) {
  await loginAdmin(page);
  await stableGoto(page, "/guardrails");
  await page.getByRole("tab", { name: t("pages.guardrails.title"), exact: true }).click();
  await page.getByRole("button", { name: t("pages.guardrails.create") }).click();
  await page.getByRole("menuitem", { name: t("XGo 自定义脚本护栏"), exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("heading", { name: "创建 XGo 护栏", exact: true })).toBeVisible();
  await expect(dialog).not.toContainText("PYTHON");
  await expect(dialog).not.toContainText("External API Check");
  return dialog;
}

/**
 * 用途：在 XGo 编辑器中提交 JSON 调试输入并返回真实网关执行结果。
 * 参数：page 为浏览器页面；text 为测试文本；allowFailure 表示是否允许编译失败响应。
 * 返回：Promise<Record<string, any>>，包含 success、result 或 error_type。
 * 调用：正常脚本断言传 false；非法脚本断言编译错误时传 true。
 * 异常：网络层失败或不符合调用方预期的 HTTP 状态仍由断言报告。
 */
async function trial(page: Page, text: string, allowFailure = false) {
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("textbox", { name: "测试输入（JSON）", exact: true }).fill(JSON.stringify({ texts: [text], model: "e2e-model", metadata: {} }));
  const submitted = page.waitForResponse(
    (res) => new URL(res.url()).pathname === "/guardrails/test_custom_code" && res.request().method() === "POST",
  );
  await dialog.getByRole("button", { name: "测试当前代码", exact: true }).click();
  const response = await submitted;
  const body = await response.json();
  if (!allowFailure) expect(response.ok(), JSON.stringify(body)).toBeTruthy();
  return body;
}

// 前置：真实控制台与 XGo 编译器；验证拦截、放行和脱敏代码保存回显，schema 统一清理。
test("XGo editor executes, persists, reloads, and edits real scripts", async ({ page }) => {
  const guard = watchGateway(page);
  const dialog = await openEditor(page);
  const name = "e2e-xgo-script";
  const code = dialog.getByRole("textbox", { name: "XGo 代码", exact: true });
  await expect(code).toHaveValue(/func ApplyGuardrail\(/);
  // This syntax needs the XGo compiler, so merely accepting Go-shaped text is insufficient.
  const xgo = (await code.inputValue()).replace("for _, text := range texts", "for text <- texts");
  expect(xgo).toContain("for text <- texts");
  await code.fill(xgo);
  await dialog.getByRole("textbox", { name: "护栏名称", exact: true }).fill(name);
  const blocked = await trial(page, "hello SECRET");
  expect(blocked.success).toBe(true);
  expect(blocked.result.action).toBe("block");
  await expect(dialog.getByLabel("测试结果")).toContainText('"action": "block"');
  const allowed = await trial(page, "ordinary message");
  expect(allowed.success).toBe(true);
  expect(allowed.result.action).toBe("allow");
  await expect(dialog.getByLabel("测试结果")).toContainText('"action": "allow"');
  const saved = page.waitForResponse(
    (res) => new URL(res.url()).pathname === "/guardrails" && res.request().method() === "POST",
  );
  await dialog.getByRole("button", { name: "保存护栏", exact: true }).click();
  const created = await saved;
  expect(created.ok(), await created.text()).toBeTruthy();
  expect(created.request().postDataJSON().guardrail.litellm_params).toMatchObject({
    custom_code_language: "xgo",
    mode: "pre_call",
    custom_code: xgo,
    default_on: false,
  });
  await expect(dialog).not.toBeVisible();
  await page.reload();
  const row = page.getByRole("row").filter({ has: page.getByText(name, { exact: true }) });
  await expect(row).toBeVisible();
  await row.getByRole("button").first().click();
  await expect(dialog.getByRole("heading", { name: "编辑 XGo 护栏", exact: true })).toBeVisible();
  await expect(code).toHaveValue(xgo);
  await dialog.getByRole("combobox", { name: "模板", exact: true }).selectOption("redact");
  const redactCode = await code.inputValue();
  expect(redactCode).toContain("return Modify(texts)");
  const modified = await trial(page, "phone 13800138000");
  expect(modified.success).toBe(true);
  expect(modified.result).toMatchObject({ action: "modify", texts: ["phone [手机号已隐藏]"] });
  await expect(dialog.getByLabel("测试结果")).toContainText("[手机号已隐藏]");
  const updated = page.waitForResponse(
    (res) => res.request().method() === "PATCH" && new URL(res.url()).pathname.startsWith("/guardrails/"),
  );
  await dialog.getByRole("button", { name: "保存护栏", exact: true }).click();
  expect((await updated).ok()).toBeTruthy();
  await expect(dialog).not.toBeVisible();
  await page.reload();
  await row.getByRole("button").first().click();
  await expect(code).toHaveValue(redactCode);
  guard.assertOk();
});

// 前置：未保存的 XGo 编辑器；无效 JSON、错误 texts 类型与旧字段都在客户端拒绝，
// 验证无调试请求、无旧结果残留，并可用请求示例恢复执行；关闭编辑器，无需清理持久化数据。
test("XGo JSON input rejects invalid payloads and recovers with a request example", async ({ page }) => {
  const guard = watchGateway(page);
  const dialog = await openEditor(page);
  const input = dialog.getByRole("textbox", { name: "测试输入（JSON）", exact: true });
  const requests: string[] = [];
  page.on("request", request => {
    if (new URL(request.url()).pathname === "/guardrails/test_custom_code") requests.push(request.url());
  });
  for (const payload of ["{", '{"texts": [1]}', '{"texts": ["secret"], "images": []}']) {
    await input.fill(payload);
    await dialog.getByRole("button", { name: "测试当前代码", exact: true }).click();
    await expect(dialog.getByRole("alert")).toBeVisible();
    await expect(dialog.getByLabel("测试结果")).toHaveCount(0);
    expect(requests, "非法调试输入不应发送到网关").toHaveLength(0);
  }
  await dialog.getByRole("button", { name: "加载请求示例", exact: true }).click();
  expect(JSON.parse(await input.inputValue()).texts).toEqual(["hello secret"]);
  const submitted = page.waitForResponse(response => new URL(response.url()).pathname === "/guardrails/test_custom_code");
  await dialog.getByRole("button", { name: "测试当前代码", exact: true }).click();
  const response = await submitted;
  expect(response.ok(), await response.text()).toBeTruthy();
  expect(await response.json()).toMatchObject({ success: true, result: { action: "block" } });
  await expect(dialog.getByRole("alert")).toHaveCount(0);
  await expect(dialog.getByLabel("测试结果")).toContainText('"action": "block"');
  expect(requests).toHaveLength(1);
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  guard.assertOk();
});

// 前置：管理会话；验证编译失败、保存拒绝及刷新无脏数据，schema 统一清理。
test("invalid XGo fails compilation and cannot create a saved guardrail", async ({ page }) => {
  const guard = watchGateway(page);
  const dialog = await openEditor(page);
  const name = "e2e-invalid-xgo";
  await dialog.getByRole("textbox", { name: "护栏名称", exact: true }).fill(name);
  await dialog.getByRole("textbox", { name: "XGo 代码", exact: true }).fill("func ApplyGuardrail(");
  const failed = await trial(page, "hello", true);
  expect(failed.success).toBe(false);
  expect(failed.error_type).toBe("compilation");
  await expect(dialog.getByRole("alert")).toBeVisible();
  await expect(dialog.getByLabel("测试结果")).toHaveCount(0);
  const saved = page.waitForResponse(
    (res) => new URL(res.url()).pathname === "/guardrails" && res.request().method() === "POST",
  );
  await dialog.getByRole("button", { name: "保存护栏", exact: true }).click();
  expect((await saved).status()).toBe(400);
  await expect(dialog.getByRole("alert")).toBeVisible();
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  await page.reload();
  await expect(page.getByText(name, { exact: true })).toHaveCount(0);
  guard.assertOk();
});

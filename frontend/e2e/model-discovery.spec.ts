import { expect, test, type Page } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t, watchGateway } from "./helpers";

/** configureSupplier 创建使用指定 API 根地址的隔离供应商。
 * 参数 page：已登录浏览器；name、base：用例名称与本地上游地址。返回：创建完成的 Promise。
 * 调用：目录回退 E2E；失败立即断言，凭据随测试 schema 删除，不覆盖用户数据。 */
async function configureSupplier(page: Page, name: string, base: string) {
  const response = await page.request.post(GATEWAY + "/credentials", {
    headers: { Authorization: "Bearer " + await sessionBearer(page) },
    data: { credential_name: name, credential_info: { custom_llm_provider: "openai" },
      credential_values: { api_key: "sk-fake", api_base: base, custom_llm_provider: "openai" } },
  });
  expect(response.status(), await response.text()).toBe(200);
}

for (const scenario of [
  { name: "root-to-v1", base: "/discovery-v1", error: false },
  { name: "v1-to-root", base: "/discovery-root/v1", error: false },
  { name: "both-missing", base: "/discovery-missing", error: true },
]) {
  /** 前置：真实浏览器、网关、PostgreSQL 与只支持一种目录路径的本地供应商。
   * 验证：自动获取或手动降级、保存、刷新、连接数据面、删除。模型显式删除，凭据随 schema 清理。 */
  test(`model discovery ${scenario.name} saves and invokes a deployment`, async ({ page }) => {
    test.setTimeout(90_000);
    const guard = watchGateway(page);
    await loginAdmin(page);
    const name = "e2e-discovery-" + scenario.name;
    const base = UPSTREAM + scenario.base;
    await configureSupplier(page, name, base);
    await stableGoto(page, "/models-and-endpoints");
    await page.getByRole("button", { name: t("pages.models.add") }).click();
    const form = page.locator("form").filter({ has: page.getByLabel("模型提供商 *") });
    const pending = page.waitForResponse(r => new URL(r.url()).pathname === "/model/builtin/models"
      && r.request().postDataJSON()?.credential_name === name);
    await form.getByLabel("模型提供商 *").selectOption(name);
    const response = await pending;
    expect(response.status()).toBe(200);
    const catalog = await response.json();
    expect(catalog.api_base).toBe(base);
    const upstream = form.getByRole("combobox", { name: "上游模型 *" });
    if (scenario.error) {
      expect(catalog.models).toEqual([]);
      expect(catalog.error).toContain("/discovery-missing/models: 404");
      expect(catalog.error).toContain("/discovery-missing/v1/models: 404");
      await expect(form.getByRole("alert")).toContainText("可重试或直接输入模型 ID");
      await upstream.fill("gpt-4o-mini");
      await upstream.press("Escape");
    } else {
      expect(catalog.error).toBeUndefined();
      expect(catalog.models).toHaveLength(155);
      await expect(form.getByText("已获取 155 个模型", { exact: false })).toBeVisible();
      await upstream.fill("gpt-4o-mini");
      await page.getByRole("option", { name: "gpt-4o-mini", exact: true }).click();
      await expect(form.getByRole("alert")).toHaveCount(0);
    }
    await form.getByLabel("对外模型名称 *").fill(name);
    await form.getByRole("combobox", { name: t("Endpoint type"), exact: true }).click();
    await page.getByRole("option", { name: t("Chat"), exact: true }).click();
    await form.getByLabel("价格来源").selectOption("manual");
    await form.locator("#editor-input_cost_per_token").fill("0.15");
    await form.locator("#editor-output_cost_per_token").fill("0.60");
    const submitted = page.waitForResponse(r => new URL(r.url()).pathname === "/model/new" && r.request().method() === "POST");
    await form.getByRole("button", { name: "添加模型", exact: true }).click();
    const saved = await submitted;
    expect(saved.status(), await saved.text()).toBe(200);
    expect(saved.request().postDataJSON().litellm_params.litellm_credential_name).toBe(name);
    await page.reload();
    await stableGoto(page, "/models-and-endpoints");
    await page.getByRole("tab", { name: t("pages.models.all") }).click();
    await page.getByRole("button", { name, exact: true }).click();
    await page.getByRole("button", { name: "测试连接", exact: true }).click();
    await expect(page.getByRole("status").filter({ hasText: "连接正常，模型已响应。" })).toBeVisible();
    // 通过真实数据面调用确认已保存凭据仍可用于推理，目录回退不修改推理根地址。
    const inference = await page.request.post(GATEWAY + "/v1/chat/completions", {
      headers: { Authorization: "Bearer " + await sessionBearer(page) },
      data: { model: name, messages: [{ role: "user", content: "discovery-e2e" }] },
    });
    expect(inference.status(), await inference.text()).toBe(200);
    expect((await inference.json()).choices[0].message.content).toBe("e2e-ok");
    await page.getByRole("button", { name: "删除", exact: true }).click();
    await page.getByRole("dialog").getByPlaceholder(name).fill(name);
    await page.getByRole("dialog").getByRole("button", { name: "删除", exact: true }).click();
    await expect(page.getByRole("button", { name, exact: true })).toHaveCount(0);
    guard.assertOk();
  });
}

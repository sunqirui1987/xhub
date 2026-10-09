import path from "path";
import { expect, test } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/** 前置真实浏览器、隔离 PostgreSQL 和本地 Responses 上游；验证路径模型的保留分隔符的默认公开名、
 * 中文校验、接口分行及窄屏无溢出、Bypass 勾选、保存刷新和跨协议数据面调用；finally 删除部署及凭据。 */
test("model editor presents endpoints clearly and saves a path model with a safe alias", async ({ page }) => {
  test.setTimeout(120_000);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + await sessionBearer(page) };
  const credential = "e2e-alias-presentation";
  const upstreamModel = "group/model:latest";
  const name = "e2e-group/model:latest";
  let id = "";
  expect((await page.request.post(GATEWAY + "/credentials", { headers, data: {
    credential_name: credential, credential_info: { custom_llm_provider: "openai" },
    credential_values: { api_base: UPSTREAM, api_key: "sk-fake" },
  } })).status()).toBe(200);
  try {
    await stableGoto(page, "/models-and-endpoints");
    await page.getByRole("button", { name: t("pages.models.add") }).click();
    const form = page.locator("form").filter({ has: page.getByLabel("模型提供商 *") });
    await form.getByLabel("模型提供商 *").selectOption(credential);
    const upstream = form.getByRole("combobox", { name: "上游模型 *" });
    await upstream.fill(upstreamModel);
    await upstream.press("Escape");
    const alias = form.getByLabel("对外模型名称 *");
    await expect(alias).toHaveValue(upstreamModel);
    await form.getByRole("combobox", { name: "上游接口协议", exact: true }).click();
    await page.getByRole("option", { name: "OpenAI · Responses", exact: true }).click();
    const published = form.getByRole("region", { name: "XHub 对外接口" });
    await expect(published.getByRole("listitem")).toHaveCount(5);
    const gemini = published.getByRole("listitem", { name: "Gemini · Generate Content" });
    await expect(gemini.getByText("/v1beta/models/{model}:generateContent", { exact: true })).toBeVisible();
    await expect(gemini.getByText("/v1beta/models/{model}:streamGenerateContent", { exact: true })).toBeVisible();
    await published.getByRole("checkbox", { name: "Bypass · OpenAI Responses" }).check();
    // 可见卡片和路径均不能超出容器；以实际浏览器几何验证长 Google 路径的折行。
    await page.setViewportSize({ width: 900, height: 1000 });
    expect(await published.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
    await page.screenshot({ path: path.resolve(__dirname, "../../.e2e/model-discovery-fix/editor-narrow.png"), fullPage: true });
    await page.setViewportSize({ width: 1440, height: 1100 });
    await published.screenshot({ path: path.resolve(__dirname, "../../.e2e/model-discovery-fix/endpoints.png") });
    await form.getByLabel("价格来源").selectOption("manual");
    await form.locator("#editor-input_cost_per_token").fill("1");
    await form.locator("#editor-output_cost_per_token").fill("2");
    await alias.fill("group/../model");
    await form.getByRole("button", { name: "添加模型", exact: true }).click();
    await expect(form.getByRole("alert")).toContainText("对外模型名称的路径段不能留空");
    await expect(upstream).toHaveValue(upstreamModel);
    await alias.fill(name);
    const pending = page.waitForResponse(response => new URL(response.url()).pathname === "/model/new" && response.request().method() === "POST");
    await form.getByRole("button", { name: "添加模型", exact: true }).click();
    const saved = await pending;
    expect(saved.status(), await saved.text()).toBe(200);
    expect(saved.request().postDataJSON().litellm_params.model).toBe(upstreamModel);
    expect(saved.request().postDataJSON().model_info.endpoint_types).toContain("bypass:openai-responses");
    id = (await saved.json()).model_info.id;
    await page.reload();
    await stableGoto(page, "/models-and-endpoints");
    await page.getByRole("tab", { name: t("pages.models.all") }).click();
    await page.getByLabel("搜索模型").fill(name);
    await page.getByRole("region", { name: "公开模型 " + name, exact: true }).getByRole("button", { name: "详情", exact: true }).click();
    await page.getByRole("button", { name: "编辑模型", exact: true }).click();
    await expect(alias).toHaveValue(name);
    await expect(upstream).toHaveValue(upstreamModel);
    await expect(published.getByRole("checkbox", { name: "Bypass · OpenAI Responses" })).toBeChecked();
    for (const target of [
      { path: "/v1/chat/completions", body: { model: name, messages: [{ role: "user", content: "hello" }] } },
      { path: "/v1beta/models/" + encodeURIComponent(name) + ":generateContent", body: { contents: [{ role: "user", parts: [{ text: "hello" }] }] } },
      { path: "/bypass/openai/v1/responses", body: { model: name, input: "hello" } },
    ]) {
      const response = await page.request.post(GATEWAY + target.path, { headers, data: target.body });
      expect(response.status(), await response.text()).toBe(200);
      expect(await response.text()).toContain("e2e-ok");
    }
  } finally {
    if (id) expect((await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } })).status()).toBe(200);
    expect((await page.request.delete(GATEWAY + "/credentials/" + credential, { headers })).status()).toBe(200);
  }
});

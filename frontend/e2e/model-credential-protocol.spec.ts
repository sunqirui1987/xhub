import { expect, test } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t, watchGateway } from "./helpers";

/** 前置：真实浏览器、网关和隔离 PostgreSQL，七牛凭据保留旧 openai 声明。
 * 验证截图 Dreamina 模型的目录选择、保存、刷新、编辑及精简调试页提交、查询与结算；上游仅使用本地服务。
 * 参数 page：浏览器页面；返回 Promise<void>。显式删除部署，凭据及账单随隔离 schema 清理。 */
test("legacy Qiniu credential saves edits and calls the Dreamina Fal model", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + await sessionBearer(page) };
  const credential = "e2e-legacy-qiniu-protocol";
  const name = "e2e-legacy-dreamina";
  const model = "byteplus/seedance-2.0/text-to-video";
  const createdCredential = await page.request.post(GATEWAY + "/credentials", { headers, data: {
    credential_name: credential, credential_info: { builtin: "qiniu", custom_llm_provider: "openai" },
    credential_values: { custom_llm_provider: "openai", api_key: "sk-fake", api_base: UPSTREAM },
  } });
  expect(createdCredential.status(), await createdCredential.text()).toBe(200);
  await stableGoto(page, "/models-and-endpoints");
  await page.getByRole("button", { name: t("pages.models.add") }).click();
  const form = page.locator("form").filter({ has: page.getByLabel("模型提供商 *") });
  const discovering = page.waitForResponse(r => new URL(r.url()).pathname === "/model/builtin/models"
    && r.request().method() === "POST" && r.request().postDataJSON().credential_name === credential);
  await form.getByLabel("模型提供商 *").selectOption(credential);
  const directory = await discovering;
  expect(directory.status(), await directory.text()).toBe(200);
  expect((await directory.json()).models).toEqual(expect.arrayContaining([expect.objectContaining({ id: model })]));
  const upstream = form.getByRole("combobox", { name: "上游模型 *" });
  await upstream.fill(model);
  await page.getByRole("option", { name: model, exact: true }).click();
  const endpoint = form.getByRole("combobox", { name: t("Endpoint type"), exact: true });
  await expect(endpoint).toContainText("Fal");
  await form.getByLabel("对外模型名称 *").fill(name);
  await form.getByLabel("目录定价模型 *").selectOption("byteplus/dreamina-seedance-2-0-260128");
  const saving = page.waitForResponse(r => new URL(r.url()).pathname === "/model/new" && r.request().method() === "POST");
  await form.getByRole("button", { name: "添加模型", exact: true }).click();
  const saved = await saving;
  expect(saved.status(), await saved.text()).toBe(200);
  expect(saved.request().postDataJSON()).toMatchObject({
    litellm_params: { custom_llm_provider: "qiniu", litellm_credential_name: credential, model },
    model_info: { transport: "qiniu_fal_dreamina_20", endpoint_types: ["bypass:fal-video"] },
  });
  const id = (await saved.json()).model_info.id;
  await page.reload();
  await stableGoto(page, "/models-and-endpoints");
  await page.getByRole("tab", { name: t("pages.models.all") }).click();
  await page.getByRole("button", { name, exact: true }).click();
  await page.getByRole("button", { name: "编辑模型", exact: true }).click();
  await expect(upstream).toHaveValue(model);
  await expect(endpoint).toContainText("Fal");
  await expect(form.getByLabel("模型提供商 *")).toHaveValue(credential);
  await form.getByLabel("对外模型名称 *").fill(name + "-edited");
  const updating = page.waitForResponse(r => r.request().method() === "PATCH" && new URL(r.url()).pathname === "/model/" + id + "/update");
  await form.getByRole("button", { name: "保存修改", exact: true }).click();
  const edited = await updating;
  expect(edited.status(), await edited.text()).toBe(200);
  await page.reload();
  await page.getByRole("button", { name: "编辑模型", exact: true }).click();
  await expect(form.getByLabel("对外模型名称 *")).toHaveValue(name + "-edited");
  await expect(endpoint).toContainText("Fal");

  // 请求经过真实网关，假供应商检查 Key 密钥、固定路径及不含 model 的 Fal 正文。
  const queued = await page.request.post(GATEWAY + "/queue/" + model, { headers, data: {
    model: name + "-edited", prompt: "legacy-qiniu-e2e", resolution: "1080p",
  } });
  expect(queued.status(), await queued.text()).toBe(200);
  const task = await queued.json();
  expect(task.request_id).toMatch(/^fal-e2e-/);
  const result = "/queue/byteplus/seedance-2.0/requests/" + task.request_id;
  expect(task.response_url).toBe(result);
  expect(task.status_url).toBe(result + "/status");
  for (const path of [task.status_url, task.response_url]) {
    const response = await page.request.get(GATEWAY + path, { headers });
    expect(response.status(), await response.text()).toBe(200);
    expect(await response.json()).toMatchObject({ status: "COMPLETED", usage: { completion_tokens: 100 } });
    expect(Number(response.headers()["x-litellm-response-cost"])).toBeCloseTo(0.00077, 10);
  }
  await form.getByRole("button", { name: "取消", exact: true }).click();
  await stableGoto(page, "/playground");
  await expect(page.getByRole("heading", { name: "模型与端点" })).toBeVisible();
  await expect(page.getByRole("combobox", { name: t("Virtual Key Source") })).not.toBeVisible();
  await page.getByText("连接设置", { exact: true }).click();
  await expect(page.getByRole("combobox", { name: t("Virtual Key Source") })).toBeVisible();
  await page.getByText("连接设置", { exact: true }).click();
  await page.getByPlaceholder(t("Select a Model"), { exact: true }).click();
  await page.getByRole("option", { name: name + "-edited", exact: true }).click();
  await expect(page.getByText("提交请求后，结果显示在这里", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "查询状态", exact: true })).toBeDisabled();
  await page.getByLabel("原生请求参数").fill("[]");
  await page.getByRole("button", { name: "提交请求", exact: true }).click();
  await expect(page.getByRole("region", { name: "请求结果" }).getByRole("alert")).toContainText("请求参数必须是 JSON 对象");
  await page.getByLabel("原生请求参数").fill(JSON.stringify({ prompt: "legacy-qiniu-e2e", resolution: "1080p" }, null, 2));
  const creatingTask = page.waitForResponse(r => new URL(r.url()).pathname === "/queue/" + model && r.request().method() === "POST");
  await page.getByRole("button", { name: "提交请求", exact: true }).click();
  const uiTask = await creatingTask;
  expect(uiTask.status(), await uiTask.text()).toBe(200);
  await expect(page.getByLabel("任务 ID")).toHaveValue((await uiTask.json()).request_id);
  await page.getByRole("button", { name: "查询状态", exact: true }).click();
  await expect(page.getByLabel("原生响应")).toContainText("COMPLETED");
  await page.getByRole("button", { name: "获取结果", exact: true }).click();
  await expect(page.getByLabel("原生响应")).toContainText("fal-e2e.mp4");
  await page.screenshot({ path: "../.e2e/playground-refactor/desktop.png", fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  // 等待侧栏折叠动画完成，并验证编辑区的实际可用宽度；仅检查无溢出会漏掉被导航挤窄的页面。
  await expect.poll(async () => (await page.getByLabel("原生请求参数").boundingBox())?.width ?? 0,
    { message: "手机请求编辑区应保留足够宽度，避免 JSON 按单个字符换行" }).toBeGreaterThanOrEqual(220);
  await expect(page.getByLabel("原生请求参数")).toBeVisible();
  await expect(page.getByRole("button", { name: "获取结果", exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
  await page.screenshot({ path: "../.e2e/playground-refactor/mobile.png", fullPage: true });
  await page.getByRole("button", { name: "获取结果", exact: true }).click();
  await expect(page.getByLabel("原生响应")).toContainText("fal-e2e.mp4");
  await page.getByLabel("原生响应").scrollIntoViewIfNeeded();
  await page.screenshot({ path: "../.e2e/playground-refactor/mobile-result.png", fullPage: true });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await stableGoto(page, "/models-and-endpoints");
  await page.getByRole("tab", { name: t("pages.models.all") }).click();
  await page.getByRole("button", { name: name + "-edited", exact: true }).click();
  await page.getByRole("button", { name: "删除", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "删除模型" });
  await dialog.getByPlaceholder(name + "-edited", { exact: true }).fill(name + "-edited");
  await dialog.getByRole("button", { name: /删除|确认/ }).click();
  await stableGoto(page, "/models-and-endpoints");
  await page.getByRole("tab", { name: t("pages.models.all") }).click();
  await expect(page.getByRole("button", { name: name + "-edited", exact: true })).toHaveCount(0);
  guard.assertOk();
});

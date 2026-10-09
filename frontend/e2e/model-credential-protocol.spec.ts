import { expect, test } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t, watchGateway } from "./helpers";

/** 前置：真实浏览器、网关和隔离 PostgreSQL，官方方舟凭据声明 volcengine 协议。
 * 验证官方 Ark 模型选择、保存、刷新、编辑及精简调试页提交、查询与结算；上游仅使用本地服务。
 * 参数 page：浏览器页面；返回 Promise<void>。显式删除部署，凭据及账单随隔离 schema 清理。 */
test("official Ark credential saves edits and calls its native task model", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + await sessionBearer(page) };
  const credential = "e2e-official-ark-protocol";
  const name = "e2e-official-ark";
  const model = "doubao-seedance-2-0-260128";
  const createdCredential = await page.request.post(GATEWAY + "/credentials", { headers, data: {
    credential_name: credential, credential_info: { custom_llm_provider: "volcengine" },
    credential_values: { custom_llm_provider: "volcengine", api_key: "sk-fake", api_base: UPSTREAM },
  } });
  expect(createdCredential.status(), await createdCredential.text()).toBe(200);
  await stableGoto(page, "/models-and-endpoints");
  await page.getByRole("button", { name: t("pages.models.add") }).click();
  const form = page.locator("form").filter({ has: page.getByLabel("模型提供商 *") });
  await form.getByLabel("模型提供商 *").selectOption(credential);
  const upstream = form.getByRole("combobox", { name: "上游模型 *" });
  await upstream.fill(model); await upstream.press("Escape");
  const endpoint = form.getByRole("combobox", { name: t("Endpoint type"), exact: true });
  await expect(endpoint).toContainText("Ark Video");
  await form.getByLabel("对外模型名称 *").fill(name);
  await form.getByLabel("价格来源").selectOption("manual");
  await form.locator("#editor-output_cost_per_token").fill("10");
  const saving = page.waitForResponse(r => new URL(r.url()).pathname === "/model/new" && r.request().method() === "POST");
  await form.getByRole("button", { name: "添加模型", exact: true }).click();
  const saved = await saving;
  expect(saved.status(), await saved.text()).toBe(200);
  expect(saved.request().postDataJSON()).toMatchObject({
    litellm_params: { custom_llm_provider: "volcengine", litellm_credential_name: credential, model },
    model_info: { transport: "ark_contents_generation", endpoint_types: ["bypass:ark-video"] },
  });
  const id = (await saved.json()).model_info.id;
  await page.reload();
  await stableGoto(page, "/models-and-endpoints");
  await page.getByRole("tab", { name: t("pages.models.all") }).click();
  await page.getByRole("button", { name, exact: true }).click();
  await page.getByRole("button", { name: "编辑模型", exact: true }).click();
  await expect(upstream).toHaveValue(model);
  await expect(endpoint).toContainText("Ark Video");
  await expect(form.getByLabel("模型提供商 *")).toHaveValue(credential);
  await form.getByLabel("对外模型名称 *").fill(name + "-edited");
  const updating = page.waitForResponse(r => r.request().method() === "PATCH" && new URL(r.url()).pathname === "/model/" + id + "/update");
  await form.getByRole("button", { name: "保存修改", exact: true }).click();
  const edited = await updating;
  expect(edited.status(), await edited.text()).toBe(200);
  await page.reload();
  await page.getByRole("button", { name: "编辑模型", exact: true }).click();
  await expect(form.getByLabel("对外模型名称 *")).toHaveValue(name + "-edited");
  await expect(endpoint).toContainText("Ark Video");

  // 真实网关转发官方任务路径，本地上游检查 Bearer 鉴权与模型名。
  const queued=await page.request.post(GATEWAY+"/api/v3/contents/generations/tasks",{headers,data:{model:name+"-edited",content:[{type:"text",text:"official-ark-e2e"}]}});
  expect(queued.status(),await queued.text()).toBe(200);
  const task=await queued.json();expect(task.id).toMatch(/^ark-e2e-/);
  for(let i=0;i<2;i++){
   const r=await page.request.get(GATEWAY+"/api/v3/contents/generations/tasks/"+task.id,{headers});
   expect(r.status(),await r.text()).toBe(200);expect(await r.json()).toMatchObject({status:"succeeded",usage:{completion_tokens:100}});
   expect(Number(r.headers()["x-litellm-response-cost"])).toBeCloseTo(0.001,10);
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
  await expect(page.getByRole("button", { name: "获取结果", exact: true })).toBeDisabled();
  await page.getByLabel("原生请求参数").fill("[]");
  await page.getByRole("button", { name: "提交请求", exact: true }).click();
  await expect(page.getByRole("region", { name: "请求结果" }).getByRole("alert")).toContainText("请求参数必须是 JSON 对象");
  await page.getByLabel("原生请求参数").fill(JSON.stringify({ content: [{type:"text",text:"official-ark-e2e"}] }, null, 2));
  const creatingTask = page.waitForResponse(r => new URL(r.url()).pathname === "/api/v3/contents/generations/tasks" && r.request().method() === "POST");
  await page.getByRole("button", { name: "提交请求", exact: true }).click();
  const uiTask = await creatingTask;
  expect(uiTask.status(), await uiTask.text()).toBe(200);
  await expect(page.getByLabel("任务 ID")).toHaveValue((await uiTask.json()).id);
  await page.getByRole("button", { name: "获取结果", exact: true }).click();
  await expect(page.getByLabel("原生响应")).toContainText("succeeded");
  await page.getByRole("button", { name: "获取结果", exact: true }).click();
  await expect(page.getByLabel("原生响应")).toContainText("ark-e2e.mp4");
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
  await expect(page.getByLabel("原生响应")).toContainText("ark-e2e.mp4");
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

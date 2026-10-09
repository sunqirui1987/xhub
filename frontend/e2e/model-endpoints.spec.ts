import { expect, test, type Page } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t, watchGateway } from "./helpers";

const nativeModels = [
  { id: "fal-ai/kling-video/v3/pro/text-to-video", transport: "qiniu_fal_kling", name: "e2e-native-kling" },
  { id: "fal-ai/vidu/q3/text-to-video/pro", transport: "qiniu_fal_vidu", name: "e2e-native-vidu" },
];

/** supplier 为 page 创建 name 命名的本地目录凭据，native 决定七牛或 OpenAI 协议。
 * 返回创建完成的 Promise；HTTP 失败立即断言。仅用于隔离 E2E schema，结束时统一删除，不调用付费媒体服务。 */
async function supplier(page: Page, name: string, native = true) {
  const response = await page.request.post(GATEWAY + "/credentials", {
    headers: { Authorization: "Bearer " + await sessionBearer(page) },
    data: { credential_name: name, credential_info: { ...(native ? { builtin: "qiniu" } : {}), custom_llm_provider: native ? "qiniu" : "openai" },
      credential_values: { custom_llm_provider: native ? "qiniu" : "openai", api_key: "sk-fake", api_base: UPSTREAM + "/v1" } },
  });
  expect(response.ok(), await response.text()).toBeTruthy();
}

/** discover 通过用户选择凭据获取真实网关目录；每个用例使用独立凭据，避免缓存掩盖请求。 */
async function discover(page: Page, credential: string) {
  const form = page.locator("form").filter({ has: page.getByLabel("模型提供商 *") });
  const pending = page.waitForResponse(r => new URL(r.url()).pathname === "/model/builtin/models"
    && r.request().method() === "POST" && r.request().postDataJSON().credential_name === credential, { timeout: 15_000 });
  await form.getByLabel("模型提供商 *").selectOption(credential);
  const response = await pending;
  expect(response.status(), await response.text()).toBe(200);
  return { form, models: (await response.json()).models as { id: string }[] };
}

for (const model of nativeModels) {
  // 前置：真实网关、本地七牛目录；验证完整列表、价格拒绝、保存回显及编辑型号重新绑定，隔离 schema 统一清理。
  test(`complete supplier list selects and persists ${model.transport}`, async ({ page }) => {
    const guard = watchGateway(page);
    await loginAdmin(page);
    const credential = model.name + "-supplier";
    await supplier(page, credential);
    await stableGoto(page, "/models-and-endpoints");
    await page.getByRole("button", { name: t("pages.models.add") }).click();
    const { form, models } = await discover(page, credential);
    expect(models.length).toBeGreaterThan(155);
    expect(new Set(models.map(item => item.id)).size).toBe(models.length);
    expect(models.filter(item => item.id === model.id)).toHaveLength(1);
    const upstream = form.getByRole("combobox", { name: "上游模型 *" });
    await upstream.click();
    const list = page.getByRole("listbox", { name: "上游模型联想列表" });
    await expect(list.getByRole("option")).toHaveCount(models.length);
    // 末尾选项超过旧 80 项上限；真实点击证明它可滚动访问。
    await list.getByRole("option", { name: models.at(-1)!.id, exact: true }).click();
    await expect(upstream).toHaveValue(models.at(-1)!.id);
    await upstream.fill(model.id);
    await expect(list.getByRole("option", { name: model.id, exact: true })).toBeVisible();
    await upstream.press("ArrowDown");
    await upstream.press("Enter");
    await expect(list).not.toBeVisible();
    await expect(upstream).toHaveValue(model.id);
    await expect(form.getByRole("combobox", { name: t("Endpoint type"), exact: true })).toContainText("Fal");
    await form.getByLabel("对外模型名称 *").fill(model.name);
    await form.getByLabel("价格来源").selectOption("manual");
    await form.getByLabel("费率设置方式").selectOption("flat");
    await form.getByLabel("计费方式").selectOption("second");
    const requests: string[] = [];
    page.on("request", request => {
      if (new URL(request.url()).pathname === "/model/new" && request.method() === "POST") requests.push(request.url());
    });
    await form.getByRole("button", { name: "添加模型", exact: true }).click();
    await expect(form.getByRole("alert")).toContainText("请至少填写一个单价");
    expect(requests).toHaveLength(0);
    await form.locator("#editor-output_cost_per_second").fill("0.1");
    const submitted = page.waitForResponse(r => new URL(r.url()).pathname === "/model/new" && r.request().method() === "POST", { timeout: 15_000 });
    await form.getByRole("button", { name: "添加模型", exact: true }).click();
    const saved = await submitted;
    expect(saved.status(), await saved.text()).toBe(200);
    expect(saved.request().postDataJSON().model_info).toMatchObject({ endpoint_types: ["bypass:fal-video"], transport: model.transport });
    expect(requests).toHaveLength(1);
    await page.reload();
    await page.getByRole("tab", { name: t("pages.models.all") }).click();
    await page.getByRole("button", { name: model.name, exact: true }).click();
    await page.getByRole("button", { name: "编辑模型", exact: true }).click();
    await expect(upstream).toHaveValue(model.id);
    await expect(form.getByRole("combobox", { name: t("Endpoint type"), exact: true })).toContainText("Fal");
    await expect(form.locator("#editor-output_cost_per_second")).toHaveValue("0.1");
    // 编辑原生型号必须重新选择该型号的传输，不能沿用另一个 Fal 模型的固定路径。
    const replacement = nativeModels.find(item => item.transport !== model.transport)!;
    await upstream.fill(replacement.id);
    await page.getByRole("option", { name: replacement.id, exact: true }).click();
    await expect(form.getByRole("combobox", { name: t("Endpoint type"), exact: true })).toContainText("Fal");
    await expect(form.locator("#editor-output_cost_per_second")).toHaveValue("0.1");
    const updating = page.waitForResponse(r => r.request().method() === "PATCH" && /^\/model\/[^/]+\/update$/.test(new URL(r.url()).pathname));
    await form.getByRole("button", { name: "保存修改", exact: true }).click();
    const updated = await updating;
    expect(updated.status(), await updated.text()).toBe(200);
    expect(updated.request().postDataJSON().model_info).toMatchObject({ endpoint_types: ["bypass:fal-video"], transport: replacement.transport });
    await page.reload();
    await stableGoto(page, "/models-and-endpoints");
    await page.getByRole("tab", { name: t("pages.models.all") }).click();
    await page.getByRole("button", { name: model.name, exact: true }).click();
    await page.getByRole("button", { name: "编辑模型", exact: true }).click();
    await expect(upstream).toHaveValue(replacement.id);
    await expect(form.getByRole("combobox", { name: t("Endpoint type"), exact: true })).toContainText("Fal");
    await expect(form.locator("#editor-output_cost_per_second")).toHaveValue("0.1");
    guard.assertOk();
  });
}

for (const mode of ["delayed", "failed"] as const) {
  // 前置：显式 Chat 部署；仅注入端点目录延迟或网络失败，验证声明持久化，schema 统一清理。
  test(`editing retains the saved chat declaration when endpoint directory is ${mode}`, async ({ page }) => {
    const guard = watchGateway(page);
    await loginAdmin(page);
    const name = "e2e-endpoint-edit-" + mode;
    const created = await page.request.post(GATEWAY + "/model/new", {
      headers: { Authorization: "Bearer " + await sessionBearer(page) },
      data: { model_name: name, litellm_params: { model: "glm-4.5", custom_llm_provider: "openai", api_key: "sk-fake", api_base: UPSTREAM, input_cost_per_token: 0.000001, output_cost_per_token: 0.000002 },
        model_info: { transport: "adapted", endpoint_types: ["chat"] } },
    });
    expect(created.status(), await created.text()).toBe(200);
    await stableGoto(page, "/models-and-endpoints");
    await page.getByRole("tab", { name: t("pages.models.all") }).click();
    await page.getByRole("button", { name, exact: true }).click();
    let release!: () => void;
    const gate = new Promise<void>(resolve => { release = resolve; });
    await page.route("**/public/endpoints", async route => {
      if (mode === "failed") { await route.abort("failed"); return; }
      const real = await route.fetch();
      await gate;
      await route.fulfill({ response: real });
    });
    await page.getByRole("button", { name: "编辑模型", exact: true }).click();
    const endpoint = page.getByRole("combobox", { name: t("Endpoint type"), exact: true });
    await expect(endpoint).toBeDisabled();
    await page.getByLabel("对外模型名称 *").fill(name + "-saved");
    if (mode === "delayed") {
      release();
      await expect(endpoint).toBeEnabled();
      await expect(endpoint).toContainText(t("Chat"));
    } else {
      await expect(page.getByRole("alert").filter({ hasText: "已保存的端点声明已保留" })).toBeVisible();
    }
    const submitted = page.waitForResponse(r => r.request().method() === "PATCH" && /^\/model\/[^/]+\/update$/.test(new URL(r.url()).pathname), { timeout: 15_000 });
    await page.getByRole("button", { name: "保存修改", exact: true }).click();
    const saved = await submitted;
    expect(saved.status(), await saved.text()).toBe(200);
    expect(saved.request().postDataJSON().model_info).toMatchObject({ endpoint_types: ["chat"], transport: "adapted" });
    await page.unroute("**/public/endpoints");
    await page.reload();
    // 编辑保存后当前 URL 是详情页；显式返回列表再点击，验证列表和详情均读取持久化数据。
    await stableGoto(page, "/models-and-endpoints");
    await page.getByRole("tab", { name: t("pages.models.all") }).click();
    await page.getByRole("button", { name: name + "-saved", exact: true }).click();
    await page.getByRole("button", { name: "编辑模型", exact: true }).click();
    await expect(endpoint).toBeEnabled();
    await expect(endpoint).toContainText(t("Chat"));
    guard.assertOk();
  });
}

// 前置：七牛与普通 OpenAI 凭据；验证目录隔离及未知型号提示，测试数据随 schema 删除。
test("switching supplier clears native selection and isolates the model directory", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  await supplier(page, "e2e-switch-qiniu");
  await supplier(page, "e2e-switch-openai", false);
  await stableGoto(page, "/models-and-endpoints");
  await page.getByRole("button", { name: t("pages.models.add") }).click();
  const { form } = await discover(page, "e2e-switch-qiniu");
  const upstream = form.getByRole("combobox", { name: "上游模型 *" });
  await upstream.fill(nativeModels[0].id);
  await page.getByRole("option", { name: nativeModels[0].id, exact: true }).click();
  const endpoint = form.getByRole("combobox", { name: t("Endpoint type"), exact: true });
  await expect(endpoint).toContainText("Fal");
  const { models } = await discover(page, "e2e-switch-openai");
  expect(models).toHaveLength(155);
  expect(models.some(item => item.id === nativeModels[0].id)).toBe(false);
  await expect(upstream).toHaveValue("");
  await expect(endpoint).toBeDisabled();
  await upstream.click();
  // 供应商切换时输入框可能仍保有焦点；键盘展开避免依赖重复触发 onFocus。
  await upstream.press("ArrowDown");
  const list = page.getByRole("listbox", { name: "上游模型联想列表" });
  await expect(list.getByRole("option")).toHaveCount(155);
  await upstream.fill("e2e-unknown-model");
  await expect(list).toContainText("没有匹配项");
  await upstream.press("Escape");
  await expect(upstream).toHaveValue("e2e-unknown-model");
  await expect(endpoint).toBeEnabled();
  await expect(form.getByRole("status")).toContainText("尚未声明调用端点");
  guard.assertOk();
});

// 前置：普通 OpenAI 凭据与真实网关；未知型号由用户显式声明 Chat 能力，
// 验证提示、提交契约、刷新回显、连接按钮及数据面实际响应和计量；仅调用本地供应商，schema 统一清理。
test("unknown upstream model persists a manually declared chat endpoint and price", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  const credential = "e2e-manual-endpoint-supplier";
  const name = "e2e-manual-endpoint-model";
  const model = "e2e-unregistered-chat-model";
  await supplier(page, credential, false);
  await stableGoto(page, "/models-and-endpoints");
  await page.getByRole("button", { name: t("pages.models.add") }).click();
  const { form } = await discover(page, credential);
  const upstream = form.getByRole("combobox", { name: "上游模型 *" });
  await upstream.fill(model);
  await upstream.press("Escape");
  await expect(form.getByRole("status")).toContainText("尚未声明调用端点");
  const endpoint = form.getByRole("combobox", { name: t("Endpoint type"), exact: true });
  await endpoint.click();
  await page.getByRole("option", { name: t("Chat"), exact: true }).click();
  await expect(endpoint).toContainText(t("Chat"));
  await expect(form.getByRole("status").filter({ hasText: "尚未声明调用端点" })).toHaveCount(0);
  await form.getByLabel("对外模型名称 *").fill(name);
  await form.getByLabel("价格来源").selectOption("manual");
  await form.locator("#editor-input_cost_per_token").fill("0.15");
  await form.locator("#editor-output_cost_per_token").fill("0.60");
  const submitted = page.waitForResponse(r => new URL(r.url()).pathname === "/model/new" && r.request().method() === "POST");
  await form.getByRole("button", { name: "添加模型", exact: true }).click();
  const saved = await submitted;
  expect(saved.status(), await saved.text()).toBe(200);
  expect(saved.request().postDataJSON().model_info).toMatchObject({ endpoint_types: ["chat"], transport: "adapted" });
  await page.reload();
  await stableGoto(page, "/models-and-endpoints");
  await page.getByRole("tab", { name: t("pages.models.all") }).click();
  await page.getByRole("button", { name, exact: true }).click();
  await page.getByRole("button", { name: "编辑模型", exact: true }).click();
  await expect(upstream).toHaveValue(model);
  await expect(endpoint).toContainText(t("Chat"));
  await expect(form.getByLabel("价格来源")).toHaveValue("manual");
  await expect(form.locator("#editor-input_cost_per_token")).toHaveValue("0.15");
  await expect(form.locator("#editor-output_cost_per_token")).toHaveValue("0.6");
  // 回显正确之外，还必须验证用户点击的连接结果与网关推理的业务内容。
  await form.getByRole("button", { name: "取消", exact: true }).click();
  await page.getByRole("button", { name: "测试连接", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "连接正常，模型已响应。" })).toBeVisible();
  const inference = await page.request.post(GATEWAY + "/v1/chat/completions", {
    headers: { Authorization: "Bearer " + await sessionBearer(page) },
    data: { model: name, messages: [{ role: "user", content: "manual-endpoint-e2e" }] },
  });
  expect(inference.status(), await inference.text()).toBe(200);
  const completion = await inference.json();
  expect(completion.choices[0].message.content).toBe("e2e-ok");
  expect(completion.usage).toMatchObject({ prompt_tokens: 8, completion_tokens: 2, total_tokens: 10 });
  const callId = inference.headers()["x-litellm-call-id"];
  expect(callId, "手动端点推理必须返回可关联账单的 call_id").toBeTruthy();
  await expect.poll(async () => {
    const detail = await page.request.get(GATEWAY + "/spend/logs/ui/" + callId, {
      headers: { Authorization: "Bearer " + await sessionBearer(page) },
    });
    if (!detail.ok()) return 0;
    return Number((await detail.json()).spend);
  }, { message: "保存的人工费率必须用于实际账单：8×0.15/1M + 2×0.60/1M" }).toBeCloseTo(0.0000024, 10);
  guard.assertOk();
});

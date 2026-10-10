import { expect, test, type APIRequestContext, type Page } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto } from "./helpers";

const JSON_DIAGNOSTIC =
  "E2E upstream JSON diagnostic begin | " +
  "0123456789abcdef".repeat(128) +
  " | E2E upstream JSON diagnostic end";

/**
 * 创建仅属于当前隔离 E2E schema 的模型部署。
 * 参数：request 为浏览器请求客户端，headers 为管理员鉴权头，公开模型名与上游模型名用于隔离本次数据。
 * 返回值：后台生成的部署 ID，供 finally 精确删除。
 * 调用场景：错误日志流程分别创建失败和成功部署；创建失败会直接抛出且不会修改用户数据。
 */
async function createDeployment(
  request: APIRequestContext,
  headers: Record<string, string>,
  modelName: string,
  upstreamModel: string,
): Promise<string> {
  const response = await request.post(GATEWAY + "/model/new", {
    headers,
    data: {
      model_name: modelName,
      litellm_params: {
        model: upstreamModel,
        api_base: UPSTREAM,
        api_key: "sk-fake",
        custom_llm_provider: "custom",
        input_cost_per_token: 0.000001,
        output_cost_per_token: 0.000002,
      },
      model_info: { transport: "bypass_openai_chat", pricing_source: "manual" },
    },
  });
  expect(response.status(), await response.text()).toBe(200);
  return (await response.json()).model_info.id;
}

/**
 * 等待真实数据面请求进入隔离日志存储。
 * 参数：page 为当前浏览器页、headers 为管理员鉴权头、callId 为网关响应关联 ID。
 * 返回值：日志详情可读取后结束。
 * 调用场景：打开日志页前消除异步落库竞态；仅查询当前请求，不创建或覆盖任何数据。
 */
async function waitForStoredLog(page: Page, headers: Record<string, string>, callId: string): Promise<void> {
  await expect
    .poll(
      async () => {
        const response = await page.request.get(GATEWAY + "/spend/logs/ui/" + callId, { headers });
        return response.status();
      },
      { message: `请求 ${callId} 应写入隔离日志存储` },
    )
    .toBe(200);
}

/**
 * 在当前日志页按请求 ID 定位并打开详情。
 * 参数：page 为日志页面，callId 为本测试真实网关请求 ID。
 * 返回值：详情抽屉可见后结束。
 * 调用场景：错误日志和普通日志 tab 共用；只操作可访问名称与当前测试行，不触碰其他日志。
 */
async function openLogDetails(page: Page, callId: string): Promise<void> {
  const search = page.getByPlaceholder(/按 ID 搜索日志|Search logs by ID/);
  await search.fill(callId);
  const row = page.getByRole("row").filter({ hasText: callId });
  await expect(row, `应在当前日志 tab 找到请求 ${callId}`).toBeVisible();
  await row.click();
  await expect(page.getByRole("dialog")).toBeVisible();
}

/**
 * 验证真实日志诊断默认折叠、展开内容与后台一致并可再次收起。
 * 参数为当前详情页、管理员头、真实调用 ID 和预期上游状态；返回验证完成的 Promise。
 * 用于成功及失败请求流程；只读取隔离日志并操作当前抽屉，日志由 schema 清理。
 */
async function verifyUpstreamCollapse(page: Page, headers: Record<string, string>, callId: string, status: number): Promise<void> {
  const detail = await page.request.get(GATEWAY + "/spend/logs/ui/" + callId, { headers });
  expect(detail.status()).toBe(200);
  const upstream = (await detail.json()).metadata.upstream_response;
  expect(upstream.status_code, "后台应保存真实上游 HTTP 状态").toBe(status);
  const region = page.getByRole("dialog").getByRole("region", { name: /^(上游响应|Upstream Response)$/ });
  const trigger = region.getByRole("button", { name: /^(上游响应|Upstream Response)$/ });
  const header = region.getByRole("heading", { name: /上游响应头|Upstream Response Headers/ });
  await expect(trigger, "打开日志详情时上游响应应默认折叠").toHaveAttribute("aria-expanded", "false");
  await expect(header).toBeHidden();
  await expect(region.getByRole("button", { name: /复制上游响应|Copy upstream response/ })).toBeVisible();
  await trigger.click();
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await expect(header).toBeVisible();
  await expect(region.getByText(/^(HTTP 状态码|HTTP Status):/)).toContainText(String(status));
  await expect(region).toContainText(JSON.stringify(upstream.headers, null, 2));
  await expect(region).toContainText(JSON.stringify(upstream.billing_usage ?? {}, null, 2));
  if (upstream.usage_reported) {
    await expect(region).toContainText(JSON.stringify(upstream.usage, null, 2));
  } else {
    await expect(region).toContainText(/未上报|Not reported/);
  }
  await trigger.click();
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  await expect(header).toBeHidden();
}

/**
 * 前置条件：使用隔离 PostgreSQL schema、真实网关和浏览器，并由本地上游返回 HTTP 502 长 JSON 正文。
 * 验证结果：错误及成功日志的上游响应均默认折叠，可展开对照真实后台诊断并收起；失败正文保持完整可见。
 * 清理方式：显式启用正文存储并在 finally 恢复配置、删除本测试部署；日志随隔离 schema 删除，不依赖其他测试的设置。
 */
test("错误日志详情完整显示上游正文并可返回普通日志", async ({ page }) => {
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  const suffix = Date.now().toString();
  const failedModel = "error-logs-failed-" + suffix;
  const successModel = "error-logs-success-" + suffix;
  const deploymentIds: string[] = [];
  const settings = await page.request.get(GATEWAY + "/config/list?config_type=general_settings", { headers });
  expect(settings.status()).toBe(200);
  const logging = (await settings.json()).find((field: any) => field.field_name === "store_prompts_in_spend_logs");

  try {
    // 正文详情是本场景的断言前置；显式配置，防止用例选择或执行顺序改变结果。
    const enabled = await page.request.post(GATEWAY + "/config/update", { headers,
      data: { general_settings: { store_prompts_in_spend_logs: true } } });
    expect(enabled.status()).toBe(200);
    deploymentIds.push(
      await createDeployment(page.request, headers, failedModel, "e2e-error-log-json-" + suffix),
      await createDeployment(page.request, headers, successModel, "e2e-error-log-success-" + suffix),
    );

    const failed = await page.request.post(GATEWAY + "/chat/completions", {
      headers,
      data: { model: failedModel, messages: [{ role: "user", content: "记录完整上游错误正文" }] },
    });
    expect(failed.status(), await failed.text()).toBe(502);
    const failedCallId = failed.headers()["x-litellm-call-id"];
    expect(failedCallId, "失败响应应包含可关联日志的 call id").toBeTruthy();

    const succeeded = await page.request.post(GATEWAY + "/chat/completions", {
      headers,
      data: { model: successModel, messages: [{ role: "user", content: "记录普通成功日志" }] },
    });
    expect(succeeded.status(), await succeeded.text()).toBe(200);
    expect((await succeeded.json()).choices[0].message.content).toBe("e2e-ok");
    const successCallId = succeeded.headers()["x-litellm-call-id"];
    expect(successCallId, "成功响应应包含可关联日志的 call id").toBeTruthy();

    await waitForStoredLog(page, headers, failedCallId);
    await waitForStoredLog(page, headers, successCallId);
    await stableGoto(page, "/logs");

    // 验证互斥筛选通过后台分页生效，普通日志不出现失败请求，错误日志不出现成功请求。
    const search = page.getByPlaceholder(/按 ID 搜索日志|Search logs by ID/);
    await search.fill(failedCallId);
    await expect(page.getByRole("row").filter({ hasText: failedCallId })).toHaveCount(0);
    await expect(page.getByText(/没有结果|No results/)).toBeVisible();
    await page.getByRole("tab", { name: /错误日志|Error Logs/, exact: true }).click();
    await search.fill(successCallId);
    await expect(page.getByRole("row").filter({ hasText: successCallId })).toHaveCount(0);
    await expect(page.getByText(/没有结果|No results/)).toBeVisible();
    await openLogDetails(page, failedCallId);
    const failedDialog = page.getByRole("dialog");
    await verifyUpstreamCollapse(page, headers, failedCallId, 502);
    await expect(failedDialog.getByRole("alert")).toContainText(/HTTP 状态码:\s*502|HTTP Status:\s*502/);
    const errorDetails = failedDialog.getByRole("region", {
      name: /完整错误详情|全部错误详情|Full Error Details/,
    });
    await expect(errorDetails.getByRole("button", { name: /复制错误详情|Copy error details/ })).toBeVisible();
    const errorBody = errorDetails.locator("pre");
    await expect(errorBody).toContainText("E2E_UPSTREAM_502");
    await expect(errorBody).toContainText("e2e-upstream-json-request");
    await expect(errorBody).toContainText("e2e-json-terminal-cause");
    expect(await errorBody.textContent(), "完整错误详情的 pre 必须保留上游长诊断正文").toContain(JSON_DIAGNOSTIC);
    // 在正文断言通过后保存可视化证据，确保截图呈现的是已展开且内容完整的错误详情。
    await page.screenshot({ path: "../.e2e/error-logs-detail.png", fullPage: true });
    await page.keyboard.press("Escape");
    await expect(failedDialog).toBeHidden();

    await page.getByRole("tab", { name: /^(日志|Logs)$/, exact: true }).click();
    await openLogDetails(page, successCallId);
    const successDialog = page.getByRole("dialog");
    await verifyUpstreamCollapse(page, headers, successCallId, 200);
    await expect(successDialog).toContainText("e2e-ok");
    await expect(successDialog).toContainText(/成功|Success/);
    await page.screenshot({ path: "../.e2e/upstream-response-collapsed.png", fullPage: true });
  } finally {
    try {
      const restored = logging?.stored_in_db
        ? await page.request.post(GATEWAY + "/config/update", { headers,
          data: { general_settings: { store_prompts_in_spend_logs: logging.field_value } } })
        : await page.request.post(GATEWAY + "/config/field/delete", { headers,
          data: { config_type: "general_settings", field_name: "store_prompts_in_spend_logs" } });
      expect(restored.status()).toBe(200);
    } finally {
      for (const id of deploymentIds) {
        const deleted = await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } });
        expect(deleted.status(), await deleted.text()).toBe(200);
      }
    }
  }
});

/**
 * 前置条件：隔离数据库、真实浏览器及网关；通过真实 HTTP 调用已退役 Agent 和 Tool 接口。
 * 验证结果：旧接口仍返回 410，但普通和错误日志均无记录；真实未知模型错误仍可打开完整详情。
 * 清理方式：仅生成隔离 schema 中的诊断日志，运行结束由 E2E 清理 schema，不修改用户配置。
 */
test("退役接口不进入模型日志且真实推理错误继续可见", async ({ page }) => {
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  const retiredIds: string[] = [];
  for (const path of ["/v1/agents/agent1", "/v1/tool/x"]) {
    const response = await page.request.get(GATEWAY + path, { headers });
    expect(response.status(), await response.text()).toBe(410);
    expect((await response.json()).error.type).toBe("removed");
    const id = response.headers()["x-litellm-call-id"];
    expect(id).toBeTruthy();
    retiredIds.push(id);
  }
  const rejected = await page.request.post(GATEWAY + "/v1/chat/completions", {
    headers, data: { model: "retired-control-missing-" + Date.now(), messages: [] },
  });
  expect(rejected.status()).toBeGreaterThanOrEqual(400);
  const callId = rejected.headers()["x-litellm-call-id"];
  await waitForStoredLog(page, headers, callId);
  await stableGoto(page, "/logs");
  const search = page.getByPlaceholder(/按 ID 搜索日志|Search logs by ID/);
  for (const tabName of [/^(日志|Logs)$/, /错误日志|Error Logs/]) {
    await page.getByRole("tab", { name: tabName, exact: true }).click();
    for (const id of retiredIds) {
      const detail = await page.request.get(GATEWAY + "/spend/logs/ui/" + id, { headers });
      expect(detail.status(), "退役请求不得写入真实日志存储").toBe(404);
      await search.fill(id);
      await expect(page.getByText(/没有结果|No results/)).toBeVisible();
      await expect(page.getByRole("row").filter({ hasText: id })).toHaveCount(0);
    }
  }
  await openLogDetails(page, callId);
  const diagnostic = page.getByRole("dialog").getByRole("region", { name: /完整错误详情|全部错误详情|Full Error Details/ }).locator("pre");
  expect(await diagnostic.textContent()).toBe(await rejected.text());
});

for (const kind of ["text", "network", "rejected"] as const) {
  /** 前置隔离库、真实网关和本地可控上游；分别验证纯文本长错误、断网和未知模型拒绝。
   * 普通日志查询为空，错误 tab 可打开完整诊断；finally 删除部署，日志由私有 schema 清理，无外部凭据。 */
  test("错误日志独立记录并显示诊断：" + kind, async ({ page }) => {
    await loginAdmin(page);
    const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
    const suffix = Date.now().toString();
    const model = "error-log-" + kind + "-" + suffix;
    let deployment: string | undefined;
    try {
      if (kind !== "rejected") {
        deployment = await createDeployment(page.request, headers, model, "e2e-error-log-" + kind + "-" + suffix);
      }
      const response = await page.request.post(GATEWAY + "/chat/completions", {
        headers, data: { model, messages: [{ role: "user", content: "错误诊断测试" }] },
      });
      expect(response.status()).toBeGreaterThanOrEqual(400);
      const callId = response.headers()["x-litellm-call-id"];
      expect(callId).toBeTruthy();
      await waitForStoredLog(page, headers, callId);
      await stableGoto(page, "/logs");
      await page.getByPlaceholder(/按 ID 搜索日志|Search logs by ID/).fill(callId);
      await expect(page.getByText(/没有结果|No results/)).toBeVisible();
      await expect(page.getByRole("row").filter({ hasText: callId })).toHaveCount(0);
      await page.getByRole("tab", { name: /错误日志|Error Logs/, exact: true }).click();
      await openLogDetails(page, callId);
      const diagnostic = page.getByRole("dialog").getByRole("region", { name: /完整错误详情|全部错误详情|Full Error Details/ }).locator("pre");
      if (kind === "text") {
        expect(await diagnostic.textContent()).toBe("E2E upstream plain-text diagnostic begin | " + "fedcba9876543210".repeat(128) + " | E2E upstream plain-text diagnostic end");
      } else if (kind === "network") {
        await expect(diagnostic).toContainText(/EOF|connection|network/i);
      } else {
        expect(await diagnostic.textContent()).toBe(await response.text());
      }
    } finally {
      if (deployment) {
        const deleted = await page.request.post(GATEWAY + "/model/delete", { headers, data: { id: deployment } });
        expect(deleted.status(), await deleted.text()).toBe(200);
      }
    }
  });
}

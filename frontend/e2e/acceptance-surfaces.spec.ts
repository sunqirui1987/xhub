import { expect, test, type APIRequestContext, type Page } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t, watchGateway } from "./helpers";

const guardrailNames = ["acceptance-ui-block", "acceptance-ui-redact", "acceptance-ui-flag"];
const deploymentName = "acceptance-ui-lifecycle";
const deploymentId = "acceptance-ui-lifecycle-id";

/** createGuardrail 通过真实管理 API 创建非默认护栏；参数为页面、名称和执行参数，返回持久化 ID，规则由 afterEach 删除。 */
async function createGuardrail(page: Page, name: string, params: Record<string, unknown>): Promise<string> {
  const response = await page.request.post(GATEWAY + "/guardrails", {
    headers: await adminHeaders(page),
    data: { guardrail: { guardrail_name: name, litellm_params: params } },
  });
  expect(response.status(), await response.text()).toBe(200);
  const body = await response.json();
  expect(body.guardrail_id).toBeTruthy();
  return body.guardrail_id;
}

/** adminHeaders 从浏览器登录态生成管理 API 请求头；参数为已登录页面，返回 bearer 和 JSON 头，不输出令牌。 */
async function adminHeaders(page: Page): Promise<Record<string, string>> {
  return { Authorization: "Bearer " + (await sessionBearer(page)), "Content-Type": "application/json" };
}

/** chat 调用测试网关的真实聊天数据面；参数为页面、模型、正文和可选护栏，返回原始 API 响应供状态与数据面断言。 */
async function chat(page: Page, model: string, content: string, guardrail?: string) {
  return page.request.post(GATEWAY + "/v1/chat/completions", {
    headers: await adminHeaders(page),
    data: {
      model,
      messages: [{ role: "user", content }],
      ...(guardrail ? { guardrails: [guardrail] } : {}),
    },
  });
}

/** cleanupGuardrails 删除本文件创建的规则；参数为请求上下文和管理头，返回清理完成，缺失规则按已清理处理。 */
async function cleanupGuardrails(request: APIRequestContext, headers: Record<string, string>) {
  const listed = await request.get(GATEWAY + "/guardrails/list", { headers });
  if (!listed.ok()) return;
  const body = await listed.json();
  for (const row of Array.isArray(body.guardrails) ? body.guardrails : []) {
    if (guardrailNames.includes(row.guardrail_name) && row.guardrail_id) {
      await request.delete(GATEWAY + "/guardrails/" + encodeURIComponent(row.guardrail_id), { headers });
    }
  }
}

/** cleanupDeployment 删除本文件创建的部署；参数为请求上下文和管理头，返回清理完成，404/400 表示无需清理。 */
async function cleanupDeployment(request: APIRequestContext, headers: Record<string, string>) {
  await request.post(GATEWAY + "/model/enable", { headers, data: { id: deploymentId } });
  await request.post(GATEWAY + "/model/delete", { headers, data: { id: deploymentId } });
}

test.describe("真实监控与模型生命周期验收", () => {
  test.beforeEach(async ({ page }) => {
    await loginAdmin(page);
  });

  test.afterEach(async ({ page }) => {
    const headers = await adminHeaders(page);
    await cleanupGuardrails(page.request, headers);
    await cleanupDeployment(page.request, headers);
  });

  test("护栏真实拦截、脱敏和标记后在监控总览、详情与日志可见", async ({ page }) => {
    const guard = watchGateway(page);
    const blockId = await createGuardrail(page, guardrailNames[0], {
      guardrail: "blocked_words", blocked_words: ["acceptance-ui-block-secret"], mode: "pre_call", default_on: false,
    });
    const redactId = await createGuardrail(page, guardrailNames[1], {
      guardrail: "redact", blocked_words: ["acceptance-ui-redact-secret"], mode: "pre_call", default_on: false,
    });
    const flagId = await createGuardrail(page, guardrailNames[2], {
      guardrail: "custom_code",
      custom_code_language: "xgo",
      custom_code: 'import . "xhub/guardrail"\nfunc ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any { return Flag("acceptance review", map[string]any{"source":"browser"}) }',
      mode: "pre_call",
      default_on: false,
    });

    const blocked = await chat(page, "gpt-4o-mini", "acceptance-ui-block-secret", guardrailNames[0]);
    expect(blocked.status(), await blocked.text()).toBe(400);
    const redacted = await chat(page, "gpt-4o-mini", "acceptance-ui-redact-secret", guardrailNames[1]);
    expect(redacted.status(), await redacted.text()).toBe(200);
    expect((await redacted.json()).choices[0].message.content).toBe("upstream-saw-redacted");
    const flagged = await chat(page, "gpt-4o-mini", "ordinary browser text", guardrailNames[2]);
    expect(flagged.status(), await flagged.text()).toBe(200);

    await expect
      .poll(async () => {
        const response = await page.request.get(GATEWAY + "/guardrails/usage/overview", { headers: await adminHeaders(page) });
        if (!response.ok()) return 0;
        return Number((await response.json()).totalRequests ?? 0);
      })
      .toBeGreaterThanOrEqual(3);

    await stableGoto(page, "/guardrails-monitor");
    await expect(page.getByRole("heading", { name: t("pages.guardrailsMonitor.title") })).toBeVisible();
    const totalCard = page.getByRole("group", { name: t("Total Evaluations") });
    await expect(totalCard.locator(".text-3xl")).toHaveText(/[1-9][0-9,]*/);
    for (const name of guardrailNames) await expect(page.getByText(name, { exact: true })).toBeVisible();

    await page.getByText(guardrailNames[0], { exact: true }).click();
    await expect(page.getByText(guardrailNames[0], { exact: true }).first()).toBeVisible();
    const requestsCard = page.getByRole("group", { name: t("Requests Evaluated") });
    await expect(requestsCard.locator(".text-3xl")).toHaveText(/[1-9][0-9,]*/);
    await page.getByRole("tab", { name: t("Logs"), exact: true }).click();
    await expect(page.getByText(t("Blocked"), { exact: true }).first()).toBeVisible();
    const blockedLog = page.getByRole("button", { name: new RegExp(`Guardrail blocked the request: ${guardrailNames[0]}`) });
    await expect(blockedLog).toContainText("执行器：blocked_words");
    await expect(blockedLog).toContainText("阶段：pre-call");
    await expect(blockedLog).toContainText(/耗时：\d+ms/);

    const redactLogs = await page.request.get(GATEWAY + "/guardrails/usage/logs?guardrail_id=" + encodeURIComponent(redactId), { headers: await adminHeaders(page) });
    const redactLog = (await redactLogs.json()).logs[0];
    expect(redactLog.action).toBe("flagged");
    expect(redactLog.guardrail_name).toBe(guardrailNames[1]);
    expect(redactLog.guardrail_mode).toBe("pre_call");
    const flagLogs = await page.request.get(GATEWAY + "/guardrails/usage/logs?guardrail_id=" + encodeURIComponent(flagId), { headers: await adminHeaders(page) });
    expect((await flagLogs.json()).logs[0].action).toBe("flagged");
    expect(blockId).toBeTruthy();
    guard.assertOk();
  });

  test("大模型广场身份明确且部署添加、下架失败、恢复可用走真实数据面", async ({ page }) => {
    const guard = watchGateway(page);
    await stableGoto(page, "/price-data");
    await expect(page.getByRole("heading", { name: "AI 大模型广场" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "模型广场", exact: true })).toBeVisible();
    await expect(page.getByRole("tab", { name: "本地模型列表", exact: true })).toBeVisible();
    await expect(page.getByText(/Modelink 市场目录/)).toBeVisible();

    const headers = await adminHeaders(page);
    const modelInfo = { id: deploymentId, transport: "bypass_openai_chat", endpoint_types: ["chat"], pricing_source: "manual" };
    const missing = await page.request.post(GATEWAY + "/model/new", {
      headers,
      data: { model_name: deploymentName, litellm_params: { model: "acceptance-ui-upstream", api_key: "local-only", input_cost_per_token: 0.000001 }, model_info: modelInfo },
    });
    expect(missing.status()).toBe(400);
    expect(await missing.text()).toContain("api_base");
    const zero = await page.request.post(GATEWAY + "/model/new", {
      headers,
      data: { model_name: deploymentName, litellm_params: { model: "acceptance-ui-upstream", api_key: "local-only", api_base: UPSTREAM, input_cost_per_token: 0, output_cost_per_token: 0 }, model_info: modelInfo },
    });
    expect(zero.status()).toBe(400);
    expect(await zero.text()).toContain("positive");
    const created = await page.request.post(GATEWAY + "/model/new", {
      headers,
      data: { model_name: deploymentName, litellm_params: { model: "acceptance-ui-upstream", api_key: "local-only", api_base: UPSTREAM, custom_llm_provider: "custom", input_cost_per_token: 0.000001, output_cost_per_token: 0.000002 }, model_info: modelInfo },
    });
    expect(created.status(), await created.text()).toBe(200);
    expect((await created.json()).model_info.id).toBe(deploymentId);

    const first = await chat(page, deploymentName, "browser lifecycle first call");
    expect(first.status(), await first.text()).toBe(200);
    const disabled = await page.request.post(GATEWAY + "/model/disable", { headers, data: { id: deploymentId } });
    expect(disabled.status(), await disabled.text()).toBe(200);
    const stored = await page.request.get(GATEWAY + "/v2/model/info?modelId=" + deploymentId, { headers });
    expect((await stored.json()).data[0].model_info.disabled).toBe(true);
    const unavailable = await chat(page, deploymentName, "must not reach local upstream while disabled");
    expect(unavailable.status()).toBeGreaterThanOrEqual(400);
    const visible = await page.request.get(GATEWAY + "/v1/models", { headers });
    expect((await visible.json()).data.map((row: { id: string }) => row.id)).not.toContain(deploymentName);
    const enabled = await page.request.post(GATEWAY + "/model/enable", { headers, data: { id: deploymentId } });
    expect(enabled.status(), await enabled.text()).toBe(200);
    const restored = await chat(page, deploymentName, "browser lifecycle restored call");
    expect(restored.status(), await restored.text()).toBe(200);
    guard.assertOk();
  });
});

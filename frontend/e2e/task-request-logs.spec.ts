import { expect, test } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/** 前置隔离数据库、真实浏览器和网关、本地 Ark 上游；验证创建到轮询、完成或失败、重复查询始终一条原请求日志与费用。
 * 浏览器刷新查看状态，按原 ID 复开输入与终态详情；失败转到错误日志。finally 恢复正文存储设置并删除部署，账单与任务由运行器清理，无外部凭据。 */
for (const failure of [false, true]) {
test("异步任务原日志展示生命周期且轮询不增加日志" + (failure ? "：失败" : "：完成"), async ({ page }) => {
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + await sessionBearer(page) };
  const name = "e2e-task-logs-" + Date.now();
  const saved = await page.request.post(GATEWAY + "/model/new", { headers, data: {
    model_name: name,
    litellm_params: { model: "volcengine/doubao-seedance-2-0-260128", custom_llm_provider: "volcengine", api_base: UPSTREAM, api_key: "sk-fake", output_cost_per_token: 0.00001 },
    model_info: { transport: "ark_contents_generation", endpoint_types: ["bypass:ark-video"], pricing_source: "manual" },
  } });
  expect(saved.status(), await saved.text()).toBe(200);
  const deployment = (await saved.json()).model_info.id;
  const settings = await page.request.get(GATEWAY + "/config/list?config_type=general_settings", { headers });
  expect(settings.status()).toBe(200);
  const logging = (await settings.json()).find((field: any) => field.field_name === "store_prompts_in_spend_logs");
  try {
    // 日志正文默认关闭；显式开启后才能证明原始输入与终态产物持久化，结束时恢复覆盖。
    const enabled = await page.request.post(GATEWAY + "/config/update", { headers,
      data: { general_settings: { store_prompts_in_spend_logs: true } } });
    expect(enabled.status()).toBe(200);
    const created = await page.request.post(GATEWAY + "/api/v3/contents/generations/tasks", { headers, data: { model: name, resolution: "480p", content: [{ type: "text", text: failure ? "e2e-task-failure" : "e2e-task-lifecycle" }] } });
    expect(created.status(), await created.text()).toBe(200);
    const task = (await created.json()).id;
    const original = created.headers()["x-litellm-call-id"];
    expect(original).toBeTruthy();
    await stableGoto(page, "/logs");
    const search = page.getByPlaceholder(/按 ID 搜索日志|Search logs by ID/);
    await search.fill(original);
    const row = page.getByRole("row").filter({ hasText: original });
    await expect(row).toHaveCount(1);
    await expect(row).toContainText("执行中");
    const pending = await page.request.get(GATEWAY + "/api/v3/contents/generations/tasks/" + task, { headers });
    expect(pending.status()).toBe(200);
    expect((await pending.json()).status).toBe("running");
    await page.getByTestId("datatable-refresh").click();
    await expect(row).toContainText("轮询中");
    for (let i = 0; i < 3; i++) {
      const result = await page.request.get(GATEWAY + "/api/v3/contents/generations/tasks/" + task, { headers });
      expect(result.status(), await result.text()).toBe(200);
      expect((await result.json()).status).toBe(failure ? "failed" : "succeeded");
    }
    await page.getByTestId("datatable-refresh").click();
    if (failure) {
      await expect(row, "失败任务必须从普通日志移除").toHaveCount(0);
    } else {
      await expect(row).toHaveCount(1);
      await expect(row).toContainText("完成");
    }
    const logs = await page.request.get(GATEWAY + "/spend/logs/ui?model=" + name, { headers });
    const body = await logs.json();
    expect(body.data).toHaveLength(1);
    expect(body.data[0]).toMatchObject({ request_id: original, status: failure ? "failed" : "completed", spend: failure ? 0 : 0.001, completion_tokens: failure ? 0 : 100 });
    if (failure) {
      await page.getByRole("tab", { name: /错误日志|Error Logs/, exact: true }).click();
      await search.fill(original);
      await expect(row).toHaveCount(1);
      await expect(row).toContainText("失败");
    }
    await row.click();
    await expect(page.getByRole("dialog")).toContainText(failure ? "失败" : "完成");
    if (failure) {
      await expect(page.getByRole("dialog").getByRole("region", { name: /完整错误详情|全部错误详情|Full Error Details/ })).toContainText("failed");
    } else {
      // 续跑按创建 ID 打开完成后的同一条日志，原始输入和终态产物必须同时可读。
      await stableGoto(page, "/logs/?log_id=" + encodeURIComponent(original));
      await expect(page.getByRole("dialog")).toBeVisible();
      const mediaRequest = page.getByTestId("media-request");
      const mediaResponse = page.getByTestId("media-response");
      // 在真实媒体日志详情中核对本轮产品翻译实际消费的字段，而非只检查目录中存在对应键。
      await expect(mediaRequest.getByText(t("Media"), { exact: true })).toBeVisible();
      await expect(mediaRequest.getByText(t("Prompt"), { exact: true })).toBeVisible();
      await expect(mediaRequest.getByText(t("Resolution"), { exact: true })).toBeVisible();
      await expect(mediaRequest).toContainText("e2e-task-lifecycle");
      await expect(mediaResponse.getByText(t("Task ID"), { exact: true })).toBeVisible();
      // Ark 终态通过 content.video_url 返回产物；response_url/status_url 是 FAL 队列字段，不能要求 Ark 日志显示。
      await expect(mediaResponse.getByRole("link", { name: "https://example.invalid/ark-e2e.mp4", exact: true })).toHaveAttribute("href", "https://example.invalid/ark-e2e.mp4");
      await expect(mediaResponse.getByText(t("Response URL"), { exact: true })).toHaveCount(0);
      await expect(mediaResponse).toContainText(task);
      await expect(mediaResponse).toContainText("succeeded");
      await expect(page.getByTestId("media-response-video")).toHaveAttribute("src", "https://example.invalid/ark-e2e.mp4");
      await expect(page.getByTestId("media-response-usage")).toContainText("100");
    }
    await page.screenshot({ path: "../artifacts/task-request-logs/" + (failure ? "failed" : "completed") + "-task.png", fullPage: true });
  } finally {
    try {
      const restored = logging?.stored_in_db
        ? await page.request.post(GATEWAY + "/config/update", { headers,
          data: { general_settings: { store_prompts_in_spend_logs: logging.field_value } } })
        : await page.request.post(GATEWAY + "/config/field/delete", { headers,
          data: { config_type: "general_settings", field_name: "store_prompts_in_spend_logs" } });
      expect(restored.status()).toBe(200);
    } finally {
      const deleted = await page.request.post(GATEWAY + "/model/delete", { headers, data: { id: deployment } });
      expect(deleted.status(), await deleted.text()).toBe(200);
    }
  }
});

}

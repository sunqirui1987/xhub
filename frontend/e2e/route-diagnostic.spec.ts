import { expect, test } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/**
 * 目的：验证删除正权重部署后，默认分配会在同一事务内自动清理，剩余单部署可直接服务两条公开 Chat 路径和 Playground。
 * 前置：隔离数据库、真实浏览器、网关与本地协议上游；创建同名双部署并保存 100:0 默认权重。
 * 结果：单部署页面隐藏权重列和编辑入口，两条公开路径及 Playground 均成功，并留下按真实 token 计费的可关联日志。
 * 清理：finally 删除仍存在的测试部署；隔离 schema 由运行器销毁。
 */
test("deployment deletion clears stale weights and preserves public inference", async ({ page }) => {
  test.setTimeout(120_000);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  const suffix = Date.now().toString();
  const model = "route-diagnostic-" + suffix;
  const ids = ["route-diagnostic-a-" + suffix, "route-diagnostic-b-" + suffix];
  try {
    for (let index = 0; index < ids.length; index++) {
      const created = await page.request.post(GATEWAY + "/model/new", { headers, data: {
        model_name: model,
        litellm_params: { model: "route-diagnostic-real-" + index, api_base: UPSTREAM, api_key: "sk-fake", deployment_id: ids[index], custom_llm_provider: "custom", input_cost_per_token: 0.000001, output_cost_per_token: 0.000002 },
        model_info: { id: ids[index], transport: "bypass_openai_chat", endpoint_types: ["chat"], pricing_source: "manual" },
      } });
      expect(created.status(), await created.text()).toBe(200);
    }
    const saved = await page.request.put(GATEWAY + "/model/default", { headers, data: {
      model_name: model, weights: { allocations: [{ deployment_id: ids[0], weight: 100 }, { deployment_id: ids[1], weight: 0 }] },
    } });
    expect(saved.status(), await saved.text()).toBe(200);
    const removed = await page.request.post(GATEWAY + "/model/delete", { headers, data: { id: ids[0] } });
    expect(removed.status(), await removed.text()).toBe(200);

    const groups = await page.request.get(GATEWAY + "/model/groups?model_name=" + encodeURIComponent(model), { headers });
    expect(groups.status(), await groups.text()).toBe(200);
    const groupData = (await groups.json()).data.find((row: any) => row.model_name === model);
    expect(groupData.default_weights?.allocations ?? []).toEqual([]);

    await stableGoto(page, "/models-and-endpoints");
    await page.getByRole("tab", { name: t("pages.models.all") }).click();
    await page.getByLabel("搜索模型").fill(model);
    const group = page.getByRole("region", { name: "公开模型 " + model, exact: true });
    await expect(group).toBeVisible();
    await expect(group.getByRole("button", { name: "编辑权重" })).toHaveCount(0);
    await expect(group.getByRole("columnheader", { name: "默认权重" })).toHaveCount(0);

    // 两种公开路径都必须在自动清理后直接恢复，并各自留下正费用日志。
    for (const path of ["/chat/completions", "/v1/chat/completions"]) {
      // 两条接口使用不同正文，避免响应缓存把第二条变成免费命中，确保两条都验证真实计费。
      const call = await page.request.post(GATEWAY + path, { headers, data: { model, messages: [{ role: "user", content: "自动清理权重 " + path }] } });
      expect(call.status(), await call.text()).toBe(200);
      expect((await call.json()).choices[0].message.content).toBe("e2e-ok");
      const callID = call.headers()["x-litellm-call-id"];
      expect(callID).toBeTruthy();
      await expect.poll(async () => {
        const detail = await page.request.get(GATEWAY + "/spend/logs/ui/" + callID, { headers });
        return detail.ok() ? Number((await detail.json()).spend) : 0;
      }, { message: path + " 应记录8输入2输出的真实费用" }).toBeCloseTo(0.000012, 10);
    }

    await stableGoto(page, "/playground");
    await page.getByPlaceholder(t("Select a Model"), { exact: true }).click();
    await page.getByRole("option", { name: model, exact: true }).click();
    await page.getByPlaceholder(t("Type your message... (Shift+Enter for new line)")).filter({ visible: true }).fill("自动清理后实调");
    const called = page.waitForResponse((response) => /^(\/v1)?\/chat\/completions$/.test(new URL(response.url()).pathname) && response.request().method() === "POST");
    await page.getByRole("button", { name: t("Send message") }).click();
    const response = await called;
    // 页面在流式完成后会更新对话区；此时 Chromium 可能已释放导航响应正文，因此这里只读取稳定的状态码和响应头。
    expect(response.status()).toBe(200);
    await expect(page.getByText("e2e-ok", { exact: true })).toBeVisible();
    const playgroundCallID = response.headers()["x-litellm-call-id"];
    await expect.poll(async () => {
      const detail = await page.request.get(GATEWAY + "/spend/logs/ui/" + playgroundCallID, { headers });
      return detail.ok() ? Number((await detail.json()).spend) : 0;
    }, { message: "Playground 请求应进入同一真实计费链" }).toBeCloseTo(0.000012, 10);
  } finally {
    for (const id of ids) {
      const deleted = await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } });
      expect([200, 400, 404]).toContain(deleted.status());
    }
  }
});

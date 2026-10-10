import { expect, test } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/** 前置隔离 PostgreSQL、真实网关/浏览器和本地协议服务；创建双部署并保存100:0，删除正权重部署。
 * 验证剩余部署的权重仍可见、真实请求有明确503原因，页面修复后Playground实调及账单恢复。
 * finally 删除本测试部署，运行器删除隔离schema内的默认分配、会话和账单，不读取用户数据。 */
test("remaining deployment exposes saved zero weight and recovers real inference", async ({ page }) => {
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + await sessionBearer(page) };
  const model = "route-diagnostic-" + Date.now();
  const ids: string[] = [];
  try {
    for (let i = 0; i < 2; i++) {
      const created = await page.request.post(GATEWAY + "/model/new", { headers, data: {
        model_name: model,
        litellm_params: { model: "route-diagnostic-real-" + i, api_base: UPSTREAM, api_key: "sk-fake", custom_llm_provider: "custom", input_cost_per_token: 0.000001, output_cost_per_token: 0.000002 },
        model_info: { transport: "bypass_openai_chat", pricing_source: "manual" },
      } });
      expect(created.status(), await created.text()).toBe(200);
      ids.push((await created.json()).model_info.id);
    }
    const saved = await page.request.put(GATEWAY + "/model/default", { headers, data: {
      model_name: model, weights: { allocations: [{ deployment_id: ids[0], weight: 100 }, { deployment_id: ids[1], weight: 0 }] },
    } });
    expect(saved.status(), await saved.text()).toBe(200);
    const removed = await page.request.post(GATEWAY + "/model/delete", { headers, data: { id: ids[0] } });
    expect(removed.status(), await removed.text()).toBe(200);
    ids.shift();
    await stableGoto(page, "/models-and-endpoints");
    await page.getByRole("tab", { name: t("pages.models.all") }).click();
    await page.getByLabel("搜索模型").fill(model);
    const group = page.getByRole("region", { name: "公开模型 " + model, exact: true });
    await expect(group.getByRole("columnheader", { name: "默认权重" })).toBeVisible();
    await expect(group.getByRole("cell", { name: "0", exact: true })).toBeVisible();
    // 两种公开路径均走真实数据面，确认同一错误契约和可关联的请求日志。
    for (const path of ["/chat/completions", "/v1/chat/completions"]) {
      const denied = await page.request.post(GATEWAY + path, { headers, data: { model, messages: [{ role: "user", content: "你好" }] } });
      expect(denied.status(), await denied.text()).toBe(503);
      expect((await denied.json()).error).toMatchObject({ type: "model_unavailable", message: expect.stringContaining("no allocated traffic") });
      expect(denied.headers()["x-litellm-call-id"]).toBeTruthy();
    }
    await group.getByRole("button", { name: "编辑权重" }).click();
    await expect(group.getByRole("button", { name: "保存权重" })).toBeDisabled();
    await group.getByLabel("部署 " + ids[0] + " 权重").fill("1");
    const updated = page.waitForResponse(r => new URL(r.url()).pathname === "/model/default" && r.request().method() === "PUT");
    await group.getByRole("button", { name: "保存权重" }).click();
    const update = await updated;
    expect(update.status(), await update.text()).toBe(200);
    expect(update.request().postDataJSON().weights.allocations).toEqual([{ deployment_id: ids[0], weight: 1 }]);
    await page.reload();
    await page.getByLabel("搜索模型").fill(model);
    await group.getByRole("button", { name: "编辑权重" }).click();
    await expect(group.getByLabel("部署 " + ids[0] + " 权重")).toHaveValue("1");
    await group.getByRole("button", { name: "取消", exact: true }).click();
    await stableGoto(page, "/playground");
    await page.getByPlaceholder(t("Select a Model"), { exact: true }).click();
    await page.getByRole("option", { name: model, exact: true }).click();
    await page.getByPlaceholder(t("Type your message... (Shift+Enter for new line)")).filter({ visible: true }).fill("修复权重后实调");
    const called = page.waitForResponse(r => /^(\/v1)?\/chat\/completions$/.test(new URL(r.url()).pathname) && r.request().method() === "POST");
    await page.getByRole("button", { name: t("Send message") }).click();
    const response = await called;
    expect(response.status()).toBe(200);
    await expect(page.getByText("e2e-ok", { exact: true })).toBeVisible();
    const callID = response.headers()["x-litellm-call-id"];
    expect(callID).toBeTruthy();
    await expect.poll(async () => {
      const detail = await page.request.get(GATEWAY + "/spend/logs/ui/" + callID, { headers });
      return detail.ok() ? Number((await detail.json()).spend) : 0;
    }, { message: "恢复请求应记录8输入2输出的真实费用" }).toBeCloseTo(0.000012, 10);
  } finally {
    for (const id of ids) expect((await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } })).status()).toBe(200);
  }
});

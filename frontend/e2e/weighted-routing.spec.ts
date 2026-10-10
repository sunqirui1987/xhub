import { expect, test } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, watchGateway } from "./helpers";

/**
 * 目的：验证删除模板与默认权重共同指向的部署时，两处失效分配在同一模型事务内清理，绑定密钥继续继承模板并路由剩余部署。
 * 前置：隔离数据库、本地协议上游和真实浏览器；建立双部署、A0/B1 默认与模板，并把虚拟密钥绑定到模板。
 * 结果：删除 beta 前真实请求落 beta；删除后默认分配为空、模板 allocations 字段移除、单部署页面隐藏权重，密钥自动落 alpha 且精确计费。
 * 清理：finally 先解绑并删除密钥，再删除模板和仍存在的部署；隔离 schema 由运行器销毁。
 */
test("deployment deletion clears default and template weights for inherited routing", async ({ page }) => {
  test.setTimeout(120_000);
  const guard = watchGateway(page);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  const suffix = Date.now().toString();
  const model = "weights-" + suffix;
  const ids = ["weight-a-" + suffix, "weight-b-" + suffix];
  const upstreams = ["custom/alpha/model", "custom/beta/model"];
  let templateID = "";
  let key = "";
  try {
    for (let index = 0; index < ids.length; index++) {
      const created = await page.request.post(GATEWAY + "/model/new", { headers, data: {
        model_name: model,
        litellm_params: { model: upstreams[index], api_base: UPSTREAM, api_key: "sk-fake", deployment_id: ids[index], custom_llm_provider: "custom", input_cost_per_token: index === 0 ? 0.000003 : 0.000001, output_cost_per_token: 0.000002 },
        model_info: { id: ids[index], transport: "bypass_openai_chat", endpoint_types: ["chat", "bypass:openai-chat"], pricing_source: "manual" },
      } });
      expect(created.status(), await created.text()).toBe(200);
    }
    const defaults = await page.request.put(GATEWAY + "/model/default", { headers, data: {
      model_name: model, weights: { allocations: [{ deployment_id: ids[0], weight: 0 }, { deployment_id: ids[1], weight: 1 }] },
    } });
    expect(defaults.status(), await defaults.text()).toBe(200);

    const body = {
      model_routes: [{ model, strategy: "traffic-split", allocations: [{ deployment_id: ids[0], weight: 0 }, { deployment_id: ids[1], weight: 1 }] }],
      routing_groups: [], fallbacks: [], context_window_fallbacks: [], content_policy_fallbacks: [],
      retry_policy: { max_attempts: 1, timeout_seconds: 60, failure_threshold: 3, cooldown_seconds: 60 },
    };
    const saved = await page.request.post(GATEWAY + "/route_template/new", { headers, data: { name: "customer-weights-" + suffix, body } });
    expect(saved.status(), await saved.text()).toBe(200);
    templateID = (await saved.json()).id;

    const teams = await (await page.request.get(GATEWAY + "/v2/team/list?page=1&page_size=500", { headers })).json();
    const team = teams.teams.find((row: any) => row.team_alias === "e2e-fixture-team");
    const jwt = (await page.context().cookies()).find((cookie) => cookie.name === "token")!.value;
    const userID = JSON.parse(Buffer.from(jwt.split(".")[1], "base64url").toString()).user_id;
    const minted = await page.request.post(GATEWAY + "/key/generate", { headers, data: { key_alias: "weights-" + suffix, key_type: "llm_api", team_id: team.team_id, user_id: userID, route_template_id: templateID } });
    expect(minted.status(), await minted.text()).toBe(200);
    key = (await minted.json()).key;

    /**
     * 用途：通过绕过适配层的真实 Chat 路径确认实际部署，并等待该请求的持久化费用。
     * 参数：want 为预期上游模型，cost 为按固定 8/2 token 计算的预期费用；返回无。
     * 调用：删除 beta 前后各一次；错误状态、部署或费用不匹配时由 Playwright 断言失败。
     * 副作用：产生一条真实推理及账单日志，finally 不删除审计日志，随隔离 schema 一并销毁。
     */
    const call = async (want: string, cost: number) => {
      const response = await page.request.post(GATEWAY + "/bypass/openai/v1/chat/completions", { headers: { Authorization: "Bearer " + key }, data: { model, messages: [{ role: "user", content: "verify inherited routing" }] } });
      expect(response.status(), await response.text()).toBe(200);
      expect((await response.json()).model).toBe(want);
      const callID = response.headers()["x-litellm-call-id"];
      expect(callID).toBeTruthy();
      await expect.poll(async () => {
        const log = await page.request.get(GATEWAY + "/spend/logs/ui/" + callID, { headers });
        return log.ok() ? Number((await log.json()).spend) : 0;
      }).toBeCloseTo(cost, 10);
    };

    await call(upstreams[1], 0.000012);
    const removed = await page.request.post(GATEWAY + "/model/delete", { headers, data: { id: ids[1] } });
    expect(removed.status(), await removed.text()).toBe(200);

    const groups = await page.request.get(GATEWAY + "/model/groups?model_name=" + encodeURIComponent(model), { headers });
    expect(groups.status(), await groups.text()).toBe(200);
    const group = (await groups.json()).data.find((row: any) => row.model_name === model);
    expect(group.default_weights?.allocations ?? []).toEqual([]);
    const stored = await page.request.get(GATEWAY + "/route_template/" + templateID, { headers });
    expect(stored.status(), await stored.text()).toBe(200);
    const rule = (await stored.json()).body.model_routes.find((row: any) => row.model === model);
    expect(rule).toBeTruthy();
    expect(rule).not.toHaveProperty("allocations");

    await stableGoto(page, "/models-and-endpoints");
    await page.getByLabel("搜索模型").fill(model);
    const modelGroup = page.getByRole("region", { name: "公开模型 " + model, exact: true });
    await expect(modelGroup).toBeVisible();
    await expect(modelGroup.getByRole("button", { name: "编辑权重" })).toHaveCount(0);
    await expect(modelGroup.getByRole("columnheader", { name: "默认权重" })).toHaveCount(0);
    await call(upstreams[0], 0.000028);
    guard.assertOk();
  } finally {
    if (key) {
      await page.request.post(GATEWAY + "/key/update", { headers, data: { key, route_template_id: "" } });
      await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys: [key] } });
    }
    if (templateID) await page.request.post(GATEWAY + "/route_template/" + templateID + "/delete", { headers, data: {} });
    for (const id of ids) await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } });
  }
});

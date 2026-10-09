import { expect, test } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t, watchGateway } from "./helpers";

/** 验证同名模型行权重保存回读、模板模型规则、恢复继承与单部署隐藏。
 * 前置隔离数据库及本地协议服务；使用真实浏览器和网关核对原生响应型号与账单。
 * finally 删除本测试创建的密钥、模板与部署；隔离 schema 由运行器清理。
 */
test("relative row weights and customer inheritance", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  const suffix = Date.now().toString(),
    model = "weights-" + suffix;
  const ids = ["weight-a-" + suffix, "weight-b-" + suffix],
    upstreams = ["custom/alpha/model", "custom/beta/model"];
  let templateID = "",
    key = "";
  try {
    for (let i = 0; i < 2; i++) {
      const r = await page.request.post(GATEWAY + "/model/new", {
        headers,
        data: {
          model_name: model,
          litellm_params: {
            model: upstreams[i],
            api_base: UPSTREAM,
            api_key: "sk-fake",
            deployment_id: ids[i],
            custom_llm_provider: "custom",
            input_cost_per_token: i === 0 ? 0.000003 : 0.000001,
            output_cost_per_token: 0.000002,
          },
          model_info: {
            id: ids[i],
            transport: "bypass_openai_chat",
            endpoint_types: ["chat", "bypass:openai-chat"],
            pricing_source: "manual",
          },
        },
      });
      expect(r.status(), await r.text()).toBe(200);
    }
    await stableGoto(page, "/models-and-endpoints");
    await page.getByLabel("搜索模型").fill(model);
    const group = page.getByRole("region", { name: "公开模型 " + model, exact: true });
    await group.getByRole("button", { name: "编辑权重" }).click();
    await group.getByLabel("部署 " + ids[0] + " 权重").fill("3");
    await group.getByLabel("部署 " + ids[1] + " 权重").fill("7");
    let saved = page.waitForResponse(
      (r) => new URL(r.url()).pathname === "/model/default" && r.request().method() === "PUT",
    );
    await group.getByRole("button", { name: "保存权重" }).click();
    expect((await saved).status()).toBe(200);
    await page.reload();
    await page.getByLabel("搜索模型").fill(model);
    await group.getByRole("button", { name: "编辑权重" }).click();
    await expect(group.getByLabel("部署 " + ids[0] + " 权重")).toHaveValue("3");
    await expect(group.getByLabel("部署 " + ids[1] + " 权重")).toHaveValue("7");
    await group.getByLabel("部署 " + ids[0] + " 权重").fill("1");
    await group.getByLabel("部署 " + ids[1] + " 权重").fill("0");
    saved = page.waitForResponse(
      (r) => new URL(r.url()).pathname === "/model/default" && r.request().method() === "PUT",
    );
    await group.getByRole("button", { name: "保存权重" }).click();
    expect((await saved).status()).toBe(200);
    const teams = await (await page.request.get(GATEWAY + "/v2/team/list?page=1&page_size=500", { headers })).json();
    const team = teams.teams.find((row: any) => row.team_alias === "e2e-fixture-team");
    const jwt = (await page.context().cookies()).find((c) => c.name === "token")!.value;
    const userID = JSON.parse(Buffer.from(jwt.split(".")[1], "base64url").toString()).user_id;
    const minted = await page.request.post(GATEWAY + "/key/generate", {
      headers,
      data: { key_alias: "weights-" + suffix, key_type: "llm_api", team_id: team.team_id, user_id: userID },
    });
    expect(minted.status(), await minted.text()).toBe(200);
    key = (await minted.json()).key;
    /** 原生响应保留实际上游型号；实测用量经真实网关计费，零权重使断言确定。 */
    const call = async (want: string) => {
      const r = await page.request.post(GATEWAY + "/bypass/openai/v1/chat/completions", {
        headers: { Authorization: "Bearer " + key },
        data: { model, messages: [{ role: "user", content: "verify " + want }] },
      });
      expect(r.status(), await r.text()).toBe(200);
      expect((await r.json()).model).toBe(want);
      const callID = r.headers()["x-litellm-call-id"];
      expect(callID).toBeTruthy();
      await expect
        .poll(async () => {
          const log = await page.request.get(GATEWAY + "/spend/logs/ui/" + callID, { headers });
          return log.ok() ? Number((await log.json()).spend) : 0;
        })
        .toBeGreaterThan(0);
    };
    await call(upstreams[0]);
    await stableGoto(page, "/route-templates");
    await page
      .getByRole("button", { name: t("pages.routeTemplates.create"), exact: true })
      .first()
      .click();
    const editor = page.getByRole("region", { name: t("pages.routeTemplates.create"), exact: true });
    await editor.getByLabel(t("pages.routeTemplates.name"), { exact: true }).fill("customer-weights-" + suffix);
    await editor.getByLabel("公开模型", { exact: true }).selectOption(model);
    await editor.getByRole("button", { name: "添加模型规则" }).click();
    await editor.getByLabel(model + " 路由逻辑").selectOption("traffic-split");
    await editor.getByLabel("模板部署 " + ids[0] + " 权重").fill("0");
    await editor.getByLabel("模板部署 " + ids[1] + " 权重").fill("1");
    await expect(editor.getByText("custom · " + upstreams[0], { exact: true })).toBeVisible();
    await expect(editor.getByText("custom · " + upstreams[1], { exact: true })).toBeVisible();
    await editor.getByLabel("模板部署 " + ids[0] + " 权重").scrollIntoViewIfNeeded();
    await page.screenshot({ path: "../.e2e/routing-redesign/independent-weights.png", fullPage: true });
    await page.getByRole("button", { name: "完整 JSON 配置指南" }).click();
    const guide = page.getByRole("dialog");
    await expect(guide.getByRole("cell", { name: "model_routes", exact: true })).toBeVisible();
    await guide.getByRole("tab", { name: "可导入示例" }).click();
    await expect(guide.getByRole("region", { name: "完整模板示例" })).toContainText('"max_attempts": 2');
    await guide.getByRole("tab", { name: "LiteLLM 对照" }).click();
    await expect(guide.getByText(/max_attempts 包含首次调用/)).toBeVisible();
    await guide.getByRole("tab", { name: "完整字段" }).click();
    await page.screenshot({ path: "../.e2e/routing-redesign/json-guide.png", fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(guide.getByRole("cell", { name: "model_routes", exact: true })).toBeVisible();
    const dialogBox = await guide.boundingBox();
    expect(dialogBox!.width).toBeLessThanOrEqual(390);
    await page.screenshot({ path: "../.e2e/routing-redesign/json-guide-mobile.png", fullPage: true });
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.keyboard.press("Escape");
    await expect(editor.getByRole("tab")).toHaveCount(4);
    for (const removed of ["自动路由", "供应商预算", "健康检查"]) {
      await expect(editor.getByRole("tab", { name: removed })).toHaveCount(0);
    }
    await expect(editor.getByRole("complementary")).toHaveCount(0);
    await expect(editor.getByRole("region", { name: "真实路由预览" })).toHaveCount(0);
    await editor.getByRole("tab", { name: "故障转移" }).click();
    await expect(editor.getByRole("button", { name: "添加故障转移" })).toBeVisible();
    await editor.getByRole("tab", { name: "负载均衡" }).click();
    // 页面移除预览入口后，仍通过真实接口验证策略解析契约。
    const preview = await page.request.post(GATEWAY + "/route_template/preview", {
      headers,
      data: {
        model_name: model,
        endpoint_id: "chat",
        body: {
          model_routes: [
            {
              model,
              strategy: "traffic-split",
              allocations: [
                { deployment_id: ids[0], weight: 0 },
                { deployment_id: ids[1], weight: 1 },
              ],
            },
          ],
          routing_groups: [],
          fallbacks: [],
          context_window_fallbacks: [],
          content_policy_fallbacks: [],
          retry_policy: { max_attempts: 1, timeout_seconds: 60, failure_threshold: 3, cooldown_seconds: 60 },
        },
      },
    });
    expect(preview.status(), await preview.text()).toBe(200);
    expect(await preview.json()).toMatchObject({
      rule_source: "template-model",
      routing_strategy: "traffic-split",
    });
    await page.evaluate(() => {
      // 控制台使用内部滚动容器，重置各容器后截图才包含模板标题与完整分区。
      for (const element of document.querySelectorAll<HTMLElement>("*")) {
        if (element.scrollTop) element.scrollTop = 0;
      }
      window.scrollTo(0, 0);
    });
    await page.screenshot({ path: "../.e2e/routing-redesign/editor.png", fullPage: true });
    const templateSaved = page.waitForResponse(
      (r) => new URL(r.url()).pathname === "/route_template/new" && r.request().method() === "POST",
    );
    await editor.getByRole("button", { name: "保存模板", exact: true }).click();
    const response = await templateSaved;
    expect(response.status(), await response.text()).toBe(200);
    templateID = (await response.json()).id;
    const persisted = await (await page.request.get(GATEWAY + "/route_template/" + templateID, { headers })).json();
    expect(persisted.body).toMatchObject({
      model_routes: [
        {
          model,
          strategy: "traffic-split",
          allocations: expect.arrayContaining([
            { deployment_id: ids[0], weight: 0 },
            { deployment_id: ids[1], weight: 1 },
          ]),
        },
      ],
      routing_groups: [],
      fallbacks: [],
    });
    // 权重按部署 ID 匹配，编辑顺序不影响分流；仍要求恰好两项，避免遗漏或额外配置。
    expect(persisted.body.model_routes[0].allocations).toHaveLength(2);
    expect(
      (
        await page.request.post(GATEWAY + "/key/update", { headers, data: { key, route_template_id: templateID } })
      ).status(),
    ).toBe(200);
    await call(upstreams[1]);
    await page.reload();
    const row = page.getByRole("row").filter({ has: page.getByText("customer-weights-" + suffix, { exact: true }) });
    await row.getByRole("button", { name: t("pages.routeTemplates.edit"), exact: true }).click();
    const editing = page.getByRole("region", { name: t("pages.routeTemplates.edit"), exact: true });
    await expect(editing.getByLabel(model + " 路由逻辑")).toHaveValue("traffic-split");
    await expect(editing.getByLabel("模板部署 " + ids[0] + " 权重")).toHaveValue("0");
    await expect(editing.getByLabel("模板部署 " + ids[1] + " 权重")).toHaveValue("1");
    await editing.getByRole("button", { name: "删除规则" }).click();
    const updated = page.waitForResponse(
      (r) => r.url().includes("/route_template/" + templateID + "/update") && r.request().method() === "POST",
    );
    await editing.getByRole("button", { name: "保存模板", exact: true }).click();
    expect((await updated).status()).toBe(200);
    await call(upstreams[0]);
    expect((await page.request.post(GATEWAY + "/model/delete", { headers, data: { id: ids[1] } })).status()).toBe(200);
    await stableGoto(page, "/models-and-endpoints");
    await page.getByLabel("搜索模型").fill(model);
    await expect(group).toBeVisible();
    await expect(group.getByRole("button", { name: "编辑权重" })).toHaveCount(0);
    await expect(group.getByRole("columnheader", { name: "默认权重" })).toHaveCount(0);
    guard.assertOk();
  } finally {
    if (key) {
      await page.request.post(GATEWAY + "/key/update", { headers, data: { key, route_template_id: null } });
      await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys: [key] } });
    }
    if (templateID)
      await page.request.post(GATEWAY + "/route_template/" + templateID + "/delete", { headers, data: {} });
    for (const id of ids) await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } });
  }
});

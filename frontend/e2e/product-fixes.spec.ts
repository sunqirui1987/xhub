import { expect, test, type APIRequestContext } from "@playwright/test";
import { GATEWAY, UPSTREAM, login, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";
import { translate } from "../src/i18n/translate";

/**
 * 用途：轮询管理日志详情，直到指定调用已经持久化并返回完整关联字段。
 * 参数：request 为浏览器请求上下文，headers 为管理员会话头，callID 为响应头中的调用标识。
 * 返回：日志详情对象；调用方断言费用、缓存和归属字段。超时或详情接口失败时由 expect.poll 报错。
 * 调用：缓存和继承路由用例；只读日志，不改变配额或业务数据。
 */
async function waitForLog(request: APIRequestContext, headers: Record<string, string>, callID: string): Promise<any> {
  let row: any;
  await expect.poll(async () => {
    const response = await request.get(GATEWAY + "/spend/logs/ui/" + callID, { headers });
    if (!response.ok()) return false;
    row = await response.json();
    return row.request_id === callID || row.call_id === callID || row.id === callID;
  }, { message: "调用日志应按 x-litellm-call-id 可关联" }).toBe(true);
  return row;
}

/**
 * 用途：按本用例唯一会话标识轮询真实推理日志，避免串行全量运行时依赖响应头读取时序。
 * 参数：request 为浏览器请求上下文，headers 为管理员会话头，sessionID 为请求携带的唯一会话标识。
 * 返回：该会话唯一日志；未落库或返回数量异常时由轮询断言失败。只读日志，无数据副作用。
 */
async function waitForSessionLog(request: APIRequestContext, headers: Record<string, string>, sessionID: string): Promise<any> {
  let rows: any[] = [];
  await expect.poll(async () => {
    const query = new URLSearchParams({ session_id: sessionID, group_by_session: "false", page: "1", page_size: "10" });
    const response = await request.get(GATEWAY + "/spend/logs/ui?" + query, { headers });
    if (!response.ok()) return 0;
    rows = (await response.json()).data ?? [];
    return rows.length;
  }, { message: "唯一会话的真实推理日志应完成落库" }).toBe(1);
  return rows[0];
}

/**
 * 目的：验证缓存页真实清空后，相同请求严格经历 miss 计费、hit 免费、再次清空、miss 再计费。
 * 前置：隔离数据库、真实浏览器和本地协议上游；创建专用部署与虚拟密钥，避免复用其他用例缓存。
 * 结果：三条响应与日志均按调用 ID 关联，缓存键一致，命中序列为 false/true/false，两次 miss 精确计费且 hit 为零。
 * 清理：finally 删除密钥和模型并再次清空缓存；隔离 schema 由运行器销毁。
 */
test("browser cache flush restores a billed miss after a free hit", async ({ page }) => {
  test.setTimeout(120_000);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  const suffix = Date.now().toString();
  const model = "cache-fix-" + suffix;
  const deploymentID = "cache-fix-dep-" + suffix;
  let key = "";
  try {
    const created = await page.request.post(GATEWAY + "/model/new", { headers, data: {
      model_name: model,
      litellm_params: { model: "cache-fix-upstream", api_base: UPSTREAM, api_key: "sk-fake", deployment_id: deploymentID, custom_llm_provider: "custom", input_cost_per_token: 0.000001, output_cost_per_token: 0.000002 },
      model_info: { id: deploymentID, transport: "bypass_openai_chat", endpoint_types: ["chat"], pricing_source: "manual" },
    } });
    expect(created.status(), await created.text()).toBe(200);
    const teams = await (await page.request.get(GATEWAY + "/v2/team/list?page=1&page_size=500", { headers })).json();
    const team = teams.teams.find((row: any) => row.team_alias === "e2e-fixture-team");
    const jwt = (await page.context().cookies()).find((cookie) => cookie.name === "token")!.value;
    const userID = JSON.parse(Buffer.from(jwt.split(".")[1], "base64url").toString()).user_id;
    const minted = await page.request.post(GATEWAY + "/key/generate", { headers, data: { key_alias: "cache-fix-" + suffix, key_type: "llm_api", team_id: team.team_id, user_id: userID } });
    expect(minted.status(), await minted.text()).toBe(200);
    key = (await minted.json()).key;

    /** 用途：从缓存控制台触发真实 POST /flushall；无参数、无返回；页面反馈和 HTTP 200 均为成功条件，副作用是清空本进程响应缓存。 */
    const flushFromBrowser = async () => {
      await stableGoto(page, "/caching");
      const flushed = page.waitForResponse((response) => new URL(response.url()).pathname === "/flushall" && response.request().method() === "POST");
      await page.getByRole("button", { name: "清空", exact: true }).click();
      const response = await flushed;
      expect(response.status(), await response.text()).toBe(200);
      await expect(page.getByTestId("column-result")).toContainText("cache flushed");
    };

    await flushFromBrowser();
    const requestBody = { model, messages: [{ role: "user", content: "identical cache request" }] };
    const logs: any[] = [];
    for (let index = 0; index < 3; index++) {
      if (index === 2) await flushFromBrowser();
      const response = await page.request.post(GATEWAY + "/v1/chat/completions", { headers: { Authorization: "Bearer " + key }, data: requestBody });
      expect(response.status(), await response.text()).toBe(200);
      const expectedHit = index === 1;
      expect(response.headers()["cache_hit"] === "true").toBe(expectedHit);
      expect(response.headers()["x-litellm-cache-hit"] === "true").toBe(expectedHit);
      const callID = response.headers()["x-litellm-call-id"];
      expect(callID).toBeTruthy();
      logs.push(await waitForLog(page.request, headers, callID));
    }
    expect(logs.map((row) => row.cache_hit)).toEqual(["false", "true", "false"]);
    expect(logs[0].cache_key).toBeTruthy();
    expect(logs.map((row) => row.cache_key)).toEqual([logs[0].cache_key, logs[0].cache_key, logs[0].cache_key]);
    expect(Number(logs[0].spend)).toBeCloseTo(0.000012, 10);
    expect(Number(logs[1].spend)).toBe(0);
    expect(Number(logs[2].spend)).toBeCloseTo(0.000012, 10);
  } finally {
    if (key) await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys: [key] } });
    await page.request.post(GATEWAY + "/model/delete", { headers, data: { id: deploymentID } });
    await page.request.post(GATEWAY + "/flushall", { headers, data: {} });
  }
});

/**
 * 目的：验证普通用户会话从专属团队继承模板，虚拟密钥从自身显式可信绑定预览同一模板，并与真实推理选择一致且预览不消耗 RPM。
 * 前置：隔离数据库和本地协议上游；创建专属团队/用户、A/B 双部署、A0/B1 模板，以及显式绑定模板且 rpm=1 的专用密钥。
 * 结果：先从模板页面打开 JSON 并保存真实模板，再验证不传 template_id/body 的团队会话和密钥预览均为 200 并选中 beta；连续密钥预览后首次推理仍成功且落 beta。
 * 清理：finally 解除团队与密钥绑定，删除密钥、模板、部署、用户和团队；显式模板与草稿权限继续由既有管理权限用例覆盖。
 */
test("team session inheritance and explicitly bound key preview match inference without consuming rpm", async ({ page }) => {
  test.setTimeout(120_000);
  await loginAdmin(page);
  const adminSession = await sessionBearer(page);
  const headers = { Authorization: "Bearer " + adminSession };
  const suffix = Date.now().toString();
  const model = "preview-inherit-" + suffix;
  const ids = ["preview-alpha-" + suffix, "preview-beta-" + suffix];
  const upstreams = ["custom/preview-alpha", "custom/preview-beta"];
  let templateID = "";
  let key = "";
  let teamID = "";
  let userID = "";
  const email = `preview-inherit-${suffix}@xhub.local`;
  const password = "preview-inherit-password";
  try {
    for (let index = 0; index < ids.length; index++) {
      const created = await page.request.post(GATEWAY + "/model/new", { headers, data: {
        model_name: model,
        litellm_params: { model: upstreams[index], api_base: UPSTREAM, api_key: "sk-fake", deployment_id: ids[index], custom_llm_provider: "custom", input_cost_per_token: 0.000001, output_cost_per_token: 0.000002 },
        model_info: { id: ids[index], transport: "bypass_openai_chat", endpoint_types: ["chat", "bypass:openai-chat"], pricing_source: "manual" },
      } });
      expect(created.status(), await created.text()).toBe(200);
    }
    const body = { model_routes: [{ model, strategy: "traffic-split", allocations: [{ deployment_id: ids[0], weight: 0 }, { deployment_id: ids[1], weight: 1 }] }], routing_groups: [], fallbacks: [], context_window_fallbacks: [], content_policy_fallbacks: [], retry_policy: { max_attempts: 1, timeout_seconds: 60, failure_threshold: 3, cooldown_seconds: 60 } };
    const saved = await page.request.post(GATEWAY + "/route_template/new", { headers, data: { name: "preview-inherit-" + suffix, body } });
    expect(saved.status(), await saved.text()).toBe(200);
    templateID = (await saved.json()).id;
    const organizations = await (await page.request.get(GATEWAY + "/organization/list", { headers })).json();
    const organization = organizations.find((row: any) => row.organization_alias === "e2e-fixture-org");
    const team = await page.request.post(GATEWAY + "/team/new", { headers, data: { team_alias: "preview-inherit-" + suffix, organization_id: organization.organization_id } });
    expect(team.status(), await team.text()).toBe(200);
    teamID = (await team.json()).team_id;
    const user = await page.request.post(GATEWAY + "/user/new", { headers, data: { user_email: email, password, user_role: "user", team_id: teamID, team_role: "user" } });
    expect(user.status(), await user.text()).toBe(200);
    userID = (await user.json()).user_id;
    const binding = await page.request.post(GATEWAY + "/route_template/binding", { headers, data: { scope: "team", scope_id: teamID, route_template_id: templateID } });
    expect(binding.status(), await binding.text()).toBe(200);

    // 从真实模板页面打开夹具并保存，覆盖用户可见的模板读取与写入；当前页面预览控件始终提交显式草稿，
    // 因而隐式团队/密钥继承仍由下方无 template_id/body 的真实预览请求核对。
    await stableGoto(page, "/route-templates");
    const templateRow = page.getByRole("row").filter({ has: page.getByText("preview-inherit-" + suffix, { exact: true }) });
    await expect(templateRow).toBeVisible();
    await templateRow.getByRole("button", { name: t("pages.routeTemplates.edit"), exact: true }).click();
    const editor = page.getByRole("region", { name: t("pages.routeTemplates.edit"), exact: true });
    await editor.getByRole("tab", { name: "JSON", exact: true }).click();
    await expect(editor.getByRole("textbox", { name: "JSON", exact: true })).toContainText(model);
    const updated = page.waitForResponse((response) => new URL(response.url()).pathname === "/route_template/" + templateID + "/update" && response.request().method() === "POST");
    await editor.getByRole("button", { name: "保存模板", exact: true }).click();
    expect((await updated).status()).toBe(200);

    const minted = await page.request.post(GATEWAY + "/key/generate", { headers, data: { key_alias: "preview-inherit-" + suffix, key_type: "llm_api", team_id: teamID, user_id: userID, route_template_id: templateID, rpm_limit: 1 } });
    expect(minted.status(), await minted.text()).toBe(200);
    key = (await minted.json()).key;

    /** 用途：调用不含显式模板和草稿的可信继承预览；参数 bearer 为会话或密钥；返回解析后的预览，网络或路由错误由断言失败。 */
    const preview = async (bearer: string) => {
      const response = await page.request.post(GATEWAY + "/route_template/preview", { headers: { Authorization: "Bearer " + bearer }, data: { model_name: model, endpoint_id: "chat" } });
      expect(response.status(), await response.text()).toBe(200);
      const result = await response.json();
      expect(result.template_id).toBe(templateID);
      expect(result.rule_source).toBe("template-model");
      const alpha = result.data.find((row: any) => row.deployment_id === ids[0]);
      const beta = result.data.find((row: any) => row.deployment_id === ids[1]);
      expect(alpha.target_share).toBe(0);
      expect(alpha.excluded_reason).toContain("no allocated traffic");
      expect(beta.target_share).toBe(1);
      return result;
    };

    await login(page, email, password);
    // login() 在提交表单后即返回；等待普通用户导航出现，确保团队会话 cookie 已落盘。
    await expect(page.getByText(t("nav.apiKeys")).first()).toBeVisible({ timeout: 20_000 });
    const memberSession = await sessionBearer(page);
    const sessionPreview = await preview(memberSession);
    expect(sessionPreview.source).toBe("team");
    const sessionInference = await page.request.post(GATEWAY + "/bypass/openai/v1/chat/completions", { headers: { Authorization: "Bearer " + memberSession }, data: { model, messages: [{ role: "user", content: "session inherited template" }] } });
    expect(sessionInference.status(), await sessionInference.text()).toBe(200);
    expect((await sessionInference.json()).model).toBe(upstreams[1]);

    for (let index = 0; index < 3; index++) {
      const keyPreview = await preview(key);
      expect(keyPreview.source).toBe("key");
    }
    const inferenceSession = "preview-inference-" + suffix;
    const keyInference = await page.request.post(GATEWAY + "/bypass/openai/v1/chat/completions", { headers: { Authorization: "Bearer " + key, "session-id": inferenceSession }, data: { model, messages: [{ role: "user", content: "key inherited template" }] } });
    expect(keyInference.status(), await keyInference.text()).toBe(200);
    expect((await keyInference.json()).model).toBe(upstreams[1]);
    const log = await waitForSessionLog(page.request, headers, inferenceSession);
    expect(log.team_id).toBe(teamID);
    expect(Number(log.spend)).toBeCloseTo(0.000012, 10);
  } finally {
    if (teamID) await page.request.post(GATEWAY + "/route_template/binding", { headers, data: { scope: "team", scope_id: teamID, route_template_id: "" } });
    if (key) {
      await page.request.post(GATEWAY + "/key/update", { headers, data: { key, route_template_id: "" } });
      await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys: [key] } });
    }
    if (templateID) await page.request.post(GATEWAY + "/route_template/" + templateID + "/delete", { headers, data: {} });
    for (const id of ids) await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } });
    if (userID) await page.request.post(GATEWAY + "/user/delete", { headers, data: { user_id: userID } });
    if (teamID) await page.request.post(GATEWAY + "/team/delete", { headers, data: { team_id: teamID } });
  }
});

/**
 * 目的：验证真实登录控制台可在中文与英文间切换，关键导航和页面标题随语言同步更新。
 * 前置：真实浏览器、网关和管理员夹具；不创建业务数据。
 * 结果：虚拟密钥与路由模板入口在两种语言下均可访问，按钮状态、导航文案和标题一致。
 * 清理：结束前切回中文，语言 cookie 随隔离浏览器上下文销毁，无后台资源需要删除。
 */
test("dashboard language switch updates key navigation in both locales", async ({ page }) => {
  await loginAdmin(page);
  await stableGoto(page, "/api-keys");
  await expect(page.getByRole("heading", { name: t("pages.apiKeys.title"), exact: true })).toBeVisible();
  const sidebar = page.locator('[data-slot="sidebar"]');
  await expect(sidebar.getByRole("link", { name: t("nav.apiKeys"), exact: true })).toBeVisible();

  const english = page.getByRole("button", { name: "English", exact: true });
  await english.click();
  await expect(english).toHaveAttribute("aria-pressed", "true");
  const en = (key: string) => translate("en", key);
  await expect(page.getByRole("heading", { name: en("pages.apiKeys.title"), exact: true })).toBeVisible();
  await sidebar.getByRole("link", { name: en("nav.routeTemplates"), exact: true }).click();
  await expect(page.getByRole("heading", { name: en("pages.routeTemplates.title"), exact: true })).toBeVisible();
  const englishEmptyState = page.getByText(en("pages.routeTemplates.emptyTitle"), { exact: true }).locator("..");
  await expect(englishEmptyState.getByRole("button", { name: en("pages.routeTemplates.create"), exact: true })).toBeVisible();

  const chinese = page.getByRole("button", { name: en("language.zhCN"), exact: true });
  await chinese.click();
  await expect(chinese).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("heading", { name: t("pages.routeTemplates.title"), exact: true })).toBeVisible();
  const chineseEmptyState = page.getByText(t("pages.routeTemplates.emptyTitle"), { exact: true }).locator("..");
  await expect(chineseEmptyState.getByRole("button", { name: t("pages.routeTemplates.create"), exact: true })).toBeVisible();
});

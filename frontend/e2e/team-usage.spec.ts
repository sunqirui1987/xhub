import { expect, test } from "@playwright/test";
import { GATEWAY, loginAdmin, sessionBearer, stableGoto, t, watchGateway } from "./helpers";

/** 前置真实浏览器、隔离网关与 PostgreSQL、本地供应商；管理员创建专用团队和密钥，实调后打开团队用量。
 * 验证真实账单、聚合接口、费用/模型/密钥页签及重新打开页面均可用，且无 Agent 请求或 404。
 * 参数为 Playwright 页面夹具，无返回值；finally 删除密钥和团队，外层脚本删除私有 schema 中的账单。 */
test("team usage displays recorded calls without requesting the removed Agent endpoint", async ({ page }) => {
  await loginAdmin(page);
  const guard = watchGateway(page);
  const headers = { Authorization: "Bearer " + await sessionBearer(page) };
  const organizations = await page.request.get(GATEWAY + "/organization/list", { headers });
  expect(organizations.status()).toBe(200);
  const org = (await organizations.json()).find((row: { organization_alias: string }) => row.organization_alias === "e2e-fixture-org");
  let teamID = "", key = "";
  const agentRequests: string[] = [], errors: string[] = [];
  page.on("request", request => {
    if (new URL(request.url()).pathname === "/agent/daily/activity") agentRequests.push(request.url());
  });
  page.on("response", response => {
    if (response.url().includes("/daily/activity") && response.status() >= 400) errors.push(response.status() + " " + response.url());
  });
  page.on("console", message => {
    if (message.type() === "error" && message.text().includes("Failed to fetch daily activity")) errors.push(message.text());
  });
  try {
    const created = await page.request.post(GATEWAY + "/team/new", { headers, data: { organization_id: org.organization_id, team_alias: "team-usage-" + Date.now() } });
    expect(created.status(), await created.text()).toBe(200);
    const team = await created.json();
    teamID = team.team_id;
    const minted = await page.request.post(GATEWAY + "/key/generate", { headers, data: { key_alias: "team-usage-probe", team_id: teamID } });
    expect(minted.status(), await minted.text()).toBe(200);
    key = (await minted.json()).key;
    const completion = await page.request.post(GATEWAY + "/v1/chat/completions", { headers: { Authorization: "Bearer " + key }, data: { model: "gpt-4o-mini", messages: [{ role: "user", content: "team usage regression" }] } });
    expect(completion.status(), await completion.text()).toBe(200);
    expect((await completion.json()).choices[0].message.content).toBe("e2e-ok");
    const callID = completion.headers()["x-litellm-call-id"];
    expect(callID).toBeTruthy();
    await expect.poll(async () => {
      const detail = await page.request.get(GATEWAY + "/spend/logs/ui/" + callID, { headers });
      return detail.ok() ? (await detail.json()).model : "";
    }, { message: "数据面调用必须持久化账单" }).toBe("gpt-4o-mini");
    const activity = await page.request.get(GATEWAY + "/team/daily/activity/aggregated?team_ids=" + encodeURIComponent(teamID), { headers });
    expect(activity.status(), await activity.text()).toBe(200);
    expect((await activity.json()).metadata.total_api_requests).toBe(1);
    for (let visit = 0; visit < 2; visit++) {
      await stableGoto(page, "/usage");
      await expect(page.getByRole("tab", { name: t("pages.usage.customer"), exact: true })).toHaveCount(0);
      await expect(page.getByRole("tab", { name: t("pages.usage.tag"), exact: true })).toHaveCount(0);
      const loaded = page.waitForResponse(response => new URL(response.url()).pathname === "/team/daily/activity/aggregated");
      await page.getByRole("tab", { name: t("pages.usage.team"), exact: true }).click();
      const response = await loaded;
      expect(response.status(), await response.text()).toBe(200);
      expect((await response.json()).metadata.total_api_requests).toBeGreaterThanOrEqual(1);
      await expect(page.getByText(t("{capitalizedEntityLabel} Spend Overview").replace("{capitalizedEntityLabel}", "Team"), { exact: true })).toBeVisible();
      await expect(page.getByText(team.team_alias, { exact: true }).first()).toBeVisible();
      await page.getByRole("tab", { name: t("pages.usage.modelActivity"), exact: true }).click();
      await expect(page.getByText("gpt-4o-mini", { exact: true }).filter({ visible: true }).first()).toBeVisible();
      await page.getByRole("tab", { name: t("Key Activity"), exact: true }).click();
      await expect(page.getByRole("button", { name: /^team-usage-probe \(team:/ })).toBeVisible();
      await expect(page.getByRole("tab", { name: t("pages.usage.agentActivity"), exact: true })).toHaveCount(0);
      await expect(page.getByText(t("Top Agents Driving Spend"), { exact: true })).toHaveCount(0);
    }
    expect(agentRequests, "团队用量不得请求已移除的 Agent 接口").toEqual([]);
    expect(errors, "用量请求应成功且无控制台异常").toEqual([]);
    guard.assertOk();
  } finally {
    if (key) expect((await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys: [key] } })).status()).toBe(200);
    if (teamID) expect((await page.request.post(GATEWAY + "/team/delete", { headers, data: { team_id: teamID } })).status()).toBe(200);
  }
});

/** 前置真实浏览器、隔离网关/PostgreSQL及本地供应商；创建收费模型、失败模型和专用团队密钥后实调。
 * 验证全局/组织/团队/用户统计与响应一致、模型/密钥/端点、零价供应商开关、日期及用户筛选、CSV 下载。
 * 参数为页面及证据夹具，无返回值；finally 删除密钥、团队和模型，脚本清理全部隔离账单及 schema。 */
test("usage views reconcile stored successes, failures, dimensions, filters and CSV", async ({ page }, testInfo) => {
  test.setTimeout(180_000);
  await loginAdmin(page);
  const guard = watchGateway(page);
  const headers = { Authorization: "Bearer " + await sessionBearer(page) };
  const organizations = await (await page.request.get(GATEWAY + "/organization/list", { headers })).json();
  const org = organizations.find((row: any) => row.organization_alias === "e2e-fixture-org");
  let teamID = "", key = "";
  const models: string[] = [];
  const modelName = "usage-priced-" + Date.now(), failedModel = modelName + "-failure";
  const requests: string[] = [];
  page.on("request", request => { if (/\/(agent|customer|tag)\/daily\/activity/.test(request.url())) requests.push(request.url()); });
  try {
    for (const [name, upstream] of [[modelName, "usage-success"], [failedModel, "e2e-error-log-json-usage"]]) {
      const response = await page.request.post(GATEWAY + "/model/new", { headers, data: { model_name: name, litellm_params: { model: upstream, api_base: process.env.E2E_UPSTREAM || "http://127.0.0.1:4110", api_key: "sk-fake", custom_llm_provider: "custom", input_cost_per_token: 0.01, output_cost_per_token: 0.02 }, model_info: { transport: "bypass_openai_chat", pricing_source: "manual" } } });
      expect(response.status(), await response.text()).toBe(200);
      models.push((await response.json()).model_info.id);
    }
    const created = await page.request.post(GATEWAY + "/team/new", { headers, data: { organization_id: org.organization_id, team_alias: modelName } });
    expect(created.status(), await created.text()).toBe(200); teamID = (await created.json()).team_id;
    const minted = await page.request.post(GATEWAY + "/key/generate", { headers, data: { key_alias: modelName, team_id: teamID } });
    expect(minted.status(), await minted.text()).toBe(200); key = (await minted.json()).key;
    for (const [model, success] of [[modelName, true], [failedModel, false]] as const) {
      const result = await page.request.post(GATEWAY + "/v1/chat/completions", { headers: { Authorization: "Bearer " + key }, data: { model, messages: [{ role: "user", content: "usage reconciliation" }] } });
      expect(result.status(), await result.text()).toBe(success ? 200 : 502);
      const callID = result.headers()["x-litellm-call-id"];
      expect(callID).toBeTruthy();
      await expect.poll(async () => { const log = await page.request.get(GATEWAY + "/spend/logs/ui/" + callID, { headers }); return log.ok() ? (await log.json()).model : ""; }).toBe(model);
    }
    const exact = await (await page.request.get(GATEWAY + "/team/daily/activity/aggregated?team_ids=" + teamID, { headers })).json();
    expect(exact.metadata).toMatchObject({ total_api_requests: 2, total_successful_requests: 1, total_failed_requests: 1, total_tokens: 10 });
    expect(exact.metadata.total_spend).toBeCloseTo(0.12, 8);
    const initial = page.waitForResponse(r => new URL(r.url()).pathname === "/user/daily/activity/aggregated");
    await stableGoto(page, "/usage");
    const initialBody = await (await initial).json();
    await assertUsageTiles(page, initialBody.metadata);
    await page.getByRole("button", { name: new RegExp("^" + t("Total Tokens")) }).click();
    await expect(page.getByRole("heading", { name: t("Input Tokens"), exact: true })).toBeVisible();
    // 显示零价及未知供应商后逐行对账，避免失败和免费调用在费用表中消失。
    await page.getByText(t("Show Zero Spend"), { exact: true }).locator("..").getByRole("switch").check();
    await page.getByText(t("Show Unknown"), { exact: true }).locator('xpath=ancestor::*[descendant::*[@role="switch"]][1]').getByRole("switch").check();
    const providers: Record<string, any> = {};
    for (const day of initialBody.results) for (const [name, bucket] of Object.entries(day.breakdown.providers ?? {})) {
      const metrics = (bucket as any).metrics;
      const sum = providers[name] ??= { successful_requests: 0, failed_requests: 0, total_tokens: 0, spend: 0 };
      for (const metric of Object.keys(sum)) sum[metric] += metrics[metric] ?? 0;
    }
    expect(Object.values(providers).some(metrics => metrics.spend === 0)).toBe(true);
    for (const [name, metrics] of Object.entries(providers)) {
      const row = page.getByRole("row").filter({ has: page.getByRole("cell", { name, exact: true }) });
      await expect(row.getByRole("cell").nth(2)).toHaveText(metrics.successful_requests.toLocaleString());
      await expect(row.getByRole("cell").nth(3)).toHaveText(metrics.failed_requests.toLocaleString());
      await expect(row.getByRole("cell").nth(4)).toHaveText(metrics.total_tokens.toLocaleString());
    }
    await page.getByRole("tab", { name: t("pages.usage.modelActivity"), exact: true }).click();
    await expect(page.getByText(modelName, { exact: true }).filter({ visible: true }).first()).toBeVisible();
    await expect(page.getByText(failedModel, { exact: true }).filter({ visible: true }).first()).toBeVisible();
    await expect(page.getByRole("tabpanel")).not.toContainText(/NaN|Infinity/);
    for (const [view, path] of [["organization", "/organization/daily/activity"], ["team", "/team/daily/activity/aggregated"], ["user", "/user/daily/activity"]]) {
      const loading = page.waitForResponse(r => new URL(r.url()).pathname === path && r.ok());
      await page.getByRole("tab", { name: t("pages.usage." + view), exact: true }).click();
      const body = await (await loading).json();
      await assertUsageTiles(page, body.metadata);
      await page.getByRole("tab", { name: t("pages.usage.modelActivity"), exact: true }).click();
      await expect(page.getByText(modelName, { exact: true }).filter({ visible: true }).first()).toBeVisible();
      await page.getByRole("tab", { name: t("Key Activity"), exact: true }).click();
      await page.getByRole("textbox", { name: t("Search keys"), exact: true }).fill(modelName);
      await expect(page.getByRole("button", { name: new RegExp("^" + modelName + " [(]team:") })).toBeVisible();
      await page.getByRole("tab", { name: t("Endpoint Activity"), exact: true }).click();
      // 日报按网关规范化后的端点聚合，用真实响应标识定位并逐列核对，避免写死 /v1 前缀。
      const endpoints: Record<string, any> = {};
      for (const day of body.results) for (const [name, bucket] of Object.entries(day.breakdown.endpoints ?? {})) {
        const metrics = (bucket as any).metrics;
        const sum = endpoints[name] ??= { api_requests: 0, successful_requests: 0, failed_requests: 0, total_tokens: 0, spend: 0 };
        for (const metric of Object.keys(sum)) sum[metric] += metrics[metric] ?? 0;
      }
      expect(Object.keys(endpoints).length).toBeGreaterThan(0);
      for (const [name, metrics] of Object.entries(endpoints)) {
        const row = page.getByRole("row").filter({ has: page.getByRole("cell", { name, exact: true }) });
        await expect(row.getByRole("cell").nth(2)).toHaveText(metrics.api_requests.toLocaleString());
        await expect(row.getByRole("cell").nth(4)).toHaveText(metrics.total_tokens.toLocaleString());
        await expect(row.getByRole("cell").nth(3)).toHaveText((metrics.successful_requests / metrics.api_requests * 100).toFixed(2) + "%");
      }
      await page.getByRole("tab", { name: t("pages.usage.cost"), exact: true }).click();
      const download = page.waitForEvent("download");
      await page.getByRole("button", { name: t("Export Data"), exact: true }).click();
      await page.getByRole("dialog").getByRole("button", { name: t("Export {value0}").replace("{value0}", "CSV"), exact: true }).click();
      const file = await download;
      const output = testInfo.outputPath(view + "-usage.csv"); await file.saveAs(output);
      const csv = await import("node:fs/promises").then(fs => fs.readFile(output, "utf8"));
      const parsed = (await import("papaparse")).default.parse<Record<string, string>>(csv, { header: true, skipEmptyLines: true });
      expect(parsed.errors).toEqual([]);
      expect(parsed.data.reduce((sum, row) => sum + Number(row.Requests), 0)).toBe(body.metadata.total_api_requests);
      expect(parsed.data.reduce((sum, row) => sum + Number(row["Total Tokens"]), 0)).toBe(body.metadata.total_tokens);
      expect(parsed.data.reduce((sum, row) => sum + Number(row["Successful Requests"]), 0)).toBe(body.metadata.total_successful_requests);
      expect(parsed.data.reduce((sum, row) => sum + Number(row["Failed Requests"]), 0)).toBe(body.metadata.total_failed_requests);
      expect(parsed.data.reduce((sum, row) => sum + Number(row["Spend ($)"].replaceAll(",", "")), 0)).toBeCloseTo(body.metadata.total_spend, 4);
      await testInfo.attach(view + " CSV", { path: output, contentType: "text/csv" });
    }
    await page.getByRole("tab", { name: t("pages.usage.global"), exact: true }).click();
    const userInput = page.getByTestId("user-dropdown").getByRole("combobox");
    await userInput.click(); await userInput.fill("admin");
    const userLoaded = page.waitForResponse(r => { const url = new URL(r.url()); return url.pathname === "/user/daily/activity/aggregated" && !!url.searchParams.get("user_id"); });
    await page.getByRole("option", { name: /^admin/ }).first().click();
    await assertUsageTiles(page, (await (await userLoaded).json()).metadata);
    const dateLoaded = page.waitForResponse(r => { const url = new URL(r.url()); return url.pathname === "/user/daily/activity/aggregated" && url.searchParams.get("start_date") === url.searchParams.get("end_date"); });
    await page.getByRole("button", { name: /\d.* - .*\d/ }).first().click();
    await page.getByRole("button", { name: new RegExp("^" + t("Today")) }).click();
    await assertUsageTiles(page, (await (await dateLoaded).json()).metadata);
    await expect(page.getByRole("tab", { name: t("pages.usage.customer"), exact: true })).toHaveCount(0);
    await expect(page.getByRole("tab", { name: t("pages.usage.tag"), exact: true })).toHaveCount(0);
    expect(requests, "剩余视图不得请求移除维度").toEqual([]);
    await testInfo.attach("用量核对", { body: await page.screenshot({ fullPage: true }), contentType: "image/png" });
    guard.assertOk();
  } finally {
    if (key) expect((await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys: [key] } })).status()).toBe(200);
    if (teamID) expect((await page.request.post(GATEWAY + "/team/delete", { headers, data: { team_id: teamID } })).status()).toBe(200);
    for (const id of models) expect((await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } })).status()).toBe(200);
  }
});

/** 对照真实接口验证当前成本页可见请求、成功、失败与 Token；参数为页面和元数据，无返回值。
 * 供全部用量视图调用，允许异步渲染完成；只读取可访问标题及同卡片数字，无写入及清理需求。 */
async function assertUsageTiles(page: import("@playwright/test").Page, metadata: Record<string, number>) {
  for (const [title, key] of [["Total Requests", "total_api_requests"], ["Successful Requests", "total_successful_requests"], ["Failed Requests", "total_failed_requests"], ["Total Tokens", "total_tokens"]]) {
    const heading = page.getByRole("heading", { name: t(title), exact: true }).first();
    // 成功/失败标题可能含提示容器，选取最近含数字段落的卡片，避免绑定固定父层级。
    await expect(heading.locator("xpath=ancestor::*[p][1]").locator(":scope > p")).toHaveText(metadata[key].toLocaleString());
  }
  // 全局费用使用四位小数，实体总览使用两位；均按界面的实际显示精度对账。
  const spendHeading = page.getByRole("heading", { name: t("Total Spend"), exact: true });
  const spendLabel = page.getByText(t("Total Spend"), { exact: true }).filter({ visible: true });
  const entityView = await spendHeading.count() > 0;
  const spendValue = entityView
    ? spendHeading.locator("xpath=ancestor::*[p][1]").locator(":scope > p")
    : spendLabel.locator("..").locator("p").last();
  await expect(spendValue).toHaveText("$" + metadata.total_spend.toLocaleString("en-US", { minimumFractionDigits: entityView ? 2 : 4, maximumFractionDigits: entityView ? 2 : 4 }));
}

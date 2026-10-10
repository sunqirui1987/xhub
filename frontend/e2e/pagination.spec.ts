import { expect, test, type Page } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/** 按公开模型筛选真实日志；参数是浏览器与当前测试模型名，返回筛选完成状态。
 * 用于隔离成功/失败分页数据，只操作页面控件，失败由断言抛出，数据由调用方清理。 */
async function filterModel(page: Page, model: string): Promise<void> {
  await page.getByTestId("datatable-filters-trigger").click();
  await page.getByPlaceholder(t("Enter public model or search tool…")).fill(model);
  await page.getByTestId("filter-drawer-apply").click();
}

/** 在真实表格切换每页条数；参数指定浏览器与条数，返回选择完成状态。
 * 用于验证共享控件回到第一页，不拦截后台请求，无额外数据需要清理。 */
async function changeSize(page: Page, size: number): Promise<void> {
  await page.getByRole("combobox", { name: t("Rows per page"), exact: true }).click();
  await page.getByRole("option", { name: String(size), exact: true }).click();
}

for (const failure of [false, true]) {
  /** 前置真实浏览器、网关、私有数据库和本地上游；成功请求两次一会话，失败请求同会话。
   * 验证完整合并后分页/失败逐条分页、首页末页边界、无重复遗漏、条数切换及空搜索。
   * 参数 page 是浏览器；finally 删除专属密钥与部署，日志随运行器私有 schema 清理，不用外部凭据。 */
  test("日志分页完整流程：" + (failure ? "失败请求逐条翻页" : "会话合并后翻页"), async ({ page }) => {
    test.setTimeout(120_000);
    await loginAdmin(page);
    const headers = { Authorization: "Bearer " + await sessionBearer(page) };
    const model = "pagination-" + (failure ? "error-" : "session-") + Date.now();
    const deployment = await page.request.post(GATEWAY + "/model/new", { headers, data: {
      model_name: model,
      litellm_params: { model: failure ? "e2e-error-log-json-pagination" : "e2e-pagination-success", api_base: UPSTREAM,
        api_key: "sk-fake", custom_llm_provider: "custom", input_cost_per_token: 0.000001, output_cost_per_token: 0.000002 },
      model_info: { transport: "bypass_openai_chat", pricing_source: "manual" },
    } });
    expect(deployment.status(), await deployment.text()).toBe(200);
    const deploymentId = (await deployment.json()).model_info.id;
    let keyId: string | undefined;
    try {
      const generated = await page.request.post(GATEWAY + "/key/generate", { headers, data: { key_alias: model } });
      expect(generated.status(), await generated.text()).toBe(200);
      const key = await generated.json();
      keyId = key.token_id;
      const count = failure ? 26 : 60;
      for (let i = 0; i < count; i++) {
        const response = await page.request.post(GATEWAY + "/v1/chat/completions", {
          headers: { Authorization: "Bearer " + key.key, "session-id": model + "-" + (failure ? "shared" : Math.floor(i / 2)) },
          data: { model, messages: [{ role: "user", content: "pagination " + i }] },
        });
        expect(response.status(), await response.text()).toBe(failure ? 502 : 200);
      }
      const total = failure ? 26 : 30;
      const query = new URLSearchParams({ model, group_by_session: String(!failure), status_filter: failure ? "error" : "non_error", page_size: "25" });
      await expect.poll(async () => {
        const response = await page.request.get(GATEWAY + "/spend/logs/ui?" + query, { headers });
        expect(response.status()).toBe(200);
        return (await response.json()).total;
      }, { message: "所有真实推理请求必须落库后再验证分页" }).toBe(total);
      const first = await (await page.request.get(GATEWAY + "/spend/logs/ui?" + query + "&page=1", { headers })).json();
      const last = await (await page.request.get(GATEWAY + "/spend/logs/ui?" + query + "&page=2", { headers })).json();
      expect(first.data).toHaveLength(25);
      expect(last.data).toHaveLength(total - 25);
      expect(new Set([...first.data, ...last.data].map((row: any) => failure ? row.request_id : row.session_id)).size).toBe(total);
      if (!failure) {
        for (const row of [...first.data, ...last.data]) {
          expect(row.session_total_count).toBe(2);
          expect(row.session_total_tokens).toBeGreaterThan(0);
          expect(row.session_total_spend).toBeGreaterThan(0);
        }
      }
      await stableGoto(page, "/logs");
      if (failure) await page.getByRole("tab", { name: t("Error Logs"), exact: true }).click();
      await filterModel(page, model);
      await expect(page.getByTestId("pagination-range")).toHaveText(t("Showing {start}-{end} of {count}", { start: 1, end: 25, count: total }));
      await expect(page.getByTestId("pagination-first")).toBeDisabled();
      const firstIds = await page.getByRole("row").filter({ hasText: model }).allTextContents();
      expect(firstIds).toHaveLength(25);
      const nextResponse = page.waitForResponse((res) => res.url().includes("/spend/logs/ui?") && new URL(res.url()).searchParams.get("page") === "2");
      await page.getByTestId("pagination-next").click();
      expect((await (await nextResponse).json()).page).toBe(2);
      await expect(page.getByTestId("pagination-range")).toHaveText(t("Showing {start}-{end} of {count}", { start: 26, end: total, count: total }));
      await expect(page.getByTestId("pagination-next")).toBeDisabled();
      await expect(page.getByTestId("pagination-last")).toBeDisabled();
      await expect(page.getByRole("row").filter({ hasText: model })).toHaveCount(total - 25);
      await page.getByTestId("pagination-first").click();
      await expect(page.getByTestId("pagination-prev")).toBeDisabled();
      await expect.poll(() => page.getByRole("row").filter({ hasText: model }).allTextContents()).toEqual(firstIds);
      await page.getByTestId("pagination-last").click();
      await expect(page.getByTestId("pagination-next")).toBeDisabled();
      await changeSize(page, 50);
      await expect(page.getByTestId("pagination-range")).toHaveText(t("Showing {start}-{end} of {count}", { start: 1, end: total, count: total }));
      await expect(page.getByTestId("pagination-first")).toBeDisabled();
      await expect(page.getByRole("row").filter({ hasText: model })).toHaveCount(total);
      await page.getByTestId("datatable-search").fill("pagination-missing-request");
      await expect(page.getByTestId("pagination-range")).toHaveText(t("No results"));
      await expect(page.getByTestId("pagination-next")).toBeDisabled();
      await page.getByTestId("datatable-search").fill("");
      await expect(page.getByRole("row").filter({ hasText: model })).toHaveCount(total);
    } finally {
      if (keyId) {
        const deleted = await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys: [keyId] } });
        expect(deleted.status(), await deleted.text()).toBe(200);
      }
      const removed = await page.request.post(GATEWAY + "/model/delete", { headers, data: { id: deploymentId } });
      expect(removed.status(), await removed.text()).toBe(200);
    }
  });
}

/** 前置真实浏览器/网关/数据库；创建 11 项目及 26 个人和 6 项目密钥产生可分页数据与审计。
 * 验证客户端项目、服务端密钥/项目密钥/审计翻页、请求页码、筛选和每页条数重置。
 * 参数 page 是浏览器；finally 精确删除本测试的密钥、项目和团队，审计随私有 schema 清理。 */
test("共享分页覆盖密钥、项目、项目密钥和审计", async ({ page }) => {
  test.setTimeout(120_000);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + await sessionBearer(page) };
  const prefix = "pagination-lists-" + Date.now();
  const orgs = await (await page.request.get(GATEWAY + "/organization/list", { headers })).json();
  const teamResponse = await page.request.post(GATEWAY + "/team/new", { headers, data: {
    team_alias: prefix, organization_id: orgs.find((org: any) => org.organization_alias === "e2e-fixture-org").organization_id,
  } });
  expect(teamResponse.status(), await teamResponse.text()).toBe(200);
  const teamId = (await teamResponse.json()).team_id;
  const projects: string[] = [];
  const keys: string[] = [];
  try {
    for (let i = 0; i < 11; i++) {
      const created = await page.request.post(GATEWAY + "/project/new", { headers, data: { team_id: teamId, project_alias: prefix + "-project-" + i } });
      expect(created.status(), await created.text()).toBe(200);
      projects.push((await created.json()).project_id);
    }
    for (let i = 0; i < 32; i++) {
      const created = await page.request.post(GATEWAY + "/key/generate", { headers, data: {
        key_alias: prefix + (i < 26 ? "-personal-" : "-project-key-") + i,
        ...(i >= 26 ? { team_id: teamId, project_id: projects[0] } : {}),
      } });
      expect(created.status(), await created.text()).toBe(200);
      keys.push((await created.json()).token_id);
    }
    await stableGoto(page, "/api-keys?page_size=25");
    await page.getByPlaceholder(t("pages.apiKeys.searchPlaceholder")).fill(prefix + "-personal-");
    await expect(page.getByRole("row").filter({ hasText: prefix })).toHaveCount(25);
    const keyResponse = page.waitForResponse((res) => res.url().includes("/key/list?") && new URL(res.url()).searchParams.get("page") === "2");
    await page.getByTestId("pagination-next").click();
    expect((await (await keyResponse).json()).total_count).toBe(26);
    await expect(page.getByRole("row").filter({ hasText: prefix })).toHaveCount(1);
    await expect(page.getByTestId("pagination-next")).toBeDisabled();
    await changeSize(page, 50);
    await expect(page.getByRole("row").filter({ hasText: prefix })).toHaveCount(26);
    await expect(page.getByTestId("pagination-prev")).toBeDisabled();

    await stableGoto(page, "/projects");
    await page.getByPlaceholder(t("pages.projects.searchPlaceholder")).fill(prefix);
    await expect(page.getByRole("row").filter({ hasText: prefix })).toHaveCount(10);
    await page.getByTestId("pagination-next").click();
    await expect(page.getByRole("row").filter({ hasText: prefix })).toHaveCount(1);
    await expect(page).toHaveURL(/page=2/);
    await changeSize(page, 25);
    await expect(page.getByRole("row").filter({ hasText: prefix })).toHaveCount(11);
    await expect(page.getByTestId("pagination-prev")).toBeDisabled();

    await stableGoto(page, "/projects?project=" + projects[0]);
    await expect(page.getByRole("row").filter({ hasText: prefix + "-project-key-" })).toHaveCount(5);
    const projectKeyResponse = page.waitForResponse((res) => res.url().includes("/key/list?") && new URL(res.url()).searchParams.get("page") === "2");
    await page.getByTestId("pagination-next").click();
    expect((await (await projectKeyResponse).json()).total_count).toBe(6);
    await expect(page.getByRole("row").filter({ hasText: prefix + "-project-key-" })).toHaveCount(1);
    await page.getByPlaceholder(t("Filter by key name...")).fill(prefix + "-project-key-26");
    await expect(page.getByTestId("pagination-prev")).toBeDisabled();
    await expect(page.getByRole("row").filter({ hasText: prefix + "-project-key-26" })).toHaveCount(1);
    await page.getByPlaceholder(t("Filter by key name...")).fill("");
    await expect(page.getByRole("row").filter({ hasText: prefix + "-project-key-" })).toHaveCount(5);
    await page.getByTestId("pagination-next").click();
    await changeSize(page, 10);
    await expect(page.getByRole("row").filter({ hasText: prefix + "-project-key-" })).toHaveCount(6);
    await expect(page.getByTestId("pagination-prev")).toBeDisabled();

    await stableGoto(page, "/audit-logs");
    await changeSize(page, 25);
    await expect(page.getByTestId("pagination-next")).toBeEnabled();
    const auditResponse = page.waitForResponse((res) => res.url().includes("/audit/logs?") && new URL(res.url()).searchParams.get("page") === "2");
    await page.getByTestId("pagination-next").click();
    expect((await (await auditResponse).json()).page).toBe(2);
    await expect(page.getByTestId("pagination-prev")).toBeEnabled();
    await changeSize(page, 100);
    await expect(page.getByTestId("pagination-prev")).toBeDisabled();
    await page.getByPlaceholder(t("audit.search")).fill("pagination-no-audit-match");
    await expect(page.getByTestId("pagination-range")).toHaveText(t("No results"));
    await expect(page.getByTestId("pagination-next")).toBeDisabled();
  } finally {
    if (keys.length) {
      const deleted = await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys } });
      expect(deleted.status(), await deleted.text()).toBe(200);
    }
    for (const projectId of projects) {
      const deleted = await page.request.post(GATEWAY + "/project/delete", { headers, data: { project_id: projectId } });
      expect(deleted.status(), await deleted.text()).toBe(200);
    }
    const deleted = await page.request.post(GATEWAY + "/team/delete", { headers, data: { team_id: teamId } });
    expect(deleted.status(), await deleted.text()).toBe(200);
  }
});

import { execFileSync } from "node:child_process";
import { expect, test } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/** 前置真实浏览器、隔离数据库和本地视频上游；验证两个 Seedance 路径的模型详情不再展示公共定义入口，
 * 详情 curl 创建/查询、参数文档、调试台正文和任务 ID 同步、无效 JSON 降级、认证与计费。参数为页面夹具，无返回值；finally 删除部署，任务和账单随私有 schema 清理。
 * 本地上游仅验证协议、降级提示和结算，不依赖外部供应商凭据。 */
for (const native of [
  {
    transport: "ark_contents_generation",
    supplier: "volcengine",
    model: "volcengine/doubao-seedance-2-0-260128",
    path: "/api/v3/contents/generations/tasks",
  },
  {
    transport: "qiniu_contents_generation",
    supplier: "custom",
    model: "bytedance/doubao-seedance-2-0-mini-260615",
    path: "/v3/contents/generations/tasks",
  },
]) {
  test("视频模型无需公共接口定义即可创建查询任务：" + native.path, async ({ page }) => {
    const schemaRequests: string[] = [];
    page.on("request", (request) => {
      if (new URL(request.url()).pathname === "/openapi.json") schemaRequests.push(request.url());
    });
    await loginAdmin(page);
    const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
    const name = "e2e-no-schema-" + native.transport + "-" + Date.now();
    let id = "";
    let key = "";
    try {
      const saved = await page.request.post(GATEWAY + "/model/new", {
        headers,
        data: {
          model_name: name,
          litellm_params: {
            model: native.model,
            custom_llm_provider: native.supplier,
            api_base: UPSTREAM,
            api_key: "sk-fake",
            output_cost_per_token: 0.00001,
          },
          model_info: { transport: native.transport, endpoint_types: ["bypass:ark-video"], pricing_source: "manual" },
        },
      });
      expect(saved.status(), await saved.text()).toBe(200);
      id = (await saved.json()).model_info.id;
      const teams = await (await page.request.get(GATEWAY + "/v2/team/list?page=1&page_size=500", { headers })).json();
      const team = teams.teams.find((item: { team_alias: string }) => item.team_alias === "e2e-fixture-team");
      const jwt = (await page.context().cookies()).find((cookie) => cookie.name === "token")!.value;
      const userId = JSON.parse(Buffer.from(jwt.split(".")[1], "base64url").toString()).user_id;
      const minted = await page.request.post(GATEWAY + "/key/generate", {
        headers,
        data: { key_alias: name, key_type: "llm_api", team_id: team.team_id, user_id: userId, models: [name] },
      });
      expect(minted.status(), await minted.text()).toBe(200);
      key = (await minted.json()).key;
      await stableGoto(page, "/mine-models");
      await page.getByRole("textbox", { name: t("Search"), exact: true }).fill(name);
      const card = page.getByRole("article").filter({ has: page.getByRole("heading", { name, exact: true }) });
      await card.getByRole("button", { name: "API 接入", exact: true }).click();
      const dialog = page.getByRole("dialog", { name, exact: true });
      await expect(dialog.getByRole("combobox", { name: "选择调用接口" })).toHaveValue(native.path);
      await expect(dialog.getByRole("link", { name: "查看 API 接口定义" })).toHaveCount(0);
      await expect(dialog.getByRole("link", { name: "管理虚拟密钥" })).toBeVisible();
      await expect(dialog.getByRole("heading", { name: "2. 创建生成任务" })).toBeVisible();
      const curl = await dialog.getByLabel("复制调用示例", { exact: true }).filter({ hasText: "curl" }).textContent();
      expect(curl).toContain(GATEWAY + native.path);
      expect(curl).toContain('"content"');
      expect(curl).not.toMatch(/[\u4e00-\u9fff]/);
      await dialog.getByRole("button", { name: "查看协议参数" }).click();
      await expect(dialog.getByRole("table", { name: "协议参数说明" })).toContainText("content.video_url");
      await dialog.getByRole("button", { name: "返回 curl 步骤" }).click();
      expect(schemaRequests, "浏览器不应请求公共定义").toEqual([]);
      const payload = { model: name, content: [{ type: "text", text: "e2e-no-schema-video" }] };
      const denied = await page.request.post(GATEWAY + native.path, { data: payload });
      expect(denied.status(), "视频调用仍要求认证").toBe(401);
      const created = JSON.parse(
        execFileSync("bash", ["-c", curl!], {
          env: { ...process.env, XHUB_API_KEY: key },
          timeout: 20_000,
          encoding: "utf8",
        }),
      );
      const taskId = created.id;
      expect(taskId).toBeTruthy();
      const query = await dialog.getByLabel("复制结果查询", { exact: true }).filter({ hasText: "curl" }).textContent();
      const result = JSON.parse(
        execFileSync("bash", ["-c", query!], {
          env: { ...process.env, XHUB_API_KEY: key, TASK_ID: taskId },
          timeout: 20_000,
          encoding: "utf8",
        }),
      );
      expect(result).toMatchObject({ id: taskId, status: "succeeded", usage: { completion_tokens: 100 } });
      await expect
        .poll(async () => {
          const response = await page.request.get(GATEWAY + "/spend/logs/ui?page=1&page_size=50", { headers });
          const logs = await response.json();
          const rows = Array.isArray(logs) ? logs : (logs.data ?? logs.logs ?? []);
          const body = rows.find((row: { model: string }) => row.model === name);
          return body
            ? {
                model: body.model,
                status: body.status,
                spend: Number(body.spend),
                completion_tokens: body.completion_tokens,
              }
            : null;
        })
        .toEqual({ model: name, status: "completed", spend: 0.001, completion_tokens: 100 });
      await page.keyboard.press("Escape");
      await stableGoto(page, "/playground");
      await page.getByPlaceholder(t("Select a Model"), { exact: true }).click();
      await page.getByRole("option", { name, exact: true }).click();
      await page
        .getByLabel("原生请求参数")
        .fill(JSON.stringify({ content: [{ type: "text", text: "A dog walking in the park" }], resolution: "480p" }));
      const guide = page.getByLabel("完整 curl 调用", { exact: true });
      await guide.locator("summary").first().click();
      const live = guide.locator('pre[aria-label="复制调用示例"]');
      await expect(live).toContainText('"resolution": "480p"');
      await page.getByRole("button", { name: "提交请求", exact: true }).click();
      await expect(page.getByLabel("任务 ID", { exact: true })).not.toHaveValue("");
      const liveId = await page.getByLabel("任务 ID", { exact: true }).inputValue();
      await expect(guide.locator('pre[aria-label="复制任务 ID 设置"]')).toContainText(liveId);
      const liveQuery = await guide.locator('pre[aria-label="复制结果查询"]').textContent();
      // 调试台默认以 UI 会话提交；任务归属凭据必须一致，不能用另一把虚拟密钥查询。
      const liveResult = JSON.parse(
        execFileSync("bash", ["-c", liveQuery!], {
          env: { ...process.env, XHUB_API_KEY: await sessionBearer(page), TASK_ID: liveId },
          timeout: 20_000,
          encoding: "utf8",
        }),
      );
      expect(liveResult.status).toBe("succeeded");
      await page.getByRole("button", { name: "获取结果", exact: true }).click();
      await expect(page.locator('pre[aria-label="原生响应"]')).toContainText('"succeeded"');
      await page.getByLabel("原生请求参数").fill("{");
      await expect(guide.getByRole("alert")).toContainText("有效 JSON 对象");
      await expect(guide.getByRole("button", { name: "复制调用示例" })).toHaveCount(0);
    } finally {
      if (key)
        expect((await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys: [key] } })).status()).toBe(
          200,
        );
      if (id)
        expect(
          (await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } })).status(),
          "清理临时模型",
        ).toBe(200);
    }
  });
}

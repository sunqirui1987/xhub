import { expect, test } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/** 前置真实浏览器、隔离数据库和本地视频上游；验证两个 Seedance 路径的模型详情不再展示公共定义入口，
 * 任务创建、查询、认证拒绝与计费仍正常。参数为页面夹具，无返回值；finally 删除部署，任务和账单随私有 schema 清理。
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
      await stableGoto(page, "/mine-models");
      await page.getByRole("textbox", { name: t("Search"), exact: true }).fill(name);
      const card = page.getByRole("article").filter({ has: page.getByRole("heading", { name, exact: true }) });
      await card.getByRole("button", { name: "API 接入", exact: true }).click();
      const dialog = page.getByRole("dialog", { name, exact: true });
      await expect(dialog.getByRole("combobox", { name: "选择调用接口" })).toHaveValue(native.path);
      await expect(dialog.getByRole("link", { name: "查看 API 接口定义" })).toHaveCount(0);
      await expect(dialog.getByRole("link", { name: "管理虚拟密钥" })).toBeVisible();
      await expect(dialog.getByText(/请按供应商协议文档构造参数/)).toBeVisible();
      await expect(dialog.getByText("get · GET " + native.path + "/{id}", { exact: true })).toBeVisible();
      expect(schemaRequests, "浏览器不应请求公共定义").toEqual([]);
      const payload = { model: name, content: [{ type: "text", text: "e2e-no-schema-video" }] };
      const denied = await page.request.post(GATEWAY + native.path, { data: payload });
      expect(denied.status(), "视频调用仍要求认证").toBe(401);
      const created = await page.request.post(GATEWAY + native.path, { headers, data: payload });
      expect(created.status(), await created.text()).toBe(200);
      const taskId = (await created.json()).id;
      expect(taskId).toBeTruthy();
      const callId = created.headers()["x-litellm-call-id"];
      const result = await page.request.get(GATEWAY + native.path + "/" + encodeURIComponent(taskId), { headers });
      expect(result.status(), await result.text()).toBe(200);
      expect(await result.json()).toMatchObject({ id: taskId, status: "succeeded", usage: { completion_tokens: 100 } });
      await expect
        .poll(async () => {
          const bill = await page.request.get(GATEWAY + "/spend/logs/ui/" + callId, { headers });
          if (!bill.ok()) return null;
          const body = await bill.json();
          return {
            model: body.model,
            status: body.status,
            spend: Number(body.spend),
            completion_tokens: body.completion_tokens,
          };
        })
        .toEqual({ model: name, status: "completed", spend: 0.001, completion_tokens: 100 });
      await page.keyboard.press("Escape");
    } finally {
      if (id)
        expect(
          (await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } })).status(),
          "清理临时模型",
        ).toBe(200);
    }
  });
}

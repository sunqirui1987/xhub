import { execFileSync } from "node:child_process";
import { expect, test } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/** 目的：验证真实验收路由生成器在同名双供应商基线下生成独立GLM回退。
 * 前置隔离数据库、本地429上游和真实浏览器；从页面保存完整JSON，核对持久化、
 * 两次429回退的计费与无缓存、目标权限及禁用回退，再从页面删除模板。
 * finally只清理本测试密钥、模板和部署，运行器删除schema，无外部凭据。 */
test("acceptance fallback excludes both shared-model healthy deployments", async ({ page }) => {
  test.setTimeout(120_000);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  const suffix = Date.now().toString();
  const primary = "acceptance-shared-" + suffix, backup = "acceptance-glm-" + suffix;
  const name = "acceptance-fallback-" + suffix;
  const ids = ["fault-", "fenno-", "qiniu-", "glm-"].map((prefix) => prefix + suffix);
  const upstreams = ["e2e-fallback-429-" + suffix, "healthy-fenno-" + suffix, "healthy-qiniu-" + suffix, "good-glm-" + suffix];
  let templateID = "";
  const keys: string[] = [];
  try {
    for (let i = 0; i < ids.length; i++) {
      const created = await page.request.post(GATEWAY + "/model/new", { headers, data: {
        model_name: i === 3 ? backup : primary,
        litellm_params: { model: upstreams[i], api_base: UPSTREAM, api_key: "sk-fake",
          custom_llm_provider: "custom", input_cost_per_token: 0.000001, output_cost_per_token: 0.000002 },
        model_info: { id: ids[i], transport: "bypass_openai_chat", endpoint_types: ["chat"], pricing_source: "manual" },
      } });
      expect(created.status(), await created.text()).toBe(200);
    }
    // 调用本次修复的Python生成器，确保浏览器实际验证验收脚本产出的正文。
    const body = JSON.parse(execFileSync("python3", ["-c",
      "import json,sys; sys.path.insert(0,'e2e'); from real_dataset import fallback_acceptance_deployments,fallback_acceptance_route; d=json.load(sys.stdin); healthy,target=fallback_acceptance_deployments(d['deployments'],d['primary'],d['backup'],'qiniu'); print(json.dumps(fallback_acceptance_route(d['primary'],d['fault'],healthy,d['backup'],target,60)))"],
      { cwd: "..", encoding: "utf8", input: JSON.stringify({ primary, backup, fault: ids[0], deployments: [
        { id: ids[2], public_name: primary, provider: "qiniu" },
        { id: ids[1], public_name: primary, provider: "fennoai" },
        { id: ids[3], public_name: backup, provider: "qiniu" },
      ] }) }));
    await stableGoto(page, "/route-templates");
    await page.getByRole("button", { name: t("pages.routeTemplates.create"), exact: true }).first().click();
    const editor = page.getByRole("region", { name: t("pages.routeTemplates.create"), exact: true });
    await editor.getByLabel(t("pages.routeTemplates.name"), { exact: true }).fill(name);
    await editor.getByRole("tab", { name: "JSON", exact: true }).click();
    await editor.getByRole("textbox", { name: "JSON", exact: true }).fill(JSON.stringify(body, null, 2));
    const saved = page.waitForResponse((r) => new URL(r.url()).pathname === "/route_template/new" && r.request().method() === "POST");
    await editor.getByRole("button", { name: "保存模板", exact: true }).click();
    const response = await saved;
    expect(response.status(), await response.text()).toBe(200);
    templateID = (await response.json()).id;
    const persisted = await (await page.request.get(GATEWAY + "/route_template/" + templateID, { headers })).json();
    expect(persisted.body.model_routes).toEqual(body.model_routes);
    expect(persisted.body.fallbacks).toEqual([{ [primary]: [backup] }]);
    const teams = await (await page.request.get(GATEWAY + "/v2/team/list?page=1&page_size=500", { headers })).json();
    const team = teams.teams.find((row: any) => row.team_alias === "e2e-fixture-team");
    const jwt = (await page.context().cookies()).find((cookie) => cookie.name === "token")!.value;
    const userID = JSON.parse(Buffer.from(jwt.split(".")[1], "base64url").toString()).user_id;
    for (const models of [[primary, backup], [primary]]) {
      const minted = await page.request.post(GATEWAY + "/key/generate", { headers, data: {
        key_alias: name + "-" + keys.length, owner_type: "personal", user_id: userID, team_id: team.team_id, models, route_template_id: templateID,
      } });
      expect(minted.status(), await minted.text()).toBe(200);
      keys.push((await minted.json()).key);
    }
    const request = { model: primary, messages: [{ role: "user", content: "repeat acceptance fallback " + suffix }] };
    for (let repeat = 0; repeat < 2; repeat++) {
      const call = await page.request.post(GATEWAY + "/v1/chat/completions", { headers: { Authorization: "Bearer " + keys[0] }, data: request });
      expect(call.status(), await call.text()).toBe(200);
      expect(call.headers()["cache_hit"]).not.toBe("true");
      expect(call.headers()["x-litellm-cache-hit"]).not.toBe("true");
      // 标准Chat接口返回模拟上游的固定正文并保留请求公开模型，部署选择由后台回归核对。
      const answer = await call.json();
      expect(answer.choices[0].message.content).toBe("e2e-ok");
      expect(answer.model).toBe(primary);
      const callID = call.headers()["x-litellm-call-id"];
      expect(callID).toBeTruthy();
      await expect.poll(async () => {
        const log = await page.request.get(GATEWAY + "/spend/logs/ui/" + callID, { headers });
        return log.ok() ? Number((await log.json()).spend) : 0;
      }).toBeGreaterThan(0);
    }
    const denied = await page.request.post(GATEWAY + "/v1/chat/completions", { headers: { Authorization: "Bearer " + keys[1] }, data: request });
    expect(denied.status(), await denied.text()).toBe(401);
    expect(await denied.text()).toContain("model not in allowed model list");
    const disabled = await page.request.post(GATEWAY + "/v1/chat/completions", { headers: { Authorization: "Bearer " + keys[0] }, data: { ...request, disable_fallbacks: true } });
    expect(disabled.status(), await disabled.text()).toBe(502);
    const deletedKeys = await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys } });
    expect(deletedKeys.status(), await deletedKeys.text()).toBe(200);
    keys.length = 0;
    await page.reload();
    const row = page.getByRole("row").filter({ has: page.getByText(name, { exact: true }) });
    await row.getByRole("button", { name: name + " 的操作" }).click();
    await page.getByRole("menuitem", { name: t("pages.routeTemplates.delete"), exact: true }).click();
    const deleted = page.waitForResponse((r) => new URL(r.url()).pathname === "/route_template/" + templateID + "/delete");
    await page.getByRole("dialog").getByRole("button", { name: t("pages.routeTemplates.delete"), exact: true }).click();
    expect((await deleted).status()).toBe(200);
    await expect(row).toHaveCount(0);
    templateID = "";
  } finally {
    if (keys.length) await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys } });
    if (templateID) await page.request.post(GATEWAY + "/route_template/" + templateID + "/delete", { headers, data: {} });
    for (const id of ids) await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } });
  }
});

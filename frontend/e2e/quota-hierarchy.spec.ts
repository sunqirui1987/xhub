import { expect, test } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/** 验证页面与真实接口/数据面额度一致；前置隔离 schema、本地供应商和管理员登录。
 * 参数 page 为真实浏览器，覆盖页面说明、超配、单团队、共享拦截与固定密钥消费；finally 清理组织与用户，脚本删除 schema。
 */
test("hierarchical budgets reserve allocations and enforce a single team", async ({ page }) => {
  test.setTimeout(120_000);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  await stableGoto(page, "/teams");
  await expect(page.getByRole("complementary", { name: t("quotaGuide.title") })).toBeVisible();
  const orgResponse = await page.request.post(GATEWAY + "/organization/new", {
    headers,
    data: { organization_alias: "quota-e2e", max_budget: 1000 },
  });
  expect(orgResponse.ok(), await orgResponse.text()).toBeTruthy();
  const orgID = (await orgResponse.json()).organization_id;
  let userID = "";
  let modelID = "";
  const teamIDs: string[] = [];
  try {
    const teamResponse = await page.request.post(GATEWAY + "/team/new", {
      headers,
      data: { organization_id: orgID, team_alias: "quota-fixed", max_budget: 600 },
    });
    expect(teamResponse.ok(), await teamResponse.text()).toBeTruthy();
    const team = await teamResponse.json();
    teamIDs.push(team.team_id);
    const otherResponse = await page.request.post(GATEWAY + "/team/new", {
      headers,
      data: { organization_id: orgID, team_alias: "quota-shared", max_budget: 400 },
    });
    expect(otherResponse.ok(), await otherResponse.text()).toBeTruthy();
    const other = await otherResponse.json();
    teamIDs.push(other.team_id);
    const overflow = await page.request.post(GATEWAY + "/team/new", {
      headers,
      data: { organization_id: orgID, team_alias: "quota-overflow", max_budget: 1 },
    });
    expect(overflow.status()).toBe(400);
    expect((await overflow.json()).error.type).toBe("quota_allocation_exceeded");
    const memberResponse = await page.request.post(GATEWAY + "/user/new", {
      headers,
      data: {
        user_email: "quota-member@xhub.local",
        password: "quota-password",
        team_id: team.team_id,
        team_role: "member",
        max_budget: 400,
      },
    });
    expect(memberResponse.ok(), await memberResponse.text()).toBeTruthy();
    userID = (await memberResponse.json()).user_id;
    const second = await page.request.post(GATEWAY + "/team/member_add", {
      headers,
      data: { team_id: other.team_id, member: { user_email: "quota-member@xhub.local", role: "user" } },
    });
    expect(second.status()).toBe(400);
    expect((await second.json()).error.type).toBe("single_team_required");
    // 用真实页面编辑个人额度，保存后读取真实成员接口验证持久化。
    await stableGoto(page, "/teams?team=" + team.team_id);
    await page.getByRole("tab", { name: t("Members"), exact: true }).click();
    await page
      .getByRole("row")
      .filter({ hasText: "quota-member@xhub.local" })
      .getByRole("button", { name: t("quotaGuide.editMember") })
      .click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel(t("Team Member Budget (USD)"), { exact: true }).fill("601");
    const rejectedSave = page.waitForResponse(
      (response) => response.url().endsWith("/team/member_update") && response.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: t("Save Changes") }).click();
    expect((await rejectedSave).status()).toBe(400);
    await expect(page.getByText(t("quotaGuide.allocationError"), { exact: true })).toBeVisible();
    await dialog.getByLabel(t("Team Member Budget (USD)"), { exact: true }).fill("401");
    const saved = page.waitForResponse(
      (response) => response.url().endsWith("/team/member_update") && response.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: t("Save Changes") }).click();
    expect((await saved).status()).toBe(200);
    const roster = await page.request.get(GATEWAY + "/team/info?team_id=" + team.team_id, { headers });
    expect(
      (await roster.json()).team_memberships.find((member: { user_id: string }) => member.user_id === userID)
        .max_budget,
    ).toBe(401);
    const adjusted = await page.request.post(GATEWAY + "/team/member_update", {
      headers,
      data: { team_id: team.team_id, user_id: userID, role: "user", max_budget_in_team: 400 },
    });
    expect(adjusted.ok(), await adjusted.text()).toBeTruthy();
    const model = await page.request.post(GATEWAY + "/model/new", {
      headers,
      data: {
        model_name: "quota-priced-model",
        litellm_params: {
          model: "gpt-4o-mini",
          custom_llm_provider: "openai",
          api_base: UPSTREAM + "/v1",
          api_key: "sk-fake",
          input_cost_per_token: 0.000002,
          output_cost_per_token: 0.000008,
        },
        model_info: { transport: "bypass_openai_chat", pricing_source: "manual" },
      },
    });
    expect(model.ok(), await model.text()).toBeTruthy();
    modelID = (await model.json()).model_info.id;
    const fixedResponse = await page.request.post(GATEWAY + "/key/generate", {
      headers,
      data: { user_id: userID, key_alias: "quota-reserved", max_budget: 400 },
    });
    expect(fixedResponse.ok(), await fixedResponse.text()).toBeTruthy();
    const fixed = await fixedResponse.json();
    const sharedResponse = await page.request.post(GATEWAY + "/key/generate", {
      headers,
      data: { user_id: userID, key_alias: "quota-shared-key" },
    });
    expect(sharedResponse.ok(), await sharedResponse.text()).toBeTruthy();
    const shared = await sharedResponse.json();
    const body = { model: "quota-priced-model", messages: [{ role: "user", content: "Verify hierarchical quota" }] };
    const blocked = await page.request.post(GATEWAY + "/v1/chat/completions", {
      headers: { Authorization: "Bearer " + shared.key },
      data: body,
    });
    expect(blocked.status()).toBe(429);
    const inference = await page.request.post(GATEWAY + "/v1/chat/completions", {
      headers: { Authorization: "Bearer " + fixed.key },
      data: body,
    });
    expect(inference.ok(), await inference.text()).toBeTruthy();
    await expect
      .poll(async () => {
        const response = await page.request.get(GATEWAY + "/team/info?team_id=" + team.team_id, { headers });
        return Number((await response.json()).team_info.spend);
      })
      .toBeGreaterThan(0);
    // 退出只解除归属：独立个人密钥继续消费，旧团队与组织账单不再增加。
    const removed = await page.request.post(GATEWAY + "/team/member_delete", {
      headers,
      data: { team_id: team.team_id, user_id: userID },
    });
    expect(removed.ok(), await removed.text()).toBeTruthy();
    const detachedInfo = await page.request.get(GATEWAY + "/team/info?team_id=" + team.team_id, { headers });
    const previousSpend = Number((await detachedInfo.json()).team_info.spend);
    const independent = await page.request.post(GATEWAY + "/v1/chat/completions", {
      headers: { Authorization: "Bearer " + fixed.key },
      data: body,
    });
    expect(independent.ok(), await independent.text()).toBeTruthy();
    const afterExit = await page.request.get(GATEWAY + "/team/info?team_id=" + team.team_id, { headers });
    const afterExitData = await afterExit.json();
    expect(Number(afterExitData.team_info.spend)).toBe(previousSpend);
    expect(afterExitData.team_memberships.some((member: { user_id: string }) => member.user_id === userID)).toBe(false);
    await stableGoto(page, "/api-keys");
    await page.getByRole("button", { name: "English", exact: true }).click();
    await expect(page.getByRole("complementary", { name: "How quota allocation works" })).toBeVisible();
    await page.getByRole("button", { name: "中文", exact: true }).click();
    await expect(page.getByRole("complementary", { name: t("quotaGuide.title") })).toBeVisible();
  } finally {
    // 为失败后的真实接口清理保留独立时间，避免测试超时掩盖原始错误。
    test.setTimeout(test.info().timeout + 30_000);
    if (userID) await page.request.post(GATEWAY + "/user/delete", { headers, data: { user_ids: [userID] } });
    for (const id of teamIDs) {
      const deleted = await page.request.post(GATEWAY + "/team/delete", { headers, data: { team_id: id } });
      expect(deleted.ok(), await deleted.text()).toBeTruthy();
    }
    if (modelID) await page.request.post(GATEWAY + "/model/delete", { headers, data: { id: modelID } });
    const deleted = await page.request.delete(GATEWAY + "/organization/delete", {
      headers, data: { organization_id: orgID },
    });
    expect(deleted.ok(), await deleted.text()).toBeTruthy();
  }
});

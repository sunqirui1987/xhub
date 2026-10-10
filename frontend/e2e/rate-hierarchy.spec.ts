import { expect, test } from "@playwright/test";
import { GATEWAY, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/** 真实浏览器验证四层 RPM/TPM 编辑、超配回滚、共享保留和 429 数据面；隔离 schema 与本地供应商，finally 清理自建数据。 */
test("rate allocations are editable and enforced along the unique ownership chain", async ({ page }) => {
  test.setTimeout(210_000);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + await sessionBearer(page) };
  await stableGoto(page, "/organizations");
  await page.getByRole("button", { name: "+ " + t("pages.organizations.create"), exact: true }).click();
  const organizationDialog = page.getByRole("dialog");
  await organizationDialog.getByLabel(t("Organization Name"), { exact: true }).fill("rates-e2e");
  await organizationDialog.getByLabel(t("Max Budget (USD)"), { exact: true }).fill("1000");
  await organizationDialog.getByLabel(t("Requests per minute Limit (RPM)"), { exact: true }).fill("10");
  await organizationDialog.getByLabel(t("Tokens per minute Limit (TPM)"), { exact: true }).fill("640");
  const organizationCreated = page.waitForResponse(r => r.url().endsWith("/organization/new") && r.request().method() === "POST");
  await organizationDialog.getByRole("button", { name: t("pages.organizations.createTitle"), exact: true }).click();
  const created = await organizationCreated;
  expect(created.ok(), await created.text()).toBeTruthy();
  const orgID = (await created.json()).organization_id;
  let teamID = "", userID = "";
  try {
    // 新建团队和个人也必须通过页面设置三项额度，验证真实请求和持久化。
    await stableGoto(page, "/teams");
    await page.getByTestId("create-team-button").click();
    await page.getByLabel(t("Organization"), { exact: true }).fill("rates-e2e");
    await page.getByRole("option").filter({ hasText: "rates-e2e" }).click();
    await page.getByTestId("team-name-input").fill("rates-team");
    await page.getByLabel(t("Max Budget (USD)"), { exact: true }).fill("600");
    await page.getByLabel(t("Requests per minute Limit (RPM)"), { exact: true }).fill("6");
    await page.getByLabel(t("Tokens per minute Limit (TPM)"), { exact: true }).fill("384");
    const teamCreated = page.waitForResponse(r => r.url().endsWith("/team/new") && r.request().method() === "POST");
    await page.getByTestId("create-team-submit").click();
    const teamResponse = await teamCreated;
    expect(teamResponse.ok(), await teamResponse.text()).toBeTruthy(); teamID = (await teamResponse.json()).team_id;
    const createdTeamInfo = await page.request.get(GATEWAY + "/team/info?team_id=" + teamID, { headers });
    expect((await createdTeamInfo.json()).team_info).toMatchObject({ max_budget: 600, rpm_limit: 6, tpm_limit: 384 });
    await stableGoto(page, "/users");
    await page.getByRole("button", { name: "+ " + t("pages.users.invite"), exact: true }).click();
    const creation = page.getByRole("dialog");
    await creation.getByLabel(t("pages.users.userEmail")).fill("rates-member@xhub.local");
    await creation.getByLabel(t("Initial password")).fill("rates-password");
    await creation.getByRole("switch", { name: t("Budget"), exact: true }).click();
    await creation.getByLabel(t("Budget in dollars"), { exact: true }).fill("200");
    await creation.getByLabel(t("Requests per minute Limit (RPM)"), { exact: true }).fill("4");
    await creation.getByLabel(t("Tokens per minute Limit (TPM)"), { exact: true }).fill("128");
    await creation.getByLabel(t("Add to a team"), { exact: true }).fill("rates-team");
    await page.getByRole("option").filter({ hasText: "rates-team" }).click();
    const userCreated = page.waitForResponse(r => r.url().endsWith("/user/new") && r.request().method() === "POST");
    await creation.getByRole("button", { name: t("pages.users.invite"), exact: true }).click();
    const userResponse = await userCreated;
    expect(userResponse.ok(), await userResponse.text()).toBeTruthy(); userID = (await userResponse.json()).user_id;
    const createdUserInfo = await page.request.get(GATEWAY + "/user/info?user_id=" + userID, { headers });
    expect((await createdUserInfo.json()).user_info).toMatchObject({ max_budget: 200, rpm_limit: 4, tpm_limit: 128 });
    // 组织页面持久化 RPM/TPM；通过稳定标签操作完整保存流程。
    await stableGoto(page, "/organizations?org=" + orgID);
    await page.getByRole("tab", { name: t("Settings"), exact: true }).click();
    await page.getByRole("button", { name: t("Edit Settings"), exact: true }).click();
    await page.getByLabel(t("Requests per minute Limit (RPM)"), { exact: true }).fill("12");
    await page.getByLabel(t("Tokens per minute Limit (TPM)"), { exact: true }).fill("768");
    let saved = page.waitForResponse(r => r.url().endsWith("/organization/update") && r.request().method() === "PATCH");
    await page.getByRole("button", { name: t("Save Changes"), exact: true }).click(); expect((await saved).status()).toBe(200);
    // 团队页面保存两种分配，之后用户页面拒绝超过团队的个人分配。
    await stableGoto(page, "/teams?team=" + teamID);
    await page.getByRole("tab", { name: t("Settings"), exact: true }).click();
    await page.getByRole("button", { name: t("Edit Settings"), exact: true }).click();
    await page.getByLabel(t("Requests per minute Limit (RPM)"), { exact: true }).fill("8");
    await page.getByLabel(t("Tokens per minute Limit (TPM)"), { exact: true }).fill("512");
    saved = page.waitForResponse(r => r.url().endsWith("/team/update") && r.request().method() === "POST");
    await page.getByRole("button", { name: t("Save Changes"), exact: true }).click(); expect((await saved).status()).toBe(200);
    // 成员入口编辑同一个个人配额，避免形成团队内独立的第二份限额。
    await page.getByRole("tab", { name: t("Members"), exact: true }).click();
    await page.getByRole("row").filter({ hasText: "rates-member@xhub.local" }).getByRole("button", { name: t("quotaGuide.editMember") }).click();
    const memberDialog = page.getByRole("dialog");
    await memberDialog.getByLabel(t("Requests per minute Limit (RPM)"), { exact: true }).fill("4");
    await memberDialog.getByLabel(t("Tokens per minute Limit (TPM)"), { exact: true }).fill("96");
    saved = page.waitForResponse(r => r.url().endsWith("/team/member_update") && r.request().method() === "POST");
    await memberDialog.getByRole("button", { name: t("Save Changes"), exact: true }).click(); expect((await saved).status()).toBe(200);
    const roster = await page.request.get(GATEWAY + "/team/info?team_id=" + teamID, { headers });
    expect((await roster.json()).team_memberships.find((entry: { user_id: string }) => entry.user_id === userID)).toMatchObject({ rpm_limit: 4, tpm_limit: 96 });
    await stableGoto(page, "/users?user=" + userID);
    await page.getByRole("button", { name: t("pages.users.editUser"), exact: true }).click();
    await page.getByLabel(t("Requests per minute Limit (RPM)"), { exact: true }).fill("9");
    await page.getByLabel(t("pages.users.maxBudget"), { exact: true }).fill("201");
    saved = page.waitForResponse(r => r.url().endsWith("/user/update") && r.request().method() === "POST");
    await page.getByRole("button", { name: t("pages.users.saveChanges"), exact: true }).click(); expect((await saved).status()).toBe(400);
    await expect(page.getByText(t("quotaGuide.rateAllocationError"), { exact: true })).toBeVisible();
    await expect(page.getByLabel(t("Requests per minute Limit (RPM)"), { exact: true })).toHaveValue("9");
    // RPM 和金额合法而 TPM 超配时也必须整体回滚；读取真实接口确认金额没有先保存。
    await page.getByLabel(t("Requests per minute Limit (RPM)"), { exact: true }).fill("4");
    await page.getByLabel(t("Tokens per minute Limit (TPM)"), { exact: true }).fill("513");
    saved = page.waitForResponse(r => r.url().endsWith("/user/update") && r.request().method() === "POST");
    await page.getByRole("button", { name: t("pages.users.saveChanges"), exact: true }).click();
    expect((await saved).status()).toBe(400);
    const rollbackInfo = await page.request.get(GATEWAY + "/user/info?user_id=" + userID, { headers });
    expect((await rollbackInfo.json()).user_info).toMatchObject({ max_budget: 200, rpm_limit: 4, tpm_limit: 96 });
    await expect(page.getByLabel(t("pages.users.maxBudget"), { exact: true })).toHaveValue("201");
    await page.getByLabel(t("Tokens per minute Limit (TPM)"), { exact: true }).fill("64");
    saved = page.waitForResponse(r => r.url().endsWith("/user/update") && r.request().method() === "POST");
    await page.getByRole("button", { name: t("pages.users.saveChanges"), exact: true }).click(); expect((await saved).status()).toBe(200);
    const info = await page.request.get(GATEWAY + "/user/info?user_id=" + userID, { headers });
    expect(await info.json()).toMatchObject({ user_info: { max_budget: 201, rpm_limit: 4, tpm_limit: 64 } });
    const fixed = await page.request.post(GATEWAY + "/key/generate", { headers, data: { user_id: userID, key_alias: "rates-fixed", rpm_limit: 4, tpm_limit: 64 } });
    expect(fixed.ok(), await fixed.text()).toBeTruthy(); const fixedKey = await fixed.json();
    const shared = await page.request.post(GATEWAY + "/key/generate", { headers, data: { user_id: userID, key_alias: "rates-shared" } });
    expect(shared.ok(), await shared.text()).toBeTruthy(); const sharedKey = await shared.json();
    const body = { model: "gpt-4o-mini", messages: [{ role: "user", content: "ok" }] };
    const blocked = await page.request.post(GATEWAY + "/v1/chat/completions", { headers: { Authorization: "Bearer " + sharedKey.key }, data: body });
    expect(blocked.status()).toBe(429); expect((await blocked.json()).error.message).toContain("rpm_limit");
    for (let i = 0; i < 2; i++) {
      const allowed = await page.request.post(GATEWAY + "/v1/chat/completions", { headers: { Authorization: "Bearer " + fixedKey.key }, data: body });
      expect(allowed.ok(), await allowed.text()).toBeTruthy();
    }
    const tokensBlocked = await page.request.post(GATEWAY + "/v1/chat/completions", { headers: { Authorization: "Bearer " + fixedKey.key }, data: body });
    expect(tokensBlocked.status()).toBe(429); expect((await tokensBlocked.json()).error.message).toContain("tpm_limit");
    // API Key 创建页面支持 RPM/TPM，中文/英文说明完整切换。
    await stableGoto(page, "/api-keys");
    await page.getByRole("button", { name: "English", exact: true }).click();
    await expect(page.getByRole("complementary", { name: "How quota allocation works" })).toContainText("RPM");
    await page.getByRole("button", { name: "中文", exact: true }).click();
    await expect(page.getByRole("complementary", { name: t("quotaGuide.title") })).toContainText("TPM");
    await page.getByTestId("create-key-button").click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel(t("Key Name")).fill("rates-page-key");
    await dialog.getByLabel(t("Max Budget (USD)"), { exact: true }).fill("5");
    await dialog.getByLabel(t("Requests per minute Limit (RPM)"), { exact: true }).fill("0");
    await dialog.getByLabel(t("Tokens per minute Limit (TPM)"), { exact: true }).fill("64");
    saved = page.waitForResponse(r => r.url().endsWith("/key/generate") && r.request().method() === "POST");
    await dialog.getByRole("button", { name: t("pages.apiKeys.createSubmit"), exact: true }).click();
    const response = await saved; expect(response.ok(), await response.text()).toBeTruthy(); const own = await response.json();
    try {
      const zero = await page.request.post(GATEWAY + "/v1/chat/completions", { headers: { Authorization: "Bearer " + own.key }, data: body });
      expect(zero.status()).toBe(429);
      // 编辑页面清空 RPM 恢复共享；保存接口和真实推理同时验证，删除释放预留容量。
      await stableGoto(page, "/api-keys?key=" + own.token_id);
      await page.getByRole("tab", { name: t("Settings"), exact: true }).click();
      await page.getByRole("button", { name: t("Edit Settings"), exact: true }).click();
      await page.getByLabel(t("RPM Limit"), { exact: true }).fill("");
      await page.getByLabel(t("TPM Limit"), { exact: true }).fill("64");
      saved = page.waitForResponse(r => r.url().endsWith("/key/update") && r.request().method() === "POST");
      await page.getByRole("button", { name: t("Save Changes"), exact: true }).click();
      const edited = await saved; expect(edited.ok(), await edited.text()).toBeTruthy();
      const keyInfo = await page.request.get(GATEWAY + "/key/info?key=" + own.token_id, { headers });
      expect((await keyInfo.json()).info).toMatchObject({ max_budget: 5, rpm_limit: null, tpm_limit: 64 });
      const restored = await page.request.post(GATEWAY + "/v1/chat/completions", { headers: { Authorization: "Bearer " + own.key }, data: body });
      expect(restored.ok(), await restored.text()).toBeTruthy();

    } finally {
      await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys: [own.token_id] } });
    }
  } finally {
    test.setTimeout(test.info().timeout + 30_000);
    if (userID) await page.request.post(GATEWAY + "/user/delete", { headers, data: { user_ids: [userID] } });
    if (teamID) await page.request.post(GATEWAY + "/team/delete", { headers, data: { team_id: teamID } });
    const deleted = await page.request.delete(GATEWAY + "/organization/delete", { headers, data: { organization_id: orgID } });
    expect(deleted.ok(), await deleted.text()).toBeTruthy();
  }
});

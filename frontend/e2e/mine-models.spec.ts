import { expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/** 前置真实浏览器、隔离数据库和本地上游；创建授权模型和 API Key，验证四入口、价格、接口切换、复制、
 * 实际执行页面 curl、响应和用量日志，并检查移动端；finally 删除测试模型及密钥，schema 清理账单。 */
test("my models exposes prices, access, executable API examples and call guide", async ({ page }) => {
  test.setTimeout(120_000);
  // 无头浏览器默认拒绝剪贴板；授予隔离测试上下文权限，验证真实复制而非替换 API。
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  const name = "e2e-mine-call-" + Date.now();
  let id = "";
  let key = "";
  try {
    const created = await page.request.post(GATEWAY + "/model/new", {
      headers,
      data: {
        model_name: name,
        litellm_params: {
          model: "custom/mine-call",
          custom_llm_provider: "custom",
          api_base: UPSTREAM,
          api_key: "sk-fake",
          input_cost_per_token: 0.000001,
          output_cost_per_token: 0.000002,
        },
        model_info: { transport: "bypass_openai_chat", pricing_source: "manual", input_price: 1, output_price: 2 },
      },
    });
    expect(created.status(), await created.text()).toBe(200);
    id = (await created.json()).model_info.id;
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
    const loaded = page.waitForResponse((response) => new URL(response.url()).pathname === "/model/available");
    await stableGoto(page, "/mine-models");
    const available = await loaded;
    expect(available.status()).toBe(200);
    const cardData = (await available.json()).data.find((item: { id: string }) => item.id === name);
    expect(cardData.endpoints.some((item: { path: string }) => item.path === "/v1/chat/completions")).toBe(true);
    await page.getByRole("textbox", { name: t("Search"), exact: true }).fill(name);
    const card = page.getByRole("article").filter({ has: page.getByRole("heading", { name, exact: true }) });
    for (const title of ["模型价格", "接入信息", "API 接入", "调用文档"])
      await expect(card.getByRole("button", { name: title, exact: true })).toBeVisible();
    await card.getByRole("button", { name: "模型价格", exact: true }).click();
    const dialog = page.getByRole("dialog", { name, exact: true });
    const prices = dialog.getByRole("table", { name: "模型价格" });
    await expect(prices.getByText("$1", { exact: true })).toBeVisible();
    await expect(prices.getByText("$2", { exact: true })).toBeVisible();
    await dialog.getByRole("tab", { name: "接入信息" }).click();
    await expect(dialog.getByText("POST /v1/chat/completions", { exact: true })).toBeVisible();
    await dialog.getByRole("tab", { name: "API 接入" }).click();
    await dialog.getByRole("combobox", { name: "选择调用接口" }).selectOption("/v1/responses");
    await expect(dialog.getByLabel("复制调用示例", { exact: true }).filter({ hasText: "curl" })).toContainText(
      '"input": "你好"',
    );
    await dialog.getByRole("combobox", { name: "选择调用接口" }).selectOption("/v1/chat/completions");
    await dialog.getByRole("button", { name: "复制调用示例", exact: true }).click();
    await expect(dialog.getByRole("status")).toHaveText("已复制");
    const sample = await dialog.getByLabel("复制调用示例", { exact: true }).filter({ hasText: "curl" }).textContent();
    expect(await page.evaluate(() => navigator.clipboard.readText()), "剪贴板应包含完整调用示例").toBe(sample);
    expect(sample).toContain(GATEWAY + "/v1/chat/completions");
    // 直接执行页面展示的命令，确保 URL、认证方式和正文可真正通过网关到达上游。
    const output = execFileSync("bash", ["-c", sample!], {
      env: { ...process.env, XHUB_API_KEY: key },
      timeout: 20_000,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
    });
    const result = JSON.parse(output);
    expect(result.choices[0].message.content).toBeTruthy();
    await expect
      .poll(async () => {
        const response = await page.request.get(GATEWAY + "/spend/logs/ui?page=1&page_size=50", { headers });
        expect(response.status(), "调用日志接口应成功返回").toBe(200);
        const logs = await response.json();
        const rows = Array.isArray(logs) ? logs : (logs.data ?? logs.logs ?? []);
        return rows.filter((row: { model: string; spend: number }) => row.model === name && Number(row.spend) > 0)
          .length;
      })
      .toBeGreaterThan(0);
    await dialog.getByRole("tab", { name: "调用文档" }).click();
    await expect(dialog.getByRole("heading", { name: "1. 准备 API Key" })).toBeVisible();
    await expect(
      dialog.getByRole("tabpanel", { name: "调用文档", exact: true }).getByRole("link", { name: "管理虚拟密钥" }),
    ).toHaveAttribute("href", /api-keys/);
    await page.setViewportSize({ width: 390, height: 844 });
    for (const title of ["模型价格", "接入信息", "API 接入", "调用文档"])
      await expect(dialog.getByRole("tab", { name: title, exact: true })).toBeVisible();
    await dialog.getByRole("tab", { name: "API 接入" }).click();
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth + 1)).toBe(true);
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.screenshot({ path: (process.env.MINE_MODEL_REPORT || "../.e2e/mine-models") + "/api-detail.png" });
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(card.getByRole("button", { name: "模型价格", exact: true })).toBeFocused();
  } finally {
    if (key)
      expect((await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys: [key] } })).status()).toBe(200);
    if (id) expect((await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } })).status()).toBe(200);
  }
});

/** 前置隔离配置中包含未绑定端点的旧模型；验证待配置状态与可观察降级，禁止虚构聊天接口；无新增数据。 */
test("my models explains unavailable endpoint configuration", async ({ page }) => {
  test.skip(process.env.E2E_ENDPOINT_UNBOUND !== "1", "requires isolated unbound model fixture");
  await loginAdmin(page);
  await stableGoto(page, "/mine-models");
  await page.getByRole("textbox", { name: t("Search"), exact: true }).fill("e2e-unbound");
  const card = page
    .getByRole("article")
    .filter({ has: page.getByRole("heading", { name: "e2e-unbound", exact: true }) });
  await expect(card).toContainText("待配置");
  await card.getByRole("button", { name: "如何调用", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "e2e-unbound", exact: true });
  await expect(dialog.getByRole("alert")).toContainText("暂未提供可调用接口");
  await expect(dialog.getByRole("combobox")).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "复制调用示例" })).toHaveCount(0);
});

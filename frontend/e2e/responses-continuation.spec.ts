import { expect, test } from "@playwright/test";
import { GATEWAY, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

for (const stream of [true, false]) {
  /** 前置隔离模型及本地供应商，操作真实 Playground 三轮并刷新恢复会话。
   * 验证浏览器只发新消息和 previous_response_id，上游回复证明完整历史，真实账单记费；schema 和服务由 runner 清理。 */
  test("Responses incremental continuation " + (stream ? "SSE" : "JSON"), async ({ page }) => {
    await loginAdmin(page);
    await page.evaluate(value => sessionStorage.setItem("streamingEnabled", JSON.stringify(value)), stream);
    await stableGoto(page, "/playground");
    await page.getByPlaceholder(t("Select a Model"), { exact: true }).click();
    await page.getByRole("option", { name: "e2e-responses-history", exact: true }).click();
    await page.getByPlaceholder(t("Select an endpoint"), { exact: true }).click();
    await page.getByRole("option").filter({ hasText: /\/v1\/responses$/ }).click();
    await expect(page.getByRole("switch", { name: t("Use API session management") })).toBeChecked();
    const headers = { Authorization: "Bearer " + await sessionBearer(page) };
    let previous = "";
    for (let turn = 1; turn <= 3; turn++) {
      if (turn === 3) {
        await expect.poll(() => page.evaluate(() => sessionStorage.getItem("chatHistory"))).toContain("history-ok-2");
        await expect.poll(() => page.evaluate(() => sessionStorage.getItem("responsesSessionId"))).toBe(previous);
        await page.reload();
        // 页面刷新后重新选择模型及入口，历史和 response ID 由会话存储恢复。
        await page.getByPlaceholder(t("Select a Model"), { exact: true }).click();
        await page.getByRole("option", { name: "e2e-responses-history", exact: true }).click();
        await page.getByPlaceholder(t("Select an endpoint"), { exact: true }).click();
        await page.getByRole("option").filter({ hasText: /\/v1\/responses$/ }).click();
        await expect(page.getByText("history-ok-2", { exact: true })).toBeVisible();
      }
      await page.getByPlaceholder("Type your message... (Shift+Enter for new line)").fill("continuation-" + turn);
      const pending = page.waitForResponse(response => /\/(v1\/)?responses$/.test(new URL(response.url()).pathname) && response.request().method() === "POST");
      await page.getByRole("button", { name: t("Send message") }).click();
      const response = await pending;
      const raw = await response.text();
      expect(response.status(), raw).toBe(200);
      const body = response.request().postDataJSON();
      expect(body.stream).toBe(stream);
      expect(body.input, "浏览器必须只发送本轮消息").toHaveLength(1);
      expect(body.input[0].content).toBe("continuation-" + turn);
      if (turn > 1) expect(body.previous_response_id).toBe(previous);
      else expect(body.previous_response_id).toBeUndefined();
      if (stream) {
        const event = raw.split("\n").filter(line => line.startsWith("data: ")).map(line => JSON.parse(line.slice(6))).find(event => event.type === "response.completed");
        expect(event.response.store).toBe(false);
        previous = event.response.id;
      } else {
        const result = JSON.parse(raw);
        expect(result.store).toBe(false);
        previous = result.id;
      }
      expect(previous).toMatch(/^chatcmpl-/);
      await expect(page.getByText("history-ok-" + turn, { exact: true })).toBeVisible();
      const call = response.headers()["x-litellm-call-id"];
      await expect.poll(async () => {
        const detail = await page.request.get(GATEWAY + "/spend/logs/ui/" + call, { headers });
        return detail.ok() ? Number((await detail.json()).spend) : 0;
      }).toBeCloseTo(0.000012, 10);
    }
  });
}

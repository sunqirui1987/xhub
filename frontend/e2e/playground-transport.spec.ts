import { expect, test } from "@playwright/test";
import { GATEWAY, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

for (const stream of [true, false]) {
  for (const protocol of ["chat/completions", "v1/messages"]) {
    /** 前置隔离网关、数据库和本地上游，真实登录并在 Playground 发送消息。
     * 验证 JSON/SSE 请求、鉴权、模型回复及真实账单，数据随 runner 的私有 schema 清理。 */
    test("Playground fetch " + protocol + " " + (stream ? "SSE" : "JSON"), async ({ page }) => {
      await loginAdmin(page);
      await page.evaluate((value) => sessionStorage.setItem("streamingEnabled", JSON.stringify(value)), stream);
      await stableGoto(page, "/playground");
      await page.getByPlaceholder(t("Select a Model"), { exact: true }).click();
      await page.getByRole("option", { name: "gpt-4o-mini", exact: true }).click();
      await page.getByPlaceholder(t("Select an endpoint"), { exact: true }).click();
      await page
        .getByRole("option")
        .filter({ hasText: protocol === "v1/messages" ? /\/v1\/messages$/ : /\/v1\/chat\/completions$/ })
        .click();
      await page.getByPlaceholder("Type your message... (Shift+Enter for new line)").fill("transport-check");
      const pending = page.waitForResponse(
        (response) =>
          new URL(response.url()).pathname.endsWith("/" + protocol) && response.request().method() === "POST",
      );
      await page.getByRole("button", { name: t("Send message") }).click();
      const response = await pending;
      // SSE 响应会被页面持续读取，Chromium 不保证测试端还能二次取得响应体。
      expect(response.status(), "Playground 客户接口应成功返回").toBe(200);
      expect(response.request().postDataJSON()).toMatchObject({ model: "gpt-4o-mini", stream });
      expect(response.request().headers().authorization).toMatch(/^Bearer /);
      await expect(page.getByText("e2e-ok", { exact: true })).toBeVisible();
      const callId = response.headers()["x-litellm-call-id"];
      expect(callId, "实际模型调用必须有关联账单").toBeTruthy();
      const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
      await expect
        .poll(async () => {
          const detail = await page.request.get(GATEWAY + "/spend/logs/ui/" + callId, { headers });
          return detail.ok() ? (await detail.json()).model : "";
        })
        .toBe("gpt-4o-mini");
    });
  }
}

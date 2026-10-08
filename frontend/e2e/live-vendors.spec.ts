import { expect, test } from "@playwright/test";
import { GATEWAY, loginAdmin, sessionBearer, t, uiPath, watchGateway } from "./helpers";
import { recordChain } from "./report";

const vendors = process.env.E2E_LIVE === "1" ? (process.env.E2E_LIVE_VENDORS || "FENNO").split(",") : [];
for (const vendor of vendors) {
  const models = (process.env[`XHUB_REGRESSION_${vendor}_MODELS`] || "").split(",").filter(Boolean);
  for (const model of models) {
    test(`real ${vendor}/${model}: click Send, display answer, persist billed usage`, async ({ page }) => {
      test.setTimeout(150_000);
      const guard = watchGateway(page);
      await loginAdmin(page);
      const alias = `e2e-live-${vendor}/${model}`;
      await page.goto(uiPath("/playground"));
      await page.getByPlaceholder(t("Select a Model"), { exact: true }).click();
      await page.getByRole("option", { name: alias, exact: true }).click();
      const response = page.waitForResponse(res => res.url().includes("/v1/chat/completions") && res.request().method() === "POST", { timeout: 100_000 });
      await page.getByTestId("chat-composer-input").fill("Reply with only the word ok.");
      await page.getByTestId("chat-send-button").click();
      const res = await response;
      expect(res.status(), `real ${vendor} must answer successfully`).toBe(200);
      await expect(page.getByTestId("message-surface").filter({ hasText: /^ok[.!]?$/i })).toBeVisible({ timeout: 100_000 });
      const callId = res.headers()["x-litellm-call-id"];
      expect(callId, "gateway call id").toBeTruthy();
      const headers = { Authorization: `Bearer ${await sessionBearer(page)}` };
      let row: any;
      await expect.poll(async () => {
        const detail = await page.request.get(`${GATEWAY}/spend/logs/ui/${callId}`, { headers });
        if (!detail.ok()) return 0;
        row = await detail.json();
        return Number(row.spend);
      }, { timeout: 20_000, message: "actual supplier usage must produce a persisted positive bill" }).toBeGreaterThan(0);
      expect(Number(row.prompt_tokens)).toBeGreaterThan(0);
      expect(Number(row.completion_tokens)).toBeGreaterThan(0);
      const bill = row.metadata.cost_breakdown;
      expect(bill.source).toBe("snapshot");
      const total = bill.applied.reduce((sum: number, rate: any) => sum + Number(rate.quantity) * Number(rate.usd), 0);
      expect(total).toBeCloseTo(Number(row.spend), 9);
      await page.goto(uiPath("/logs"));
      await expect(page.getByText(alias, { exact: true }).first()).toBeVisible();
      recordChain(`real vendor=${vendor} model=${model} answer=ok usage=positive billing=snapshot`);
      guard.assertOk();
    });
  }
}

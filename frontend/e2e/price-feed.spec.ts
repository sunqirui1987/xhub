import { expect, test } from "@playwright/test";
import { GATEWAY, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/** 前置：真实浏览器、网关和隔离数据库，未配置远程价格源。
 * 验证管理页刷新失败提示、定时计划拒绝启用以及价格目录保持原值。
 * 参数 page：浏览器页面；返回 Promise<void>；不写价格或计划，隔离 schema 由脚本清理，无外部调用。 */
test("unconfigured price feed reports failure and preserves catalog", async ({ page }) => {
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + await sessionBearer(page) };
  const before = await page.request.get(GATEWAY + "/price/catalog", { headers });
  expect(before.status(), await before.text()).toBe(200);
  const original = await before.json();
  await stableGoto(page, "/price-data");
  await page.getByRole("button", { name: t("Price Data Management"), exact: true }).click();
  const management = page.getByRole("dialog", { name: t("Price Data Management"), exact: true });
  await expect(management.getByText(t("No periodic reload scheduled"), { exact: true })).toBeVisible();
  await management.getByRole("button", { name: t("Reload Price Data"), exact: true }).click();
  const reload = page.waitForResponse(r => new URL(r.url()).pathname === "/reload/model_cost_map" && r.request().method() === "POST");
  await page.getByRole("alertdialog").getByRole("button", { name: t("Yes"), exact: true }).click();
  const failed = await reload;
  expect(failed.status(), await failed.text()).toBe(502);
  expect(await failed.text()).toContain("price feed URL must be explicitly configured");
  await expect(page.getByText(t("Failed to reload price data"), { exact: true })).toBeVisible();
  await management.getByRole("button", { name: t("Set Up Periodic Reload"), exact: true }).click();
  const schedule = page.getByRole("dialog", { name: t("Set Up Periodic Reload"), exact: true });
  await schedule.getByRole("spinbutton", { name: t("Reload interval in hours") }).fill("6");
  const scheduling = page.waitForResponse(r => new URL(r.url()).pathname === "/schedule/model_cost_map_reload" && r.request().method() === "POST");
  await schedule.getByRole("button", { name: t("Schedule"), exact: true }).click();
  const rejected = await scheduling;
  expect(rejected.status(), await rejected.text()).toBe(400);
  expect(await rejected.text()).toContain("XHUB_PRICE_FEED_URL must be configured");
  await expect(page.getByText(t("Failed to schedule periodic reload"), { exact: true })).toBeVisible();
  await schedule.getByRole("button", { name: t("Cancel"), exact: true }).click();
  const status = await page.request.get(GATEWAY + "/schedule/model_cost_map_reload/status", { headers });
  expect(status.status()).toBe(200);
  expect(await status.json()).toMatchObject({ scheduled: false, last_run: null });
  const after = await page.request.get(GATEWAY + "/price/catalog", { headers });
  expect(after.status()).toBe(200);
  expect(await after.json(), "刷新失败应保留目录和手工价格").toEqual(original);
});

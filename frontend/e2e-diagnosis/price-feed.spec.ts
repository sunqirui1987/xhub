import { expect, test } from "@playwright/test";
import { GATEWAY, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/** 前置真实浏览器、后台、隔离数据库与本地市场源；验证刷新、待添加提示、公开筛选、详情和上下架持久化。
 * 不自动部署模型，最后重新上架并取消计划；隔离 schema 由脚本清理，不依赖外部凭据。 */
test("market refresh preserves local records and listing state", async ({ page }) => {
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  await stableGoto(page, "/price-data");
  await expect(page.getByRole("tab", { name: "模型供应商", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: t("Price Data Management"), exact: true }).click();
  const management = page.getByRole("dialog", { name: t("Price Data Management"), exact: true });
  await management.getByRole("button", { name: t("Reload Price Data"), exact: true }).click();
  const reload = page.waitForResponse(
    (r) => new URL(r.url()).pathname === "/reload/model_cost_map" && r.request().method() === "POST",
  );
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: t("Yes"), exact: true })
    .click();
  expect((await reload).status()).toBe(200);
  const prompt = page.getByRole("dialog", { name: "是否添加价格表中的模型？" });
  await expect(prompt.getByRole("status")).toHaveText("发现 2 个尚未部署的在售公开模型。");
  await expect(prompt.getByRole("link", { name: "添加此模型" })).toHaveCount(2);
  await prompt.getByRole("button", { name: "暂不添加" }).click();
  await expect(page.getByTestId("price-row-e2e-market-chat")).toBeVisible();
  await expect(page.getByTestId("price-row-e2e-market-private")).toHaveCount(0);
  await expect(page.getByTestId("price-row-e2e-market-retired")).toHaveCount(0);
  const features = page.getByRole("group", { name: "特性标签" });
  await features.getByRole("button", { name: "视频生成 1", exact: true }).click();
  await expect(page.getByTestId("price-row-e2e-market-video")).toBeVisible();
  await expect(page.getByTestId("price-row-e2e-market-chat")).toHaveCount(0);
  await page.getByRole("button", { name: t("priceCatalog.reset"), exact: true }).click();
  await page.getByRole("button", { name: "查看 e2e-market-chat 详情" }).click();
  const detail = page.getByRole("dialog", { name: "e2e-market-chat", exact: true });
  await expect(detail.getByRole("table", { name: "完整市场价格" })).toContainText("$2");
  await detail.getByRole("tab", { name: "接入信息", exact: true }).click();
  await expect(detail.getByText(/尚未部署此模型/)).toBeVisible();
  await detail.getByRole("tab", { name: "API 接入", exact: true }).click();
  await expect(detail.getByText(/XHUB_API_KEY 环境变量/)).toBeVisible();
  await detail.getByRole("tab", { name: "调用文档", exact: true }).click();
  await expect(detail.getByText(/内部调用流程/)).toBeVisible();
  await page.keyboard.press("Escape");
  await page.getByTestId("price-row-e2e-market-chat").getByRole("button", { name: "下架", exact: true }).click();
  await expect(page.getByTestId("price-row-e2e-market-chat")).toHaveCount(0);
  await page.getByRole("tab", { name: "本地模型列表", exact: true }).click();
  const local = page.getByTestId("local-model-e2e-market-chat");
  await expect(local).toContainText("已下架");
  await expect(page.getByTestId("local-model-e2e-market-private")).toContainText("非公开");
  const refreshed = await page.request.post(GATEWAY + "/reload/model_cost_map", { headers });
  expect(refreshed.status(), await refreshed.text()).toBe(200);
  await page.reload();
  await page.getByRole("tab", { name: "本地模型列表", exact: true }).click();
  await expect(local).toContainText("已下架");
  const saved = await page.request.get(GATEWAY + "/price/catalog", { headers });
  const row = (await saved.json()).models.find((r: { id: string }) => r.id === "e2e-market-chat");
  expect(row).toMatchObject({ delisted: true, pricing_rules_v2: expect.any(Array) });
  await local.getByRole("button", { name: "上架", exact: true }).click();
  await expect(local).toContainText("未下架");
  await page.getByRole("tab", { name: "模型广场", exact: true }).click();
  await expect(page.getByTestId("price-row-e2e-market-chat")).toBeVisible();
  const plan = await page.request.post(GATEWAY + "/schedule/model_cost_map_reload?hours=6", { headers });
  expect(plan.status()).toBe(200);
  expect(
    (await (await page.request.get(GATEWAY + "/schedule/model_cost_map_reload/status", { headers })).json()).scheduled,
  ).toBe(true);
  expect((await page.request.delete(GATEWAY + "/schedule/model_cost_map_reload", { headers })).status()).toBe(200);
});

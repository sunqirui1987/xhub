import { expect, test } from "@playwright/test";
import { GATEWAY, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/** 前置真实浏览器、后台、隔离数据库及本地市场源；验证分类下拉、保存、重开编辑、清空与计价结果。
 * 仅编辑测试源专用模型，finally 恢复分类与单价；脚本清理隔离 schema，不依赖外部服务凭据。 */
test("price model classifications use dropdowns and persist selections", async ({ page }) => {
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  expect((await page.request.post(GATEWAY + "/reload/model_cost_map", { headers })).status()).toBe(200);
  const catalog = await page.request.get(GATEWAY + "/price/catalog", { headers });
  const original = (await catalog.json()).models.find((row: { id: string }) => row.id === "e2e-market-chat");
  expect(original, "市场源应提供专用测试模型").toBeTruthy();
  await stableGoto(page, "/price-data");
  const row = page.getByTestId("price-row-e2e-market-chat");
  try {
    await row.getByRole("button", { name: t("Edit"), exact: true }).click();
    const dialog = page.getByRole("dialog", { name: t("priceData.editModel"), exact: true });
    const provider = dialog.getByRole("combobox", { name: t("Provider"), exact: true });
    const mode = dialog.getByRole("combobox", { name: t("priceData.endpointMode"), exact: true });
    const endpoint = dialog.getByRole("combobox", { name: t("priceData.endpointType"), exact: true });
    await expect(provider).toBeVisible();
    await expect(mode).toBeVisible();
    await expect(endpoint).toBeEnabled();
    await provider.selectOption("openai");
    await mode.selectOption("chat");
    await endpoint.selectOption("bypass_openai_chat");
    await dialog.getByLabel(t("Input"), { exact: true }).fill("3");
    const saved = page.waitForResponse(
      (r) => new URL(r.url()).pathname === "/price/model" && r.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: t("Save"), exact: true }).click();
    const response = await saved;
    expect(response.status(), await response.text()).toBe(200);
    expect(response.request().postDataJSON()).toMatchObject({
      litellm_provider: "openai",
      mode: "chat",
      endpoint_id: "bypass_openai_chat",
      input_cost_per_token: 0.000003,
    });
    await expect(dialog).toHaveCount(0);
    const savedCatalog = await page.request.get(GATEWAY + "/price/catalog", { headers });
    expect((await savedCatalog.json()).models.find((entry: { id: string }) => entry.id === original.id)).toMatchObject({
      litellm_provider: "openai",
      mode: "chat",
      endpoint_id: "bypass_openai_chat",
      input_cost_per_token: 0.000003,
    });
    const calculated = await page.request.post(GATEWAY + "/spend/calculate", {
      headers,
      data: {
        model: original.id,
        completion_response: { model: original.id, usage: { prompt_tokens: 1000000, completion_tokens: 0 } },
      },
    });
    expect(calculated.status(), await calculated.text()).toBe(200);
    expect((await calculated.json()).cost).toBeCloseTo(3);
    await page.reload();
    await row.getByRole("button", { name: t("Edit"), exact: true }).click();
    await expect(provider).toHaveValue("openai");
    await expect(mode).toHaveValue("chat");
    await expect(endpoint).toHaveValue("bypass_openai_chat");
    await expect(endpoint).toBeEnabled();
    await mode.selectOption("");
    await endpoint.selectOption("");
    const cleared = page.waitForResponse(
      (r) => new URL(r.url()).pathname === "/price/model" && r.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: t("Save"), exact: true }).click();
    expect((await cleared).status()).toBe(200);
    await expect(dialog).toHaveCount(0);
    await page.reload();
    await row.getByRole("button", { name: t("Edit"), exact: true }).click();
    await expect(mode).toHaveValue("");
    await expect(endpoint).toHaveValue("");
  } finally {
    const restored = await page.request.post(GATEWAY + "/price/model", {
      headers,
      data: {
        id: original.id,
        litellm_provider: original.litellm_provider,
        mode: original.mode ?? null,
        endpoint_id: original.endpoint_id ?? null,
        endpoint_type: original.endpoint_type ?? null,
        input_cost_per_token: original.input_cost_per_token ?? null,
      },
    });
    expect(restored.status(), "恢复测试模型原始分类与价格").toBe(200);
  }
});

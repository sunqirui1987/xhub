import { expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto } from "./helpers";
/** 前置真实网关和隔离图片上游；检查供应商页中文/英文切换，创建图片部署，执行英文调试台 curl 并验证真实响应/账单。
 * 图片参数和用户中文输入切换后保留；finally 删除部署，调用日志随隔离 schema 清理，不调用外部供应商。 */
test("provider and image playground translate completely with executable curl", async ({ page }) => {
  test.setTimeout(120000);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  let id = "";
  const name = "e2e-curl-i18n-" + Date.now();
  try {
    await stableGoto(page, "/model-providers");
    await page.getByRole("button", { name: "English", exact: true }).click();
    await expect(page.getByRole("button", { name: "Add provider", exact: true })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Model providers" })).toHaveCount(2);
    await expect(page.getByText("模型提供商", { exact: true })).toHaveCount(0);
    await expect(page.getByText(/Each provider can have multiple/)).toBeVisible();
    await page.getByRole("button", { name: "中文", exact: true }).click();
    await expect(page.getByRole("button", { name: "添加提供商", exact: true })).toBeVisible();
    const created = await page.request.post(GATEWAY + "/model/new", {
      headers,
      data: {
        model_name: name,
        litellm_params: {
          model: "openai/gpt-image-2",
          custom_llm_provider: "openai",
          api_base: UPSTREAM + "/v1",
          api_key: "sk-fake",
          input_cost_per_token: 0.000001,
          output_cost_per_token: 0.000002,
          // 图片夹具按张返回结果，不保证 token 用量；显式按张定价才能验证真实图片账单。
          output_cost_per_image: 0.01,
        },
        model_info: { transport: "openai_image_generation", pricing_source: "manual" },
      },
    });
    expect(created.status(), await created.text()).toBe(200);
    id = (await created.json()).model_info.id;
    await stableGoto(page, "/playground");
    await page.getByPlaceholder("选择模型", { exact: true }).click();
    await page.getByRole("option", { name, exact: true }).click();
    await page.getByLabel("图片提示词", { exact: true }).fill("用户的猫");
    await page.getByRole("button", { name: "English", exact: true }).click();
    await expect(page.getByLabel("Image prompt", { exact: true })).toHaveValue("用户的猫");
    await expect(page.getByLabel("Image size", { exact: true })).toHaveValue("1024x1024");
    await expect(page.getByRole("heading", { name: "Image playground" })).toBeVisible();
    await expect(page.getByPlaceholder("Leave blank to use the provider default")).toBeVisible();
    await page.getByLabel("Image prompt", { exact: true }).fill("A cat in warm sunlight.");
    const guide = page.getByLabel("Complete curl request", { exact: true });
    await guide.locator("summary").click();
    const curl = await guide
      .getByLabel("Copy request example", { exact: true })
      .filter({ hasText: "curl" })
      .textContent();
    expect(curl).toContain('"size": "1024x1024"');
    expect(curl).toContain('"prompt": "A cat in warm sunlight."');
    const output = JSON.parse(
      execFileSync("bash", ["-c", curl!], {
        env: { ...process.env, XHUB_API_KEY: await sessionBearer(page) },
        timeout: 20000,
        encoding: "utf8",
      }),
    );
    expect(output.data[0].b64_json || output.data[0].url).toBeTruthy();
    const sending = page.waitForResponse(
      (r) => new URL(r.url()).pathname === "/v1/images/generations" && r.request().method() === "POST",
    );
    await page.getByRole("button", { name: "Submit request", exact: true }).click();
    const response = await sending;
    expect(response.status(), await response.text()).toBe(200);
    await expect(page.getByRole("img", { name: "Generated image 1", exact: true })).toBeVisible();
    const callId = response.headers()["x-litellm-call-id"];
    expect(callId).toBeTruthy();
    await expect
      .poll(async () => {
        const log = await page.request.get(GATEWAY + "/spend/logs/ui/" + callId, { headers });
        return log.ok() ? Number((await log.json()).spend) : 0;
      })
      .toBeGreaterThan(0);
    await page.getByRole("button", { name: "中文", exact: true }).click();
    await expect(page.getByLabel("图片提示词", { exact: true })).toHaveValue("A cat in warm sunlight.");
    await expect(page.getByRole("heading", { name: "图片调试" })).toBeVisible();
    await page.screenshot({
      path: (process.env.MINE_MODEL_REPORT || "../.e2e/curl-guide/browser") + "/image-i18n.png",
      fullPage: true,
    });
  } finally {
    if (id) expect((await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } })).status()).toBe(200);
  }
});

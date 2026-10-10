import { expect, test, type Page } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t } from "./helpers";

/** 选择公开模型及客户聊天端点；参数 page/name 为页面和别名，返回完成 Promise；只操作隔离测试页面。 */
async function selectChat(page: Page, name: string) {
  await page.getByPlaceholder(t("Select a Model"), { exact: true }).click();
  await page.getByRole("option", { name, exact: true }).click();
  await page.getByPlaceholder(t("Select an endpoint"), { exact: true }).click();
  await page
    .getByRole("option")
    .filter({ hasText: /\/v1\/chat\/completions$/ })
    .click();
}

/** 创建隔离部署；参数 page/name/key 为浏览器、公开别名和是否配密钥，返回部署 ID。
 * 仅使用本地上游，调用方 finally 删除部署，账单由 runner 私有 schema 清理。 */
async function deployment(page: Page, name: string, key = true) {
  const response = await page.request.post(GATEWAY + "/model/new", {
    headers: { Authorization: "Bearer " + (await sessionBearer(page)) },
    data: {
      model_name: name,
      litellm_params: {
        model: "playground-local",
        custom_llm_provider: "openai",
        api_base: UPSTREAM,
        ...(key ? { api_key: "sk-fake" } : {}),
      },
      model_info: { transport: "bypass_openai_chat", endpoint_types: ["chat", "responses"] },
    },
  });
  expect(response.status(), await response.text()).toBe(200);
  return (await response.json()).model_info.id as string;
}

/** 前置真实部署缺少密钥；验证 HTTP401 独立诊断、草稿保留、配置补齐后重试成功和真实账单。
 * 同时禁用已选择部署复现 deployments=0；finally 删除模型，runner 清理账单。 */
test("chat explains missing credentials and unavailable deployment, then recovers", async ({ page }) => {
  await loginAdmin(page);
  const name = "e2e-playground-recovery";
  const id = await deployment(page, name, false);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  let deleted = false;
  try {
    await stableGoto(page, "/playground");
    await selectChat(page, name);
    const input = page.getByPlaceholder(t("Type your message... (Shift+Enter for new line)")).filter({ visible: true });
    await input.fill("保留这条调试草稿");
    const denied = page.waitForResponse(
      (r) => /chat\/completions$/.test(new URL(r.url()).pathname) && r.request().method() === "POST",
    );
    await page.getByRole("button", { name: t("Send message") }).click();
    expect((await denied).status()).toBe(401);
    await expect(page.getByRole("alert").filter({ hasText: "模型未配置上游密钥" })).toBeVisible();
    await expect(input).toHaveValue("保留这条调试草稿");
    await expect(page.getByRole("link", { name: "检查模型与端点配置" })).toHaveAttribute(
      "href",
      "/ui/models-and-endpoints",
    );
    await page.screenshot({ path: "../.e2e/playground/credential-diagnostic.png", fullPage: true });
    expect(
      (
        await page.request.patch(GATEWAY + "/model/" + id + "/update", {
          headers,
          data: { litellm_params: { api_key: "sk-fake" } },
        })
      ).status(),
    ).toBe(200);
    const success = page.waitForResponse(
      (r) => /chat\/completions$/.test(new URL(r.url()).pathname) && r.request().method() === "POST",
    );
    await page.getByRole("button", { name: t("Send message") }).click();
    const response = await success;
    expect(response.status()).toBe(200);
    expect(response.request().postDataJSON().messages).toHaveLength(1);
    await expect(page.getByText("e2e-ok", { exact: true })).toBeVisible();
    await expect(input).toHaveValue("");
    const callId = response.headers()["x-litellm-call-id"];
    expect(callId).toBeTruthy();
    await expect
      .poll(async () => {
        const detail = await page.request.get(GATEWAY + "/spend/logs/ui/" + callId, { headers });
        return detail.ok() ? (await detail.json()).model : "";
      })
      .toBe(name);
    expect(
      (
        await page.request.patch(GATEWAY + "/model/" + id + "/update", {
          headers,
          data: { model_info: { disabled: true } },
        })
      ).status(),
    ).toBe(200);
    await input.fill("不可用部署");
    await page.getByRole("button", { name: t("Send message") }).click();
    await expect(page.getByRole("alert").filter({ hasText: "模型没有可用部署" })).toBeVisible();
    await page.getByRole("button", { name: t("Clear Chat"), exact: true }).click();
    await expect(page.getByRole("alert").filter({ hasText: "模型没有可用部署" })).toHaveCount(0);
    expect((await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } })).status()).toBe(200);
    deleted = true;
    await input.fill("已删除部署");
    const unavailable = page.waitForResponse(
      (r) => /chat\/completions$/.test(new URL(r.url()).pathname) && r.request().method() === "POST",
    );
    await page.getByRole("button", { name: t("Send message") }).click();
    expect((await unavailable).status()).toBe(400);
    await expect(page.getByRole("alert").filter({ hasText: "模型没有可用部署" })).toBeVisible();
  } finally {
    if (!deleted)
      expect((await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } })).status()).toBe(200);
  }
});

/** 前置两张独立真实对比卡片；一张缺密钥、一张成功，验证失败隔离和补齐后重试；finally 删除专用部署。 */
test("comparison isolates card failures and supports recovery", async ({ page }) => {
  await loginAdmin(page);
  const name = "e2e-compare-missing-key";
  const id = await deployment(page, name, false);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  try {
    await stableGoto(page, "/playground");
    await page.getByRole("tab", { name: t("pages.playground.compare"), exact: true }).click();
    const first = page.getByRole("region", { name: "对比卡片 1", exact: true });
    const second = page.getByRole("region", { name: "对比卡片 2", exact: true });
    for (const [card, model] of [
      [first, name],
      [second, "gpt-4o-mini"],
    ] as const) {
      await card.getByRole("combobox").first().click();
      await page.getByRole("option", { name: model, exact: true }).click();
    }
    const input = page.getByPlaceholder(t("Type your message... (Shift+Enter for new line)")).filter({ visible: true });
    await input.fill("对比失败隔离");
    await page.getByRole("button", { name: t("Send message") }).click();
    await expect(first.getByRole("alert")).toContainText("模型未配置上游密钥");
    await expect(second.getByText("e2e-ok", { exact: true })).toBeVisible();
    await expect(input).toHaveValue("对比失败隔离");
    expect(
      (
        await page.request.patch(GATEWAY + "/model/" + id + "/update", {
          headers,
          data: { litellm_params: { api_key: "sk-fake" } },
        })
      ).status(),
    ).toBe(200);
    await page.getByRole("button", { name: t("Send message") }).click();
    await expect(first.getByText("e2e-ok", { exact: true })).toBeVisible();
    await expect(input).toHaveValue("");
    await expect(second.getByText("e2e-ok", { exact: true })).toHaveCount(1);
    await expect(first.getByText("e2e-ok", { exact: true })).toBeInViewport();
    await expect(second.getByText("e2e-ok", { exact: true })).toBeInViewport();
    await page.screenshot({ path: "../.e2e/playground/comparison-desktop.png", fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await input.fill("窄屏对比");
    await page.getByRole("button", { name: t("Send message") }).click();
    await expect(input).toHaveValue("");
    await expect(first.getByText("e2e-ok", { exact: true })).toHaveCount(2);
    await expect(second.getByText("e2e-ok", { exact: true })).toHaveCount(2);
    await second.getByText("e2e-ok", { exact: true }).last().scrollIntoViewIfNeeded();
    await expect(second.getByText("e2e-ok", { exact: true }).last()).toBeInViewport();
    await page.screenshot({ path: "../.e2e/playground/comparison-mobile.png", fullPage: true });
    await page.getByRole("button", { name: t("Clear All Chats") }).click();
    await expect(page.getByText("e2e-ok", { exact: true })).toHaveCount(0);
  } finally {
    expect((await page.request.post(GATEWAY + "/model/delete", { headers, data: { id } })).status()).toBe(200);
  }
});

/** 前置真实本地上游；通过网络门闩延后真实响应，验证清空/停止隔离迟到数据，并在手机宽度实际发送。
 * 门闩仅控制传输时间，响应由真实后台生成；无配置写入，schema 清理账单。 */
test("cancel and clear ignore delayed real responses; mobile chat remains usable", async ({ page }) => {
  await loginAdmin(page);
  await stableGoto(page, "/playground");
  // 损坏的旧开关不能使工作区崩溃；随后仍通过真实网关完成取消和发送。
  await page.evaluate(() => sessionStorage.setItem("codeInterpreterEnabled", "{bad"));
  await page.reload();
  await selectChat(page, "gpt-4o-mini");
  const input = page.getByPlaceholder(t("Type your message... (Shift+Enter for new line)")).filter({ visible: true });
  for (const action of ["clear", "stop"]) {
    let release!: () => void;
    let received!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const fetched = new Promise<void>((resolve) => {
      received = resolve;
    });
    await page.route(/chat\/completions$/, async (route) => {
      const real = await route.fetch();
      expect(real.status()).toBe(200);
      received();
      await gate;
      await route.fulfill({ response: real }).catch(() => {});
    });
    await input.fill("delayed-" + action);
    await page.getByRole("button", { name: t("Send message") }).click();
    await fetched;
    if (action === "clear") await page.getByRole("button", { name: t("Clear Chat"), exact: true }).click();
    else await page.getByRole("button", { name: t("Stop request"), exact: true }).click();
    release();
    await page.unroute(/chat\/completions$/);
    await expect(page.getByText("e2e-ok", { exact: true })).toHaveCount(0);
    await expect(input).toHaveValue("delayed-" + action);
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await input.fill("mobile-real-request");
  await page.getByRole("button", { name: t("Send message") }).click();
  await expect(page.getByText("e2e-ok", { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: "../.e2e/playground/chat-mobile.png", fullPage: true });
});

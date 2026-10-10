import { expect, test } from "@playwright/test";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFileSync } from "node:child_process";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer } from "./helpers";

/** 目的：验证控制台入口、四分类、搜索、语言切换、复制、文章直达与移动布局；前置真实隔离网关，执行文档 curl 并核对返回及日志费用。
 * 创建专用模型和虚拟密钥，finally 删除；其余日志和登录夹具随隔离 schema 清理，不调用外部供应商。 */
test("documentation guides a real gateway request in both languages", async ({ page, context }) => {
  test.setTimeout(120_000);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + (await sessionBearer(page)) };
  await page.getByRole("link", { name: "文档中心", exact: true }).click();
  await expect(page).toHaveURL(/\/docs$/);
  await expect(page.getByRole("heading", { name: "欢迎使用 XHub" })).toBeVisible();
  await expect(page.getByRole("searchbox", { name: "搜索文档" })).toHaveValue("");
  await expect(page.getByRole("searchbox", { name: "搜索文档" })).toHaveAttribute("autocomplete", "off");
  const categories = page.getByRole("navigation", { name: "文档分类" });
  for (const [name, title] of [
    ["工具接入", "工具接入概览"],
    ["计费说明", "按量计费"],
    ["API 文档", "模型列表"],
    ["产品文档", "欢迎使用 XHub"],
  ]) {
    await categories.getByRole("link", { name, exact: true }).click();
    await expect(page.getByRole("heading", { name: title, exact: true })).toBeVisible();
    const directory = page.getByRole("navigation", { name: "文档目录" });
    // HTTP 方法只属于 API 目录；覆盖条件分支，避免非 API 分类混入接口标记。
    if (name === "API 文档") {
      await expect(directory.getByText("GET", { exact: true }).first()).toBeVisible();
    } else {
      await expect(directory.getByText("POST", { exact: true })).toHaveCount(0);
    }
    const forbidden = name === "产品文档" ? "模型列表" : "快速开始";
    await expect(directory.getByRole("link", { name: forbidden, exact: true })).toHaveCount(0);
  }
  await page
    .getByRole("navigation", { name: "文档目录" })
    .getByRole("link", { name: "常见问题与故障排查", exact: true })
    .click();
  await expect(page.getByRole("main")).toContainText("401：");
  await categories.getByRole("link", { name: "产品文档", exact: true }).click();
  await expect(page).toHaveURL(/\/docs$/);
  await expect(page.getByRole("searchbox", { name: "搜索文档" })).toHaveValue("");
  await page.getByRole("searchbox", { name: "搜索文档" }).fill("不存在的文档xyz");
  await expect(page.getByRole("status")).toHaveText("找到 0 篇文档");
  await page.getByRole("button", { name: "English", exact: true }).click();
  // 文档组件订阅语言并保持组件身份，搜索原文不得因切换而丢失。
  await expect(page.getByRole("searchbox", { name: "Search documentation" })).toHaveValue("不存在的文档xyz");
  await expect(page.getByText("No matching documents. Try another keyword.")).toBeVisible();
  await page.getByRole("searchbox").fill("quickstart");
  await page
    .getByRole("navigation", { name: "Documentation directory" })
    .getByRole("link", { name: "Quickstart", exact: true })
    .click();
  await expect(page).toHaveURL(/\/docs\/product\/quickstart$/);
  await page.reload();
  await expect(page.getByRole("heading", { name: "Quickstart", exact: true })).toBeVisible();
  const code = page.getByLabel("Request example", { exact: true });
  const example = await code.textContent();
  expect(example).toContain("Hello, introduce yourself.");
  expect(example).not.toMatch(/[\u4e00-\u9fff]/);
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.getByRole("button", { name: "Copy example" }).click();
  await expect(page.getByRole("status")).toHaveText("Copied");
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(example);
  let modelId = "";
  let apiKey = "";
  const name = "e2e-docs-" + Date.now();
  const temp = mkdtempSync(join(tmpdir(), "xhub-docs-"));
  const headerPath = join(temp, "headers");
  try {
    const model = await page.request.post(GATEWAY + "/model/new", {
      headers,
      data: {
        model_name: name,
        litellm_params: {
          model: "gpt-4o-mini",
          custom_llm_provider: "openai",
          api_base: UPSTREAM + "/v1",
          api_key: "sk-fake",
          input_cost_per_token: 0.000002,
          output_cost_per_token: 0.000008,
        },
        model_info: { transport: "bypass_openai_chat", pricing_source: "manual" },
      },
    });
    expect(model.status(), await model.text()).toBe(200);
    modelId = (await model.json()).model_info.id;
    const key = await page.request.post(GATEWAY + "/key/generate", {
      headers,
      data: { models: [name], key_alias: name },
    });
    expect(key.status(), await key.text()).toBe(200);
    apiKey = (await key.json()).key;
    const publicList = await page.request.get(GATEWAY + "/models", { headers: { Authorization: "" } });
    expect(publicList.status()).toBe(200);
    const publicRows = (await publicList.json()).data;
    expect(publicRows.some((row: { id: string }) => row.id === name)).toBe(true);
    expect(Object.keys(publicRows.find((row: { id: string }) => row.id === name)).sort()).toEqual([
      "created",
      "id",
      "object",
      "owned_by",
    ]);

    const curl = example!.replace("YOUR_MODEL_NAME", name);
    const raw = execFileSync("bash", ["-c", curl.replace("curl ", "curl -sS -D " + headerPath + " ")], {
      env: { ...process.env, XHUB_BASE_URL: GATEWAY, XHUB_API_KEY: apiKey },
      timeout: 20_000,
      encoding: "utf8",
    });
    const response = JSON.parse(raw);
    expect(response.choices[0].message.content).toBeTruthy();
    expect(response.usage.prompt_tokens).toBeGreaterThan(0);
    const callId = readFileSync(headerPath, "utf8").match(/x-litellm-call-id:\s*([^\r\n]+)/i)?.[1];
    expect(callId).toBeTruthy();
    await expect
      .poll(async () => {
        const log = await page.request.get(GATEWAY + "/spend/logs/ui/" + callId, { headers });
        return log.ok() ? Number((await log.json()).spend) : 0;
      })
      .toBeGreaterThan(0);
    // 在线运行必须经过浏览器到网关及真实数据面，并验证持久化费用。
    await page.goto("/docs/api/chat-completions");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await page.getByRole("button", { name: "Run online", exact: true }).click();
    await expect(page.getByRole("dialog", { name: "Run online" })).toBeVisible();
    await expect(page.getByRole("searchbox", { includeHidden: true })).toHaveValue("");
    await expect(page.getByRole("searchbox", { includeHidden: true })).toHaveAttribute(
      "id",
      "xhub-documentation-filter-text",
    );
    await expect(page.getByLabel("XHub API key", { exact: true })).toHaveAttribute("autocomplete", "new-password");
    await page.getByLabel("XHub API key", { exact: true }).fill(apiKey);
    await page
      .getByLabel("Request body (JSON)")
      .fill(
        JSON.stringify({ model: name, messages: [{ role: "user", content: "Hello from the documentation runner." }] }),
      );
    await page.getByRole("button", { name: "Run request", exact: true }).click();
    const liveResult = page.getByRole("region", { name: "Run online" }).getByRole("status");
    await expect(liveResult).toContainText("HTTP 200");
    await expect(liveResult).toContainText("prompt_tokens");
    const liveCall = (await liveResult.textContent())?.match(/x-litellm-call-id: ([^\n]+)/)?.[1];
    expect(liveCall).toBeTruthy();
    await expect
      .poll(async () => {
        const log = await page.request.get(GATEWAY + "/spend/logs/ui/" + liveCall, { headers });
        return log.ok() ? Number((await log.json()).spend) : 0;
      })
      .toBeGreaterThan(0);
    await page.screenshot({ path: "../.e2e/docs-center/runner.png", fullPage: true });
    await page.goto("/docs/api/models");
    await page.getByRole("button", { name: "Run online", exact: true }).click();
    await expect(page.getByLabel("XHub API key", { exact: true })).toHaveValue("");
    // 公开模型发现不填写密钥即可运行，仍经过真实网关读取已启用目录。
    await page.getByRole("button", { name: "Run request", exact: true }).click();
    const discoveryResult = page.getByRole("region", { name: "Run online" }).getByRole("status");
    await expect(discoveryResult).toContainText("HTTP 200");
    await expect(discoveryResult).toContainText(name);
    await page.getByLabel("XHub API key", { exact: true }).fill("invalid-docs-key");
    await page.getByRole("button", { name: "Run request", exact: true }).click();
    await expect(page.getByRole("region", { name: "Run online" }).getByRole("status")).toContainText("HTTP 401");
    await page.getByRole("button", { name: "Close runner" }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await page.getByRole("button", { name: "Run online", exact: true }).click();
    await expect(page.getByLabel("XHub API key", { exact: true })).toHaveValue("");
    await page.keyboard.press("Escape");
    const modelsPanel = page.getByRole("region", { name: "shell code panel", exact: true });
    await modelsPanel.getByRole("button", { name: "Copy code" }).click();
    await expect(modelsPanel.getByRole("status")).toHaveText("Copied");
    const modelsExample = await page.evaluate(() => navigator.clipboard.readText());
    const models = JSON.parse(
      execFileSync("bash", ["-c", modelsExample], {
        env: { ...process.env, XHUB_BASE_URL: GATEWAY, XHUB_API_KEY: apiKey },
        timeout: 20_000,
        encoding: "utf8",
      }),
    );
    expect(models.data.some((model: { id: string }) => model.id === name)).toBe(true);
    expect(
      (
        await page.request.get(GATEWAY + "/v1/models", { headers: { Authorization: "Bearer invalid-docs-key" } })
      ).status(),
    ).toBe(401);
  } finally {
    rmSync(temp, { recursive: true, force: true });
    if (apiKey)
      expect((await page.request.post(GATEWAY + "/key/delete", { headers, data: { keys: [apiKey] } })).status()).toBe(
        200,
      );
    if (modelId)
      expect((await page.request.post(GATEWAY + "/model/delete", { headers, data: { id: modelId } })).status()).toBe(
        200,
      );
  }
  await page.goto("/docs");
  await page.getByRole("button", { name: "中文", exact: true }).click();
  await expect(page.getByRole("heading", { name: "欢迎使用 XHub" })).toBeVisible();
  await page.screenshot({ path: "../.e2e/docs-center/desktop.png", fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.getByRole("searchbox", { name: "搜索文档" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: "../.e2e/docs-center/mobile.png", fullPage: true });
  expect((await page.goto("/docs/missing-article"))?.status()).toBe(404);
  await page.goto("/docs");
  await page.getByRole("link", { name: "返回控制台" }).click();
  await expect(page.getByRole("link", { name: "文档中心", exact: true })).toBeVisible();
});

/** 目的：验证未登录可直接阅读；前置无会话浏览器，根路径与 /ui 别名都可显示文档，无测试数据写入或清理。 */
test("documentation supports anonymous deep links and console aliases", async ({ page }) => {
  for (const path of ["/docs/tools-agents-clients/overview", "/ui/docs/tools-agents-clients/overview"]) {
    expect((await page.goto(path))?.status()).toBe(200);
    await expect(page.getByRole("heading", { name: "工具接入概览", exact: true })).toBeVisible();
  }
});

/** 目的：验收完整文档的配置、计费、参数及响应操作；前置公开阅读页面，中英切换与锚点直达可用；不写业务数据，接口实际调用由首个场景覆盖。 */
test("expanded documentation exposes tools, billing examples and API schemas", async ({ page }) => {
  await page.goto("/docs/tools-agents-clients/codex");
  await expect(page.getByRole("heading", { name: "Codex CLI", exact: true }).first()).toBeVisible();
  await expect(page.getByLabel("请求示例", { exact: true })).toContainText('wire_api = "responses"');
  await expect(page.getByLabel("请求示例", { exact: true })).toContainText('base_url = "' + GATEWAY + '"');
  await page.goto("/docs/tools-agents-clients/opencode");
  await expect(page.getByLabel("请求示例", { exact: true })).toContainText('"baseURL": "' + GATEWAY + '"');
  await expect(page.getByLabel("请求示例", { exact: true })).not.toContainText("YOUR_GATEWAY_HOST");
  await page.goto("/docs/billing/usage-based-billing");
  await expect(page.getByRole("heading", { name: "完整计算示例" })).toBeVisible();
  await expect(page.getByRole("main")).toContainText("$0.0051");
  await page.goto("/docs/api/ark");
  await expect(page.getByRole("heading", { name: "内容生成任务协议", exact: true, level: 1 })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "文档目录" })).toContainText("OpenAI 兼容协议");
  await expect(page.getByRole("navigation", { name: "文档目录" })).toContainText("原厂协议 Bypass");
  await page.goto("/docs/api/models#listModels");
  await expect(page.getByRole("link", { name: /下一篇.*对话补全/ })).toHaveAttribute(
    "href",
    "/docs/api/chat-completions",
  );
  await expect(page.getByRole("cell", { name: "data[].id", exact: true })).toBeVisible();
  await expect(page.getByRole("main")).toContainText("XHub 凭据缺失或无效");
  await expect(page.getByRole("region", { name: "json 代码面板" })).toBeVisible();
  await page.getByRole("region", { name: "shell 代码面板" }).getByRole("button", { name: "自动换行" }).click();
  await expect(
    page.getByRole("region", { name: "shell 代码面板" }).getByRole("button", { name: "自动换行" }),
  ).toHaveAttribute("aria-pressed", "true");
  await page.getByRole("button", { name: "English", exact: true }).click();
  await expect(page.getByRole("main")).toContainText("Missing or invalid XHub credential.");
  await expect(page.getByRole("heading", { name: "Request parameters", exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "json code panel" })).toBeVisible();
  await page.screenshot({ path: "../.e2e/docs-center/api-reference.png", fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Run online", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "Run online" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

/** 目的：验证真实 API 三级导航、折叠、模型专页及运行初始路径；前置隔离网关，打开弹窗不发送付费请求，无新增业务数据。 */
test("API navigation separates images and video queue protocols", async ({ page }) => {
  await page.goto("/docs/api/fal-seedance");
  const nav = page.getByRole("navigation", { name: "文档目录" });
  await expect(nav.getByRole("heading", { name: "图像生成与编辑" })).toBeVisible();
  await expect(nav.getByRole("link", { name: "Seedance", exact: true })).toHaveAttribute("aria-current", "page");
  await expect(page.getByRole("main")).toContainText("/queue/bytedance/seedance-2.5/text-to-video");
  await page.getByRole("button", { name: "在线运行", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("/queue/bytedance/seedance-2.0/text-to-video");
  await expect(page.getByRole("searchbox", { includeHidden: true })).toHaveValue("");
  await page.getByRole("button", { name: "关闭在线运行", exact: true }).click();
  await nav.getByText("Fal · 异步队列", { exact: true }).click();
  await expect(nav.getByRole("link", { name: "Seedance", exact: true })).not.toBeVisible();
  await nav.getByText("Fal · 异步队列", { exact: true }).click();
  await nav.getByRole("link", { name: "Veo 3.1", exact: true }).click();
  await expect(page.getByRole("main")).toContainText("/queue/fal-ai/veo3.1");
  await page.getByRole("button", { name: "English", exact: true }).click();
  await expect(
    page
      .getByRole("navigation", { name: "Documentation directory" })
      .getByText("Fal · asynchronous queue", { exact: true }),
  ).toBeVisible();
});

/** 目的：验证原厂总览、独立接口、弹窗和双语目录；前置真实网关，无凭据请求返回真实认证错误，搜索不自动填充，无业务数据写入。 */
test("native protocol endpoints have dedicated guides and runners", async ({ page }) => {
  await page.goto("/docs/api/native-bypass");
  await expect(page.getByRole("button", { name: "在线运行", exact: true })).toHaveCount(0);
  const nav = page.getByRole("navigation", { name: "文档目录" });
  await expect(nav.getByRole("heading", { name: "原厂协议 Bypass", exact: true })).toBeVisible();
  const bypassSection = nav
    .locator("section")
    .filter({ has: page.getByRole("heading", { name: "原厂协议 Bypass", exact: true }) });
  await expect(bypassSection.getByRole("link", { name: "Bypass Anthropic 消息", exact: true })).toBeVisible();
  await expect(bypassSection.getByRole("link", { name: "Anthropic Messages", exact: true })).toHaveCount(0);
  await nav.getByRole("link", { name: "Bypass Anthropic 消息", exact: true }).click();
  await expect(page.getByRole("main")).toContainText("x-api-key: $XHUB_API_KEY");
  await page.getByRole("button", { name: "在线运行", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("/bypass/anthropic/v1/messages");
  await expect(page.getByRole("searchbox", { includeHidden: true })).toHaveValue("");
  await page.getByRole("button", { name: "关闭在线运行", exact: true }).click();
  expect((await page.request.post(GATEWAY + "/bypass/anthropic/v1/messages", { data: {} })).status()).toBe(401);
  await nav.getByRole("link", { name: "Bypass Vertex 内容生成", exact: true }).click();
  await expect(page.getByRole("main")).toContainText("/bypass/vertex/v1/models/YOUR_MODEL_NAME:generateContent");
  await page.getByRole("button", { name: "English", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Native protocol Bypass", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Bypass Vertex generateContent", level: 1 })).toBeVisible();
  await expect(page.getByRole("main")).toContainText("Administrators configure the Vertex project");
});

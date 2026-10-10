import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import { once } from "node:events";
import { chromium, expect } from "@playwright/test";
import { serve } from "../src/serve.mjs";
import { checkSite } from "./regression.mjs";

const directory = new URL("../dist/", import.meta.url).pathname;
const reportDirectory = new URL("../test-results/", import.meta.url).pathname;
await mkdir(reportDirectory, { recursive: true });
const server = serve({ directory, port: 0, mount: "/xhub/" });
await once(server, "listening");
const base = `http://127.0.0.1:${server.address().port}/xhub/`;
const results = [];
const regression = await checkSite(directory);
let browser;

/** 运行一个真实浏览器流程并收集结果；参数为名称与异步操作，异常记录后继续，浏览器上下文由主流程最终清理。 */
async function scenario(name, action) {
  try { await action(); results.push({ name, passed: true }); console.log(`PASS ${name}`); }
  catch (error) { results.push({ name, passed: false, error: error.stack }); console.error(`FAIL ${name}: ${error.message}`); }
}

try {
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, permissions: ["clipboard-read", "clipboard-write"] });
  const page = await context.newPage();
  const pageErrors = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  await scenario("构建回归：文档、资源、搜索页面和锚点均可达", async () => { assert.deepEqual(regression.errors, []); assert.ok(regression.indexedDocuments >= 100); });
  // 前置为真实静态网站和共享文档源，四类切换只显示本类正文及目录，验证帮助、中英对应页；无业务写入。
  await scenario("同步文档中心 → 帮助 → 四分类 → 英文", async () => {
    await page.goto(base + "docs/");
    await page.getByRole("heading", {name: "欢迎使用 XHub", exact: true}).waitFor();
    await page.getByRole("link", {name: "常见问题与故障排查", exact: true}).click();
    assert.match(await page.locator("article").innerText(), /401/);
    await page.getByRole("link", {name: "API 文档", exact: true}).click();
    await page.getByRole("heading", {name: "模型列表", exact: true}).waitFor();
    assert.equal(await page.locator(".docs-sidebar").getByRole("link", {name: "快速开始", exact: true}).count(), 0);
    assert.match(await page.locator("article").innerText(), /data\[\]\.id/);
    await page.getByRole("link", {name: "Switch to English", exact: true}).click();
    await page.getByRole("heading", {name: "List models", exact: true}).waitFor();
  });
  // 前置为独立静态服务器，验证首页入口到安装页、命令和刷新；不写业务数据，结束关闭上下文。
  await scenario("首页 → Docker 安装文档 → 刷新保留正文", async () => {
    await page.goto(base);
    await page.getByRole("heading", { name: "让企业 AI， 有序生长。" }).waitFor();
    assert.equal(await page.locator(".capability-card").count(), 8);
    await page.getByRole("link", { name: "开始使用", exact: true }).first().click();
    await page.getByRole("heading", { name: "XHub 安装与运行指南", exact: true }).waitFor();
    assert.match(await page.locator("article").innerText(), /bash deploy\/build.sh/);
    assert.match(await page.locator("article").innerText(), /admin@xhub.local/);
    await page.reload();
    await page.getByRole("heading", { name: "Docker Compose 部署", exact: true }).waitFor();
  });
  // 前置为真实本地索引，验证搜索跳到护栏正文并读到脚本；未 mock 请求，无业务数据清理。
  await scenario("全文搜索 → XGo 护栏 → 脚本与目录", async () => {
    await page.goto(base);
    await page.getByRole("button", { name: "搜索文档 /", exact: true }).click();
    await page.getByRole("searchbox").fill("XGo");
    await page.locator(".search-results").getByRole("link", { name: /XHub 护栏/ }).click();
    await page.getByRole("heading", { name: "XHub 护栏：配置与 XGo 自定义脚本", exact: true }).waitFor();
    assert.match(await page.locator("article").innerText(), /ApplyGuardrail/);
    await page.getByRole("complementary", { name: "本页内容" }).getByRole("link", { name: "用 Go 语法写 XGo", exact: true }).click();
    assert.match(page.url(), /#用-go-语法写-xgo|#%E7%94%A8/);
  });
  // 前置为首页代码块，验证复制实际写入剪贴板；测试凭据均为占位符，无需清理业务数据。
  await scenario("复制 Docker 命令到剪贴板", async () => {
    await page.goto(base);
    await page.locator(".code-panel").first().getByRole("button", { name: "复制代码" }).click();
    assert.match(await page.evaluate(() => navigator.clipboard.readText()), /docker compose up -d --no-build/);
    assert.equal(await page.getByRole("status").innerText(), "已复制");
  });
  // 前置为双语页面，验证首页和客户文档切换正确语言并保留对应指南，无业务数据写入。
  await scenario("中英文首页与安装文档切换", async () => {
    await page.goto(base);
    await page.getByRole("link", { name: "Switch to English", exact: true }).click();
    await page.getByRole("heading", { name: "Give enterprise AI room to grow.", exact: true }).waitFor();
    await page.getByRole("link", { name: "Get started", exact: true }).first().click();
    await page.getByRole("heading", { name: "Installing and Running XHub", exact: true }).waitFor();
    await page.getByRole("link", { name: "切换到简体中文", exact: true }).click();
    await page.getByRole("heading", { name: "XHub 安装与运行指南", exact: true }).waitFor();
  });
  // 前置为真实站点和仓库配图，验证双语安装/客户指南的图片完整加载、截图原图可达；无 mock 或业务数据，截图写入专用目录。
  await scenario("双语图文指南 → 图片加载 → 截图原图", async () => {
    for (const [document, count] of [["getting-started.zh-CN", 3], ["getting-started", 3], ["user-guide.zh-CN", 5], ["user-guide", 5]]) {
      await page.goto(`${base}docs/${document}.html`);
      const illustrations = page.getByRole("article").getByRole("img");
      assert.equal(await illustrations.count(), count, `${document} 应显示 ${count} 张配图`);
      for (const illustration of await illustrations.all()) {
        assert.ok(await illustration.getAttribute("alt"), "文档配图必须有可访问说明");
        await illustration.scrollIntoViewIfNeeded();
        await expect.poll(() => illustration.evaluate((element) => element.complete && element.naturalWidth > 0), { message: `${document} 图片应从真实静态服务器加载` }).toBeTruthy();
      }
    }
    await page.goto(`${base}docs/user-guide.zh-CN.html`);
    const screenshot = page.getByRole("img", { name: "模型与端点页面：Providers 标签、Add Model 入口及模型部署列表", exact: true });
    await screenshot.click();
    assert.equal(page.url(), `${base}assets/console-deployments.png`);
    const response = await page.request.get(page.url());
    assert.equal(response.status(), 200);
    assert.match(response.headers()["content-type"], /image\/png/);
    await page.goBack();
    await page.getByRole("heading", { name: "XHub 客户使用与管理手册", exact: true }).waitFor();
    await screenshot.scrollIntoViewIfNeeded();
    await page.screenshot({ path: `${reportDirectory}/illustrated-guide-desktop.png` });
    await page.getByRole("img", { name: "工作空间配置流程：模型接入、组织团队与成员、虚拟密钥、调用和用量核对", exact: true }).click();
    assert.equal(page.url(), `${base}assets/guide-setup.zh-CN.svg`);
    const vector = await page.request.get(page.url());
    assert.equal(vector.status(), 200);
    assert.match(vector.headers()["content-type"], /image\/svg\+xml/);
  });
  // 前置为真实索引加载，验证空结果与 Escape 退出的客户可见结果；不修改索引，无需清理。
  await scenario("搜索空结果与键盘关闭", async () => {
    await page.goto(base);
    await page.keyboard.press("/");
    await page.getByRole("searchbox").fill("zzzz-unavailable-918273");
    await page.getByText("没有找到相关文档", { exact: true }).waitFor();
    await page.keyboard.press("Escape");
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    assert.equal(await page.getByRole("dialog").isVisible(), false);
  });
  // 前置为网络真实失败，验证静态数据面失败提示；只中止请求而不替换响应，关闭页面后自动清理拦截。
  await scenario("搜索索引请求失败显示可恢复提示", async () => {
    const failurePage = await context.newPage();
    try {
      await failurePage.route("**/search-index.json", (route) => route.abort("failed"));
      await failurePage.goto(base);
      await failurePage.getByRole("button", { name: "搜索文档 /", exact: true }).click();
      await failurePage.getByRole("searchbox").fill("预算");
      await failurePage.getByText("搜索索引加载失败，请使用文档目录或稍后重试。", { exact: true }).waitFor();
      await failurePage.unroute("**/search-index.json");
      await failurePage.getByRole("searchbox").fill("XGo");
      await failurePage.locator(".search-results").getByRole("link", { name: /XHub 护栏/ }).waitFor();
    } finally { await failurePage.close(); }
  });
  // 前置为架构文档，验证本地 Mermaid 脚本生成实际 SVG，无外部 CDN 或后台依赖。
  await scenario("架构图由本地资源实际渲染", async () => {
    await page.goto(`${base}docs/development/architecture.html`);
    await page.locator(".mermaid svg").first().waitFor();
  });
  // 前置为移动设备尺寸，验证菜单、文档入口与横向布局，截图保存到专用报告目录。
  await scenario("移动端菜单 → 帮助文档，页面无横向溢出", async () => {
    const mobile = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
    const mobilePage = await mobile.newPage();
    try {
      await mobilePage.goto(base);
      assert.ok(await mobilePage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth));
      await mobilePage.screenshot({ path: `${reportDirectory}/homepage-mobile.png`, fullPage: true });
      await mobilePage.getByRole("button", { name: "打开导航" }).click();
      await mobilePage.getByRole("navigation").getByRole("link", { name: "帮助文档", exact: true }).click();
      await mobilePage.getByRole("heading", { name: "XHub 文档", exact: true }).waitFor();
      assert.ok(await mobilePage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth));
      await mobilePage.goto(`${base}docs/user-guide.zh-CN.html`);
      const diagram = mobilePage.getByRole("img", { name: "工作空间配置流程：模型接入、组织团队与成员、虚拟密钥、调用和用量核对", exact: true });
      await diagram.scrollIntoViewIfNeeded();
      await expect.poll(() => diagram.evaluate((element) => element.complete && element.naturalWidth > 0)).toBeTruthy();
      assert.ok(await mobilePage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), "移动端图文指南不能产生横向溢出");
      await mobilePage.screenshot({ path: `${reportDirectory}/illustrated-guide-mobile.png` });
    } finally { await mobile.close(); }
  });
  await page.goto(base);
  await page.screenshot({ path: `${reportDirectory}/homepage-desktop.png`, fullPage: true });
  await page.goto(`${base}docs/development/guardrails.html`);
  await page.screenshot({ path: `${reportDirectory}/documentation-desktop.png`, fullPage: true });
  await scenario("浏览器无未处理脚本错误", async () => { assert.deepEqual(pageErrors, []); });
  await context.close();
} finally {
  await browser?.close();
  await new Promise((resolve) => server.close(resolve));
  await writeFile(`${reportDirectory}/report.json`, JSON.stringify({ regression, results, passed: results.filter((result) => result.passed).length, failed: results.filter((result) => !result.passed).length }, null, 2));
}
if (results.some((result) => !result.passed)) process.exitCode = 1;

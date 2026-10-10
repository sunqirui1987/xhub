import test from "node:test";
import assert from "node:assert/strict";
import { documentPath, escapeHTML, headingSlug, rootPrefix } from "../src/core.mjs";
import { searchDocuments, searchExcerpt } from "../src/search.mjs";
import { renderMarkdown, rewriteLink } from "../src/markdown.mjs";

// 验证嵌套路径保留文档层级；前置为正常 Markdown 路径，输出静态 HTML，不创建文件无需清理。
test("文档路径保留目录与双语文件名", () => {
  assert.equal(documentPath("development/routing.md"), "docs/development/routing.html");
  assert.equal(documentPath("getting-started.zh-CN.md"), "docs/getting-started.zh-CN.html");
});

// 验证边界和非法路径不能逃出站点；前置为恶意输入，期望抛错，无文件副作用。
test("文档路径拒绝目录穿越和非文档输入", () => {
  for (const value of ["../secret.md", "/secret.md", "foo.txt", null]) assert.throws(() => documentPath(value));
});

// 验证项目子路径资源定位；前置为不同嵌套深度，结果不使用域名根路径，无需清理。
test("资源前缀兼容根页面、英文页和嵌套文档", () => {
  assert.equal(rootPrefix("index.html"), "./");
  assert.equal(rootPrefix("en/index.html"), "../");
  assert.equal(rootPrefix("docs/development/routing.html"), "../../");
});

// 验证标题锚点与 GitHub 风格保持一致；前置含中英文和标点，输出可用于页内导航，无需清理。
test("标题锚点保留中英文并移除标点", () => {
  assert.equal(headingSlug("企业 AI 能力，一处管理"), "企业-ai-能力一处管理");
  assert.equal(headingSlug("Why XHub?"), "why-xhub");
  assert.equal(headingSlug(""), "");
});

// 验证属性与正文转义；前置为可注入的字符和空值，结果不能直接成为标签，无需清理。
test("HTML 转义覆盖标签与引号", () => {
  assert.equal(escapeHTML('<script a="x">&'), "&lt;script a=&quot;x&quot;&gt;&amp;");
  assert.equal(escapeHTML(null), "");
});

const documents = [{ title: "预算管理", text: "设置团队预算与密钥额度", url: "a" }, { title: "运行限制", text: "预算不预占，Docker 部署", url: "b" }];

// 验证全文搜索和标题优先；前置索引包含中文正文，返回实际命中文档顺序，无需清理。
test("全文搜索优先标题并支持中文和大小写", () => {
  assert.deepEqual(searchDocuments(documents, "预算").map((item) => item.url), ["a", "b"]);
  assert.equal(searchDocuments(documents, "docker")[0].url, "b");
});

// 验证关键词交集、空查询和数量边界；不依赖网络，返回可预测结果，无需清理。
test("搜索多个词取交集并处理空查询与非法参数", () => {
  assert.equal(searchDocuments(documents, "预算 密钥").length, 1);
  assert.equal(searchDocuments(documents, "不存在").length, 0);
  assert.equal(searchDocuments(documents, "  ").length, 0);
  assert.equal(searchDocuments(null, "预算").length, 0);
  assert.equal(searchDocuments(documents, "预算", 0).length, 0);
  assert.equal(searchDocuments(documents, "预算", 1).length, 1);
});

// 验证损坏索引不会中断纯搜索函数；前置为缺失字段条目，结果仅保留合法记录，无需清理。
test("搜索忽略缺少正文的条目", () => {
  assert.equal(searchDocuments([{ title: "预算" }, ...documents], "预算").length, 2);
});

// 验证正文片段靠近命中且长度受控；前置为长文本与空输入，不修改索引，无需清理。
test("搜索摘要定位命中并处理非法长度", () => {
  assert.match(searchExcerpt("a".repeat(100) + "预算" + "b".repeat(100), "预算", 60), /预算/);
  assert.equal(searchExcerpt("", "", 40), "");
  assert.equal(searchExcerpt("abc", "a", -1), "");
});

const mapping = new Map([["docs/getting-started.md", "getting-started.md"], ["docs/development/routing.md", "development/routing.md"]]);

// 验证 Markdown 相对链接指向真实站内页面和图片；前置映射已登记，无需网络或文件清理。
test("Markdown 链接重写保留锚点与相对目录", () => {
  assert.equal(rewriteLink("../getting-started.md#docker", "docs/development/routing.md", "docs/development/routing.html", mapping), "../getting-started.html#docker");
  assert.equal(rewriteLink("../assets/console-models.png", "docs/development/routing.md", "docs/development/routing.html", mapping), "../../assets/console-models.png");
  assert.equal(rewriteLink("#01-启动、配置与健康检查", "docs/a.md", "docs/a.html", mapping), "#01-启动配置与健康检查");
  assert.equal(rewriteLink("../README.zh-CN.md", "docs/README.zh-CN.md", "docs/README.zh-CN.html", mapping), "../index.html");
});

// 验证外链、源码与危险协议处理；前置有脚本及非法转义，危险地址返回空串，无需清理。
test("链接拒绝脚本和畸形编码，源码指向 GitHub", () => {
  assert.equal(rewriteLink("javascript:alert(1)", "docs/a.md", "docs/a.html", mapping), "");
  assert.equal(rewriteLink("%zz.md", "docs/a.md", "docs/a.html", mapping), "");
  assert.equal(rewriteLink("https://example.com", "docs/a.md", "docs/a.html", mapping), "https://example.com");
  assert.match(rewriteLink("../go.mod", "docs/a.md", "docs/a.html", mapping), /github.com.*blob\/main\/go.mod/);
});

// 验证标题、表格、代码复制和重复锚点生成；前置为 GFM 内容，返回完整文档结构，无需清理。
test("Markdown 渲染表格、代码及唯一标题锚点", () => {
  const result = renderMarkdown("# 示例\n## 设置\n## 设置\n\n|a|b|\n|--|--|\n|1|2|\n\n```bash\ndocker compose ps\n```", "docs/example.md", "example.md", mapping);
  assert.equal(result.title, "示例");
  assert.deepEqual(result.headings.map((heading) => heading.id), ["示例", "设置", "设置-1"]);
  assert.match(result.html, /<table>/);
  assert.match(result.html, /class="copy-code"/);
});

// 验证 Markdown 原始 HTML 不能注入脚本；前置为恶意标签、事件和 URI，消毒后均不可执行，无需清理。
test("Markdown 消毒移除脚本、事件与危险链接", () => {
  const result = renderMarkdown('# 安全\n<script>alert(1)</script><img src="x" onerror="alert(1)">\n\n[恶意](javascript:alert(1))', "docs/a.md", "a.md", mapping);
  assert.doesNotMatch(result.html, /<script|onerror|href="javascript:/);
});

// 验证 Mermaid 保留安全源码并单独标记；前置为架构图，运行时再加载本地模块，无需清理。
test("Markdown 图表以安全 Mermaid 源码输出", () => {
  const result = renderMarkdown("# 架构\n```mermaid\nflowchart LR\n A-->B\n```", "docs/a.md", "a.md", mapping);
  assert.match(result.html, /class="mermaid"/);
  assert.match(result.html, /A--&gt;B/);
  assert.throws(() => renderMarkdown(null, "docs/a.md", "a.md", mapping));
});

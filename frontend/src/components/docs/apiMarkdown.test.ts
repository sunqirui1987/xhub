import { expect, it } from "vitest";
import { readApiMarkdown } from "./apiMarkdown";
import { docPages } from "./catalog";

/** 目的：验证每个 API 独立双语文件和统一结构；前置仓库真实 Markdown，全部文章有参数、示例与边界，无写入无需清理。 */
it("loads every API from its bilingual Markdown contract", () => {
  for (const page of docPages.filter((item) => item.group === "api")) {
    const document = readApiMarkdown(page.id);
    expect(document.en).toContain("## Request parameters");
    for (const content of Object.values(document)) expect(content).not.toContain("YOUR_GATEWAY_HOST");
    expect(document.en).toContain("## Capabilities and billing boundaries");
    expect(document["zh-CN"]).toContain("## 请求参数");
    expect(document["zh-CN"]).toContain("## 成功响应");
  }
});
/** 目的：非法或缺失文章不能读取任意路径或降级；前置非法输入，期望明确抛错，不写磁盘。 */
it("rejects traversal and missing documents", () => {
  expect(() => readApiMarkdown("../README")).toThrow("Invalid API document ID");
  expect(() => readApiMarkdown("missing-api-document")).toThrow();
});

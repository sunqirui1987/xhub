import { test } from "node:test";
import assert from "node:assert/strict";
import { consoleDocuments } from "../src/console-docs.mjs";
/** 目的：双端同源文章、帮助、四分类与英文示例；前置真实仓库源，静态生成完整两语言且路径唯一，不写数据库，无需清理。 */
test("console documents synchronize help, categories and API contracts", async () => {
  const {documents, groups} = await consoleDocuments(new URL("../../", import.meta.url).pathname);
  assert.equal(groups.length, 8);
  const native = documents.find((d) => d.lang === "en" && d.source.endsWith("/api/native-bypass.md"));
  assert.equal(native.section, "bypass");
  assert.equal(native.protocol, "bypass");
  assert.ok(documents.filter((d) => d.source.includes("/api/native-")).every((d) => d.section === "bypass"));
  assert.ok(native.markdown.includes("(native-anthropic.md)"));
  assert.ok(documents.some((d) => d.source.endsWith("/api/native-vertex.md")));
  assert.equal(new Set(documents.map((d) => d.source)).size, documents.length);
  for (const lang of ["zh", "en"]) {
    const help = documents.filter((d) => d.lang === lang && d.source.includes("/help/"));
    assert.equal(help.length, 8);
    assert.ok(help.every((d) => !d.markdown.includes("undefined")));
    assert.match(documents.find((d) => d.lang === lang && d.source.endsWith("/api/models.md")).markdown, /data\[\]\.id/);
    assert.ok(groups.filter((g) => g.lang === lang).every((g) => g.files.length > 0));
    for (const document of documents.filter((d) => d.lang === lang && d.group === "api")) {
      assert.ok(document.repositorySource.startsWith("docs/api/"));
      assert.ok(document.markdown.includes(lang === "en" ? "## Request parameters" : "## 请求参数"));
      assert.ok(document.markdown.includes(lang === "en" ? "## Success response" : "## 成功响应"));
      assert.ok(!document.markdown.includes("YOUR_GATEWAY_HOST"));
    }
  }
});
/** 目的：失败输入不悄悄降级成旧文档；前置不存在的仓库路径，构建明确失败，不产生临时文件。 */
test("missing source fails synchronization", async () => {
  await assert.rejects(consoleDocuments("/does-not-exist-xhub"));
});

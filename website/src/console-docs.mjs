import { readFile } from "node:fs/promises";
import { build } from "esbuild";
/** 从控制台共享目录和双语正文生成静态文章；参数为仓库根，返回文章与四分类，在内存打包，不修改源文件，错误交给构建器。 */
export async function consoleDocuments(root) {
  const entry = [
    'import { docPages, docSection, docProtocol } from "' + root + '/frontend/src/components/docs/catalog.ts";',
    'import { docsEn } from "' + root + '/frontend/src/i18n/messages/docs.en.ts";',
    'import { docsZhCN } from "' + root + '/frontend/src/i18n/messages/docs.zh-CN.ts";',
    'import { apiSpecs } from "' + root + '/frontend/src/components/docs/apiSpecs.ts";',
    'export { docPages, docSection, docProtocol, docsEn, docsZhCN, apiSpecs };',
  ].join("\n");
  const bundled = await build({ stdin: { contents: entry, resolveDir: root }, bundle: true, write: false, platform: "node", format: "esm" });
  const data = await import("data:text/javascript;base64," + Buffer.from(bundled.outputFiles[0].text).toString("base64"));
  const documents = [];
  const groups = [];
  for (const [lang, translations] of [["zh", data.docsZhCN], ["en", data.docsEn]]) {
    for (const page of data.docPages) {
      const article = translations.articles[page.id];
      const source = "center/" + lang + "/" + (page.path || "index") + ".md";
      let markdown = "# " + article.title + "\n\n" + article.description;
      for (let i = 1; i <= 3; i++) if (article["heading" + i]) markdown += "\n\n## " + article["heading" + i] + "\n\n" + article["body" + i];
      if (article.detail) markdown += "\n\n" + article.detail;
      const spec = data.apiSpecs.find((item) => item.id === page.id);
      if (spec) {
        markdown += "\n\n## " + translations.parameters + "\n\n" + spec.method + " " + spec.endpoint + "\n\n| " + translations.field + " | " + translations.type + " | " + translations.description + " |\n| --- | --- | --- |";
        for (const [i, field] of spec.fields.entries()) markdown += "\n| " + field.name + " | " + field.type + " | " + (article.fields?.[i] || "") + " |";
        markdown += "\n\n## " + translations.response + "\n\n| " + translations.field + " | " + translations.type + " |\n| --- | --- |";
        for (const field of spec.responseFields) markdown += "\n| " + field.name + " | " + field.type + " |";
      }
      if (page.code) markdown += "\n\n## " + translations.example + "\n\n~~~\n" + page.code.replaceAll("https://YOUR_GATEWAY_HOST", "http://localhost:4000") + "\n~~~";
      if (page.group === "api") {
        markdown = await readFile(root + "/docs/api/" + (lang === "zh" ? "zh-CN" : "en") + "/" + page.id + ".md", "utf8");
        // 控制台使用无扩展名路由，静态文档映射使用 Markdown 路径；只转换已确认的同级接口链接。
        for (const target of data.docPages.filter((item) => item.group === "api")) {
          const slug = target.path.split("/").at(-1);
          markdown = markdown.replaceAll("(" + slug + ")", "(" + slug + ".md)");
        }
      }
      documents.push({ source, lang, markdown, group: page.group, section: data.docSection(page.id), sectionTitle: translations.sections[data.docSection(page.id)], protocol: data.docProtocol(page.id), protocolTitle: translations.protocols[data.docProtocol(page.id)], repositorySource: page.group === "api" ? "docs/api/" + (lang === "zh" ? "zh-CN" : "en") + "/" + page.id + ".md" : "frontend/src/i18n/messages/docs" + (page.id.startsWith("help-") ? "-help" : article.detail ? "-expanded" : "") + (lang === "zh" ? ".zh-CN.ts" : ".en.ts") });
    }
    for (const group of ["product", "tools", "billing", "api"]) groups.push({ lang, group, title: translations.groups[group], files: documents.filter((d) => d.lang === lang && d.group === group).map((d) => d.source) });
  }
  return { documents, groups };
}

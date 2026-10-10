import { consoleDocuments } from "./console-docs.mjs";
import { cp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { build as bundle } from "esbuild";
import { documentPath } from "./core.mjs";
import { renderMarkdown } from "./markdown.mjs";
import { documentPage, homepage } from "./templates.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "../..");
const output = path.resolve(here, "../dist");

/** 将生成内容写入输出目录；参数为相对路径和内容，自动创建父目录；构建流程使用，错误向调用方传播。 */
async function emit(relative, content) {
  const target = path.join(output, relative);
  await mkdir(path.dirname(target), { recursive: true });
  await writeFile(target, content);
}

// 仅清理网站专用输出目录，避免触及网关或控制台构建产物。
await rm(output, { recursive: true, force: true });
await mkdir(output, { recursive: true });
// 仅发布纳入版本管理的正式文档，不收集其他并行任务产生的临时文件。
const sources = execFileSync("git", ["ls-files", "-z", "docs", "internal", "cmd/regression"], { cwd: root, encoding: "utf8" }).split("\0").filter((source) => /\.md$/i.test(source) && !/\/(?:testdata|assets|model-marketplace-proof)\/|\/(?:AGENTS|CLAUDE)\.md$|project-review|verification|validation-report/.test(source)).sort();
const mappings = new Map(sources.map((source) => [source, source.startsWith("docs/") ? source.slice(5) : `modules/${source}`]));
const documents = [];
for (const source of sources) {
  const websiteSource = mappings.get(source);
  const markdown = await readFile(path.join(root, source), "utf8");
  const rendered = renderMarkdown(markdown, source, websiteSource, mappings);
  const lang = source.startsWith("docs/development/") || /zh-CN|_cn\.md$/.test(source) ? "zh" : "en";
  documents.push({ source: websiteSource, repositorySource: source, lang, ...rendered });
}
const synced = await consoleDocuments(root);
for (const document of synced.documents) {
  const rendered = renderMarkdown(document.markdown, document.repositorySource, document.source, mappings);
  documents.push({ ...document, ...rendered });
}
for (const document of documents) {
  const alternateCandidates = document.source.includes("zh-CN") ? [document.source.replace(".zh-CN", "")] : document.source.endsWith("_cn.md") ? [document.source.replace("_cn.md", ".md")] : [document.source.replace(/\.md$/, ".zh-CN.md"), document.source.replace(/\.md$/, "_cn.md")];
  const alternate = alternateCandidates.find((candidate) => documents.some((item) => item.source === candidate)) || (document.lang === "zh" ? "README.md" : "README.zh-CN.md");
  await emit(documentPath(document.source), documentPage({ ...document, documents, alternate: document.source.startsWith("center/") ? document.source.replace("/" + document.lang + "/", document.lang === "zh" ? "/en/" : "/zh/") : alternate, centerGroups: synced.groups }));
}
await emit("docs/index.html", '<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta http-equiv="refresh" content="0;url=./center/zh/index.html"><title>XHub 文档中心</title><a href="./center/zh/index.html">XHub 文档中心 / Documentation</a></html>');
await emit("index.html", homepage("zh"));
await emit("en/index.html", homepage("en"));
await emit("search-index.json", JSON.stringify(documents.map(({ source, title, text, lang }) => ({ url: documentPath(source), title, text, lang }))));
await emit("404.html", `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>页面未找到 · XHub</title><body><h1>页面未找到 / Page not found</h1><p>请从站点首页重新打开文档。 / Open documentation from the site home.</p><a href="https://sunqirui1987.github.io/xhub/">XHub 官网 / Website</a></body></html>`);
await emit(".nojekyll", "");
await emit("assets/favicon.svg", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48"><rect width="48" height="48" rx="13" fill="#146b50"/><path d="M14 13h6l4 7 4-7h6L27 24l7 11h-6l-4-7-4 7h-6l7-11z" fill="#fff"/></svg>`);
await cp(path.join(root, "docs/assets"), path.join(output, "assets"), { recursive: true });
await cp(path.join(here, "style.css"), path.join(output, "assets/style.css"));
await bundle({ entryPoints: [path.join(here, "app.mjs")], outdir: path.join(output, "assets"), entryNames: "app", chunkNames: "chunks/[name]-[hash]", bundle: true, splitting: true, format: "esm", minify: true, target: ["es2022"], logLevel: "warning" });
console.log(`Built ${documents.length} documentation pages, 2 homepages, and a local search index in website/dist.`);

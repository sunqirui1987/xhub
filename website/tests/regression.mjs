import { readFile, readdir, stat } from "node:fs/promises";
import path from "node:path";

/** 检查构建产物的全部站内链接和锚点；参数为输出目录，返回统计与错误数组；不连接后台、不更改生成内容。 */
export async function checkSite(directory) {
  const files = [];
  const errors = [];
  let links = 0;
  /** 遍历专用输出目录；参数为当前路径，追加 HTML 文件列表，错误交由外层报告，无副作用。 */
  async function walk(current) {
    for (const entry of await readdir(current, { withFileTypes: true })) {
      const file = path.join(current, entry.name);
      if (entry.isDirectory()) await walk(file);
      else if (entry.name.endsWith(".html")) files.push(file);
    }
  }
  await walk(directory);
  const contents = new Map(await Promise.all(files.map(async (file) => [file, await readFile(file, "utf8")])));
  for (const [file, html] of contents) {
    for (const [, attribute, value] of html.matchAll(/(href|src)="([^"]*)"/g)) {
      if (!value || /^(https?:|mailto:|data:)/.test(value)) continue;
      links++;
      try {
        const url = new URL(value.replaceAll("&amp;", "&"), `http://site.test/${path.relative(directory, file)}`);
        if (value.startsWith("/")) throw new Error("绝对资源路径不兼容 GitHub 项目页面");
        const target = path.join(directory, decodeURIComponent(url.pathname));
        await stat(target);
        if (attribute === "href" && url.hash && contents.has(target)) {
          const id = decodeURIComponent(url.hash.slice(1));
          const ids = [...contents.get(target).matchAll(/id="([^"]+)"/g)].map((match) => match[1]);
          if (!ids.includes(id)) throw new Error(`锚点不存在：${id}`);
        }
      } catch (error) { errors.push(`${path.relative(directory, file)} → ${value}: ${error.message}`); }
    }
  }
  const index = JSON.parse(await readFile(path.join(directory, "search-index.json"), "utf8"));
  for (const document of index) if (!contents.has(path.join(directory, document.url))) errors.push(`搜索结果页面不存在：${document.url}`);
  return { pages: files.length, links, indexedDocuments: index.length, errors };
}

import path from "node:path";
import { Marked } from "marked";
import sanitizeHtml from "sanitize-html";
import { documentPath, escapeHTML as e, headingSlug } from "./core.mjs";
import { repository } from "./content.mjs";

/** 重写仓库相对链接为站内文档或源码地址；参数含源文件、页面路径与文档映射，返回安全地址，未知路径保留源码链接。 */
export function rewriteLink(href, source, output, mappings) {
  if (typeof href !== "string") return "";
  if (/^(?:https?:|mailto:)/i.test(href)) return href;
  // 旧文档的手写锚点含中文标点，统一为与渲染标题一致的锚点，保证页内目录可达。
  if (href.startsWith("#")) {
    try { return `#${headingSlug(decodeURIComponent(href.slice(1)))}`; } catch { return ""; }
  }
  if (/^[a-z][a-z\d+.-]*:/i.test(href) || href.startsWith("//")) return "";
  const [file, fragment = ""] = href.split("#");
  let decoded;
  try { decoded = decodeURIComponent(file); } catch { return ""; }
  const target = path.posix.normalize(path.posix.join(path.posix.dirname(source), decoded));
  let destination;
  if (mappings.has(target)) destination = documentPath(mappings.get(target));
  else if (target === "README.zh-CN.md") destination = "index.html";
  else if (target === "README.md") destination = "en/index.html";
  else if (target.startsWith("docs/assets/")) destination = `assets/${target.slice(12)}`;
  else if (target === "docs" || target === "docs/") destination = documentPath("README.zh-CN.md");
  if (destination) {
    let anchor = "";
    try { anchor = fragment ? `#${headingSlug(decodeURIComponent(fragment))}` : ""; } catch { return ""; }
    return `${path.posix.relative(path.posix.dirname(output), destination)}${anchor}`;
  }
  return `${repository}/blob/main/${target.split("/").map(encodeURIComponent).join("/")}${fragment ? `#${fragment}` : ""}`;
}

/** 渲染并消毒仓库 Markdown；参数包含路径映射，返回正文、标题和纯文本索引；原始 HTML 不允许脚本或事件属性。 */
export function renderMarkdown(markdown, source, websiteSource, mappings) {
  if (typeof markdown !== "string") throw new TypeError("Markdown 必须为字符串");
  const output = documentPath(websiteSource);
  const headings = [];
  const counts = new Map();
  const parser = new Marked({ gfm: true, breaks: false });
  parser.use({ renderer: {
    heading({ tokens, depth }) {
      const inner = this.parser.parseInline(tokens);
      const text = sanitizeHtml(inner, { allowedTags: [], allowedAttributes: {} });
      const slug = headingSlug(text);
      const count = counts.get(slug) || 0;
      counts.set(slug, count + 1);
      const id = count ? `${slug}-${count}` : slug;
      headings.push({ text, depth, id });
      return `<h${depth} id="${e(id)}">${inner}</h${depth}>`;
    },
    link({ href, title, tokens }) {
      const destination = rewriteLink(href, source, output, mappings);
      return `<a href="${e(destination)}"${title ? ` title="${e(title)}"` : ""}>${this.parser.parseInline(tokens)}</a>`;
    },
    image({ href, title, text }) {
      return `<img src="${e(rewriteLink(href, source, output, mappings))}" alt="${e(text)}"${title ? ` title="${e(title)}"` : ""} loading="lazy">`;
    },
    code({ text, lang }) {
      if (lang === "mermaid") return `<pre class="mermaid">${e(text)}</pre>`;
      return `<div class="doc-code"><button class="copy-code" aria-label="${/zh-CN|_cn|development/.test(source) ? "复制代码" : "Copy code"}">${/zh-CN|_cn|development/.test(source) ? "复制代码" : "Copy code"}</button><pre><code class="language-${e((lang || "text").split(" ")[0])}">${e(text)}</code></pre></div>`;
    },
  } });
  const html = sanitizeHtml(parser.parse(markdown), {
    allowedTags: [...sanitizeHtml.defaults.allowedTags, "img", "button", "pre", "code", "input", "del", "details", "summary"],
    allowedAttributes: { ...sanitizeHtml.defaults.allowedAttributes, "*": ["id", "class", "aria-label"], a: ["href", "title"], img: ["src", "alt", "title", "loading", "width", "height"], input: ["type", "checked", "disabled"] },
    allowedSchemes: ["http", "https", "mailto"],
  });
  return { html, headings, title: headings.find((item) => item.depth === 1)?.text || websiteSource, text: sanitizeHtml(html, { allowedTags: [], allowedAttributes: {} }).replace(/\s+/g, " ").trim() };
}

import path from "node:path";

/** 将 Markdown 源路径映射为静态 HTML；供构建器调用，非法路径抛错，不读写文件。 */
export function documentPath(source) {
  if (typeof source !== "string" || !source.endsWith(".md") || source.startsWith("/") || source.split("/").includes("..")) throw new Error("非法文档路径");
  return `docs/${source.replace(/\.md$/i, ".html")}`;
}

/** 计算页面到静态资源根目录的相对地址；参数为输出路径，返回带斜线前缀，不依赖部署子路径。 */
export function rootPrefix(output) {
  return `${path.posix.relative(path.posix.dirname(output), ".") || "."}/`;
}

/** 生成 GitHub 风格标题锚点；供 Markdown 渲染器调用，空标题返回空串，不维护重复计数。 */
export function headingSlug(text) {
  return text.toLowerCase().replace(/<[^>]*>/g, "").replace(/[^\p{L}\p{N}\p{M} _-]/gu, "").replace(/ /g, "-");
}

/** 转义插入 HTML 的文本和属性；供模板使用，空值转为空串，无 DOM 副作用。 */
export function escapeHTML(value) {
  return String(value ?? "").replace(/[&<>"']/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "\"": "&quot;", "'": "&#39;" })[character]);
}

/** 按全部关键词搜索静态索引；供浏览器与单元测试调用，返回排序后的有限结果，非法输入返回空数组。 */
export function searchDocuments(documents, query, limit = 12) {
  if (!Array.isArray(documents) || typeof query !== "string" || !Number.isInteger(limit) || limit <= 0) return [];
  const words = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return [];
  return documents.filter((doc) => typeof doc.title === "string" && typeof doc.text === "string")
    .map((doc) => {
      const title = doc.title.toLocaleLowerCase();
      const body = doc.text.toLocaleLowerCase();
      return { ...doc, score: words.every((word) => `${title} ${body}`.includes(word)) ? words.reduce((score, word) => score + (title.includes(word) ? 10 : 1), 0) : 0 };
    }).filter((doc) => doc.score > 0).sort((a, b) => b.score - a.score || a.title.localeCompare(b.title)).slice(0, limit);
}

/** 获取命中附近的正文片段；参数为正文、查询和最大长度，供搜索结果展示使用，不解释 HTML。 */
export function searchExcerpt(text, query, length = 140) {
  if (typeof text !== "string" || typeof query !== "string" || !Number.isInteger(length) || length <= 0) return "";
  const needle = query.trim().split(/\s+/)[0]?.toLocaleLowerCase() || "";
  const position = needle ? text.toLocaleLowerCase().indexOf(needle) : 0;
  const start = Math.max(0, position - 35);
  return `${start ? "…" : ""}${text.slice(start, start + length)}${text.length > start + length ? "…" : ""}`;
}

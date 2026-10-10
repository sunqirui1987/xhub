import { searchDocuments, searchExcerpt } from "./search.mjs";
import { copy } from "./content.mjs";
import { handleSearchKey } from "./keyboard.mjs";

const lang = document.body.dataset.language || "zh";
const t = copy[lang];
const prefix = document.body.dataset.root;
const dialog = document.querySelector(".search-dialog");
const input = document.querySelector("#doc-search");
const status = document.querySelector(".search-status");
const results = document.querySelector(".search-results");
let documents;
let loading;
let searchRevision = 0;
let toastTimer;

/** 显示可访问的操作反馈；参数为文本，返回空，复制交互调用，计时器会替换上次提示。 */
function showToast(text) {
  const toast = document.querySelector(".toast");
  toast.textContent = text;
  toast.classList.add("visible");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toast.classList.remove("visible"), 2500);
}

/** 根据当前输入加载并搜索本地索引；无参数和返回值，输入事件调用；加载失败展示重试提示，避免旧查询覆盖新结果。 */
async function updateSearch() {
  const revision = ++searchRevision;
  results.replaceChildren();
  if (!input.value.trim()) { status.textContent = t.searchHint; return; }
  status.textContent = t.searching;
  try {
    if (!documents) {
      loading ||= fetch(`${prefix}search-index.json`).then((response) => {
        if (!response.ok) throw new Error("索引请求失败");
        return response.json();
      }).then((data) => {
        if (!Array.isArray(data) || data.some((entry) => typeof entry.title !== "string" || typeof entry.text !== "string" || !/^docs\/[a-zA-Z0-9_./-]+\.html$/.test(entry.url) || entry.url.includes(".."))) throw new Error("索引结构错误");
        documents = data;
      }).finally(() => { loading = undefined; });
      await loading;
    }
    if (revision !== searchRevision) return;
    const matches = searchDocuments(documents, input.value);
    status.textContent = matches.length ? `${matches.length} ${lang === "zh" ? "条结果" : "results"}` : t.noResults;
    for (const match of matches) {
      const link = document.createElement("a");
      link.className = "search-result";
      link.href = `${prefix}${match.url}`;
      const title = document.createElement("strong");
      title.textContent = match.title;
      const snippet = document.createElement("span");
      snippet.textContent = searchExcerpt(match.text, input.value);
      link.append(title, snippet);
      results.append(link);
    }
  } catch { if (revision === searchRevision) status.textContent = t.searchError; }
}

/** 打开原生搜索对话框并聚焦输入；无参数和返回值，由按钮与快捷键触发，浏览器负责焦点恢复。 */
function openSearch() {
  if (!dialog.open) dialog.showModal();
  input.focus();
}

document.querySelectorAll(".search-open").forEach((button) => button.addEventListener("click", openSearch));
input.addEventListener("input", updateSearch);
document.addEventListener("keydown", (event) => {
  handleSearchKey(event, document.activeElement, dialog, openSearch);
});
document.querySelector(".menu-button").addEventListener("click", (event) => {
  const button = event.currentTarget;
  const expanded = button.getAttribute("aria-expanded") !== "true";
  button.setAttribute("aria-expanded", String(expanded));
  document.querySelector("#main-navigation").classList.toggle("expanded", expanded);
});
document.querySelectorAll(".copy-code").forEach((button) => button.addEventListener("click", async () => {
  const code = button.parentElement.closest(".code-panel, .doc-code").querySelector("pre code").textContent;
  try { await navigator.clipboard.writeText(code); showToast(t.copied); } catch { showToast(t.copyFailed); }
}));

// 仅有架构图的页面加载 Mermaid，本地打包资源不依赖 CDN；解析失败仍保留可读源码。
if (document.querySelector(".mermaid")) {
  import("mermaid").then(async ({ default: mermaid }) => {
    mermaid.initialize({ startOnLoad: false, securityLevel: "strict", theme: "neutral", fontFamily: "system-ui" });
    await mermaid.run({ querySelector: ".mermaid", suppressErrors: true });
  }).catch(() => {});
}

"use client";
import { Children, isValidElement, useState, type ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { vscDarkPlus } from "react-syntax-highlighter/dist/esm/styles/prism";
import { Copy, WrapText } from "lucide-react";
import { useI18n } from "@/i18n";

/** 生成 Markdown 标题锚点；参数为标题，返回稳定 ID，正文和页内目录共同调用，无副作用。 */
export function documentationHeadingId(title: string): string {
  return "doc-" + title.trim().toLowerCase().replace(/\s+/g, "-");
}
/** 提取正文二级目录；参数为 Markdown，返回标题和锚点，忽略代码围栏内标题，不修改输入。 */
export function documentationHeadings(markdown: string): { title: string; id: string }[] {
  let fence = "";
  const headings: { title: string; id: string }[] = [];
  for (const line of markdown.split("\n")) {
    const marker = line.match(/^\s*([\x60]{3,}|~{3,})/);
    if (marker) {
      fence = fence ? "" : marker[1][0];
      continue;
    }
    if (!fence && line.startsWith("## ")) {
      const title = line.slice(3).trim();
      headings.push({ title, id: documentationHeadingId(title) });
    }
  }
  return headings;
}
/** 提取 React Markdown 原文；参数为节点，返回文本，供复制和标题定位使用，不执行 HTML 或修改输入。 */
function nodeText(node: ReactNode): string {
  return Children.toArray(node)
    .map((child) => (isValidElement<{ children?: ReactNode }>(child) ? nodeText(child.props.children) : String(child)))
    .join("");
}
/** 代码面板；参数为代码及围栏语言，返回高亮、换行和复制控件。复制保留原文，失败可见，不发送请求或存储凭据。 */
export function DocumentationCode({ code, language }: { code: string; language: string }) {
  const { t } = useI18n();
  const [wrapped, setWrapped] = useState(false);
  const [state, setState] = useState<"copyCode" | "copied" | "copyFailed">("copyCode");
  /** 主动复制面板原文；无参数，返回完成 Promise，剪贴板不可用时显示失败，不抛未处理异常。 */
  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      setState("copied");
    } catch {
      setState("copyFailed");
    }
  }
  return (
    <div
      className="my-5 min-w-0 overflow-hidden rounded-xl border border-zinc-700 bg-[#1e1e1e] text-zinc-100"
      role="region"
      aria-label={t("docs.codePanel", { language })}
    >
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-zinc-700 bg-zinc-900 px-4 py-2 text-xs">
        <span className="font-mono uppercase text-zinc-400">{language}</span>
        <div className="flex items-center gap-3">
          <button
            type="button"
            aria-pressed={wrapped}
            onClick={() => setWrapped(!wrapped)}
            className="flex items-center gap-1 rounded px-2 py-1 hover:bg-zinc-700"
          >
            <WrapText size={14} />
            {t("docs.wrapCode")}
          </button>
          <button
            type="button"
            onClick={copy}
            aria-label={t("docs.copyCode")}
            className="flex items-center gap-1 rounded px-2 py-1 hover:bg-zinc-700"
          >
            <Copy size={14} />
            {t("docs." + state)}
          </button>
        </div>
      </div>
      <div className="max-h-[560px] overflow-auto text-xs sm:text-sm">
        <SyntaxHighlighter
          language={language === "shell" ? "bash" : language}
          style={vscDarkPlus}
          wrapLongLines={wrapped}
          customStyle={{ margin: 0, padding: "1.25rem", background: "transparent", fontSize: "inherit" }}
          codeTagProps={{ style: { fontFamily: "var(--font-mono, monospace)" } }}
        >
          {code}
        </SyntaxHighlighter>
      </div>
      {state !== "copyCode" && (
        <div role="status" className="border-t border-zinc-700 px-4 py-2 text-xs">
          {t("docs." + state)}
        </div>
      )}
    </div>
  );
}
/** Markdown 阅读区域；参数为当前语言正文，返回代码面板与标题锚点，行内代码保留，不渲染原始 HTML。 */
export function DocumentationMarkdown({ markdown }: { markdown: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        h2: ({ children }) => (
          <h2 id={documentationHeadingId(nodeText(children))} className="scroll-mt-40">
            {children}
          </h2>
        ),
        pre: ({ children }) => {
          const child = Children.toArray(children)[0];
          const language = isValidElement<{ className?: string }>(child)
            ? child.props.className?.replace("language-", "") || "text"
            : "text";
          return <DocumentationCode language={language} code={nodeText(children).replace(/\n$/, "")} />;
        },
      }}
    >
      {markdown}
    </ReactMarkdown>
  );
}

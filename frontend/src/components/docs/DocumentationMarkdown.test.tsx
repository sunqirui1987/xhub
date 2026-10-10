import { fireEvent, render, screen, within } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { I18nProvider } from "@/i18n/I18nProvider";
import { DocumentationMarkdown, documentationHeadings } from "./DocumentationMarkdown";

/** 目的：验证双语面板、复制原文、失败反馈和换行；前置真实 Markdown，保留英文示例和用户文本；仅操作剪贴板边界，DOM 由框架清理。 */
it.each([
  ["en", "Copy code", "Wrap lines", "Copied"],
  ["zh-CN", "复制代码", "自动换行", "已复制"],
] as const)("copies and wraps fenced code in %s", async (locale, copy, wrap, copied) => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
  render(
    <I18nProvider initialLocale={locale}>
      <DocumentationMarkdown
        markdown={
          '## Request example\n\n~~~shell\ncurl "$XHUB_BASE_URL/models"\n~~~\n\n~~~json\n{"content":"用户原文"}\n~~~'
        }
      />
    </I18nProvider>,
  );
  const panel = screen.getByRole("region", { name: locale === "en" ? "shell code panel" : "shell 代码面板" });
  fireEvent.click(within(panel).getByRole("button", { name: copy }));
  expect(await within(panel).findByRole("status")).toHaveTextContent(copied);
  expect(writeText).toHaveBeenCalledWith('curl "$XHUB_BASE_URL/models"');
  fireEvent.click(within(panel).getByRole("button", { name: wrap }));
  expect(within(panel).getByRole("button", { name: wrap })).toHaveAttribute("aria-pressed", "true");
  writeText.mockRejectedValueOnce(new Error("denied"));
  fireEvent.click(within(panel).getByRole("button", { name: copy }));
  expect(await within(panel).findByRole("status")).toHaveTextContent(locale === "en" ? "Copy failed" : "复制失败");
  expect(screen.getByText('"用户原文"')).toBeInTheDocument();
});

/** 目的：验证目录不解析围栏伪标题和空输入；前置确定性文本，返回真实锚点，无数据写入或清理。 */
it("indexes real headings outside code fences", () => {
  expect(documentationHeadings("")).toEqual([]);
  expect(documentationHeadings("## Overview\n~~~text\n## Hidden\n~~~\n## 请求参数")).toEqual([
    { title: "Overview", id: "doc-overview" },
    { title: "请求参数", id: "doc-请求参数" },
  ]);
});

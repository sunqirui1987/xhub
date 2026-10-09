import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import SyntaxHighlighter from "./SyntaxHighlighter";

describe("有限语法高亮", () => {
  /** 前置常用语言及别名；验证代码内容可见且生成语法节点，RTL 自动卸载。 */
  it.each(["python", "json", "shell", "typescript"])("显示并高亮 %s", (language) => {
    const { container } = render(<SyntaxHighlighter language={language}>{'print("hello")'}</SyntaxHighlighter>);
    expect(container.querySelector("code")?.textContent).toBe('print("hello")');
    expect(container.querySelector("code span")).toBeTruthy();
  });
  /** 前置未知语言和空代码；验证安全回退和空边界，RTL 自动卸载。 */
  it("未知语言按纯文本渲染，空代码不会报错", () => {
    const { rerender } = render(<SyntaxHighlighter language="unknown-lang">{"<hello>"}</SyntaxHighlighter>);
    expect(screen.getByText("<hello>")).toBeVisible();
    rerender(<SyntaxHighlighter language="python">{""}</SyntaxHighlighter>);
  });
});

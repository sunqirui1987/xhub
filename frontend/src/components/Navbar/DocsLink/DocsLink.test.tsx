import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { I18nProvider } from "@/i18n/I18nProvider";
import { DocsLink } from "./DocsLink";

describe("DocsLink", () => {
  /** 目的：验证双语同站入口；前置指定语言，结果是可访问名称和 /docs 地址正确；渲染树由测试环境清理，不写业务数据。 */
  it.each([
    ["en", "Documentation"],
    ["zh-CN", "文档中心"],
  ] as const)("links locally in %s", (locale, label) => {
    render(
      <I18nProvider initialLocale={locale}>
        <DocsLink />
      </I18nProvider>,
    );
    expect(screen.getByRole("link", { name: label })).toHaveAttribute("href", "/docs");
  });
});

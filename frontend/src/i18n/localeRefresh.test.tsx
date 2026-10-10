import React, { memo, useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { t as runtimeT } from "./runtime";
import { useT } from "./I18nProvider";
import { I18nProvider } from "./I18nProvider";
import { LocaleBoundary } from "./LocaleBoundary";
import LanguageSwitcher from "@/components/LanguageSwitcher";

const route = vi.hoisted(() => ({ pathname: "/ui/playground" }));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn() }),
  usePathname: () => route.pathname,
}));

/** 模拟 memo 页面容器；参数 children 为页面内容，返回同一子树，无副作用。 */
const Frozen = memo(function Frozen({ children }: { children: React.ReactNode }) {
  return children;
});

/** 模拟调试台标题与草稿；无参数，返回订阅语言的标题和输入框，状态只由用户输入更新。 */
function Title() {
  const t = useT();
  const [draft, setDraft] = useState("");
  return (
    <>
      <h1>{t("login.title")}</h1>
      <input aria-label="draft" value={draft} onChange={(event) => setDraft(event.target.value)} />
    </>
  );
}

describe("locale switch", () => {
  /** 前置真实语言 Provider；验证文档与调试页面 memo 内标题切换和空/非空中文草稿往返保留，DOM 由测试框架清理。 */
  it.each(["/ui/playground", "/docs", "/ui/docs/api/models"])(
    "re-renders subscribed text and preserves drafts on %s",
    async (pathname) => {
      route.pathname = pathname;
      const user = userEvent.setup();
      render(
        <I18nProvider initialLocale="zh-CN">
          <LocaleBoundary>
            <Frozen>
              <Title />
              <LanguageSwitcher />
            </Frozen>
          </LocaleBoundary>
        </I18nProvider>,
      );

      expect(screen.getByRole("heading", { name: "登录" })).toBeInTheDocument();
      expect(screen.getByRole("textbox", { name: "draft" })).toHaveValue("");
      await user.type(screen.getByRole("textbox", { name: "draft" }), "用户的猫");
      await user.click(screen.getByRole("button", { name: "English" }));
      expect(screen.getByRole("heading", { name: "Login" })).toBeInTheDocument();
      expect(screen.getByRole("textbox", { name: "draft" })).toHaveValue("用户的猫");
      await user.click(screen.getByRole("button", { name: "English" }));
      expect(screen.getByRole("textbox", { name: "draft" })).toHaveValue("用户的猫");
      await user.click(screen.getByRole("button", { name: "中文" }));
      expect(screen.getByRole("textbox", { name: "draft" })).toHaveValue("用户的猫");
      expect(screen.getByRole("heading", { name: "登录" })).toBeInTheDocument();
    },
  );
});

/** 模拟旧页面读取运行时翻译；无参数，返回标题，用于验证迁移边界兼容行为。 */
function LegacyTitle() {
  return <h1>{runtimeT("login.title")}</h1>;
}

/** 前置尚未订阅的旧页面；验证未知路由仍切换文案，finally 恢复路径，DOM 自动清理。 */
it("keeps locale refresh compatibility for legacy pages", async () => {
  route.pathname = "/ui/old-usage";
  try {
    render(
      <I18nProvider initialLocale="zh-CN">
        <LocaleBoundary>
          <Frozen>
            <LegacyTitle />
            <LanguageSwitcher />
          </Frozen>
        </LocaleBoundary>
      </I18nProvider>,
    );
    await userEvent.click(screen.getByRole("button", { name: "English" }));
    expect(screen.getByRole("heading", { name: "Login" })).toBeInTheDocument();
  } finally {
    route.pathname = "/ui/playground";
  }
});

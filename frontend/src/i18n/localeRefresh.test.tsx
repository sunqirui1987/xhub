import React, { memo } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { t } from "./runtime";
import { I18nProvider } from "./I18nProvider";
import { LocaleBoundary } from "./LocaleBoundary";
import LanguageSwitcher from "@/components/LanguageSwitcher";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn() }),
}));

const Frozen = memo(function Frozen({ children }: { children: React.ReactNode }) {
  return children;
});

function Title() {
  return <h1>{t("login.title")}</h1>;
}

describe("locale switch", () => {
  it("re-renders text that only reads the module translator when a parent bails out", async () => {
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
    await user.click(screen.getByRole("button", { name: "English" }));
    expect(screen.getByRole("heading", { name: "Login" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "中文" }));
    expect(screen.getByRole("heading", { name: "登录" })).toBeInTheDocument();
  });
});

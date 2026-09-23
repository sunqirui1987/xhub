import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { I18nProvider } from "./I18nProvider";
import LanguageSwitcher from "@/components/LanguageSwitcher";
import { translate } from "./translate";

describe("LanguageSwitcher", () => {
  it("defaults to Chinese chrome and switches the document language to English", async () => {
    const user = userEvent.setup();
    render(
      <I18nProvider initialLocale="zh-CN">
        <LanguageSwitcher />
      </I18nProvider>,
    );

    expect(screen.getByRole("group", { name: translate("zh-CN", "language.label") })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: translate("zh-CN", "language.en") }));
    expect(document.documentElement.lang).toBe("en");
    expect(screen.getByRole("group", { name: translate("en", "language.label") })).toBeInTheDocument();
  });
});

import { fireEvent, render, screen } from "@testing-library/react";
import userEvent, { PointerEventsCheckLevel } from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import PromptCodeSnippets from "./PromptCodeSnippets";

// 语言选项在模块加载时翻译；固定英文以验证触发器显示完整 SDK 标签。
vi.mock("@/i18n", async (importOriginal) => {
  const original = await importOriginal<typeof import("@/i18n")>();
  return { ...original, t: (key: string, vars?: Record<string, unknown>) => original.translate("en", key, vars) };
});

describe("PromptCodeSnippets", () => {
  it("opens generated code for the selected prompt", async () => {
    render(
      <PromptCodeSnippets
        promptId="welcome"
        model="gpt-4o"
        promptVariables={{ name: "Ada" }}
        accessToken="token"
        version="2"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /get code/i }));
    expect(await screen.findByText("Generated Code")).toBeInTheDocument();
    expect(screen.getByText(/welcome/)).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Language" })).toBeInTheDocument();
    expect(screen.getByRole("tablist", { name: "Generated code type" })).toBeInTheDocument();
    expect(screen.getByRole("dialog")).toHaveClass("max-h-[calc(100dvh-2rem)]", "overflow-y-auto");
  });

  it("shows the selected language by its human label on the trigger", async () => {
    const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never });
    render(
      <PromptCodeSnippets
        promptId="welcome"
        model="gpt-4o"
        promptVariables={{ name: "Ada" }}
        accessToken="token"
        version="2"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /get code/i }));

    const trigger = await screen.findByRole("combobox", { name: "Language" });
    expect(trigger).toHaveTextContent("cURL");

    await user.click(trigger);
    const python = await screen.findByRole("option", { name: "Python (OpenAI SDK)" });
    await user.click(python);

    expect(screen.getByRole("combobox", { name: "Language" })).toHaveTextContent("Python (OpenAI SDK)");
  });

  it("includes the viewed environment in every generated request", async () => {
    const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never });
    render(
      <PromptCodeSnippets
        promptId="welcome"
        model="gpt-4o"
        accessToken="token"
        version="2"
        environment="development"
      />,
    );
    await user.click(screen.getByRole("button", { name: /get code/i }));
    await screen.findByText("Generated Code");

    await user.click(screen.getByRole("button", { name: /copy to clipboard/i }));
    expect(await navigator.clipboard.readText()).toContain('"prompt_environment": "development"');

    await user.click(screen.getByRole("tab", { name: "With Version" }));
    await user.click(screen.getByRole("button", { name: /copy to clipboard/i }));
    const versionSnippet = await navigator.clipboard.readText();
    expect(versionSnippet).toContain('"prompt_environment": "development"');
    expect(versionSnippet).toContain('"prompt_version": 2');
  });
});

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import PromptEditorHeader from "./PromptEditorHeader";

// 环境选项在模块加载时翻译；固定英文以验证所选环境的人类可读标签。
vi.mock("@/i18n", async (importOriginal) => {
  const original = await importOriginal<typeof import("@/i18n")>();
  return { ...original, t: (key: string, vars?: Record<string, unknown>) => original.translate("en", key, vars) };
});

vi.mock("./PromptCodeSnippets", () => ({
  default: ({ environment }: { environment?: string }) => <button data-environment={environment}>Get Code</button>,
}));

describe("PromptEditorHeader", () => {
  it("preserves navigation, naming, and save actions", () => {
    const onBack = vi.fn();
    const onSave = vi.fn();
    const onNameChange = vi.fn();
    render(
      <PromptEditorHeader
        promptName="welcome"
        onNameChange={onNameChange}
        onBack={onBack}
        onSave={onSave}
        isSaving={false}
        accessToken="token"
        environment="development"
        onEnvironmentChange={vi.fn()}
      />,
    );
    fireEvent.change(screen.getByDisplayValue("welcome"), { target: { value: "greeting" } });
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onNameChange).toHaveBeenCalledWith("greeting");
    expect(onBack).toHaveBeenCalledOnce();
    expect(onSave).toHaveBeenCalledOnce();
  });

  it.each([
    ["development", "Development"],
    ["staging", "Staging"],
    ["production", "Production"],
  ])("shows the %s environment by its human label", (environment, label) => {
    render(
      <PromptEditorHeader
        promptName="welcome"
        onNameChange={vi.fn()}
        onBack={vi.fn()}
        onSave={vi.fn()}
        isSaving={false}
        accessToken="token"
        environment={environment}
        onEnvironmentChange={vi.fn()}
      />,
    );

    expect(screen.getByRole("combobox", { name: "Environment" })).toHaveTextContent(label);
    expect(screen.getByRole("button", { name: "Get Code" })).toHaveAttribute("data-environment", environment);
  });
});

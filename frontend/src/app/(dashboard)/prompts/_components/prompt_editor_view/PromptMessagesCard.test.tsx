import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import PromptMessagesCard from "./PromptMessagesCard";

// 角色选项在模块加载时翻译；固定英文以验证每种消息角色的可见标签。
vi.mock("@/i18n", async (importOriginal) => {
  const original = await importOriginal<typeof import("@/i18n")>();
  return { ...original, t: (key: string, vars?: Record<string, unknown>) => original.translate("en", key, vars) };
});

vi.mock("../variable_textarea", () => ({
  default: (props: any) => <textarea value={props.value} onChange={(event) => props.onChange(event.target.value)} />,
}));

describe("PromptMessagesCard", () => {
  it("renders messages and adds another message", () => {
    const onAddMessage = vi.fn();
    render(
      <PromptMessagesCard
        messages={[{ role: "user", content: "Hello" }]}
        onAddMessage={onAddMessage}
        onUpdateMessage={vi.fn()}
        onRemoveMessage={vi.fn()}
        onMoveMessage={vi.fn()}
      />,
    );
    expect(screen.getByDisplayValue("Hello")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /add message/i }));
    expect(onAddMessage).toHaveBeenCalledOnce();
  });

  it.each([
    ["user", "User"],
    ["assistant", "Assistant"],
    ["system", "System"],
  ])("shows the %s role by its human label", (role, label) => {
    render(
      <PromptMessagesCard
        messages={[{ role, content: "Hello" }]}
        onAddMessage={vi.fn()}
        onUpdateMessage={vi.fn()}
        onRemoveMessage={vi.fn()}
        onMoveMessage={vi.fn()}
      />,
    );

    expect(screen.getByRole("combobox", { name: "Message 1 role" })).toHaveTextContent(label);
  });
});

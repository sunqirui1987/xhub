import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import TableIconActionButton, { TableIconActionButtonMap } from "./TableIconActionButton";

describe("TableIconActionButton", () => {
  Object.keys(TableIconActionButtonMap).forEach((variant) => {
    it(`should render ${variant} button`, () => {
      render(<TableIconActionButton variant={variant} onClick={() => {}} dataTestId="test-button" />);
      expect(screen.getByTestId("test-button")).toBeInTheDocument();

      expect(screen.getByTestId("test-button")).toHaveClass(TableIconActionButtonMap[variant].className!);
    });
  });

  it("should call onClick when clicked", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(<TableIconActionButton variant="Edit" onClick={onClick} dataTestId="test-button" tooltipText="Edit" />);

    await user.click(screen.getByTestId("test-button"));

    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("should not show the tooltip before the button is hovered", () => {
    render(
      <TableIconActionButton variant="Edit" onClick={() => {}} dataTestId="test-button" tooltipText="Edit item" />,
    );
    expect(screen.queryByText("Edit item")).not.toBeInTheDocument();
  });

  it("should show tooltip when tooltipText is provided", async () => {
    const user = userEvent.setup();
    render(
      <TableIconActionButton variant="Edit" onClick={() => {}} dataTestId="test-button" tooltipText="Edit item" />,
    );

    await user.hover(screen.getByTestId("test-button"));

    expect(await screen.findByText("Edit item")).toBeInTheDocument();
  });

  it("should render disabled state with disabled styling", () => {
    render(
      <TableIconActionButton variant="Edit" onClick={() => {}} dataTestId="test-button" disabled tooltipText="Edit" />,
    );
    const button = screen.getByTestId("test-button");
    expect(button).toHaveClass("opacity-50");
    expect(button).toHaveClass("cursor-not-allowed");
  });

  it("should not call onClick when disabled", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(
      <TableIconActionButton
        variant="Edit"
        onClick={onClick}
        dataTestId="test-button"
        disabled
        tooltipText="Edit"
        disabledTooltipText="Cannot edit"
      />,
    );

    await user.click(screen.getByTestId("test-button"));

    expect(onClick).not.toHaveBeenCalled();
  });

  it("should show disabledTooltipText when disabled and disabledTooltipText is provided", async () => {
    const user = userEvent.setup();
    render(
      <TableIconActionButton
        variant="Edit"
        onClick={() => {}}
        dataTestId="test-button"
        disabled
        tooltipText="Edit"
        disabledTooltipText="Cannot edit"
      />,
    );

    await user.hover(screen.getByTestId("test-button"));

    expect(await screen.findByText("Cannot edit")).toBeInTheDocument();
  });
  /** 验证键盘和中文可访问名称；前置可用编辑按钮，回车触发一次，测试框架清理 DOM。 */
  it("supports keyboard activation with the translated action name", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(<TableIconActionButton variant="Edit" onClick={onClick} tooltipText="编辑成员" />);
    await user.tab();
    expect(screen.getByRole("button", { name: "编辑成员" })).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  /** 验证禁用边界；前置不可编辑按钮，原生 disabled 保留动作名称且鼠标不能触发，框架清理 DOM。 */
  it("exposes a disabled native button without changing its action name", async () => {
    const onClick = vi.fn();
    render(
      <TableIconActionButton
        variant="Edit"
        onClick={onClick}
        tooltipText="Edit member"
        disabled
        disabledTooltipText="No permission"
      />,
    );
    const button = screen.getByRole("button", { name: "Edit member" });
    expect(button).toBeDisabled();
    await userEvent.click(button);
    expect(onClick).not.toHaveBeenCalled();
  });
});

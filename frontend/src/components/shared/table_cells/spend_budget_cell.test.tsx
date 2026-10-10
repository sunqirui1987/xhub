import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { SpendBudgetCell } from "./spend_budget_cell";

const indicator = (container: HTMLElement) => container.querySelector('[data-slot="meter-indicator"]');

describe("SpendBudgetCell", () => {
  /** 验证消费单元格的正常与边界显示；前置本地组件，断言可访问内容，测试框架自动卸载。 */
  it("shows shared parent budget and renders no meter when there is no budget", () => {
    const { container } = render(<SpendBudgetCell spend={0.5} maxBudget={null} />);
    expect(screen.getByText("Shared parent budget")).toBeInTheDocument();
    expect(screen.queryByRole("meter")).not.toBeInTheDocument();
    expect(indicator(container)).toBeNull();
  });

  /** 验证消费单元格的正常与边界显示；前置本地组件，断言可访问内容，测试框架自动卸载。 */
  it("shows $0.00 for zero or undefined spend, never a hyphen", () => {
    const { rerender } = render(<SpendBudgetCell spend={0} maxBudget={100} />);
    expect(screen.getByText("$0.00")).toBeInTheDocument();
    expect(screen.queryByText("-")).not.toBeInTheDocument();
    rerender(<SpendBudgetCell spend={null} maxBudget={null} />);
    expect(screen.getByText("$0.00")).toBeInTheDocument();
    expect(screen.queryByText("-")).not.toBeInTheDocument();
  });

  /** 验证消费单元格的正常与边界显示；前置本地组件，断言可访问内容，测试框架自动卸载。 */
  it("renders a meter carrying the spend and budget when a budget exists", () => {
    render(<SpendBudgetCell spend={25} maxBudget={100} />);
    const meter = screen.getByRole("meter");
    expect(meter).toHaveAttribute("aria-valuenow", "25");
    expect(meter).toHaveAttribute("aria-valuemax", "100");
    expect(screen.getByText("of $100")).toBeInTheDocument();
  });

  /** 验证消费单元格的正常与边界显示；前置本地组件，断言可访问内容，测试框架自动卸载。 */
  it("supports matching spend and budget precision for summary views", () => {
    render(<SpendBudgetCell spend={98.854} maxBudget={3000} spendDecimals={2} budgetDecimals={2} />);

    expect(screen.getByText("$98.85")).toBeInTheDocument();
    expect(screen.getByText("of $3,000.00")).toBeInTheDocument();
    expect(screen.getByRole("meter")).toHaveAttribute("aria-valuetext", "$98.85 of $3,000.00");
  });

  /** 验证消费单元格的正常与边界显示；前置本地组件，断言可访问内容，测试框架自动卸载。 */
  it("keeps the default tone below 80% usage", () => {
    const { container } = render(<SpendBudgetCell spend={50} maxBudget={100} />);
    expect(indicator(container)?.className).toContain("bg-primary");
  });

  /** 验证消费单元格的正常与边界显示；前置本地组件，断言可访问内容，测试框架自动卸载。 */
  it("switches to the warning tone at 80% usage", () => {
    const { container } = render(<SpendBudgetCell spend={80} maxBudget={100} />);
    expect(indicator(container)?.className).toContain("bg-warning");
  });

  /** 验证消费单元格的正常与边界显示；前置本地组件，断言可访问内容，测试框架自动卸载。 */
  it("switches to the over tone above 100% usage", () => {
    const { container } = render(<SpendBudgetCell spend={150} maxBudget={100} />);
    expect(indicator(container)?.className).toContain("bg-destructive");
  });

  /** 验证消费单元格的正常与边界显示；前置本地组件，断言可访问内容，测试框架自动卸载。 */
  it("never meters key spend against an inherited team/org budget", () => {
    const gates = [{ scope: "Team" as const, alias: "Team A", maxBudget: 200, budgetDuration: "30d" }];
    render(<SpendBudgetCell spend={10} maxBudget={null} inheritedGates={gates} />);
    expect(screen.getByText("Shared parent budget")).toBeInTheDocument();
    expect(screen.queryByText(/\(Team\)/)).not.toBeInTheDocument();
    expect(screen.queryByRole("meter")).not.toBeInTheDocument();
    expect(screen.getByLabelText("question-circle")).toBeInTheDocument();
  });

  /** 验证消费单元格的正常与边界显示；前置本地组件，断言可访问内容，测试框架自动卸载。 */
  it("shows no inherited-budget hint when there is nothing to inherit", () => {
    render(<SpendBudgetCell spend={10} maxBudget={null} inheritedGates={[]} />);
    expect(screen.queryByLabelText("question-circle")).not.toBeInTheDocument();
  });

  /** 验证消费单元格的正常与边界显示；前置本地组件，断言可访问内容，测试框架自动卸载。 */
  it("shows no inherited-budget hint when the key has its own budget", () => {
    const gates = [{ scope: "Team" as const, alias: "Team A", maxBudget: 200, budgetDuration: null }];
    render(<SpendBudgetCell spend={10} maxBudget={50} inheritedGates={gates} />);
    expect(screen.getByText("of $50")).toBeInTheDocument();
    expect(screen.queryByLabelText("question-circle")).not.toBeInTheDocument();
  });
});

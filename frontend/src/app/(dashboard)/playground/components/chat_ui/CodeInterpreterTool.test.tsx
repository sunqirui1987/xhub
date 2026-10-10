import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import CodeInterpreterTool, { supportsCodeInterpreter } from "./CodeInterpreterTool";
import { TooltipProvider } from "@/components/ui/tooltip";
/** 前置公开模型名和空输入；验证候选能力与供应商边界；纯函数，无数据清理。 */
it.each([
  ["", false],
  ["z-ai/glm-5", false],
  ["gpt-4o", true],
  ["openai/gpt-4o", true],
])("候选 %s = %s", (model, expected) => {
  expect(supportsCodeInterpreter(model as string)).toBe(expected);
});
/** 前置本地工具组件；验证禁用状态和支持模型的启用回调；自动卸载，无真实容器。 */
it("工具开关支持状态", () => {
  const update = vi.fn();
  const props = {
    accessToken: "test",
    enabled: false,
    onEnabledChange: update,
    selectedContainerId: null,
    onContainerChange: vi.fn(),
  };
  const view = render(
    <TooltipProvider>
      <CodeInterpreterTool {...props} selectedModel="glm-5" />
    </TooltipProvider>,
  );
  expect(screen.getByRole("switch")).toHaveAttribute("aria-disabled", "true");
  view.rerender(
    <TooltipProvider>
      <CodeInterpreterTool {...props} selectedModel="gpt-4o" />
    </TooltipProvider>,
  );
  fireEvent.click(screen.getByRole("switch"));
  expect(update).toHaveBeenCalledWith(true);
});

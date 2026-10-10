import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import SessionManagement from "./SessionManagement";
import { EndpointType } from "@/components/chat_ui/mode_endpoint_mapping";
import { TooltipProvider } from "@/components/ui/tooltip";
/** 前置本地组件；验证新会话不误报就绪、模式回调及非 Responses 隐藏；自动卸载，无外部数据。 */
it("新会话状态与协议边界", () => {
  const toggle = vi.fn();
  const view = render(
    <TooltipProvider>
      <SessionManagement
        endpointType={EndpointType.RESPONSES}
        responsesSessionId={null}
        useApiSessionManagement={true}
        onToggleSessionManagement={toggle}
      />
    </TooltipProvider>,
  );
  expect(screen.getByText(/New session/)).toBeVisible();
  expect(screen.queryByText(/Ready/)).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("switch"));
  expect(toggle.mock.calls[0][0]).toBe(false);
  view.rerender(
    <TooltipProvider>
      <SessionManagement
        endpointType={EndpointType.CHAT}
        responsesSessionId={null}
        useApiSessionManagement={false}
        onToggleSessionManagement={toggle}
      />
    </TooltipProvider>,
  );
  expect(screen.queryByRole("switch")).not.toBeInTheDocument();
});
/** 前置已有响应 ID；验证真实 ID 复制及剪贴板拒绝路径；恢复浏览器属性，无后台写入。 */
it.each([true, false])("复制响应 ID 成功状态 %s", async (success) => {
  const writeText = success ? vi.fn().mockResolvedValue(undefined) : vi.fn().mockRejectedValue(new Error("denied"));
  const previous = Object.getOwnPropertyDescriptor(navigator, "clipboard");
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
  try {
    render(
      <TooltipProvider>
        <SessionManagement
          endpointType={EndpointType.RESPONSES}
          responsesSessionId="resp-real-test"
          useApiSessionManagement={true}
          onToggleSessionManagement={vi.fn()}
        />
      </TooltipProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Copy response ID" }));
    expect(writeText).toHaveBeenCalledWith("resp-real-test");
  } finally {
    if (previous) Object.defineProperty(navigator, "clipboard", previous);
    else Reflect.deleteProperty(navigator, "clipboard");
  }
});

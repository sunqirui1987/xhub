import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import SpendLogsTable from "./index";
vi.mock("./RequestLogsPanel", () => ({
  default: ({ isActive, errorsOnly }: { isActive: boolean; errorsOnly?: boolean }) => (
    <div data-testid="request-logs-panel">{isActive ? "active" : "inactive"}:{errorsOnly ? "errors" : "all"}</div>
  ),
}));
vi.mock("./AuditLogsPanel", () => ({ default: () => <div data-testid="audit-logs-panel" /> }));
const props = { accessToken: "sk-test", token: "jwt-test", userRole: "Admin", userID: "user-1", premiumUser: true };
describe("Request logs page", () => {
  /** 前置不同登录角色；验证默认全部日志及错误标签均可访问，组件卸载自动清理 DOM，不访问用户数据。 */
  it.each(["Admin", "Internal User", "Internal Viewer"])("mounts both log tabs for %s", (userRole) => {
    render(<SpendLogsTable {...props} userRole={userRole} />);
    expect(screen.getByTestId("request-logs-panel")).toHaveTextContent("active");
    expect(screen.queryByTestId("audit-logs-panel")).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Logs", exact: true })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Error Logs" })).toHaveAttribute("aria-selected", "false");
  });
  /** 前置有效凭据；切换错误视图并返回，验证固定筛选参数交给当前面板；自动卸载清理。 */
  it("switches between all requests and errors", async () => {
    const user = userEvent.setup();
    render(<SpendLogsTable {...props} />);
    await user.click(screen.getByRole("tab", { name: "Error Logs" }));
    expect(screen.getByTestId("request-logs-panel")).toHaveTextContent("active:errors");
    await user.click(screen.getByRole("tab", { name: "Logs", exact: true }));
    expect(screen.getByTestId("request-logs-panel")).toHaveTextContent("active:all");
  });
  it("waits for credentials", () => {
    render(<SpendLogsTable {...props} accessToken={null} />);
    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(screen.queryByTestId("request-logs-panel")).not.toBeInTheDocument();
  });
});

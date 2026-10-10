import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import PlaygroundPage from "./page";
import { fetchProxySettings } from "@/utils/proxyUtils";
import { setActiveLocale } from "@/i18n";

const authState = { userRole: "Admin" };

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({
  default: () => ({
    token: "token-1",
    accessToken: "sk-test",
    userId: "user-1",
    userRole: authState.userRole,
    isViewOnly: ["Admin Viewer", "Internal Viewer"].includes(authState.userRole),
    disabledPersonalKeyCreation: false,
  }),
}));

vi.mock("@/utils/proxyUtils", () => ({
  fetchProxySettings: vi.fn().mockResolvedValue(null),
}));

vi.mock("@/app/(dashboard)/playground/components/chat_ui/ChatUI", () => ({
  default: () => <div data-testid="chat-ui" />,
}));

describe("PlaygroundPage role guard", () => {
  beforeEach(() => {
    authState.userRole = "Admin";
    vi.mocked(fetchProxySettings).mockClear();
  });

  /** 前置只读角色；验证整页拒绝访问且无调试工作区，自动卸载，无持久数据。 */
  it.each(["Internal Viewer", "Admin Viewer"])("blocks the entire playground for %s", (role) => {
    authState.userRole = role;
    render(<PlaygroundPage />);

    expect(screen.getByText("Access Denied")).toBeInTheDocument();
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();
    expect(screen.queryByTestId("chat-ui")).not.toBeInTheDocument();
  });

  /** 前置允许调试的角色；验证直接渲染单一工作区、无多余页签，自动卸载，无持久数据。 */
  it.each(["Admin", "Internal User", "Org Admin"])("renders the playground for %s", (role) => {
    authState.userRole = role;
    render(<PlaygroundPage />);

    expect(screen.queryByText("Access Denied")).not.toBeInTheDocument();
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();
    expect(screen.getByTestId("chat-ui")).toBeInTheDocument();
  });

  /** 前置管理员会话；验证仍加载代理配置，自动卸载，代理调用使用 mock 无清理数据。 */
  it("loads deployment settings only for a platform administrator", async () => {
    render(<PlaygroundPage />);
    await waitFor(() => expect(fetchProxySettings).toHaveBeenCalledWith("sk-test"));
  });

  /** 前置非管理员角色；验证调试不会读取管理配置，自动卸载无持久数据。 */
  it.each(["Internal User", "Org Admin"])("does not request management settings for %s", (role) => {
    authState.userRole = role;
    render(<PlaygroundPage />);
    expect(fetchProxySettings).not.toHaveBeenCalled();
  });

  /** 前置中文或英文及只读角色；验证拒绝访问文案正确翻译，自动卸载且每例重置语言，无网络。 */
  it.each([
    ["zh-CN", "无权访问"],
    ["en", "Access Denied"],
  ] as const)("localizes the read-only boundary in %s", (locale, heading) => {
    setActiveLocale(locale);
    authState.userRole = "Admin Viewer";
    render(<PlaygroundPage />);
    expect(screen.getByRole("heading", { name: heading })).toBeInTheDocument();
    expect(screen.queryByTestId("chat-ui")).not.toBeInTheDocument();
  });

  /** 前置两种界面语言和管理员；验证均直接打开调试且没有对比入口，自动卸载、语言每例重置。 */
  it.each(["zh-CN", "en"] as const)("renders a single workspace in %s", (locale) => {
    setActiveLocale(locale);
    render(<PlaygroundPage />);
    expect(screen.getByTestId("chat-ui")).toBeInTheDocument();
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();
  });
});

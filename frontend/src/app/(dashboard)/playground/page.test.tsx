import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import PlaygroundPage from "./page";
import { fetchProxySettings } from "@/utils/proxyUtils";

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

vi.mock("@/app/(dashboard)/playground/components/compareUI/CompareUI", () => ({
  default: () => <div data-testid="compare-ui" />,
}));

describe("PlaygroundPage role guard", () => {
  beforeEach(() => {
    authState.userRole = "Admin";
    vi.mocked(fetchProxySettings).mockClear();
  });

  it.each(["Internal Viewer", "Admin Viewer"])("blocks the entire playground for %s", (role) => {
    authState.userRole = role;
    render(<PlaygroundPage />);

    expect(screen.getByText("Access Denied")).toBeInTheDocument();
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();
    expect(screen.queryByTestId("chat-ui")).not.toBeInTheDocument();
    expect(screen.queryByTestId("compare-ui")).not.toBeInTheDocument();
  });

  it.each(["Admin", "Internal User", "Org Admin"])("renders the playground for %s", (role) => {
    authState.userRole = role;
    render(<PlaygroundPage />);

    expect(screen.queryByText("Access Denied")).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Chat" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Compare" })).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Compliance" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Agent Builder (Experimental)" })).not.toBeInTheDocument();
    expect(screen.getByTestId("chat-ui")).toBeInTheDocument();
  });

  it("loads deployment settings only for a platform administrator", async () => {
    render(<PlaygroundPage />);
    await waitFor(() => expect(fetchProxySettings).toHaveBeenCalledWith("sk-test"));
  });

  it.each(["Internal User", "Org Admin"])("does not request management settings for %s", (role) => {
    authState.userRole = role;
    render(<PlaygroundPage />);
    expect(fetchProxySettings).not.toHaveBeenCalled();
  });
});

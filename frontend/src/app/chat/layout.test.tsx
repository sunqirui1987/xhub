import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import ChatLayout from "./layout";

const { mockUseAuthorized, mockUseUISettings, mockReplace, mockUiHref, state } = vi.hoisted(() => {
  const state = {
    enableChatUI: false,
    isUISettingsLoading: false,
  };
  return {
    state,
    mockReplace: vi.fn(),
    mockUiHref: vi.fn((segment: string) => `/mocked-ui/${segment}`),
    mockUseAuthorized: vi.fn(() => ({
      accessToken: "token-123",
      userRole: "Internal User",
      userId: "user-1",
      userEmail: "user@example.com",
      premiumUser: false,
    })),
    mockUseUISettings: vi.fn(() => ({
      data: { values: { enable_chat_ui: state.enableChatUI } },
      isLoading: state.isUISettingsLoading,
    })),
  };
});

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mockReplace }),
}));
vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({ default: mockUseAuthorized }));
vi.mock("@/app/(dashboard)/hooks/uiSettings/useUISettings", () => ({ useUISettings: mockUseUISettings }));
vi.mock("@/utils/uiHref", () => ({ uiHref: mockUiHref }));
vi.mock("@/components/navbar", () => ({ default: () => <div data-testid="navbar" /> }));
vi.mock("@/contexts/ThemeContext", () => ({
  ThemeProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
vi.mock("@/contexts/ChatShellContext", () => ({
  ChatShellProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
vi.mock("@/components/chat/ChatShell", () => ({
  default: ({ children }: { children: React.ReactNode }) => <div data-testid="chat-shell">{children}</div>,
}));

describe("ChatLayout", () => {
  afterEach(() => {
    state.enableChatUI = false;
    state.isUISettingsLoading = false;
    mockReplace.mockClear();
    mockUiHref.mockClear();
  });

  it("renders the chat shell for any signed-in account", () => {
    state.enableChatUI = false;
    render(
      <ChatLayout>
        <div data-testid="page-content" />
      </ChatLayout>,
    );
    expect(screen.getByTestId("chat-shell")).toBeInTheDocument();
    expect(screen.getByTestId("page-content")).toBeInTheDocument();
    expect(mockReplace).not.toHaveBeenCalled();
  });
});

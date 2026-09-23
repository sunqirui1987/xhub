import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import ChatShell from "./ChatShell";
import { translate } from "@/i18n/translate";
import { setActiveLocale } from "@/i18n/runtime";
import { I18nProvider } from "@/i18n/I18nProvider";

const { mockPush, mockUsePathname, mockUseChatShell } = vi.hoisted(() => ({
  mockPush: vi.fn(),
  mockUsePathname: vi.fn(() => "/ui/chat"),
  mockUseChatShell: vi.fn(() => ({
    conversations: [],
    activeConversationId: null,
    deleteConversation: vi.fn(),
    renameConversation: vi.fn(),
  })),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
  usePathname: mockUsePathname,
}));
// Deterministic hrefs so navigation/active-state assertions don't depend on server_root_path.
vi.mock("@/utils/uiHref", () => ({ uiHref: (seg: string) => `/ui/${seg}`.replace(/\/$/, "") || "/ui" }));
vi.mock("@/contexts/ChatShellContext", () => ({ useChatShell: mockUseChatShell }));
vi.mock("./ConversationList", () => ({ default: () => <div data-testid="conversation-list" /> }));

describe("ChatShell", () => {
  afterEach(() => {
    mockPush.mockClear();
    mockUsePathname.mockReturnValue("/ui/chat");
  });

  it("marks Chats active and shows the conversation list on the base chat route", () => {
    render(
      <ChatShell>
        <div />
      </ChatShell>,
    );
    expect(screen.getByRole("button", { name: translate("en", "pages.chat.chats") })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("button", { name: translate("en", "pages.chat.apiKeys") })).not.toHaveAttribute(
      "aria-current",
    );
    expect(screen.getByTestId("conversation-list")).toBeInTheDocument();
  });

  it("marks API Keys active while still showing the conversation list", () => {
    mockUsePathname.mockReturnValue("/ui/chat/api-keys");
    render(
      <ChatShell>
        <div />
      </ChatShell>,
    );
    expect(screen.getByRole("button", { name: translate("en", "pages.chat.apiKeys") })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("button", { name: translate("en", "pages.chat.chats") })).not.toHaveAttribute(
      "aria-current",
    );
    expect(screen.getByTestId("conversation-list")).toBeInTheDocument();
  });

  it("navigates to the dedicated route for each nav item", () => {
    render(
      <ChatShell>
        <div />
      </ChatShell>,
    );
    fireEvent.click(screen.getByRole("button", { name: translate("en", "pages.chat.integrations") }));
    expect(mockPush).toHaveBeenCalledWith("/ui/chat/integrations");

    fireEvent.click(screen.getByRole("button", { name: translate("en", "pages.chat.usage") }));
    expect(mockPush).toHaveBeenCalledWith("/ui/chat/usage");

    fireEvent.click(screen.getByRole("button", { name: translate("en", "pages.chat.logs") }));
    expect(mockPush).toHaveBeenCalledWith("/ui/chat/logs");
  });

  it("marks Logs active on the logs route", () => {
    mockUsePathname.mockReturnValue("/ui/chat/logs");
    render(
      <ChatShell>
        <div />
      </ChatShell>,
    );
    expect(screen.getByRole("button", { name: translate("en", "pages.chat.logs") })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("button", { name: translate("en", "pages.chat.usage") })).not.toHaveAttribute(
      "aria-current",
    );
  });

  it("tolerates a trailing slash on the current pathname when matching the active route", () => {
    mockUsePathname.mockReturnValue("/ui/chat/usage/");
    render(
      <ChatShell>
        <div />
      </ChatShell>,
    );
    expect(screen.getByRole("button", { name: translate("en", "pages.chat.usage") })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("renders Simplified Chinese nav labels when locale is zh-CN", () => {
    setActiveLocale("zh-CN");
    render(
      <I18nProvider initialLocale="zh-CN">
        <ChatShell>
          <div />
        </ChatShell>
      </I18nProvider>,
    );
    expect(screen.getByRole("button", { name: translate("zh-CN", "pages.chat.chats") })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: translate("zh-CN", "pages.chat.newChat") })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: translate("en", "pages.chat.chats") })).not.toBeInTheDocument();
    setActiveLocale("en");
  });
});

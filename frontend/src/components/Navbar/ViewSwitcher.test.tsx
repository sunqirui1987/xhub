import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import ViewSwitcher from "./ViewSwitcher";

const { mockUsePluginMode, mockUseUISettings, mockUsePathname, state } = vi.hoisted(() => {
  const state = {
    mode: "ai-gateway" as string,
    setMode: vi.fn(),
    plugins: [] as { name: string; display_name: string; url: string }[],
    activePlugin: null as { name: string; display_name: string; url: string } | null,
    enableChatUI: false,
    pathname: "/ui/",
  };
  return {
    state,
    mockUsePluginMode: vi.fn(() => ({
      mode: state.mode,
      setMode: state.setMode,
      plugins: state.plugins,
      activePlugin: state.activePlugin,
    })),
    mockUseUISettings: vi.fn(() => ({ data: { values: { enable_chat_ui: state.enableChatUI } } })),
    mockUsePathname: vi.fn(() => state.pathname),
  };
});

vi.mock("@/contexts/PluginModeContext", () => ({ usePluginMode: mockUsePluginMode }));
vi.mock("@/app/(dashboard)/hooks/uiSettings/useUISettings", () => ({ useUISettings: mockUseUISettings }));
vi.mock("next/navigation", () => ({ usePathname: mockUsePathname }));
// Deterministic hrefs so navigation assertions don't depend on server_root_path.
vi.mock("@/utils/uiHref", async (importOriginal) => ({ ...await importOriginal<typeof import("@/utils/uiHref")>(), uiHref: (seg: string) => `/ui/${seg}` }));

describe("ViewSwitcher", () => {
  let assignSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    assignSpy = vi.fn();
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { assign: assignSpy },
    });
  });

  afterEach(() => {
    state.mode = "ai-gateway";
    state.plugins = [];
    state.enableChatUI = false;
    state.pathname = "/ui/";
    state.setMode.mockClear();
  });

  it.each(["/chat", "/ui/chat", "/chat/logs", "/ui/chat/logs"])("labels rewritten Chat route %s consistently", (pathname) => {
    state.pathname = pathname;
    render(<ViewSwitcher />);
    expect(screen.getByRole("button")).toHaveTextContent("Chat");
  });

  it("opens Chat for everyone, without an administrator setting", async () => {
    render(<ViewSwitcher />);

    const button = screen.getByRole("button");
    expect(button).toHaveTextContent("AI Gateway");

    act(() => {
      fireEvent.click(button);
    });
    expect(await screen.findByText("Chat")).toBeInTheDocument();
    expect(screen.queryByText(/Admins can enable in Settings/i)).not.toBeInTheDocument();

    act(() => {
      fireEvent.click(screen.getByText("Chat"));
    });
    expect(assignSpy).toHaveBeenCalledWith("/ui/chat");
  });

  it("labels the button from the active plugin and lists AI Gateway + each plugin", async () => {
    state.plugins = [
      { name: "litellm-platform-plugin", display_name: "Chat UI", url: "http://localhost:3300" },
      { name: "obs", display_name: "Observability", url: "http://localhost:9000" },
    ];
    state.mode = "litellm-platform-plugin";
    render(<ViewSwitcher />);

    expect(screen.getByRole("button")).toHaveTextContent("Chat UI");

    act(() => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(await screen.findByText("AI Gateway")).toBeInTheDocument();
    expect(screen.getByText("Observability")).toBeInTheDocument();
  });

  it("switches plugin mode when a mode entry is picked", async () => {
    state.plugins = [{ name: "litellm-platform-plugin", display_name: "Chat UI", url: "http://localhost:3300" }];
    state.mode = "ai-gateway";
    render(<ViewSwitcher />);

    act(() => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(await screen.findByText("Chat UI")).toBeInTheDocument();
    act(() => {
      fireEvent.click(screen.getByText("Chat UI"));
    });
    expect(state.setMode).toHaveBeenCalledWith("litellm-platform-plugin");
    expect(assignSpy).not.toHaveBeenCalled();
  });

  it("shows a clickable Chat entry that navigates to the chat app when enabled", async () => {
    state.enableChatUI = true;
    render(<ViewSwitcher />);

    act(() => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(await screen.findByText("Chat")).toBeInTheDocument();
    expect(screen.queryByText(/Admins can enable in Settings/i)).not.toBeInTheDocument();

    act(() => {
      fireEvent.click(screen.getByText("Chat"));
    });
    expect(assignSpy).toHaveBeenCalledWith("/ui/chat");
    expect(state.setMode).not.toHaveBeenCalled();
  });

  it("navigates back to the dashboard when a mode entry is picked from the chat route", async () => {
    state.enableChatUI = true;
    state.pathname = "/ui/chat";
    state.plugins = [{ name: "litellm-platform-plugin", display_name: "Chat UI", url: "http://localhost:3300" }];
    render(<ViewSwitcher />);

    act(() => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(await screen.findByText("AI Gateway")).toBeInTheDocument();
    act(() => {
      fireEvent.click(screen.getByText("AI Gateway"));
    });
    expect(state.setMode).toHaveBeenCalledWith("ai-gateway");
    expect(assignSpy).toHaveBeenCalledWith("/ui/");
  });

  it("keeps Chat clickable when other plugins are listed", async () => {
    state.plugins = [{ name: "obs", display_name: "Observability", url: "http://localhost:9000" }];
    render(<ViewSwitcher />);

    act(() => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(await screen.findByText("Observability")).toBeInTheDocument();
    act(() => {
      fireEvent.click(screen.getByText("Chat"));
    });
    expect(assignSpy).toHaveBeenCalledWith("/ui/chat");
  });
});

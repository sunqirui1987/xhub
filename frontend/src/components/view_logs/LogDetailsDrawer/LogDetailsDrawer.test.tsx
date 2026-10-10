import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { t } from "@/i18n";
import { LogDetailsDrawer } from "./LogDetailsDrawer";
import { sessionSpendLogsCall } from "../../networking";
import { LogEntry } from "../columns";
import { AutoRouterModelGroupsProvider } from "@/components/shared/table_cells";

const detailCapture = vi.hoisted(() => ({ data: null as Record<string, unknown> | null, render: vi.fn(), header: vi.fn() }));

vi.mock("../../networking", () => ({
  sessionSpendLogsCall: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/logDetails/useLogDetails", () => ({
  useLogDetails: () => ({ data: detailCapture.data, isLoading: false }),
}));

vi.mock("./LogDetailContent", () => ({
  LogDetailContent: (props: unknown) => { detailCapture.render(props); return null; },
  GuardrailJumpLink: () => null,
}));

vi.mock("./DrawerHeader", () => ({
  DrawerHeader: (props: unknown) => { detailCapture.header(props); return null; },
}));

vi.mock("@/app/(dashboard)/hooks/models/useModels", () => ({
  useAutoRouterModelGroups: vi.fn(() => new Set(["smart-router"])),
}));

const makeLog = (overrides: Partial<LogEntry>): LogEntry => ({
  request_id: "req",
  api_key: "",
  team_id: "",
  model: "",
  model_id: "",
  call_type: "acompletion",
  spend: 0,
  total_tokens: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  startTime: "2026-07-08T10:00:00.000Z",
  endTime: "2026-07-08T10:00:01.000Z",
  cache_hit: "false",
  messages: [],
  response: {},
  ...overrides,
});

const sessionLogs = [
  makeLog({
    request_id: "llm-early",
    model: "llm-early",
    startTime: "2026-07-08T10:00:00.000Z",
    endTime: "2026-07-08T10:00:02.000Z",
  }),
  makeLog({
    request_id: "mcp-early",
    model: "tool-early",
    call_type: "call_mcp_tool",
    startTime: "2026-07-08T10:00:01.000Z",
    endTime: "2026-07-08T10:00:06.000Z",
  }),
  makeLog({
    request_id: "llm-late",
    model: "llm-late",
    startTime: "2026-07-08T10:00:02.000Z",
    endTime: "2026-07-08T10:00:05.000Z",
  }),
  makeLog({
    request_id: "mcp-late",
    model: "tool-late",
    call_type: "call_mcp_tool",
    startTime: "2026-07-08T10:00:03.000Z",
    endTime: "2026-07-08T10:00:03.500Z",
  }),
];

/** 渲染隔离会话抽屉；参数 logs 为列表夹具，返回重渲染入口；供侧栏测试调用，DOM 由框架清理。 */
const renderSessionDrawer = (logs = sessionLogs) => {
  vi.mocked(sessionSpendLogsCall).mockResolvedValue({ data: logs, total: logs.length, total_pages: 1 });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const drawer = (open: boolean) => (
    <QueryClientProvider client={queryClient}>
      <LogDetailsDrawer
        open={open}
        onClose={() => {}}
        logEntry={null}
        sessionId="session-1"
        sessionApiKey="key-1"
        sessionUserId="user-1"
        accessToken="token"
      />
    </QueryClientProvider>
  );
  const { rerender } = render(drawer(true));
  return { rerender, drawer };
};

const sidebarEventNames = () =>
  screen.queryAllByText(/^(llm-early|llm-late|tool-early|tool-late)$/).map((el) => el.textContent);

/** 前置列表中只有 error 状态、详情接口含诊断；验证懒加载正文和 HTTP 元数据传递给详情组件，finally 还原模拟数据。 */
it("merges complete lazy-loaded error diagnostics", async () => {
  detailCapture.data = { error: "full upstream diagnostic", metadata: { http_status: 502 }, response: { error: { trace: "upstream-123" } } };
  detailCapture.render.mockClear();
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  try {
    render(<QueryClientProvider client={queryClient}><LogDetailsDrawer open onClose={() => {}} logEntry={makeLog({ status: "error", metadata: { model_group: "keep-existing" } })} accessToken="token" /></QueryClientProvider>);
    await waitFor(() => expect(detailCapture.render).toHaveBeenCalledWith(expect.objectContaining({ logEntry: expect.objectContaining({ error: "full upstream diagnostic", metadata: { model_group: "keep-existing", http_status: 502 }, response: { error: { trace: "upstream-123" } } }) })));
  } finally {
    detailCapture.data = null;
    queryClient.clear();
  }
});

describe("LogDetailsDrawer session sidebar sorting", () => {
  /** 前置四条会话日志含提示词缓存，验证侧栏明确统计响应缓存且不混算；渲染由测试框架自动清理。 */
  it("labels response cache separately from provider prompt cache", async () => {
    renderSessionDrawer(sessionLogs.map(log => ({ ...log, metadata: { cached_tokens: 4 } })));
    expect(await screen.findByText(t("Response cache hits") + ": 0/4")).toBeVisible();
  });

  it("loads only the clicked caller's session", async () => {
    renderSessionDrawer();

    await waitFor(() =>
      expect(sessionSpendLogsCall).toHaveBeenCalledWith("token", "session-1", 1, 100, {
        apiKey: "key-1",
        userId: "user-1",
      }),
    );
  });

  it("waits for caller resolution before loading a deep-linked session", async () => {
    vi.mocked(sessionSpendLogsCall).mockClear();
    vi.mocked(sessionSpendLogsCall).mockResolvedValue({ data: [], total: 0, total_pages: 1 });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { rerender } = render(
      <QueryClientProvider client={queryClient}>
        <LogDetailsDrawer
          open
          onClose={() => {}}
          logEntry={null}
          sessionId="shared"
          accessToken="token"
          sessionCallerResolved={false}
        />
      </QueryClientProvider>,
    );

    expect(sessionSpendLogsCall).not.toHaveBeenCalled();

    rerender(
      <QueryClientProvider client={queryClient}>
        <LogDetailsDrawer
          open
          onClose={() => {}}
          logEntry={makeLog({ request_id: "representative", api_key: "key-1", user: "user-1" })}
          sessionId="shared"
          sessionApiKey="key-1"
          sessionUserId="user-1"
          accessToken="token"
          sessionCallerResolved
        />
      </QueryClientProvider>,
    );

    await waitFor(() => expect(sessionSpendLogsCall).toHaveBeenCalledTimes(1));
    expect(sessionSpendLogsCall).toHaveBeenCalledWith("token", "shared", 1, 100, {
      apiKey: "key-1",
      userId: "user-1",
    });
  });

  it("defaults to duration order, longest call first across LLM and MCP calls", async () => {
    renderSessionDrawer();
    await waitFor(() => expect(sidebarEventNames()).toHaveLength(4));
    expect(sidebarEventNames()).toEqual(["tool-early", "llm-late", "llm-early", "tool-late"]);
  });

  it("switches to chronological order across LLM and MCP calls when Start time is selected", async () => {
    renderSessionDrawer();
    await waitFor(() => expect(sidebarEventNames()).toHaveLength(4));

    fireEvent.click(screen.getByText("Start time"));

    await waitFor(() => expect(sidebarEventNames()).toEqual(["llm-early", "tool-early", "llm-late", "tool-late"]));

    fireEvent.click(screen.getByText("Duration"));

    await waitFor(() => expect(sidebarEventNames()).toEqual(["tool-early", "llm-late", "llm-early", "tool-late"]));
  });

  it("resets the sort mode back to duration when the drawer is closed and reopened", async () => {
    const { rerender, drawer } = renderSessionDrawer();
    await waitFor(() => expect(sidebarEventNames()).toHaveLength(4));

    fireEvent.click(screen.getByText("Start time"));
    await waitFor(() => expect(sidebarEventNames()).toEqual(["llm-early", "tool-early", "llm-late", "tool-late"]));

    rerender(drawer(false));
    rerender(drawer(true));

    await waitFor(() => expect(sidebarEventNames()).toEqual(["tool-early", "llm-late", "llm-early", "tool-late"]));
  });
});

describe("LogDetailsDrawer session sidebar auto-router icon", () => {
  const routedSessionLogs = [
    makeLog({ request_id: "routed", model: "claude-opus-4-8", model_group: "smart-router" }),
    makeLog({ request_id: "direct", model: "claude-haiku-4-5", model_group: "claude-haiku" }),
  ];

  const renderRoutedSession = () => {
    vi.mocked(sessionSpendLogsCall).mockResolvedValue({ data: routedSessionLogs, total: 2, total_pages: 1 });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
      <QueryClientProvider client={queryClient}>
        <AutoRouterModelGroupsProvider>
          <LogDetailsDrawer open onClose={() => {}} logEntry={null} sessionId="session-1" accessToken="token" />
        </AutoRouterModelGroupsProvider>
      </QueryClientProvider>,
    );
  };

  const rowFor = (label: string): HTMLElement => {
    const row = Array.from(document.body.querySelectorAll("button")).find((button) =>
      button.textContent?.includes(label),
    );
    if (!row) throw new Error(`no sidebar row for ${label}`);
    return row;
  };

  it("marks the auto-routed entry with the router icon and leaves a direct call on the default icon", async () => {
    renderRoutedSession();

    expect(await screen.findByText("claude-opus-4-8")).toBeInTheDocument();

    const routedRow = rowFor("claude-opus-4-8");
    const directRow = rowFor("claude-haiku-4-5");

    expect(routedRow.querySelector(".lucide-waypoints")).not.toBeNull();
    expect(routedRow.querySelector(".lucide-sparkles")).toBeNull();
    expect(directRow.querySelector(".lucide-sparkles")).not.toBeNull();
    expect(directRow.querySelector(".lucide-waypoints")).toBeNull();
  });
});

/** 前置列表仍是执行中、详情已完成；验证抽屉使用最新详情状态，错误状态优先；finally 清除详情夹具，DOM 自动卸载。 */
it("uses the latest task lifecycle status from detail in the header", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  detailCapture.data = { status: "completed" };
  try {
    render(<QueryClientProvider client={client}><LogDetailsDrawer open onClose={() => {}} logEntry={makeLog({ status: "executing" })} accessToken="token" /></QueryClientProvider>);
    await waitFor(() => expect(detailCapture.header).toHaveBeenLastCalledWith(expect.objectContaining({ statusLabel: t("Completed") })));
  } finally { detailCapture.data = null; client.clear(); }
});

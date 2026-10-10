import userEvent from "@testing-library/user-event";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { renderWithProviders, screen, testQueryClient, waitFor } from "../../../tests/test-utils";
import type { LogEntry as SpendLogEntry } from "@/components/view_logs/columns";
import { LogViewer } from "./LogViewer";

vi.mock("@/components/networking", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/networking")>();
  return { ...actual, uiSpendLogsCall: vi.fn() };
});

vi.mock("@/components/view_logs/LogDetailsDrawer", () => ({
  LogDetailsDrawer: function LogDetailsDrawerMock({
    open,
    logEntry,
  }: {
    open: boolean;
    logEntry?: { request_id: string } | null;
  }) {
    return (
      <div data-testid="log-details-drawer" data-log-id={logEntry?.request_id ?? ""}>
        {open ? "open" : "closed"}
      </div>
    );
  },
}));

import { uiSpendLogsCall } from "@/components/networking";

const spendLog = (overrides: Partial<SpendLogEntry>): SpendLogEntry => ({
  request_id: "req-1",
  api_key: "key-1",
  team_id: "team-1",
  model: "gpt-4o",
  model_id: "model-1",
  call_type: "acompletion",
  spend: 0.01,
  total_tokens: 10,
  prompt_tokens: 5,
  completion_tokens: 5,
  startTime: "2026-09-02T09:50:13Z",
  endTime: "2026-09-02T09:50:14Z",
  cache_hit: "false",
  messages: [],
  response: {},
  ...overrides,
});

/** 用途：构造包含护栏动作与原因的请求日志；参数覆盖默认字段，返回类型完整的请求行；仅用于本文件组件测试，无副作用。 */
const guardrailLog = {
  id: "provider-victim",
  timestamp: "2026-09-02 09:50:13",
  action: "passed" as const,
  input_snippet: "victim prompt",
  reason: "敏感词命中：victim",
  guardrail_name: "敏感词拦截",
  guardrail_provider: "xhub",
  guardrail_mode: "pre_call",
  latency_ms: 2.6,
};

describe("GuardrailsMonitor LogViewer drawer", () => {
  /** 用途：在每条用例前恢复请求 mock 和查询缓存；参数为 Vitest 测试上下文，返回无，避免测试间数据残留。 */
  beforeEach(() => {
    vi.mocked(uiSpendLogsCall).mockReset();
    testQueryClient.clear();
  });

  /** 用途：验证监控列表呈现原因、提供商、阶段、耗时和护栏名；前置为一条完整日志，结果不得只出现空内容横线，无外部数据需清理。 */
  it("renders the evaluation reason and key execution metadata instead of a blank content dash", () => {
    renderWithProviders(<LogViewer logs={[guardrailLog]} />);
    expect(screen.getByText("敏感词命中：victim")).toBeInTheDocument();
    expect(screen.getByText(/xhub$/)).toBeInTheDocument();
    expect(screen.getByText(/pre-call$/)).toBeInTheDocument();
    expect(screen.getByText(/3ms$/)).toBeInTheDocument();
    expect(screen.getByText("敏感词拦截")).toBeInTheDocument();
  });

  /** 用途：验证点击日志时按日志请求 ID 精确选择请求详情；前置为存在相似 call ID 的两行，结果选中精确 request ID，React Query 缓存由 beforeEach 清理。 */
  it("opens the row whose request_id is the clicked log id even when a newer row carries that id as its call id", async () => {
    vi.mocked(uiSpendLogsCall).mockResolvedValue({
      data: [
        spendLog({ request_id: "provider-attacker", litellm_call_id: "provider-victim" }),
        spendLog({ request_id: "provider-victim", litellm_call_id: "call-victim" }),
      ],
      total: 2,
    });

    renderWithProviders(<LogViewer logs={[guardrailLog]} accessToken="sk-test" />);
    await userEvent.click(screen.getByRole("button", { name: /敏感词命中：victim/ }));

    await waitFor(() => {
      expect(screen.getByTestId("log-details-drawer")).toHaveAttribute("data-log-id", "provider-victim");
    });
    expect(vi.mocked(uiSpendLogsCall)).toHaveBeenCalledWith(
      expect.objectContaining({ params: { request_id: "provider-victim" } }),
    );
  });

  /** 用途：验证后台未返回精确 request ID 时可用首行作为兼容回退；前置为一行相似 call ID 日志，结果显示返回行，缓存由 beforeEach 清理。 */
  it("falls back to the first returned row when none carries the clicked id as its request_id", async () => {
    vi.mocked(uiSpendLogsCall).mockResolvedValue({
      data: [spendLog({ request_id: "provider-other", litellm_call_id: "provider-victim" })],
      total: 1,
    });

    renderWithProviders(<LogViewer logs={[guardrailLog]} accessToken="sk-test" />);
    await userEvent.click(screen.getByRole("button", { name: /敏感词命中：victim/ }));

    await waitFor(() => {
      expect(screen.getByTestId("log-details-drawer")).toHaveAttribute("data-log-id", "provider-other");
    });
  });
});

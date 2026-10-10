import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { t } from "@/i18n";
import { DataTable } from "@/components/shared/DataTable";

import type { LogEntry } from "./columns";
import { getRequestLogsTableColumns } from "./RequestLogsTableColumns";

const logEntry = (overrides: Partial<LogEntry>): LogEntry => ({
  request_id: "req-1",
  api_key: "key-1",
  team_id: "team-1",
  model: "gpt-4o",
  model_id: "model-1",
  call_type: "acompletion",
  spend: 0,
  total_tokens: 10,
  prompt_tokens: 5,
  completion_tokens: 5,
  startTime: "2026-07-07T09:50:13Z",
  endTime: "2026-07-07T09:50:14Z",
  cache_hit: "false",
  messages: [],
  response: {},
  ...overrides,
});

const noopDeps = { onSessionClick: vi.fn() };

/** 前置后台 error 状态而没有旧版 metadata；验证表格显示失败而非成功，自动卸载清理 DOM。 */
it("labels backend error rows as failure", () => {
  renderRows([logEntry({ status: "error" })]);
  expect(screen.getByText("Failure")).toBeInTheDocument();
  expect(screen.queryByText("Success")).not.toBeInTheDocument();
});

/** 将参数日志与会话回调渲染为真实表格，供列行为测试使用；返回 void，测试框架自动卸载 DOM，无外部数据。 */
function renderRows(rows: LogEntry[], deps = noopDeps) {
  render(
    <DataTable
      data={rows}
      columns={getRequestLogsTableColumns(deps)}
      getRowId={(row) => row.request_id}
      size="compact"
    />,
  );
}

describe("Cost column", () => {
  it("renders '-' for zero spend with no tooltip, so hovering never shows a contradictory $0", async () => {
    const user = userEvent.setup();
    renderRows([logEntry({ request_id: "req-zero", spend: 0 })]);

    for (const dash of screen.getAllByText("-")) {
      await user.hover(dash);
    }
    expect(screen.queryByText("$0")).not.toBeInTheDocument();
  });

  it("shows the full-precision raw value in the tooltip for a real spend", async () => {
    const user = userEvent.setup();
    renderRows([logEntry({ request_id: "req-spend", spend: 0.00012345678 })]);

    await user.hover(screen.getByText("$0.000123"));
    expect(await screen.findByText("$0.00012345678")).toBeInTheDocument();
  });

  it("shows the summed session total, not the representative call's spend, for a multi-round session", () => {
    renderRows([
      logEntry({
        request_id: "req-session",
        spend: 0.01,
        session_id: "sess-1",
        session_total_count: 3,
        session_total_spend: 0.06,
      }),
    ]);

    expect(screen.getByText("$0.060000")).toBeInTheDocument();
    expect(screen.queryByText("$0.010000")).not.toBeInTheDocument();
    expect(screen.getByText("session total")).toBeInTheDocument();
  });
});

describe("Tokens column", () => {
  const sessionRow: Partial<LogEntry> = {
    request_id: "req-session-tokens",
    total_tokens: 10,
    prompt_tokens: 7,
    completion_tokens: 3,
    session_id: "sess-1",
    session_total_count: 3,
  };

  it("shows the summed session token usage, not the representative call's tokens, for a multi-round session", () => {
    const aggregatedRow: Partial<LogEntry> = {
      ...sessionRow,
      session_total_tokens: 60,
      session_total_prompt_tokens: 42,
      session_total_completion_tokens: 18,
    };
    renderRows([logEntry(aggregatedRow)]);

    const tokensCell = screen.getByRole("cell", { name: /\(42\+18\)/ });
    expect(tokensCell).toHaveTextContent("60");
    expect(tokensCell).toHaveTextContent("session total");
    expect(screen.queryByText("10")).not.toBeInTheDocument();
    expect(screen.queryByText("(7+3)")).not.toBeInTheDocument();
  });

  it("falls back to the call's own tokens with no session label when the backend sent no session token sums", () => {
    renderRows([logEntry(sessionRow)]);

    const tokensCell = screen.getByRole("cell", { name: /\(7\+3\)/ });
    expect(tokensCell).toHaveTextContent("10");
    expect(tokensCell).not.toHaveTextContent("session total");
  });
});

describe("Type column", () => {
  it("shows the conversation badge and composition even when an MCP call represents the conversation", async () => {
    const user = userEvent.setup();
    const mcpRepresentative = {
      request_id: "req-mcp-rep",
      call_type: "call_mcp_tool",
      session_id: "sess-edge",
      session_total_count: 3,
      session_llm_count: 2,
      mcp_tool_call_count: 1,
      session_agent_count: 0,
    };
    renderRows([logEntry(mcpRepresentative)]);

    expect(screen.queryByText("MCP")).not.toBeInTheDocument();
    await user.hover(screen.getByText("3"));
    expect(await screen.findByText("2 LLM • 1 MCP")).toBeInTheDocument();
  });

  it("keeps the plain MCP badge for a single MCP call", () => {
    renderRows([logEntry({ request_id: "req-mcp-solo", call_type: "call_mcp_tool", session_total_count: 1 })]);

    expect(screen.getByText("MCP")).toBeInTheDocument();
  });

  it("marks a batch cost row with the Batch badge instead of LLM", () => {
    renderRows([logEntry({ request_id: "batch_1_batch_cost", call_type: "aretrieve_batch" })]);

    expect(screen.getByText("Batch")).toBeInTheDocument();
    expect(screen.queryByText("LLM")).not.toBeInTheDocument();
  });

  it("keeps the Batch label on the grouped create-plus-cost session instead of a row count", () => {
    const groupedCostRow: Partial<LogEntry> = {
      request_id: "batch_1_batch_cost",
      call_type: "aretrieve_batch",
      session_id: "batch_1",
      session_total_count: 2,
    };
    renderRows([logEntry(groupedCostRow)]);

    expect(screen.getByText("Batch")).toBeInTheDocument();
    expect(screen.queryByText("2")).not.toBeInTheDocument();
  });
});

describe("batch rows", () => {
  const batchRow = (overrides: Partial<LogEntry>): LogEntry =>
    logEntry({
      request_id: "batch_abc123_batch_cost",
      call_type: "aretrieve_batch",
      ...overrides,
    });

  it("rolls partial failures into the status badge instead of reporting blanket Success", async () => {
    const user = userEvent.setup();
    renderRows([batchRow({ metadata: { batch_successful_requests: 2, batch_failed_requests: 1 } })]);

    expect(screen.queryByText("Success")).not.toBeInTheDocument();
    await user.hover(screen.getByText("2/3 succeeded"));
    expect(await screen.findByText("1 of 3 batch requests failed")).toBeInTheDocument();
  });

  it("keeps the Success badge when every batch request succeeded", () => {
    renderRows([batchRow({ metadata: { batch_successful_requests: 3, batch_failed_requests: 0 } })]);

    expect(screen.getByText("Success")).toBeInTheDocument();
  });

  it("keeps the Failure badge when the batch row itself failed, whatever the counts say", () => {
    renderRows([batchRow({ metadata: { status: "failure", batch_successful_requests: 2, batch_failed_requests: 1 } })]);

    expect(screen.getByText("Failure")).toBeInTheDocument();
    expect(screen.queryByText("2/3 succeeded")).not.toBeInTheDocument();
  });

  it("shows the provider batch id, not the synthetic _batch_cost request id", () => {
    renderRows([batchRow({ metadata: { batch_successful_requests: 1, batch_failed_requests: 0 } })]);

    expect(screen.getByText("batch_abc123")).toBeInTheDocument();
    expect(screen.queryByText("batch_abc123_batch_cost")).not.toBeInTheDocument();
    expect(screen.getByText("batch cost")).toBeInTheDocument();
  });

  it("leaves ordinary request ids untouched", () => {
    renderRows([logEntry({ request_id: "chatcmpl-42" })]);

    expect(screen.getByText("chatcmpl-42")).toBeInTheDocument();
    expect(screen.queryByText("batch cost")).not.toBeInTheDocument();
  });
});

describe("Model column", () => {
  it("lists every model used across a conversation, not only the representative call's model", () => {
    const conversationCall: Partial<LogEntry> = {
      request_id: "req-session",
      model: "gpt-5.6",
      session_id: "sess-1",
      session_total_count: 3,
      session_models: ["claude-sonnet-5", "gpt-5.6"],
    };
    renderRows([logEntry(conversationCall)]);

    expect(screen.getByText("claude-sonnet-5, gpt-5.6")).toBeInTheDocument();
    expect(screen.queryByText("gpt-5.6")).not.toBeInTheDocument();
  });

  it("marks a conversation whose model list was capped by the server", () => {
    const cappedCall = {
      request_id: "req-capped",
      model: "gpt-5.6",
      session_id: "sess-2",
      session_total_count: 30,
      session_models: ["claude-sonnet-5", "gpt-5.6"],
      session_models_truncated: true,
    };
    renderRows([logEntry(cappedCall)]);

    expect(screen.getByText("claude-sonnet-5, gpt-5.6, ...")).toBeInTheDocument();
  });

  it("keeps a single call's own model", () => {
    renderRows([logEntry({ request_id: "req-single", model: "gpt-5.6" })]);

    expect(screen.getByText("gpt-5.6")).toBeInTheDocument();
  });
});

describe("row action cells", () => {
  /** 前置正常 Hash 元数据；验证标识仅为文本且点击不调用会话动作，测试框架自动清理 DOM 与 mock。 */
  it("displays the key hash as plain text without a key action", async () => {
    const user = userEvent.setup();
    const deps = { onSessionClick: vi.fn() };
    renderRows([logEntry({ request_id: "req-key", metadata: { user_api_key: "sk-hash-9" } })], deps);

    const cell = screen.getByRole("cell", { name: "sk-hash-9" });
    expect(within(cell).queryByRole("button")).not.toBeInTheDocument();
    expect(within(cell).queryByRole("link")).not.toBeInTheDocument();
    await user.click(screen.getByText("sk-hash-9"));
    expect(deps.onSessionClick).not.toHaveBeenCalled();
  });

  /** 前置缺失、空值或非法类型 Hash；验证占位符与非交互行为，测试框架自动卸载 DOM，无外部数据清理。 */
  it.each([undefined, "", 42])("shows a plain placeholder for unavailable key hash %s", (hash) => {
    renderRows([logEntry({ metadata: { user_api_key: hash } })]);
    const headers = screen.getAllByRole("columnheader");
    const keyIndex = headers.findIndex((header) => header.textContent === t("Key Hash"));
    expect(keyIndex, "日志列必须包含密钥 Hash").toBeGreaterThanOrEqual(0);
    const cell = screen.getAllByRole("cell")[keyIndex];
    expect(cell).toHaveTextContent("-");
    expect(within(cell).queryByRole("button")).not.toBeInTheDocument();
    expect(within(cell).queryByRole("link")).not.toBeInTheDocument();
  });

  /** 前置两行共享会话 ID；验证点击回调仍传递目标日志，测试框架自动清理 DOM 与 mock。 */
  it("reports the clicked row from the session cell, so two rows sharing a session id stay distinguishable", async () => {
    const user = userEvent.setup();
    const deps = { onSessionClick: vi.fn() };
    renderRows(
      [
        logEntry({ request_id: "req-key-a", session_id: "sess-42", api_key: "key-a" }),
        logEntry({ request_id: "req-key-b", session_id: "sess-42", api_key: "key-b" }),
      ],
      deps,
    );

    await user.click(screen.getAllByText("sess-42")[1]);
    expect(deps.onSessionClick).toHaveBeenCalledWith(expect.objectContaining({ request_id: "req-key-b" }));
  });
});

describe("sortable headers", () => {
  it("exposes sort controls only for the backend-sortable fields", () => {
    renderRows([logEntry({})]);

    for (const field of ["startTime", "spend", "request_duration_ms", "ttft_ms", "model", "total_tokens"]) {
      expect(screen.getByTestId(`sort-trigger-${field}`)).toBeInTheDocument();
    }
    for (const field of ["request_id", "session_id", "status", "type", "end_user"]) {
      expect(screen.queryByTestId(`sort-trigger-${field}`)).not.toBeInTheDocument();
    }
  });
});

describe("TTFT column", () => {
  it("renders '-' when the completion start equals the end time, since TTFT is meaningless there", () => {
    renderRows([
      logEntry({
        request_id: "req-nonstream",
        endTime: "2026-07-07T09:50:14Z",
        completionStartTime: "2026-07-07T09:50:14Z",
      }),
    ]);

    expect(screen.queryByText("1.00")).not.toBeInTheDocument();
  });

  it("renders seconds when streaming produced a real first token", () => {
    renderRows([
      logEntry({
        request_id: "req-stream",
        startTime: "2026-07-07T09:50:13Z",
        endTime: "2026-07-07T09:50:16Z",
        completionStartTime: "2026-07-07T09:50:14Z",
      }),
    ]);

    expect(screen.getByText("1.00")).toBeInTheDocument();
  });
});

/** 前置真实表格组件和任务日志；验证三个阶段及失败均可观察，组件测试自动卸载 DOM。 */
it("renders task lifecycle badges", () => {
  renderRows(["executing", "polling", "completed", "failed"].map(status => logEntry({ request_id: status, status })));
  for (const label of ["Executing", "Polling", "Completed", "Failure"]) expect(screen.getByText(t(label))).toBeInTheDocument();
});

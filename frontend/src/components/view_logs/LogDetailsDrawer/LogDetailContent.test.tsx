import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { LogEntry } from "../columns";
import { LogDetailContent } from "./LogDetailContent";

/**
 * 为上游响应折叠组件测试构造最小日志；参数为覆盖字段，返回独立 LogEntry。
 * 仅供本文件渲染使用，不调用后台或持久化数据，测试结束由自动卸载清理。
 */
function logEntry(overrides: Partial<LogEntry> = {}): LogEntry {
  return {
    request_id: "collapse-test", model: "test-model", call_type: "chat",
    startTime: "2026-10-10T00:00:00Z", endTime: "2026-10-10T00:00:01Z",
    messages: [], response: {}, request_tags: {},
    metadata: { upstream_response: { status_code: 200, headers: { "X-Trace": ["complete"] }, usage_reported: false } },
    ...overrides,
  } as LogEntry;
}

describe("上游响应折叠", () => {
  /** 前置正常诊断日志；验证默认折叠、鼠标展开和键盘收起及未上报用量提示；自动卸载清理。 */
  it("默认折叠并支持点击展开和键盘收起", async () => {
    const user = userEvent.setup();
    render(<LogDetailContent logEntry={logEntry()} />);
    const region = screen.getByRole("region", { name: "Upstream Response" });
    const trigger = within(region).getByRole("button", { name: "Upstream Response", exact: true });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(within(region).queryByText("HTTP Status: 200")).not.toBeInTheDocument();
    expect(within(region).getByRole("button", { name: "Copy upstream response" })).toBeVisible();
    await user.click(trigger);
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(within(region).getByText("HTTP Status: 200")).toBeVisible();
    expect(within(region).getByText("Not reported")).toBeVisible();
    expect(within(region).getByText("{}")).toBeVisible();
    await user.keyboard("{Enter}");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(within(region).queryByText("HTTP Status: 200")).not.toBeInTheDocument();
  });

  /** 前置旧日志无诊断字段；验证不渲染空折叠区域且请求详情仍显示；自动卸载清理。 */
  it("无上游诊断时不显示折叠区域", () => {
    render(<LogDetailContent logEntry={logEntry({ metadata: {} })} />);
    expect(screen.queryByRole("region", { name: "Upstream Response" })).not.toBeInTheDocument();
    expect(screen.getByText("Request Details")).toBeVisible();
  });

  /** 前置失败响应且用量解析出错；验证错误主提示不受折叠影响、展开后显示解析失败与尾部响应头；自动卸载清理。 */
  it("失败诊断展开后完整可见", async () => {
    const user = userEvent.setup();
    render(<LogDetailContent logEntry={logEntry({ status: "error", error: "upstream failed", metadata: {
      http_status: 502, upstream_response: { status_code: 502, headers: {}, trailers: { "X-Trailer": ["last"] },
        usage_reported: false, usage_parse_error: "invalid upstream usage" },
    } })} />);
    expect(screen.getByText("Request Failed")).toBeVisible();
    expect(screen.getByRole("region", { name: "Full Error Details" })).toHaveTextContent("upstream failed");
    const region = screen.getByRole("region", { name: "Upstream Response" });
    expect(within(region).queryByRole("alert")).not.toBeInTheDocument();
    await user.click(within(region).getByRole("button", { name: "Upstream Response", exact: true }));
    expect(within(region).getByRole("alert")).toHaveTextContent("invalid upstream usage");
    expect(region).toHaveTextContent("last");
  });

  /** 前置同一详情组件连续展示两个请求；验证下一请求重新默认折叠；自动卸载，无数据副作用。 */
  it("切换请求时重置为折叠", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<LogDetailContent logEntry={logEntry()} />);
    await user.click(screen.getByRole("button", { name: "Upstream Response", exact: true }));
    rerender(<LogDetailContent logEntry={logEntry({ request_id: "next-request" })} />);
    expect(screen.getByRole("button", { name: "Upstream Response", exact: true })).toHaveAttribute("aria-expanded", "false");
  });
});

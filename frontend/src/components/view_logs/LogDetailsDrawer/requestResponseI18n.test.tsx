import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { setActiveLocale } from "@/i18n";
import type { LogEntry } from "../columns";
import { LogDetailContent } from "./LogDetailContent";
import { MediaRequestResponseView } from "./MediaRequestResponseView";
import { PrettyMessagesView } from "./PrettyMessagesView";

/**
 * 用途：构造日志详情本地化测试所需的最小记录。
 * 参数：overrides 覆盖测试场景中的响应、错误或协议字段。
 * 返回：可直接渲染的独立 LogEntry。
 * 调用场景：本文件验证 LogDetailContent 的成功、失败和 JSON 边界；不访问后台或持久化数据，测试卸载即完成清理。
 */
function logEntry(overrides: Partial<LogEntry> = {}): LogEntry {
  return {
    request_id: "localized-log",
    model: "gpt-test",
    call_type: "chat",
    startTime: "2026-10-10T00:00:00Z",
    endTime: "2026-10-10T00:00:01Z",
    messages: [{ role: "user", content: "hello" }],
    response: { choices: [{ message: { role: "assistant", content: "hi" } }] },
    request_tags: {},
    metadata: {},
    ...overrides,
  } as LogEntry;
}

afterEach(() => setActiveLocale("zh-CN"));

describe("请求日志详情双语组件", () => {
  /** 前提：普通聊天请求没有请求头和请求正文；结果：中文空状态与成功回答可见；仅渲染内存数据，卸载后无需额外清理。 */
  it("PrettyMessagesView 显示中文空状态与成功响应", () => {
    setActiveLocale("zh-CN");
    render(<PrettyMessagesView request={{}} response={{ choices: [{ message: { content: "成功回答" } }] }} />);
    expect(screen.getByText("未记录请求头")).toBeVisible();
    expect(screen.getByText("未记录请求正文")).toBeVisible();
    expect(screen.getByText("成功回答")).toBeVisible();
  });

  /** 前提：媒体协议解析已提供图片结果、任务状态和请求元数据；结果：英文标签、图片替代文本及协议字段完整；仅渲染内存数据，无需清理。 */
  it("MediaRequestResponseView 显示英文媒体成功结果", () => {
    setActiveLocale("en");
    render(
      <MediaRequestResponseView
        request={{ method: "POST", url: "/v1/images/generations", headers: { "x-trace": "ok" } }}
        media={{
          request: { kind: "image", model: "image-model", prompt: "a lighthouse", resolution: "1024x1024" },
          response: { kind: "image", imageUrls: ["https://example.invalid/result.png"], imageDataUrls: [], taskId: "task-1", status: "completed" },
        }}
      />,
    );
    expect(screen.getByText("Headers")).toBeVisible();
    expect(screen.getByText("Resolution")).toBeVisible();
    expect(screen.getByText("task-1")).toBeVisible();
    expect(screen.getByAltText("Generated result 1")).toHaveAttribute("src", "https://example.invalid/result.png");
  });

  /** 前提：媒体创建请求尚无任务 ID、图片或视频；结果：中文无响应提示可见且不伪造媒体；仅渲染内存数据，无需清理。 */
  it("MediaRequestResponseView 显示媒体无数据边界", () => {
    setActiveLocale("zh-CN");
    render(
      <MediaRequestResponseView
        request={{}}
        media={{ request: { kind: "video" }, response: { kind: "video", imageUrls: [], imageDataUrls: [] } }}
      />,
    );
    expect(screen.getByText("暂无媒体响应数据")).toBeVisible();
    expect(screen.queryByTestId("media-response-image")).not.toBeInTheDocument();
    expect(screen.queryByTestId("media-response-video")).not.toBeInTheDocument();
  });

  /** 目的：真实 Ark 终态只展示产物链接，FAL 创建态展示队列地址；前置中英文协议回执，验证各自字段和视频 src，渲染内存数据、卸载与 afterEach 恢复语言完成清理。 */
  it.each(["zh-CN", "en"] as const)("媒体地址按 Ark 与 FAL 协议展示：%s", (locale) => {
    setActiveLocale(locale);
    const responseLabel = locale === "zh-CN" ? "响应地址" : "Response URL";
    const statusLabel = locale === "zh-CN" ? "状态查询地址" : "Status URL";
    const videoUrl = "https://example.invalid/ark.mp4";
    const { rerender } = render(
      <PrettyMessagesView
        request={{ method: "GET", url: "/api/v3/contents/generations/tasks/ark-1" }}
        response={{ id: "ark-1", status: "succeeded", content: { video_url: videoUrl } }}
      />,
    );
    expect(screen.getByRole("link", { name: videoUrl })).toHaveAttribute("href", videoUrl);
    expect(screen.getByTestId("media-response-video")).toHaveAttribute("src", videoUrl);
    expect(screen.queryByText(responseLabel, { exact: true })).not.toBeInTheDocument();
    expect(screen.queryByText(statusLabel, { exact: true })).not.toBeInTheDocument();

    rerender(
      <PrettyMessagesView
        request={{ method: "POST", url: "/queue/fal-ai/kling-video" }}
        response={{ request_id: "fal-1", status: "IN_QUEUE", response_url: "/queue/fal-ai/kling-video/requests/fal-1", status_url: "/queue/fal-ai/kling-video/requests/fal-1/status" }}
      />,
    );
    expect(screen.getByText(responseLabel, { exact: true })).toBeVisible();
    expect(screen.getByText(statusLabel, { exact: true })).toBeVisible();
    expect(screen.getByText("/queue/fal-ai/kling-video/requests/fal-1", { exact: true })).toBeVisible();
    expect(screen.queryByTestId("media-response-video")).not.toBeInTheDocument();
  });

  /** 目的：Ark 未完成或失败不能出现成功产物或队列地址；前置真实 queued/failed 回执，验证任务 ID 与状态可见、媒体链接为空，纯组件卸载自动清理。 */
  it.each(["queued", "failed"])("Ark 无产物状态保留任务信息：%s", (status) => {
    setActiveLocale("zh-CN");
    render(
      <PrettyMessagesView
        request={{ method: "GET", url: "/api/v3/contents/generations/tasks/ark-1" }}
        response={{ id: "ark-1", status }}
      />,
    );
    expect(screen.getByText("ark-1", { exact: true })).toBeVisible();
    expect(screen.getByText(status, { exact: true })).toBeVisible();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.queryByTestId("media-response-video")).not.toBeInTheDocument();
    expect(screen.queryByText("响应地址", { exact: true })).not.toBeInTheDocument();
  });

  /** 前提：成功日志切换到 JSON 协议视图；结果：三块英文标题及复制动作可访问；仅操作组件状态，卸载后自动清理。 */
  it("LogDetailContent 在 JSON 视图显示英文请求响应标签", async () => {
    setActiveLocale("en");
    const user = userEvent.setup();
    render(<LogDetailContent logEntry={logEntry()} />);
    await user.click(screen.getByRole("tab", { name: "JSON", exact: true }));
    expect(screen.getByText("Headers")).toBeVisible();
    expect(screen.getByText("Request", { selector: "h4" })).toBeVisible();
    expect(screen.getByText("Response", { selector: "h4" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Copy headers" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Copy response" })).toBeEnabled();
  });

  /** 前提：失败日志只有错误且无正常响应正文；结果：错误仍展示，JSON 响应区保留完整失败正文；仅渲染内存数据，无需清理。 */
  it("LogDetailContent 保留失败诊断和 JSON 失败正文", async () => {
    setActiveLocale("zh-CN");
    const user = userEvent.setup();
    render(<LogDetailContent logEntry={logEntry({ status: "error", error: "upstream failed", response: null })} />);
    expect(screen.getByRole("region", { name: "完整错误详情" })).toHaveTextContent("upstream failed");
    await user.click(screen.getByRole("tab", { name: "JSON", exact: true }));
    const responseSection = screen.getByText("响应", { selector: "h4" }).closest("section");
    expect(responseSection).not.toBeNull();
    expect(within(responseSection!).getByText(/upstream failed/)).toBeVisible();
    expect(within(responseSection!).getByRole("button", { name: "复制响应" })).toBeEnabled();
  });
});

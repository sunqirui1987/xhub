import { afterEach, describe, expect, it } from "vitest";
import { setActiveLocale } from "@/i18n";
import { ROLE_STYLES, loggedResponse, parseMediaPayload, parseMessages, requestBody, requestHeaders } from "./prettyMessagesUtils";

afterEach(() => setActiveLocale("en"));

describe("ROLE_STYLES", () => {
  /** 前置同一模块已加载并切换中英文；验证 getter 每次按当前语言返回四种角色；恢复英文，无外部数据。 */
  it("reads role labels from the active locale without freezing module state", () => {
    expect([ROLE_STYLES.system.label, ROLE_STYLES.user.label, ROLE_STYLES.assistant.label, ROLE_STYLES.tool.label]).toEqual([
      "SYSTEM", "USER", "ASSISTANT", "TOOL RESULT",
    ]);
    setActiveLocale("zh-CN");
    expect([ROLE_STYLES.system.label, ROLE_STYLES.user.label, ROLE_STYLES.assistant.label, ROLE_STYLES.tool.label]).toEqual([
      "系统", "用户", "助手", "工具结果",
    ]);
  });
});

const completed = {
  type: "response.completed",
  response: {
    output: [{ type: "message", role: "assistant", content: [{ type: "output_text", text: "Could you clarify?" }] }],
  },
};

describe("request panels", () => {
  it("splits a proxy document into headers and the message body", () => {
    const stored = {
      method: "POST",
      headers: { Authorization: "***", "Content-Type": "application/json" },
      body: { messages: [{ role: "user", content: "hello" }] },
    };
    expect(requestHeaders(stored)).toEqual({ Authorization: "***", "Content-Type": "application/json" });
    expect(parseMessages(stored, null).requestMessages[0]?.content).toBe("hello");
    expect(requestBody(stored)).toEqual(stored.body);
  });
});

describe("loggedResponse", () => {
  it("reads the completed response out of a stored event stream", () => {
    const stored = {
      body: `event: response.created\ndata: {"type":"response.created"}\n\nevent: response.completed\ndata: ${JSON.stringify(completed)}\n`,
    };
    const message = parseMessages(null, stored).responseMessage;
    expect(message?.content).toBe("Could you clarify?");
    expect(loggedResponse(stored)).toEqual(completed.response);
  });

  it("keeps a streamed tool call as the assistant output", () => {
    const stored = {
      body: [
        'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","function":{"name":"exec","arguments":"{\\"input\\":"}}]}}]}',
        'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\\"ls\\""}}]}}]}',
        'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]}}]}',
        "data: [DONE]",
      ].join("\n"),
    };
    const message = parseMessages(null, stored).responseMessage;
    expect(message?.toolCalls?.[0]?.name).toBe("exec");
    expect(message?.toolCalls?.[0]?.arguments).toEqual({ input: "ls" });
  });

  it("joins chat stream deltas into one assistant message", () => {
    const stored = {
      body: [
        'data: {"choices":[{"delta":{"content":"Hel"}}]}',
        'data: {"choices":[{"delta":{"content":"lo"}}]}',
        "data: [DONE]",
      ].join("\n"),
    };
    expect(parseMessages(null, stored).responseMessage?.content).toBe("Hello");
  });
});

describe("parseMediaPayload", () => {
  /** 目的：完整 Responses 回执不能被 id/status 误判为视频；前置 JSON、流式和普通状态响应，验证返回 null，纯解析无清理。 */
  it("does not classify Responses envelopes or generic IDs and statuses as video", () => {
    const response = { id: "resp-text", object: "response", status: "completed",
      output: [{ type: "message", role: "assistant", content: [{ type: "output_text", text: "hello" }] }] };
    expect(parseMediaPayload({ url: "/v1/responses", body: { input: "hello" } }, response)).toBeNull();
    expect(parseMediaPayload(null, { body: `data: ${JSON.stringify({ type: "response.completed", response })}\n\n` })).toBeNull();
    expect(parseMediaPayload(null, { id: "record", status: "failed" })).toBeNull();
    expect(parseMediaPayload(null, { data: [{ id: "row" }] })).toBeNull();
    expect(parseMediaPayload(null, { data: [] })).toBeNull();
  });

  /** 目的：没有结果 URL 的真实视频回执仍应展示媒体详情；前置创建、查询及失败路径，验证任务状态保留，纯解析无清理。 */
  it("recognizes pending and failed videos by their request endpoint", () => {
    for (const status of ["queued", "failed"]) {
      const media = parseMediaPayload({ url: "/api/v3/contents/generations/tasks/task-1" }, { id: "task-1", status });
      expect(media?.response).toMatchObject({ kind: "video", taskId: "task-1", status });
    }
  });

  it("parses an image request wrapped in the stored proxy document", () => {
    const media = parseMediaPayload(
      { method: "POST", url: "/bypass/openai/v1/images/generations", body: { model: "gpt-image-2", prompt: "apple" } },
      { data: [{ url: "https://example.invalid/image.png" }, { b64_json: "iVBORw==" }] },
    );
    expect(media?.request).toMatchObject({ kind: "image", model: "gpt-image-2", prompt: "apple" });
    expect(media?.response.imageUrls).toEqual(["https://example.invalid/image.png"]);
    expect(media?.response.imageDataUrls).toEqual(["data:image/png;base64,iVBORw=="]);
  });

  /** 目的：Ark 产物地址与 FAL 队列查询地址不能混用；前置两种终态回执，验证视频解析正确且不伪造队列字段，纯解析无需清理。 */
  it("parses Ark content and FAL result video response shapes", () => {
    const ark = parseMediaPayload(
      { method: "GET", url: "/v3/contents/generations/tasks/qvideo-1" },
      { id: "qvideo-1", status: "succeeded", content: { video_url: "https://example.invalid/ark.mp4" }, duration: 4 },
    );
    expect(ark?.response).toMatchObject({ kind: "video", taskId: "qvideo-1", status: "succeeded", videoUrl: "https://example.invalid/ark.mp4", duration: 4 });
    expect(ark?.response.responseUrl).toBeUndefined();
    expect(ark?.response.statusUrl).toBeUndefined();

    const fal = parseMediaPayload(
      { method: "GET", url: "/queue/fal-ai/kling-video/requests/qvideo-2" },
      { result: { video: { url: "https://example.invalid/fal.mp4", duration: 5 } } },
    );
    expect(fal?.response).toMatchObject({ kind: "video", videoUrl: "https://example.invalid/fal.mp4", duration: 5 });
  });

  it("does not classify unrelated or malformed JSON as media", () => {
    expect(parseMediaPayload({ prompt: "ordinary form" }, { status: "ok" })).toBeNull();
    expect(parseMediaPayload(null, null)).toBeNull();
  });
});

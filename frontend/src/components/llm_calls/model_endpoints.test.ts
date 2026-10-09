import { afterEach, describe, expect, it, vi } from "vitest";
import {
  callTextEndpoint,
  endpointUIType,
  endpointURL,
  modelEndpoints,
  selectEndpoint,
  textEndpoints,
} from "./model_endpoints";
import type { ModelEndpoint } from "./fetch_models";
import { EndpointType } from "@/components/chat_ui/mode_endpoint_mapping";
/** binding 建立无真实凭据的协议测试绑定；参数为路径和协议，返回明确绑定。 */
const binding = (path: string, protocol = "openai-responses"): ModelEndpoint => ({
  path,
  protocol,
  method: "POST",
  kind: "bypass",
  transport: "test",
  endpoint_type: "test",
  family: "chat",
});
afterEach(() => vi.unstubAllGlobals());
/** streamResponse 逐字节切分 UTF-8 和事件边界，验证解析器不依赖网络分块。 */
const streamResponse = (events: string) => {
  const bytes = new TextEncoder().encode(events);
  return new Response(
    new ReadableStream({
      start(controller) {
        for (const byte of bytes) controller.enqueue(new Uint8Array([byte]));
        controller.close();
      },
    }),
    { headers: { "Content-Type": "text/event-stream" } },
  );
};
describe("explicit model endpoints", () => {
  it("never guesses from model mode", () => {
    expect(modelEndpoints({ model_group: "video", mode: "chat" })).toEqual([]);
    expect(selectEndpoint({ model_group: "unknown" }, "/v1/chat/completions")).toBeNull();
  });
  it("preserves supported paths and resets unsupported selections", () => {
    const endpoints = [
      binding("/bypass/openai/v1/responses"),
      binding("/bypass/anthropic/v1/messages", "anthropic-messages"),
    ];
    const model = { model_group: "text", endpoints };
    expect(selectEndpoint(model, endpoints[1].path)).toBe(endpoints[1].path);
    expect(selectEndpoint(model, "/v1/images/generations")).toBe(endpoints[0].path);
    expect(endpointUIType(endpoints[1])).toBe(EndpointType.ANTHROPIC_MESSAGES);
    expect(
      textEndpoints({ model_group: "vidu", endpoints: [binding("/queue/fal-ai/vidu/q1/text-to-video", "fal")] }),
    ).toEqual([]);
    expect(endpointURL("https://gateway.example/v1", endpoints[0].path)).toBe(
      "https://gateway.example/bypass/openai/v1/responses",
    );
  });
  it("sends native Responses fields and reads split UTF-8", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        streamResponse(
          'data: {"type":"response.output_text.delta","delta":"中文"}\r\n\r\ndata: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2}}}\r\n\r\n',
        ),
      );
    vi.stubGlobal("fetch", fetchMock);
    const onText = vi.fn(),
      onUsage = vi.fn();
    await callTextEndpoint({
      endpoint: binding("/bypass/openai/v1/responses"),
      base: "https://gateway.example/v1",
      key: "fake",
      model: "public",
      messages: [{ role: "user", content: "hello" }],
      maxTokens: 20,
      onText,
      onUsage,
    });
    expect(fetchMock.mock.calls[0][0]).toBe("https://gateway.example/bypass/openai/v1/responses");
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      model: "public",
      stream: true,
      input: [{ role: "user", content: "hello" }],
      max_output_tokens: 20,
    });
    expect(onText).toHaveBeenCalledWith("中文");
    expect(onUsage).toHaveBeenCalledWith(
      expect.objectContaining({ promptTokens: 3, completionTokens: 2, totalTokens: 5 }),
    );
  });
  it("separates Anthropic system and merges cache usage", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        streamResponse(
          'data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":5,"cache_creation_input_tokens":2}}}\n\ndata: {"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}\n\ndata: {"type":"message_delta","usage":{"output_tokens":4}}\n\ndata: {"type":"message_stop"}\n\n',
        ),
      );
    vi.stubGlobal("fetch", fetchMock);
    const onUsage = vi.fn();
    await callTextEndpoint({
      endpoint: binding("/bypass/anthropic/v1/messages", "anthropic-messages"),
      base: "https://gateway.example",
      key: "fake",
      model: "claude",
      messages: [
        { role: "system", content: "rules" },
        { role: "user", content: "hello" },
      ],
      onText: vi.fn(),
      onUsage,
    });
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      model: "claude",
      stream: true,
      messages: [{ role: "user", content: "hello" }],
      system: "rules",
      max_tokens: 2048,
    });
    expect(onUsage).toHaveBeenLastCalledWith(
      expect.objectContaining({ promptTokens: 15, completionTokens: 4, cacheReadTokens: 5, cacheCreationTokens: 2 }),
    );
  });
  it.each([
    'data: {"type":"response.output_text.delta","delta":"partial"}\n\n',
    'data: {"type":"response.failed"}\n\n',
  ])("rejects truncated or failed streams", async (events) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(streamResponse(events)));
    await expect(
      callTextEndpoint({
        endpoint: binding("/bypass/openai/v1/responses"),
        base: "https://gateway.example",
        key: "fake",
        model: "public",
        messages: [],
        onText: vi.fn(),
      }),
    ).rejects.toThrow();
  });
  it("uses event headers when native JSON omits type", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          streamResponse(
            'event: response.output_text.delta\ndata: {"delta":"中文"}\n\nevent: response.completed\ndata: {"response":{"usage":{"input_tokens":2,"output_tokens":1}}}\n\n',
          ),
        ),
    );
    const onText = vi.fn();
    await callTextEndpoint({
      endpoint: binding("/bypass/openai/v1/responses"),
      base: "https://gateway.example",
      key: "fake",
      model: "public",
      messages: [],
      onText,
    });
    expect(onText).toHaveBeenCalledWith("中文");
  });
  it.each(["data: [DONE]\n\n", "event: message_stop\ndata: {}\n\n"])(
    "requires the selected protocol completion event",
    async (events) => {
      vi.stubGlobal("fetch", vi.fn().mockResolvedValue(streamResponse(events)));
      await expect(
        callTextEndpoint({
          endpoint: binding("/bypass/openai/v1/responses"),
          base: "https://gateway.example",
          key: "fake",
          model: "public",
          messages: [],
          onText: vi.fn(),
        }),
      ).rejects.toThrow("Stream ended before completion");
    },
  );
});

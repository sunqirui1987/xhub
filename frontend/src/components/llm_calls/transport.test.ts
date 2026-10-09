import { afterEach, describe, expect, it, vi } from "vitest";
import { postLLMRequest, readJSONStream } from "./transport";

/** 把 UTF-8 文本按指定字节长度切成响应，验证网络分片边界；返回 SSE Response，无持久化副作用。 */
function sse(text: string, chunkSize = 1): Response {
  const bytes = new TextEncoder().encode(text);
  return new Response(
    new ReadableStream({
      start(controller) {
        for (let i = 0; i < bytes.length; i += chunkSize) controller.enqueue(bytes.slice(i, i + chunkSize));
        controller.close();
      },
    }),
  );
}
/** 消费真实解析器，返回事件数组；错误传播给用例，不产生测试数据。 */
async function collect(response: Response, signal?: AbortSignal) {
  const events = [];
  for await (const event of readJSONStream(response, signal)) events.push(event);
  return events;
}
afterEach(() => vi.unstubAllGlobals());

describe("fetch 数据面传输", () => {
  /** 前置 fetch 隔离；验证 URL、鉴权、标签、JSON 与 signal，结束恢复全局对象。 */
  it("保留 JSON 请求、鉴权和取消参数", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response("{}"));
    vi.stubGlobal("fetch", fetch);
    const signal = new AbortController().signal;
    await postLLMRequest(
      "/v1/messages",
      { model: "claude" },
      { baseUrl: "http://test///", accessToken: "key", tags: ["a", "b"], signal, anthropic: true },
    );
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe("http://test/v1/messages");
    expect(options.headers.get("authorization")).toBe("Bearer key");
    expect(options.headers.get("x-api-key")).toBe("key");
    expect(options.headers.get("anthropic-version")).toBe("2023-06-01");
    expect(options.headers.get("x-litellm-tags")).toBe("a,b");
    expect(JSON.parse(options.body)).toEqual({ model: "claude" });
    expect(options.signal).toBe(signal);
  });
  /** 前置真实 FormData；验证浏览器可生成 boundary，文件不丢失；恢复 fetch。 */
  it("表单不覆盖 multipart boundary", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response("{}"));
    vi.stubGlobal("fetch", fetch);
    const form = new FormData();
    form.append("file", new Blob(["audio"]), "a.wav");
    await postLLMRequest("audio/transcriptions", form, { baseUrl: "http://test", accessToken: "key" });
    expect(fetch.mock.calls[0][1].body).toBe(form);
    expect(fetch.mock.calls[0][1].headers.has("content-type")).toBe(false);
  });
  /** 前置不同供应商错误正文；验证 HTTP 状态和可观察错误；恢复 fetch。 */
  it.each(['{"error":{"message":"quota"}}', '{"error":"quota"}', "quota", "null"])(
    "HTTP 错误保留状态：%s",
    async (body) => {
      vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(body, { status: 429 })));
      await expect(
        postLLMRequest("chat/completions", {}, { baseUrl: "http://test", accessToken: "key" }),
      ).rejects.toMatchObject({ status: 429 });
    },
  );
});
describe("SSE 分片和失败边界", () => {
  /** 前置逐字节 UTF-8/CRLF/多行事件；验证中文不损坏，DONE 后停止；响应自行释放。 */
  it("解析 UTF-8、混合换行、多行和 DONE", async () => {
    await expect(
      collect(
        sse(
          ': ping\r\nevent: message\r\ndata: {"text":\r\ndata: "中文"}\r\n\r\ndata: [DONE]\n\ndata: {"ignored":true}\n\n',
        ),
      ),
    ).resolves.toEqual([{ text: "中文" }]);
  });
  /** 前置末尾没有空行的响应；验证最后事件不丢失；响应自行释放。 */
  it("解析 EOF 事件和空流", async () => {
    await expect(collect(sse('data: {"value":1}'))).resolves.toEqual([{ value: 1 }]);
    await expect(collect(sse(""))).resolves.toEqual([]);
  });
  /** 前置损坏 JSON 和供应商错误；验证不会静默成功；响应自行释放。 */
  it.each([
    "data: broken\n\n",
    'event: error\ndata: {"message":"bad"}\n\n',
    'data: {"type":"response.failed","response":{"error":{"message":"bad"}}}\n\n',
  ])("传播流失败 %s", async (body) => {
    await expect(collect(sse(body, 128))).rejects.toThrow();
  });
  /** 前置异常大事件及无正文响应；验证限制和错误；响应自行释放。 */
  it("拒绝超限事件及缺少 body", async () => {
    await expect(collect(sse("data: " + "x".repeat(8 * 1024 * 1024), 1024 * 1024))).rejects.toThrow("limit");
    await expect(collect(new Response(null))).rejects.toThrow("no body");
  });
  /** 前置挂起读取；取消必须解除等待和锁，不依赖网络；无持久化清理。 */
  it("取消挂起流并释放 reader", async () => {
    const cancelled = vi.fn();
    const response = new Response(new ReadableStream({ cancel: cancelled }));
    const controller = new AbortController();
    const pending = collect(response, controller.signal);
    controller.abort();
    await expect(pending).rejects.toMatchObject({ name: "AbortError" });
    expect(cancelled).toHaveBeenCalledOnce();
    expect(response.body?.locked).toBe(false);
  });
});

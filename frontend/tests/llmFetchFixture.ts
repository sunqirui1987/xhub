import { vi } from "vitest";

/** 将既有协议夹具接到真实 fetch Response 边界，供对话/媒体单元测试复用。
 * source 接收解析后的请求体和 signal；返回 JSON、二进制或事件序列。
 * 测试负责在 afterEach 调用 unstubAllGlobals，不产生网络请求或持久化数据。 */
export function installLLMFetchFixture(source: (body: any, options: { signal: AbortSignal | undefined }) => any) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, options: RequestInit) => {
      let body: any;
      if (options.body instanceof FormData) {
        body = Object.fromEntries(options.body.entries());
        if (body.temperature !== undefined) body.temperature = Number(body.temperature);
      } else body = JSON.parse(options.body as string);
      let result = await source(body, { signal: options.signal ?? undefined });
      let headers: HeadersInit = {};
      if (result?.withResponse) {
        const envelope = await result.withResponse();
        result = envelope.data;
        headers = envelope.response.headers;
      }
      if (result?.[Symbol.asyncIterator]) {
        const events = [];
        for await (const event of result) events.push("data: " + JSON.stringify(event) + "\n\n");
        return new Response(events.join(""), { headers: { "Content-Type": "text/event-stream" } });
      }
      if (result?.blob) return new Response(await result.blob(), { headers });
      return new Response(JSON.stringify(result), { headers });
    }),
  );
}

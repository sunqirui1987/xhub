import { t } from "@/i18n";

/** 数据面请求参数；调用方传入网关、虚拟密钥、标签和取消信号。 */
interface RequestOptions {
  baseUrl: string;
  accessToken: string;
  tags?: string[];
  signal?: AbortSignal;
  anthropic?: boolean;
}
/** 用 fetch 调用数据面；path 为协议路径，body 为 JSON/表单，options 为鉴权配置。
 * 返回成功 Response；HTTP 错误保留状态和消息，取消传播给调用方。表单自动生成 boundary。 */
export async function postLLMRequest(path: string, body: unknown, options: RequestOptions): Promise<Response> {
  const headers = new Headers({ Authorization: "Bearer " + options.accessToken });
  if (options.tags?.length) headers.set("x-litellm-tags", options.tags.join(","));
  if (options.anthropic) {
    headers.set("x-api-key", options.accessToken);
    headers.set("anthropic-version", "2023-06-01");
  }
  const multipart = body instanceof FormData;
  if (!multipart) headers.set("Content-Type", "application/json");
  const response = await fetch(options.baseUrl.replace(/\/+$/, "") + "/" + path.replace(/^\/+/, ""), {
    method: "POST",
    headers,
    body: multipart ? body : JSON.stringify(body),
    signal: options.signal,
  });
  if (!response.ok) {
    const raw = await response.text();
    let error: unknown;
    try {
      error = JSON.parse(raw).error;
    } catch {
      /* 非 JSON 错误保留原正文。 */
    }
    const detail = error as { message?: string } | string | undefined;
    throw Object.assign(
      new Error(typeof detail === "string" ? detail : detail?.message || raw || "HTTP " + response.status),
      { status: response.status, error },
    );
  }
  return response;
}
/** 共用 SSE 迭代器；参数为响应及取消信号，返回逐事件 JSON，供三个对话协议调用。
 * 支持 UTF-8 分片、CR/LF/CRLF、多行 data 和无末尾空行；错误/超限/取消抛出，结束释放 reader。 */
export async function* readJSONStream(response: Response, signal?: AbortSignal): AsyncGenerator<any> {
  if (!response.body) throw new Error(t("Streaming response has no body"));
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "",
    eventType = "";
  let data: string[] = [];
  let size = 0,
    finished = false;
  /** 分派累计事件，返回 JSON 或 undefined；DONE 终止读取，供应商错误抛出。 */
  function dispatch(): any {
    const payload = data.join("\n"),
      type = eventType;
    data = [];
    size = 0;
    eventType = "";
    if (!payload) return undefined;
    if (payload === "[DONE]") {
      finished = true;
      return undefined;
    }
    const value = JSON.parse(payload);
    if (type === "error" || value.error || value.type === "error" || value.type === "response.failed") {
      throw new Error(value.error?.message || value.response?.error?.message || value.message || t("Stream failed"));
    }
    return value;
  }
  /** 解析无换行的一行，返回完整事件或 undefined；累计事件超过 8 MiB 时抛出。 */
  function line(value: string): any {
    if (!value) return dispatch();
    size += value.length;
    if (size > 8 * 1024 * 1024) throw new Error(t("SSE event exceeds limit"));
    if (value.startsWith("data:")) data.push(value.slice(5).replace(/^ /, ""));
    else if (value.startsWith("event:")) eventType = value.slice(6).trim();
    return undefined;
  }
  /** AbortSignal 回调取消挂起读取；finally 负责释放锁。 */
  const abort = () => {
    void reader.cancel().catch(() => {});
  };
  signal?.addEventListener("abort", abort, { once: true });
  try {
    while (!finished) {
      signal?.throwIfAborted();
      const { done, value } = await reader.read();
      signal?.throwIfAborted();
      buffer += decoder.decode(value, { stream: !done });
      let match: RegExpExecArray | null;
      while ((match = /\r\n|\r|\n/.exec(buffer))) {
        // 末尾 CR 可能是分片 CRLF 的前半段，等待下一块。
        if (!done && match[0] === "\r" && match.index === buffer.length - 1) break;
        const event = line(buffer.slice(0, match.index));
        buffer = buffer.slice(match.index + match[0].length);
        if (event !== undefined) yield event;
        if (finished) break;
      }
      if (buffer.length + size > 8 * 1024 * 1024) throw new Error(t("SSE event exceeds limit"));
      if (done) {
        if (buffer) line(buffer);
        const event = dispatch();
        if (event !== undefined) yield event;
        break;
      }
    }
  } finally {
    signal?.removeEventListener("abort", abort);
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}

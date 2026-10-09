import type { ModelEndpoint, ModelGroup } from "./fetch_models";
import type { TokenUsage } from "@/components/chat_ui/ResponseMetrics";
import { EndpointType } from "@/components/chat_ui/mode_endpoint_mapping";

const UI_ENDPOINTS: Record<string, EndpointType> = {
  "/v1/chat/completions": EndpointType.CHAT,
  "/v1/responses": EndpointType.RESPONSES,
  "/v1/messages": EndpointType.ANTHROPIC_MESSAGES,
  "/v1/images/generations": EndpointType.IMAGE,
  "/v1/images/edits": EndpointType.IMAGE_EDITS,
  "/v1/embeddings": EndpointType.EMBEDDINGS,
  "/v1/audio/speech": EndpointType.SPEECH,
  "/v1/audio/transcriptions": EndpointType.TRANSCRIPTION,
};

/**
 * modelEndpoints 读取后端验证后的模型端点绑定。
 * 参数 model：所选模型元数据。返回：可调用绑定；未配置时为空，不从 mode 或供应商名称猜测。
 * 调用：对话、对比、EndpointUtils。测试：model_endpoints.test.ts。
 */
export const modelEndpoints = (model?: ModelGroup): ModelEndpoint[] => model?.endpoints ?? [];
/** endpointUIType 将已声明的端点映射为表单类型。
 * 参数 endpoint：真实绑定。返回：可用表单类型；未知端点及原生图片、视频返回 null，使用参数编辑器。
 * 原生文本只共用界面，发送时保留实际路径及协议字段。
 */
export const endpointUIType = (endpoint?: ModelEndpoint): EndpointType | null =>
  endpoint
    ? (UI_ENDPOINTS[endpoint.path] ??
      (
        {
          "openai-responses": EndpointType.RESPONSES,
          "anthropic-messages": EndpointType.ANTHROPIC_MESSAGES,
        } as Record<string, EndpointType>
      )[endpoint.protocol] ??
      null)
    : null;
/** textEndpoints 返回模型声明的文本对话端点。
 * 参数 model：可选模型元数据。返回：Chat Completions、Responses、Messages 的真实绑定。
 * 无文本绑定的视频模型不能发送文本对比请求；不根据 family 自动补建路径。
 * 调用：ChatUI、CompareUI、callTextEndpoint。测试：model_endpoints.test.ts。
 */
export const textEndpoints = (model?: ModelGroup): ModelEndpoint[] =>
  modelEndpoints(model).filter((endpoint) => /\/(chat\/completions|responses|messages)$/.test(endpoint.path));
/** selectEndpoint 校验模型切换后的选择：保留仍支持的路径，否则选首个绑定或返回 null。
 * 参数 model：新模型；previous：原路径。调用：对比模型切换。测试：model_endpoints.test.ts。
 */
export const selectEndpoint = (model: ModelGroup | undefined, previous: string | null): string | null => {
  const endpoints = modelEndpoints(model);
  return endpoints.find((item) => item.path === previous)?.path ?? endpoints[0]?.path ?? null;
};
/** endpointURL 组合网关根地址与完整公开路径，避免根地址的 /v1 重复。
 * 参数 base、path：网关地址和登记路径。返回：请求 URL。调用：原生文本、图片和视频界面。
 */
export const endpointURL = (base: string, path: string): string => {
  const root = base.replace(/\/$/, "");
  return root.replace(/\/v1$/, "") + path;
};

/**
 * callTextEndpoint 按所选模型的真实协议发送对话并读取 SSE。
 * 参数 endpoint/base/key/model：真实绑定、网关地址、测试密钥和对外别名；messages：对话记录；
 * signal：取消信号；streaming：是否流式；temperature/maxTokens：生成设置；tags：网关日志标签；
 * onText/onUsage/onTiming/onLatency：文本、实测用量、首字延迟和总延迟回调，延迟单位为秒。
 * 返回 Promise<void>。HTTP 错误、协议失败、缺少完成事件或 SSE 超限会抛出异常。
 * 不向原生请求体加入网关专属字段，也不在浏览器计算价格。
 * 调用：ChatUI、CompareUI。测试：model_endpoints.test.ts。
 */
export async function callTextEndpoint({
  endpoint,
  base,
  key,
  model,
  messages,
  onText,
  signal,
  temperature,
  maxTokens,
  streaming = true,
  tags,
  onUsage,
  onTiming,
  onLatency,
}: {
  endpoint: ModelEndpoint;
  base: string;
  key: string;
  model: string;
  messages: { role: string; content: unknown }[];
  onText: (text: string) => void;
  signal?: AbortSignal;
  temperature?: number;
  maxTokens?: number;
  streaming?: boolean;
  tags?: string[];
  onUsage?: (usage: TokenUsage) => void;
  onTiming?: (seconds: number) => void;
  onLatency?: (seconds: number) => void;
}): Promise<void> {
  const messagesAPI = endpoint.path.endsWith("/messages");
  const responsesAPI = endpoint.path.endsWith("/responses");
  if (!textEndpoints({ model_group: model, endpoints: [endpoint] }).length)
    throw new Error("Unsupported text endpoint");
  const start = performance.now();
  let firstText = false;
  /** emitText 把一段文本增量交给界面，并记录首字延迟。
   * 参数 text：协议解析后的文本。返回：无；通过 onText 和可选 onTiming 回调通知调用者。
   * 调用：非流式响应处理及 dispatch；首字延迟只报告一次，单位为秒。
   */
  const emitText = (text: string) => {
    if (!firstText) {
      firstText = true;
      onTiming?.((performance.now() - start) / 1000);
    }
    onText(text);
  };
  const usageFacts: Record<string, any> = {};
  /** emitUsage 合并一次请求分散在多个事件中的实测用量。
   * 参数 facts：可选上游 usage 对象。返回：无；通过 onUsage 报告累计事实，不计算金额。
   * Anthropic 的普通输入量与缓存读取量分开报告，界面的总输入需加缓存读取量；
   * OpenAI 的缓存读取已包含在输入量中，不能重复相加。缺失值保留 undefined。
   * 调用：非流式响应处理及 dispatch。测试：model_endpoints.test.ts 的缓存和合并用量用例。
   */
  const emitUsage = (facts?: Record<string, any>) => {
    if (!facts) return;
    Object.assign(usageFacts, facts);
    const input = usageFacts.input_tokens ?? usageFacts.prompt_tokens;
    const output = usageFacts.output_tokens ?? usageFacts.completion_tokens;
    const cached =
      usageFacts.cache_read_input_tokens ??
      usageFacts.input_tokens_details?.cached_tokens ??
      usageFacts.prompt_tokens_details?.cached_tokens;
    onUsage?.({
      promptTokens: input === undefined ? undefined : input + (messagesAPI ? (cached ?? 0) : 0),
      completionTokens: output,
      totalTokens:
        usageFacts.total_tokens ??
        (input !== undefined && output !== undefined ? input + output + (messagesAPI ? (cached ?? 0) : 0) : undefined),
      cacheReadTokens: cached,
      cacheCreationTokens: usageFacts.cache_creation_input_tokens,
    });
  };
  const body: Record<string, unknown> = { model, stream: streaming };
  if (responsesAPI) {
    body.input = messages.map((message) => ({
      ...message,
      content: Array.isArray(message.content)
        ? message.content.map((part: any) =>
            part.type === "text"
              ? { type: message.role === "assistant" ? "output_text" : "input_text", text: part.text }
              : part.type === "image_url"
                ? { type: "input_image", image_url: part.image_url.url }
                : part,
          )
        : message.content,
    }));
    if (maxTokens !== undefined) body.max_output_tokens = maxTokens;
  } else if (messagesAPI) {
    body.messages = messages
      .filter((message) => message.role !== "system")
      .map((message) => ({
        ...message,
        content: Array.isArray(message.content)
          ? message.content.map((part: any) => {
              if (part.type !== "image_url") return part;
              const url = part.image_url.url as string;
              const match = /^data:([^;]+);base64,(.+)$/.exec(url);
              return {
                type: "image",
                source: match ? { type: "base64", media_type: match[1], data: match[2] } : { type: "url", url },
              };
            })
          : message.content,
      }));
    const system = messages
      .filter((message) => message.role === "system")
      .map((message) => message.content)
      .join("\n");
    if (system) body.system = system;
    body.max_tokens = maxTokens ?? 2048;
  } else {
    body.messages = messages;
    if (maxTokens !== undefined) body.max_tokens = maxTokens;
  }
  if (temperature !== undefined) body.temperature = temperature;
  const response = await fetch(endpointURL(base, endpoint.path), {
    method: endpoint.method,
    headers: {
      Authorization: "Bearer " + key,
      "Content-Type": "application/json",
      ...(tags?.length ? { "x-litellm-tags": tags.join(",") } : {}),
    },
    body: JSON.stringify(body),
    signal,
  });
  if (!response.ok) throw new Error(await response.text());
  if (!response.body) throw new Error("Missing response body");
  if (!response.headers.get("content-type")?.includes("text/event-stream")) {
    const doc = await response.json();
    if (doc.error) throw new Error(doc.error.message ?? "Upstream error");
    if (responsesAPI && doc.status !== "completed") throw new Error("Response did not complete");
    emitUsage(doc.usage);
    const text =
      doc.choices?.[0]?.message?.content ??
      doc.content
        ?.filter((part: any) => part.type === "text")
        .map((part: any) => part.text)
        .join("") ??
      doc.output
        ?.flatMap((item: any) => item.content ?? [])
        .map((part: any) => part.text ?? "")
        .join("");
    if (text) emitText(text);
    onLatency?.((performance.now() - start) / 1000);
    return;
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let completed = false;
  /** dispatch 解析一个完整 SSE 事件并分发文本、用量和业务终态。
   * 参数 record：不含末尾空行的完整记录。返回：无；错误事件或无效 JSON 抛出异常。
   * event 头补充缺失的 JSON type；终态只能由所选协议确认，通用结束标记不能完成原生请求。
   * 调用：下方读取循环，TCP 分块先经解码和缓冲，再传入完整事件。
   * 测试：model_endpoints.test.ts 的分块、事件头、错误及协议终态用例。
   */
  const dispatch = (record: string) => {
    const data = record
      .split(/\r?\n/)
      .filter((line) => line.startsWith("data:"))
      .map((line) => line.slice(5).trimStart())
      .join("\n");
    if (data === "[DONE]") {
      // 原生 Responses/Messages 必须观察自己的业务终态，通用结束标记不能证明成功。
      if (!responsesAPI && !messagesAPI) completed = true;
      return;
    }
    if (!data) return;
    const doc = JSON.parse(data);
    const eventType = record
      .split(/\r?\n/)
      .find((line) => line.startsWith("event:"))
      ?.slice(6)
      .trim();
    if (!doc.type && eventType) doc.type = eventType;
    if (doc.error || ["error", "response.failed", "response.incomplete"].includes(doc.type))
      throw new Error(doc.error?.message ?? "Upstream stream failed");
    // 只能由当前协议的业务终态确认成功，其他协议的事件不能完成这次请求。
    if ((responsesAPI && doc.type === "response.completed") || (messagesAPI && doc.type === "message_stop"))
      completed = true;
    emitUsage(doc.response?.usage ?? doc.message?.usage ?? doc.usage);
    const delta =
      doc.choices?.[0]?.delta?.content ??
      (doc.type === "response.output_text.delta" ? doc.delta : doc.delta?.type === "text_delta" ? doc.delta.text : "");
    if (delta) emitText(delta);
  };
  try {
    while (true) {
      const { done, value } = await reader.read();
      buffer += decoder.decode(value, { stream: !done });
      let match: RegExpExecArray | null;
      while ((match = /\r?\n\r?\n/.exec(buffer))) {
        dispatch(buffer.slice(0, match.index));
        buffer = buffer.slice(match.index + match[0].length);
      }
      if (buffer.length > 8 * 1024 * 1024) throw new Error("SSE event exceeds limit");
      if (done) break;
    }
    if (!completed) throw new Error("Stream ended before completion");
    onLatency?.((performance.now() - start) / 1000);
  } finally {
    await reader.cancel();
  }
}

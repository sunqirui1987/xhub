/**
 * Utility functions for parsing and formatting messages for pretty view
 */

import {
  MessageRole,
  ParsedMessage,
  ParsedMessages,
  ParsedMediaPayload,
  RequestPayload,
  ResponsePayload,
  RoleStyle,
  ToolCall,
} from "./prettyMessagesTypes";

/**
 * Role color styles for message cards - minimal, professional design
 * Color only used for labels and left border accent
 */
export const ROLE_STYLES: Record<string, RoleStyle> = {
  system: {
    background: "transparent",
    borderColor: "var(--color-muted-foreground)",
    label: "SYSTEM",
    labelColor: "var(--color-muted-foreground)",
  },
  user: {
    background: "transparent",
    borderColor: "var(--color-info)",
    label: "USER",
    labelColor: "var(--color-info)",
  },
  assistant: {
    background: "transparent",
    borderColor: "var(--color-success)",
    label: "ASSISTANT",
    labelColor: "var(--color-success)",
  },
  tool: {
    background: "transparent",
    borderColor: "var(--color-warning)",
    label: "TOOL RESULT",
    labelColor: "var(--color-warning)",
  },
};

type UnknownRecord = Record<string, unknown>;

const isRecord = (value: unknown): value is UnknownRecord =>
  typeof value === "object" && value !== null && !Array.isArray(value);

const asString = (value: unknown): string => (typeof value === "string" ? value : "");

/**
 * 用途：把图片和视频 bypass 的请求、创建回执与终态回执整理成日志抽屉可直接展示的数据。
 * 参数：request 是已保存的代理请求或请求正文，response 是供应商原始响应，loggedModel 是日志模型名兜底。
 * 返回值：识别到媒体调用时返回结构化媒体数据，否则返回 null。
 * 调用场景：PrettyMessagesView 在聊天协议解析前调用；支持 OpenAI 图片、Ark 视频和 FAL 队列。
 * 边界：只根据明确的媒体路径或媒体响应字段识别；通用 id/status 和空 data 均不能证明媒体类型。
 */
export const parseMediaPayload = (
  request: unknown,
  response: unknown,
  loggedModel?: string,
): ParsedMediaPayload | null => {
  const body = requestBody(request);
  const requestDoc = isRecord(body) ? body : {};
  const responseDoc = loggedResponse(response);
  const responseRecord = isRecord(responseDoc) ? responseDoc : {};
  const requestURL = isRecord(request) ? asString(request.url) : "";
  const imageRows = Array.isArray(responseRecord.data) ? responseRecord.data.filter(isRecord) : [];
  const imageUrls = imageRows.map((row) => asString(row.url)).filter(Boolean);
  const imageDataUrls = imageRows
    .map((row) => asString(row.b64_json))
    .filter(Boolean)
    .map((value) => `data:${imageMime(value)};base64,${value}`);
  const result = isRecord(responseRecord.result) ? responseRecord.result : {};
  const content = isRecord(responseRecord.content) ? responseRecord.content : {};
  const video = isRecord(responseRecord.video)
    ? responseRecord.video
    : isRecord(result.video)
      ? result.video
      : {};
  const videoUrl = asString(content.video_url) || asString(video.url);
  const responseUrl = asString(responseRecord.response_url);
  const statusUrl = asString(responseRecord.status_url);
  const taskId = asString(responseRecord.id) || asString(responseRecord.request_id);
  const status = asString(responseRecord.status);
  const image = requestURL.includes("/images/") || imageUrls.length > 0 || imageDataUrls.length > 0;
  const videoRequest =
    requestURL.includes("/contents/generations/tasks") ||
    requestURL.includes("/queue/") ||
    requestURL.includes("/videos/");
  // Responses 文本回执也有 id 和 status=completed；只有视频内容或队列地址才能作为响应侧的媒体证据。
  // 尚未生成 URL 的视频创建、查询和失败回执由明确的视频请求路径识别。
  const videoResponse = Boolean(videoUrl || responseUrl || statusUrl);
  if (!image && !videoRequest && !videoResponse) return null;

  const kind = image ? "image" : "video";
  const prompt =
    asString(requestDoc.prompt) ||
    (Array.isArray(requestDoc.content)
      ? requestDoc.content
          .filter(isRecord)
          .map((item) => asString(item.text))
          .filter(Boolean)
          .join("\n")
      : "");
  return {
    request: {
      kind,
      model: asString(requestDoc.model) || asString(responseRecord.model) || loggedModel || undefined,
      prompt: prompt || undefined,
      content: requestDoc.content,
      duration: typeof requestDoc.duration === "string" || typeof requestDoc.duration === "number" ? requestDoc.duration : undefined,
      resolution: asString(requestDoc.resolution) || undefined,
      ratio: asString(requestDoc.ratio) || undefined,
      aspectRatio: asString(requestDoc.aspect_ratio) || undefined,
    },
    response: {
      kind,
      imageUrls,
      imageDataUrls,
      taskId: taskId || undefined,
      status: status || undefined,
      videoUrl: videoUrl || undefined,
      responseUrl: responseUrl || undefined,
      statusUrl: statusUrl || undefined,
      duration:
        typeof video.duration === "string" || typeof video.duration === "number"
          ? video.duration
          : typeof responseRecord.duration === "string" || typeof responseRecord.duration === "number"
            ? responseRecord.duration
            : undefined,
      usage: responseRecord.usage ?? result.usage,
    },
  };
};

/**
 * 用途：根据 base64 文件头选择浏览器 data URL 的图片类型。
 * 参数：value 是不含 data URL 前缀的 base64 内容。
 * 返回值：可供 img 使用的 MIME 类型。
 * 调用场景：图片供应商返回 b64_json 时由 parseMediaPayload 调用。
 * 边界：识别 PNG、GIF 和 WebP，其他格式按最常见的 JPEG 展示。
 */
const imageMime = (value: string): string => {
  if (value.startsWith("iVBOR")) return "image/png";
  if (value.startsWith("R0lGOD")) return "image/gif";
  if (value.startsWith("UklGR")) return "image/webp";
  return "image/jpeg";
};

const ROLES: readonly MessageRole[] = ["system", "user", "assistant", "tool"];

const toRole = (value: unknown, fallback: MessageRole): MessageRole => {
  if (value === "developer") return "system";
  if (value === "function") return "tool";
  return ROLES.includes(value as MessageRole) ? (value as MessageRole) : fallback;
};

const classifyRequest = (request: unknown): RequestPayload => {
  if (Array.isArray(request)) return { kind: "chat", messages: request };
  if (!isRecord(request)) return { kind: "unknown" };
  if (Array.isArray(request.messages)) return { kind: "chat", messages: request.messages };
  const { input } = request;
  if (typeof input === "string" || Array.isArray(input)) {
    return { kind: "responses", instructions: asString(request.instructions), input };
  }
  return { kind: "unknown" };
};

const classifyResponse = (response: unknown): ResponsePayload => {
  if (!isRecord(response)) return { kind: "unknown" };
  if (Array.isArray(response.choices)) return { kind: "chat", choices: response.choices };
  if (Array.isArray(response.output)) return { kind: "responses", output: response.output };
  return { kind: "unknown" };
};

/**
 * Parse request messages and response message from log data
 */
export const parseMessages = (request: unknown, response: unknown): ParsedMessages => ({
  requestMessages: parseRequestMessages(classifyRequest(requestBody(request))),
  responseMessage: parseResponseMessage(classifyResponse(loggedResponse(response))),
});

// requestHeaders and requestBody split a stored proxy document into the three
// panels the log drawer shows. Older rows are only a message list.
export const requestHeaders = (request: unknown): Record<string, string> | null => {
  if (!isRecord(request) || !isRecord(request.headers)) return null;
  const headers: Record<string, string> = {};
  for (const [key, value] of Object.entries(request.headers)) {
    if (value == null) continue;
    headers[key] = typeof value === "string" ? value : JSON.stringify(value);
  }
  return Object.keys(headers).length > 0 ? headers : null;
};

export const requestLine = (request: unknown): string | null => {
  if (!isRecord(request)) return null;
  const method = typeof request.method === "string" ? request.method : "";
  const url = typeof request.url === "string" ? request.url : "";
  const line = `${method} ${url}`.trim();
  return line.length > 0 ? line : null;
};

export const requestBody = (request: unknown): unknown => {
  if (isRecord(request) && request.body != null && (request.headers != null || request.method != null)) {
    return request.body;
  }
  return request;
};

// loggedResponse reads a stored event stream back into the final response.
// Console calls are often streamed, and the log keeps that stream under body
// when it is not one JSON document. The output panel only understands a
// completed response.
export const loggedResponse = (response: unknown): unknown => {
  const sse = eventStreamText(response);
  if (!sse) return response;
  const events = sseData(sse);
  for (let i = events.length - 1; i >= 0; i -= 1) {
    const doc = events[i];
    if (doc.type === "response.completed" && isRecord(doc.response)) return doc.response;
  }
  const deltas = events
    .filter((doc) => doc.type === "response.output_text.delta")
    .map((doc) => asString(doc.delta))
    .join("");
  if (deltas.length > 0) {
    return {
      output: [{ type: "message", role: "assistant", content: [{ type: "output_text", text: deltas }] }],
    };
  }
  let content = "";
  let reasoning = "";
  const tools = new Map<number, { id: string; name: string; args: string }>();
  for (const doc of events) {
    const choices = doc.choices;
    if (!Array.isArray(choices) || !isRecord(choices[0])) continue;
    const choice = choices[0];
    const delta = isRecord(choice.delta) ? choice.delta : undefined;
    if (typeof delta?.content === "string") content += delta.content;
    if (typeof delta?.reasoning_content === "string") reasoning += delta.reasoning_content;
    collectStreamTools(tools, delta?.tool_calls);
    const message = isRecord(choice.message) ? choice.message : undefined;
    if (typeof message?.content === "string" && message.content.length > 0) content = message.content;
    collectStreamTools(tools, message?.tool_calls);
  }
  if (content.length > 0 || reasoning.length > 0 || tools.size > 0) {
    const message: Record<string, unknown> = { role: "assistant", content };
    if (reasoning.length > 0) message.reasoning_content = reasoning;
    if (tools.size > 0) {
      message.tool_calls = [...tools.entries()]
        .sort(([a], [b]) => a - b)
        .map(([, tool]) => ({
          id: tool.id,
          type: "function",
          function: { name: tool.name, arguments: tool.args },
        }));
    }
    return { choices: [{ message }] };
  }
  return response;
};

const collectStreamTools = (
  tools: Map<number, { id: string; name: string; args: string }>,
  raw: unknown,
) => {
  if (!Array.isArray(raw)) return;
  raw.forEach((item, fallback) => {
    if (!isRecord(item)) return;
    const index = typeof item.index === "number" ? item.index : fallback;
    const current = tools.get(index) ?? { id: "", name: "", args: "" };
    if (typeof item.id === "string" && item.id.length > 0) current.id = item.id;
    const fn = isRecord(item.function) ? item.function : {};
    if (typeof fn.name === "string" && fn.name.length > 0) current.name = fn.name;
    if (typeof fn.arguments === "string") current.args += fn.arguments;
    tools.set(index, current);
  });
};

const eventStreamText = (response: unknown): string | null => {
  if (typeof response === "string" && looksLikeEventStream(response)) return response;
  if (isRecord(response) && typeof response.body === "string" && looksLikeEventStream(response.body)) {
    return response.body;
  }
  return null;
};

const looksLikeEventStream = (value: string): boolean =>
  value.includes("data:") && (value.includes("event:") || value.includes('"choices"'));

const sseData = (text: string): UnknownRecord[] => {
  const docs: UnknownRecord[] = [];
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed.startsWith("data:")) continue;
    const payload = trimmed.slice("data:".length).trim();
    if (payload.length === 0 || payload === "[DONE]") continue;
    try {
      const parsed: unknown = JSON.parse(payload);
      if (isRecord(parsed)) docs.push(parsed);
    } catch {
      // A non-JSON event line is not a response document.
    }
  }
  return docs;
};

const parseRequestMessages = (payload: RequestPayload): ParsedMessage[] => {
  switch (payload.kind) {
    case "chat":
      return payload.messages.map(parseChatMessage);
    case "responses": {
      const instructions: ParsedMessage[] = payload.instructions
        ? [{ role: "system", content: payload.instructions }]
        : [];
      const input: ParsedMessage[] =
        typeof payload.input === "string"
          ? [{ role: "user", content: payload.input }]
          : payload.input.flatMap(parseResponsesInputItem);
      return [...instructions, ...input];
    }
    case "unknown":
      return [];
  }
};

const parseResponseMessage = (payload: ResponsePayload): ParsedMessage | null => {
  switch (payload.kind) {
    case "chat": {
      const choice = payload.choices[0];
      const message = isRecord(choice) ? choice.message : undefined;
      if (!isRecord(message)) return null;
      return {
        role: toRole(message.role, "assistant"),
        content: parseMessageContent(message.content),
        toolCalls: parseChatToolCalls(message.tool_calls),
      };
    }
    case "responses": {
      const content = payload.output
        .filter((item): item is UnknownRecord => isRecord(item) && item.type === "message")
        .map((item) => parseMessageContent(item.content))
        .filter((text) => text.length > 0)
        .join("\n");
      const toolCalls = payload.output.filter(isResponsesFunctionCall).map(parseResponsesFunctionCall);
      if (content.length === 0 && toolCalls.length === 0) return null;
      return { role: "assistant", content, toolCalls: toolCalls.length > 0 ? toolCalls : undefined };
    }
    case "unknown":
      return null;
  }
};

const parseChatMessage = (message: unknown): ParsedMessage => {
  if (!isRecord(message)) return { role: "user", content: parseMessageContent(message) };
  return {
    role: toRole(message.role, "user"),
    content: parseMessageContent(message.content),
    toolCalls: parseChatToolCalls(message.tool_calls),
    toolCallId: typeof message.tool_call_id === "string" ? message.tool_call_id : undefined,
  };
};

const parseResponsesInputItem = (item: unknown): ParsedMessage[] => {
  if (typeof item === "string") return [{ role: "user", content: item }];
  if (!isRecord(item)) return [];
  if (item.type === "function_call") {
    return [{ role: "assistant", content: "", toolCalls: [parseResponsesFunctionCall(item)] }];
  }
  if (item.type === "function_call_output") {
    return [{ role: "tool", content: parseMessageContent(item.output), toolCallId: asString(item.call_id) }];
  }
  if (item.type === "reasoning") return [];
  if ("role" in item || "content" in item) {
    return [{ role: toRole(item.role, "user"), content: parseMessageContent(item.content) }];
  }
  return [];
};

const isResponsesFunctionCall = (item: unknown): item is UnknownRecord =>
  isRecord(item) && item.type === "function_call";

const parseResponsesFunctionCall = (item: UnknownRecord): ToolCall => ({
  id: asString(item.call_id) || asString(item.id),
  name: asString(item.name) || "unknown",
  arguments: parseToolArguments(item.arguments),
});

/**
 * Parse message content - handle strings and content arrays (for vision, etc.)
 */
const parseMessageContent = (content: unknown): string => {
  if (typeof content === "string") return content;
  if (content === null || content === undefined) return "";
  if (Array.isArray(content)) return content.map(parseContentPart).join("\n");
  return JSON.stringify(content);
};

const parseContentPart = (part: unknown): string => {
  if (typeof part === "string") return part;
  if (!isRecord(part)) return JSON.stringify(part);
  switch (part.type) {
    case "text":
    case "input_text":
    case "output_text":
      return asString(part.text);
    case "refusal":
      return asString(part.refusal);
    case "image_url":
    case "input_image":
      return "[Image]";
    case "input_file":
      return "[File]";
    case "input_audio":
      return "[Audio]";
    default:
      return JSON.stringify(part);
  }
};

/**
 * Parse tool calls from response message
 */
const parseChatToolCalls = (toolCalls: unknown): ToolCall[] | undefined => {
  if (!Array.isArray(toolCalls)) return undefined;
  return toolCalls.map((toolCall) => {
    const call = isRecord(toolCall) ? toolCall : {};
    const fn = isRecord(call.function) ? call.function : {};
    return {
      id: asString(call.id),
      name: asString(fn.name) || "unknown",
      arguments: parseToolArguments(fn.arguments),
    };
  });
};

/**
 * Parse tool arguments - handle both string and object formats
 */
const parseToolArguments = (args: unknown): Record<string, unknown> => {
  if (!args) return {};
  if (typeof args === "string") {
    try {
      const parsed: unknown = JSON.parse(args);
      return isRecord(parsed) ? parsed : { raw: args };
    } catch {
      return { raw: args };
    }
  }
  return isRecord(args) ? args : {};
};

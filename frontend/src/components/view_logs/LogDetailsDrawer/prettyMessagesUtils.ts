/**
 * Utility functions for parsing and formatting messages for pretty view
 */

import {
  MessageRole,
  ParsedMessage,
  ParsedMessages,
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

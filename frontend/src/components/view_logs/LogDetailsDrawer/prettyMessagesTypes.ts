/**
 * Type definitions for pretty messages view
 */

export type MessageRole = "system" | "user" | "assistant" | "tool";

export interface ParsedMessage {
  role: MessageRole;
  content: string;
  toolCalls?: ToolCall[];
  toolCallId?: string;
}

export type RequestPayload =
  | { kind: "chat"; messages: readonly unknown[] }
  | { kind: "responses"; instructions: string; input: string | readonly unknown[] }
  | { kind: "unknown" };

export type ResponsePayload =
  | { kind: "chat"; choices: readonly unknown[] }
  | { kind: "responses"; output: readonly unknown[] }
  | { kind: "unknown" };

export interface ToolCall {
  id: string;
  name: string;
  arguments: Record<string, unknown>;
}

export interface ParsedMessages {
  requestMessages: ParsedMessage[];
  responseMessage: ParsedMessage | null;
}

export interface RoleStyle {
  background: string;
  borderColor: string;
  label: string;
  labelColor: string;
}

export interface MediaRequestPayload {
  kind: "image" | "video";
  model?: string;
  prompt?: string;
  content?: unknown;
  duration?: string | number;
  resolution?: string;
  ratio?: string;
  aspectRatio?: string;
}

export interface MediaResponsePayload {
  kind: "image" | "video";
  imageUrls: string[];
  imageDataUrls: string[];
  taskId?: string;
  status?: string;
  videoUrl?: string;
  responseUrl?: string;
  statusUrl?: string;
  duration?: string | number;
  usage?: unknown;
}

export interface ParsedMediaPayload {
  request: MediaRequestPayload;
  response: MediaResponsePayload;
}

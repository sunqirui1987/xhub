import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";
import { buildMcpToolBlocks, type BuildMcpToolBlocksArgs } from "@/components/llm_calls/mcp_tool_blocks";

export type CurlBodyOptions = BuildMcpToolBlocksArgs & {
  endpoint: ModelEndpoint;
  input: string;
  messages: { role: string; content: unknown }[];
  stream: boolean;
  temperature?: number;
  maxTokens?: number;
  voice?: string;
  vectorStores?: string[];
  guardrails?: string[];
  policies?: string[];
  previousResponseId?: string | null;
  codeInterpreter?: boolean;
  mockFallbacks?: boolean;
  base: string;
};

/** 将调试界面的输入转换为协议正文；参数包括绑定、历史、生成设置及网关工具配置，返回可序列化对象。
 * 供 ChatUI curl 预览调用；Bypass 不发送网关扩展，Responses 续接仅发送新消息，Messages 使用自己的长度字段。
 * 不接收密钥，不产生请求；未知协议仅返回输入，原生编辑器使用其实际 JSON 草稿。 */
export function playgroundCurlBody(options: CurlBodyOptions): Record<string, unknown> {
  const { endpoint, input, stream } = options;
  const native = endpoint.kind === "bypass";
  const protocol = endpoint.protocol;
  if (protocol === "openai-images") return { prompt: input };
  if (protocol === "openai-audio-speech") return { input, voice: options.voice ?? "alloy" };
  if (["openai-audio-transcription", "openai-audio-translation"].includes(protocol)) return {};
  if (protocol === "openai-embeddings") return { input };
  let messages = options.messages;
  if (!messages.length) messages = [{ role: "user", content: input || "Hello" }];
  const body: Record<string, unknown> = { stream };
  if (protocol === "openai-responses") {
    body.input = native
      ? messages.map((message) => ({
          ...message,
          content: Array.isArray(message.content)
            ? message.content.map((part) =>
                part.type === "text"
                  ? { type: message.role === "assistant" ? "output_text" : "input_text", text: part.text }
                  : part.type === "image_url"
                    ? { type: "input_image", image_url: part.image_url.url }
                    : part,
              )
            : message.content,
        }))
      : messages.map((message) => ({ ...message, type: "message" }));
    if (!native && options.previousResponseId) {
      body.input = (body.input as unknown[]).slice(-1);
      body.previous_response_id = options.previousResponseId;
    }
  } else if (protocol === "anthropic-messages") {
    body.messages = native ? messages.filter((message) => message.role !== "system") : messages;
    if (native) {
      const system = messages
        .filter((message) => message.role === "system")
        .map((message) => message.content)
        .join("\n");
      if (system) body.system = system;
    }
    body.max_tokens = native ? 2048 : 1024;
  } else {
    body.messages = messages;
    if (!native && stream) body.stream_options = { include_usage: true };
    if (!native && options.temperature !== undefined) body.temperature = options.temperature;
    if (!native && options.maxTokens !== undefined) body.max_tokens = options.maxTokens;
  }
  if (!native) {
    if (options.vectorStores?.length) body.vector_store_ids = options.vectorStores;
    if (options.guardrails?.length) body.guardrails = options.guardrails;
    if (options.policies?.length) body.policies = options.policies;
    const tools: Record<string, unknown>[] = buildMcpToolBlocks(options).map((tool, index) => {
      if (protocol === "anthropic-messages") return { ...tool };
      const id = options.selectedMCPServers?.[index] ?? "__all__";
      const toolset = id.startsWith("toolset:");
      const server = options.mcpServers?.find((item) => item.server_id === id);
      const name = toolset
        ? tool.server_label
        : protocol === "openai-chat"
          ? server?.alias || server?.server_name || id
          : server?.server_name || id;
      const suffix =
        id === "__all__" ? "" : "/" + (protocol === "openai-responses" || toolset ? encodeURIComponent(name) : name);
      return {
        ...tool,
        server_label: id === "__all__" ? "litellm" : protocol === "openai-chat" && !toolset ? "litellm" : name,
        server_url:
          (protocol === "openai-responses" ? options.base.replace(/\/$/, "") : "litellm_proxy") + "/mcp" + suffix,
      };
    });
    if (protocol === "openai-responses" && options.codeInterpreter)
      tools.push({ type: "code_interpreter", container: { type: "auto" } });
    if (tools.length) {
      body.tools = tools;
      if (protocol !== "anthropic-messages") body.tool_choice = "auto";
    }
    if (protocol === "openai-chat" && options.mockFallbacks) body.mock_testing_fallbacks = true;
  }
  return body;
}

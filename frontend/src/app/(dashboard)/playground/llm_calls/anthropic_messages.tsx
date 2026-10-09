import { postLLMRequest, readJSONStream } from "@/components/llm_calls/transport";
import { MessageType } from "@/components/chat_ui/types";
import { TokenUsage } from "@/components/chat_ui/ResponseMetrics";
import { buildMcpToolBlocks } from "@/components/llm_calls/mcp_tool_blocks";
import { MCPServer, MCPToolset } from "@/components/mcp_tools/types";
import { getProxyBaseUrl } from "@/components/networking";
import { toast } from "@/lib/toast";
import { extractPromptCacheTokens, type ProviderCacheUsage } from "@/utils/promptCacheUsage";
import { t } from "@/i18n";

/** 将累计 Messages 用量映射为 UI 指标；参数为输入、输出和供应商缓存计数，返回 TokenUsage。
 * 调用场景为流式累计与非流式响应，不修改原始用量；缺失缓存字段时省略对应指标。 */
const toTokenUsage = (usage: ProviderCacheUsage & { input_tokens: number; output_tokens: number }): TokenUsage => ({
  completionTokens: usage.output_tokens,
  promptTokens: usage.input_tokens,
  totalTokens: usage.input_tokens + usage.output_tokens,
  ...extractPromptCacheTokens(usage),
});

/** 调用 Messages 协议并分派文本、推理和累计用量。
 * 参数为历史、模型、密钥、工具配置及 UI 回调；返回完成 Promise，供 Playground/对比调用。
 * 缺少密钥、HTTP 错误、流错误及取消向调用方传播。 */
export async function makeAnthropicMessagesRequest(
  messages: MessageType[],
  updateTextUI: (role: string, delta: string, model?: string) => void,
  selectedModel: string,
  accessToken: string | null,
  tags: string[] = [],
  signal?: AbortSignal,
  onReasoningContent?: (content: string) => void,
  onTimingData?: (timeToFirstToken: number) => void,
  onUsageData?: (usage: TokenUsage) => void,
  traceId?: string,
  vector_store_ids?: string[],
  guardrails?: string[],
  policies?: string[],
  selectedMCPServers?: string[],
  customBaseUrl?: string,
  mcpServers?: MCPServer[],
  mcpServerToolRestrictions?: Record<string, string[]>,
  mcpToolsets?: MCPToolset[],
  streamingEnabled: boolean = true,
) {
  if (!accessToken) {
    throw new Error(t("Virtual Key is required"));
  }

  const isLocal = process.env.NODE_ENV === "development";
  if (isLocal !== true) {
    console.log = function () {};
  }

  const proxyBaseUrl = customBaseUrl || getProxyBaseUrl();

  try {
    const startTime = Date.now();
    let firstTokenReceived = false;

    const requestBody: any = {
      model: selectedModel,
      messages: messages.map((m) => ({ role: m.role, content: m.content })),
      stream: streamingEnabled,
      max_tokens: 1024,
      // @ts-ignore - litellm specific parameter
      litellm_trace_id: traceId,
    };

    const tools = buildMcpToolBlocks({
      selectedMCPServers,
      mcpServers,
      mcpToolsets,
      mcpServerToolRestrictions,
    });
    if (tools.length > 0) requestBody.tools = tools;
    if (vector_store_ids) requestBody.vector_store_ids = vector_store_ids;
    if (guardrails) requestBody.guardrails = guardrails;
    if (policies) requestBody.policies = policies;

    const httpResponse = await postLLMRequest("v1/messages", requestBody, {
      baseUrl: proxyBaseUrl,
      accessToken,
      tags,
      signal,
      anthropic: true,
    });
    if (!streamingEnabled) {
      const message = await httpResponse.json();
      for (const block of message.content) {
        if (block.type === "text") {
          updateTextUI("assistant", block.text, selectedModel);
        } else if (block.type === "thinking" && onReasoningContent) {
          onReasoningContent(block.thinking);
        }
      }
      onUsageData?.(toTokenUsage(message.usage));
      return;
    }

    let usage = { input_tokens: 0, output_tokens: 0 };
    for await (const messageStreamEvent of readJSONStream(httpResponse, signal)) {
      // 输入用量在 message_start，输出用量在 message_delta，累计后通知 UI。
      if (messageStreamEvent.type === "message_start") usage = { ...usage, ...messageStreamEvent.message?.usage };
      // Process content block deltas
      if (messageStreamEvent.type === "content_block_delta") {
        const delta = messageStreamEvent.delta;

        // Measure time to first token
        if (!firstTokenReceived) {
          firstTokenReceived = true;
          const timeToFirstToken = Date.now() - startTime;
          if (onTimingData) {
            onTimingData(timeToFirstToken);
          }
        }

        // Handle different types of deltas
        if (delta.type === "text_delta") {
          updateTextUI("assistant", delta.text, selectedModel);
        }
        // @ts-ignore - reasoning_content might not be in the official types yet
        else if ((delta.type === "reasoning_delta" || delta.type === "thinking_delta") && onReasoningContent) {
          // @ts-ignore
          onReasoningContent(delta.thinking ?? delta.text);
        }
      }

      // Process usage data from message_delta events
      if (messageStreamEvent.type === "message_delta" && (messageStreamEvent as any).usage && onUsageData) {
        usage = { ...usage, ...messageStreamEvent.usage };
        onUsageData(toTokenUsage(usage));
      }
    }
  } catch (error) {
    if (signal?.aborted) {
    } else {
      toast.fromError(t("Error occurred while generating model response. Please try again. Error: {error}", { error }));
    }
    throw error;
  }
}

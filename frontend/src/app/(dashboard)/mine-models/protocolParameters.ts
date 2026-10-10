import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";

/** 提供当前绑定的协议参数说明；参数为真实端点，返回字段及国际化说明键。
 * 供调用文档使用；普通和 Bypass 共用协议说明，Google 模型在路径中，未知协议明确使用自备 JSON。
 * 只说明示例及常用可选字段，不承诺所有型号支持所有选项，无网络或持久化副作用。 */
export function protocolParameters(endpoint: ModelEndpoint): { field: string; description: string }[] {
  const fields: Record<string, [string, string][]> = {
    "openai-chat": [
      ["messages", "paramMessages"],
      ["stream", "paramStream"],
      ["max_tokens", "paramMaxTokens"],
    ],
    "openai-responses": [
      ["input", "paramInput"],
      ["stream", "paramStream"],
      ["max_output_tokens", "paramMaxTokens"],
    ],
    "anthropic-messages": [
      ["messages", "paramMessages"],
      ["max_tokens", "paramRequiredMaxTokens"],
      ["anthropic-version", "paramAnthropicVersion"],
    ],
    gemini: [
      ["contents", "paramContents"],
      ["generationConfig", "paramGenerationConfig"],
    ],
    vertex: [
      ["contents", "paramContents"],
      ["generationConfig", "paramGenerationConfig"],
    ],
    "openai-embeddings": [["input", "paramEmbeddingInput"]],
    "openai-completions": [
      ["prompt", "paramPrompt"],
      ["max_tokens", "paramMaxTokens"],
    ],
    "openai-moderations": [["input", "paramInput"]],
    rerank: [
      ["query", "paramQuery"],
      ["documents", "paramDocuments"],
      ["top_n", "paramTopN"],
    ],
    ark: [
      ["content", "paramArkContent"],
      ["duration / resolution / ratio", "paramVideoOptions"],
      ["id / status / content.video_url", "paramArkResult"],
    ],
    fal: [
      ["prompt", "paramPrompt"],
      ["image_url / image_urls", "paramFalImages"],
      ["request_id / status", "paramFalResult"],
    ],
    "openai-images": endpoint.path.endsWith("/edits")
      ? [
          ["image", "paramImageFile"],
          ["prompt", "paramPrompt"],
          ["size / n", "paramImageOptions"],
        ]
      : [
          ["prompt", "paramPrompt"],
          ["size / n", "paramImageOptions"],
          ["data", "paramImageResult"],
        ],
    "openai-videos": [
      ["prompt", "paramPrompt"],
      ["seconds / size", "paramVideoOptions"],
      ["id / status", "paramVideoResult"],
    ],
    "openai-audio-speech": [
      ["input", "paramInput"],
      ["voice", "paramVoice"],
      ["response_format", "paramAudioFormat"],
    ],
    "openai-audio-transcription": [
      ["file", "paramAudioFile"],
      ["language", "paramLanguage"],
    ],
    "openai-audio-translation": [["file", "paramAudioFile"]],
  };
  const rows: [string, string][] = [
    ["Authorization", "paramAuthorization"],
    ["gemini", "vertex"].includes(endpoint.protocol) ? ["model (URL)", "paramPathModel"] : ["model", "paramModel"],
    ...(fields[endpoint.protocol] ?? [["request.json", "paramCustomBody"]]),
  ];
  return rows.map(([field, description]) => ({ field, description }));
}

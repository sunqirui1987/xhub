import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";
/** nativeRequestTemplate 按已声明协议生成原生正文草稿。
 * 参数 endpoint：所选绑定；返回带缩进 JSON；调用原生编辑器和重置操作。
 * 不猜测未知协议字段；Google 模型在路径中，Ark 视频使用 content 而非通用 prompt。 */
export function nativeRequestTemplate(endpoint: ModelEndpoint): string {
  const doc =
    endpoint.protocol === "gemini" || endpoint.protocol === "vertex"
      ? { contents: [{ role: "user", parts: [{ text: "" }] }] }
      : endpoint.protocol === "openai-responses"
        ? { input: "", stream: false }
        : endpoint.protocol === "anthropic-messages"
          ? { messages: [{ role: "user", content: "" }], max_tokens: 1024 }
          : endpoint.path.includes("/contents/generations")
            ? { content: [{ type: "text", text: "" }] }
            : endpoint.family === "image" || endpoint.path.includes("/images/")
              ? { prompt: "" }
              : {};
  return JSON.stringify(doc, null, 2);
}

import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";

/** 拼接可调用的网关地址；参数为当前 API 根地址和后台绑定路径，返回绝对 URL。
 * 供接入详情和示例生成调用；保留反向代理前缀，移除末尾 /v1，非法协议或非本站路径返回 null，无副作用。 */
export function invocationURL(base: string, path: string): string | null {
  try {
    const root = new URL(base);
    if (!["http:", "https:"].includes(root.protocol) || !path.startsWith("/") || path.startsWith("//")) return null;
    root.search = "";
    root.hash = "";
    return root.href.replace(/\/$/, "").replace(/\/v1$/, "") + path;
  } catch {
    return null;
  }
}

/** 为后台明确开放的端点生成最小 curl 示例；参数为对外模型名、真实端点绑定和网关地址。
 * 返回示例或 null，供“我的模型”详情使用；仅支持已知 JSON 协议，不猜测媒体专用参数。
 * 模型名和 URL 使用 shell 单引号转义；密钥只引用环境变量，不能混入当前会话或上游凭据。 */
export function invocationCurl(model: string, endpoint: ModelEndpoint, base: string): string | null {
  const url = invocationURL(base, endpoint.path);
  if (!model.trim() || !url || endpoint.method !== "POST") return null;
  const bodies: Record<string, unknown> = {
    "openai-chat": { model, messages: [{ role: "user", content: "你好" }] },
    "openai-responses": { model, input: "你好" },
    "anthropic-messages": { model, max_tokens: 256, messages: [{ role: "user", content: "你好" }] },
    gemini: { contents: [{ role: "user", parts: [{ text: "你好" }] }] },
    vertex: { contents: [{ role: "user", parts: [{ text: "你好" }] }] },
    "openai-embeddings": { model, input: "你好" },
  };
  const body = bodies[endpoint.protocol];
  if (!body) return null;
  const quote = (value: string) => "'" + value.replaceAll("'", "'\\''") + "'";
  return [
    "curl --request POST " + quote(url),
    '  --header "Authorization: Bearer $XHUB_API_KEY"',
    "  --header 'Content-Type: application/json'",
    ...(endpoint.protocol === "anthropic-messages" ? ["  --header 'anthropic-version: 2023-06-01'"] : []),
    "  --data " + quote(JSON.stringify(body, null, 2)),
  ].join(" \\\n");
}

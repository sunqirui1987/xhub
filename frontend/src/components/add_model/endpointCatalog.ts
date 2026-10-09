/** EndpointDescriptor 为接入、Bypass 与 Fal 公开目录；配置只接受当前登记的 ID。 */
export interface EndpointDescriptor {
  id: string; label: string; category: "openai" | "vertex" | "claude" | "bypass"; paths?: string[];
  kind: "adapted" | "bypass"; protocol: string; family: string;
}
/** EndpointTransport 定义后台已实现的上游操作；表单展示全部连接兼容协议，固定路径用于选择后校验。 */
export interface EndpointTransport {
  id: string; label: string; kind: string; endpoint_id: string; protocol?: string; catalog_id?: string;
  model_group?: string; strip_prefix?: string; providers?: string[];
  actions?: { name: string; public_path: string; model?: string }[];
}
/** dialogueProtocol 判断是否具备标准对话协议转换；参数为协议，返回布尔值，供模型配置调用，无副作用。 */
export const dialogueProtocol = (protocol?: string): boolean =>
  ["openai-chat", "openai-responses", "anthropic-messages", "gemini", "vertex"].includes(protocol ?? "");
/** compatibleEndpoint 校验接入与执行组合；参数为当前目录条目，返回支持状态。
 * 对话接入允许已实现的转换，原生媒体要求独立端点一致；与后台同规则，不兼容旧声明。 */
export const compatibleEndpoint = (endpoint: EndpointDescriptor, transport?: EndpointTransport): boolean => {
  if (!transport) return false;
  if (endpoint.kind === "adapted" && dialogueProtocol(endpoint.protocol) && dialogueProtocol(transport.protocol)) return true;
  if (endpoint.protocol !== transport.protocol) return false;
  return endpoint.id === transport.endpoint_id || (endpoint.kind === "bypass" && dialogueProtocol(transport.protocol));
};
/** protocolLabel 返回上游原厂协议的名称；参数为执行配置，供选择框展示，无副作用。
 * Fal/Ark 固定模型组同时展示协议和型号，未登记协议保留后台标签。 */
export const protocolLabel = (transport: EndpointTransport): string => {
  const label = ({
  "openai-chat": "OpenAI · Chat Completions", "openai-responses": "OpenAI · Responses",
  "anthropic-messages": "Anthropic · Messages", "openai-images": transport.endpoint_id.endsWith("edit") ? "OpenAI Images · 编辑图片" : "OpenAI Images · 创建图片",
  "openai-videos": "OpenAI · Video", "openai-embeddings": "OpenAI · Embedding",
  "openai-audio-translation": "OpenAI · Audio Translation",
  "openai-audio-speech": "OpenAI · Audio Speech", "openai-audio-transcription": "OpenAI · Audio Transcription",
  "gemini": "Gemini · Generate Content", "vertex": "Vertex AI · Generate Content",
  "fal": "FAL · " + (transport.model_group ?? transport.label),
  "ark": "Ark Video · " + (transport.model_group ?? transport.label),
}[transport.protocol ?? ""] ?? transport.model_group ?? transport.label);
  // 媒体专用直通执行只有 Bypass 入口，与开放标准接口的同协议执行必须可区分。
  return transport.endpoint_id.startsWith("bypass:") && !dialogueProtocol(transport.protocol) ? label + " · Bypass" : label;
};

/** 返回固定公开接口分组；参数为目录条目，返回三类原生协议或 Bypass，供上游菜单与接口说明使用。
 * 内部执行类型不决定公开协议；Fal/Ark 属于 Bypass，不产生第四种原生接口。 */
export const publicEndpointGroup = (entry?: EndpointDescriptor): "openai" | "vertex" | "claude" | "bypass" => {
  if (!entry || entry.id.startsWith("bypass:") || ["fal", "ark", "rerank"].includes(entry.protocol)) return "bypass";
  if (["gemini", "vertex"].includes(entry.protocol)) return "vertex";
  if (entry.protocol === "anthropic-messages") return "claude";
  return "openai";
};
/** 由注册上游生成默认公开入口；参数为目录和执行配置，返回兼容的标准入口 ID。
 * 仅原生 Bypass 可用时选择其明确登记的入口；未知配置返回空数组，不猜测协议或迁移数据。 */
export const defaultEndpoints = (entries: EndpointDescriptor[], transport?: EndpointTransport): string[] => {
  const supported = entries.filter(entry => compatibleEndpoint(entry, transport));
  const standard = supported.filter(entry => publicEndpointGroup(entry) !== "bypass");
  return (standard.length ? standard : supported.filter(entry => entry.id === transport?.endpoint_id)).map(entry => entry.id);
};

/** 返回上游协议所属厂商分组；参数为注册传输，返回分组；上游执行与 Bypass 公开入口相互独立。 */
export const upstreamProtocolGroup = (transport: EndpointTransport): "openai" | "vertex" | "claude" | "bypass" => publicEndpointGroup({id:"upstream",protocol:transport.protocol ?? "",kind:"adapted",family:"",category:"openai",label:""});

/** catalogTransports 按连接目录和模型返回已支持的实现；供表单默认选择和专用协议提示使用。
 * 参数为注册传输、目录映射、目录 ID、上游 ID 和认证协议；返回可用实现。
 * 已登记型号使用专用协议；其余上架模型可选择通用协议，专用执行仍受目录与固定路径约束，不从价格猜测能力。 */
export function catalogTransports(transports: EndpointTransport[], catalogs: Record<string, Record<string, string[]>>, catalogId: string, model: string, slug: string): EndpointTransport[] {
  const id = catalogId.trim();
  const bare = id && model.startsWith(id + "/") ? model.slice(id.length + 1) : model;
  const knownCatalog = !!id && !!catalogs[id];
  const allowed = knownCatalog ? catalogs[id][bare] : undefined;
  return transports.filter(item => {
    if (knownCatalog && item.catalog_id && (item.catalog_id !== id || !allowed?.includes(item.id))) return false;
    if (item.providers?.length && !item.providers.includes(slug)) return false;
    const fixed = item.actions?.filter(action => action.name === "create" && action.model) ?? [];
    const prefix = item.strip_prefix ? item.strip_prefix + "/" : "";
    const upstream = prefix && model.startsWith(prefix) ? model.slice(prefix.length) : model;
    return !fixed.length || fixed.some(action => action.model === upstream);
  });
}

/** defaultTransport 按连接类型提供常规协议默认值，已登记媒体型号优先使用专用实现。
 * 参数为可选实现、连接协议和型号声明；返回执行 ID 或空串，供新建及切换型号调用，不覆盖手动选择。 */
export function defaultTransport(transports: EndpointTransport[], slug: string, declared?: string): string {
  if (declared && transports.some(item => item.id === declared)) return declared;
  const protocol = ["vertex", "vertex_ai"].includes(slug) ? "vertex"
    : ["gemini", "google", "google_ai_studio"].includes(slug) ? "gemini"
    : ["anthropic", "claude"].includes(slug) ? "anthropic-messages"
    : ["openai", "custom", "custom_openai"].includes(slug) ? "openai-chat" : undefined;
  return transports.find(item => !item.catalog_id && item.protocol === protocol)?.id ?? "";
}

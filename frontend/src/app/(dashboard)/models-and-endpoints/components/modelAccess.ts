import type { PriceCatalogRow } from "./priceCatalogRows";
import type { EndpointDescriptor } from "@/components/add_model/endpointCatalog";

export type MarketDeployment = {
  model_name: string;
  litellm_params?: { model?: string };
  model_info?: { base_model?: string; disabled?: boolean; endpoint_types?: string[] };
};

/** 匹配当前用户可读的启用部署；参数为目录行和部署列表，返回明确绑定价格的部署。
 * 供内部详情使用；无价格绑定时才比较上游 ID，不猜测型号或别名，无副作用。 */
export function modelDeployments(
  row: PriceCatalogRow,
  deployments: MarketDeployment[],
  includeDisabled = false,
): MarketDeployment[] {
  return deployments.filter(
    (item) =>
      (includeDisabled || !item.model_info?.disabled) &&
      (item.model_info?.base_model ? item.model_info.base_model === row.id : item.litellm_params?.model === row.id),
  );
}

/** 生成内部对话示例；参数为对外部署、登记入口和网关地址，返回 curl 或 null。
 * 媒体及未知协议不猜测请求体，密钥仅用环境变量；模型名和地址进行 shell 转义，供 API 接入标签使用。 */
export function modelCurl(deployment: MarketDeployment, endpoint: EndpointDescriptor, origin: string): string | null {
  const bodies: Record<string, unknown> = {
    "openai-chat": { model: deployment.model_name, messages: [{ role: "user", content: "你好" }] },
    "openai-responses": { model: deployment.model_name, input: "你好" },
    "anthropic-messages": {
      model: deployment.model_name,
      max_tokens: 256,
      messages: [{ role: "user", content: "你好" }],
    },
  };
  if (
    !bodies[endpoint.protocol] ||
    !endpoint.paths?.length ||
    !deployment.model_info?.endpoint_types?.includes(endpoint.id)
  )
    return null;
  const quote = (value: string) => "'" + value.replaceAll("'", "'\\''") + "'";
  return [
    "curl " + quote(origin.replace(/\/$/, "") + endpoint.paths[0]),
    '  -H "Authorization: Bearer $XHUB_API_KEY"',
    "  -H 'Content-Type: application/json'",
    ...(endpoint.protocol === "anthropic-messages" ? ["  -H 'anthropic-version: 2023-06-01'"] : []),
    "  -d " + quote(JSON.stringify(bodies[endpoint.protocol], null, 2)),
  ].join(" \\\n");
}

/** 筛选文档地址；参数为未知 URL，返回 http(s) 地址或 null；详情链接使用，无网络请求。 */
export function marketDocUrl(value: unknown): string | null {
  if (typeof value !== "string") return null;
  try {
    const url = new URL(value);
    return ["https:", "http:"].includes(url.protocol) ? url.href : null;
  } catch {
    return null;
  }
}

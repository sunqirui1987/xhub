"use client";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { apiClient, getProxyBaseUrl, modelInfoCall } from "@/components/networking";
import type { EndpointDescriptor } from "@/components/add_model/endpointCatalog";
import type { PriceCatalogRow } from "./priceCatalogRows";
import { marketMoney, marketPrices, marketModalities, MODALITY_NAMES, marketStrings } from "./marketCatalog";
import { modelDeployments, modelCurl, marketDocUrl, type MarketDeployment } from "./modelAccess";

/** 读取当前用户可见的完整部署与端点；参数为会话，返回部署和公开端点。
 * 供内部详情使用，分页读取避免漏掉部署；读取失败交由查询错误状态展示，不误报为未部署。 */
export async function loadMarketAccess(token: string, userId: string, role: string) {
  const endpoints = await apiClient.get<{ endpoint_types?: EndpointDescriptor[] }>("/public/endpoints");
  const deployments: MarketDeployment[] = [];
  for (let page = 1; ; page++) {
    const result = await modelInfoCall(token, userId, role, page, 1000);
    if (!result || !Array.isArray(result.data)) throw new Error("部署列表读取失败");
    deployments.push(...result.data);
    if (page >= (result.total_pages ?? 1) || !result.data.length) break;
  }
  return { deployments, endpoints: endpoints?.endpoint_types ?? [] };
}

/** 内部模型详情；参数为价格行、会话和关闭回调，返回全部价格档位与实际接入信息。
 * 仅登录且弹窗打开时读取部署；未部署、禁用和未知媒体协议不生成示例，不读取供应商密钥。 */
export function MarketModelDetail({
  row,
  accessToken,
  userId,
  userRole,
  onClose,
}: {
  row: PriceCatalogRow | null;
  accessToken: string | null;
  userId: string | null;
  userRole: string;
  onClose: () => void;
}) {
  const access = useQuery({
    queryKey: ["marketModelAccess", accessToken, userId],
    queryFn: () => loadMarketAccess(accessToken!, userId ?? "", userRole),
    enabled: !!row && !!accessToken,
    staleTime: 0,
  });
  if (!row) return null;
  const deployments = modelDeployments(row, access.data?.deployments ?? []);
  const prices = marketPrices(row);
  const origin = getProxyBaseUrl() || (typeof window !== "undefined" ? window.location.origin : "");
  const links = [
    ["原厂定价说明", row.raw.pricing_page_url],
    ["模型文档", row.raw.model_doc_url],
    ["原厂接入文档", row.raw.integration_doc_url],
  ];
  const accessState = !accessToken ? (
    <p>登录后查看内部接入信息。</p>
  ) : access.isPending ? (
    <p role="status">正在读取部署信息…</p>
  ) : access.isError ? (
    <p role="alert">
      接入信息读取失败。
      <button onClick={() => void access.refetch()} className="ml-2 underline">
        重试
      </button>
    </p>
  ) : !deployments.length ? (
    <p>尚未部署此模型。请先配置供应商、上游协议和对外模型名称，保存并启用后查看可调用接口。</p>
  ) : null;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>{row.displayName || row.id}</DialogTitle>
          <DialogDescription>{row.id} · 内部模型资料</DialogDescription>
        </DialogHeader>
        <p className="text-sm leading-6 text-muted-foreground">{String(row.raw.description || "")}</p>
        <Tabs defaultValue="pricing">
          <TabsList className="mb-5 flex flex-wrap">
            <TabsTrigger value="pricing">模型价格</TabsTrigger>
            <TabsTrigger value="access">接入信息</TabsTrigger>
            <TabsTrigger value="api">API 接入</TabsTrigger>
            <TabsTrigger value="docs">调用文档</TabsTrigger>
          </TabsList>
          <TabsContent value="pricing" className="space-y-4">
            {row.overridden && (
              <div className="rounded border p-4">
                <p>当前使用手工定价；以下市场档位供参考。</p>
                <p>
                  生效输入：{marketMoney(row.input)} / 1M tokens；生效输出：{marketMoney(row.output)} / 1M tokens
                </p>
                {row.extraPrices.map((price) => (
                  <p key={price.key}>
                    {price.key}：{marketMoney(price.value)} / {price.unit}
                  </p>
                ))}
              </div>
            )}
            <p className="text-xs text-muted-foreground">
              市场公布价，保留全部档位、变体和币种。未提供价格不会显示为免费。实际账单按网关生效定价和用量计算。
            </p>
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <caption className="sr-only">完整市场价格</caption>
                <thead>
                  <tr className="border-b">
                    <th className="p-3">计费项目 / 档位</th>
                    <th className="p-3">美元</th>
                    <th className="p-3">人民币</th>
                    <th className="p-3">单位</th>
                  </tr>
                </thead>
                <tbody>
                  {prices.map((price) => (
                    <tr className="border-b" key={price.tier + ":" + price.key}>
                      <td className="p-3">
                        {price.label}
                        <span className="block text-xs text-muted-foreground">{price.range || "默认档位"}</span>
                      </td>
                      <td className="p-3">{marketMoney(price.usd)}</td>
                      <td className="p-3">{marketMoney(price.cny, "¥")}</td>
                      <td className="p-3">{price.unit}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {!prices.length && <p>市场尚未提供定价。</p>}
          </TabsContent>
          <TabsContent value="access" className="space-y-4">
            <dl className="grid gap-3 text-sm sm:grid-cols-2">
              <div>
                <dt className="text-muted-foreground">供应商</dt>
                <dd>{String(row.raw.issuer || row.provider || "自定义")}</dd>
              </div>
              {(["input", "output"] as const).map((side) => (
                <div key={side}>
                  <dt className="text-muted-foreground">{side === "input" ? "输入" : "输出"}模态</dt>
                  <dd>
                    {marketModalities(row, side)
                      .map((m) => MODALITY_NAMES[m] || m)
                      .join("、") || "未声明"}
                  </dd>
                </div>
              ))}
              <div>
                <dt className="text-muted-foreground">市场声明协议</dt>
                <dd>{marketStrings(row.raw.support_api_protocols).join("、") || "未声明，以实际部署为准"}</dd>
              </div>
            </dl>
            {accessState}
            {deployments.map((item, i) => (
              <div key={i} className="rounded border p-3 text-sm">
                <p>对外模型名称：{item.model_name}</p>
                <p className="break-all text-muted-foreground">上游模型：{item.litellm_params?.model}</p>
                <p>已开放接口：{item.model_info?.endpoint_types?.join("、") || "未声明"}</p>
              </div>
            ))}
            <Link
              className="inline-block text-cyan-700 underline"
              href={"/models-and-endpoints?catalog=" + encodeURIComponent(row.id)}
            >
              配置并部署此模型
            </Link>
          </TabsContent>
          <TabsContent value="api" className="space-y-4">
            <p className="text-sm">
              网关地址：<code className="break-all">{origin}</code>
            </p>
            <p className="text-sm text-muted-foreground">
              使用具备模型权限的 XHub API Key，并设置 XHUB_API_KEY 环境变量。目录 ID
              用于价格绑定；请求使用对外模型名称。
            </p>
            {accessState}
            {deployments.map((item, i) => (
              <div key={i} className="space-y-3">
                <h3 className="font-semibold">{item.model_name}</h3>
                {(access.data?.endpoints ?? [])
                  .filter((endpoint) => item.model_info?.endpoint_types?.includes(endpoint.id))
                  .map((endpoint) => {
                    const example = modelCurl(item, endpoint, origin);
                    return (
                      <div key={endpoint.id} className="rounded border p-3">
                        <p className="font-medium">{endpoint.label}</p>
                        {endpoint.paths?.map((path) => (
                          <code className="block break-all text-xs" key={path}>
                            {path}
                          </code>
                        ))}
                        {example ? (
                          <pre className="mt-3 overflow-x-auto rounded bg-muted p-4 text-xs">{example}</pre>
                        ) : (
                          <p className="mt-2 text-sm text-muted-foreground">
                            该接口使用专用请求格式，请根据端点文档构造请求。
                          </p>
                        )}
                      </div>
                    );
                  })}
              </div>
            ))}
          </TabsContent>
          <TabsContent value="docs" className="space-y-4">
            <p className="text-sm">
              内部调用流程：部署模型 → 开放接口 → 创建具有模型权限的 API Key → 按 API 接入中的路径和对外模型名称调用。
            </p>
            <p className="text-sm text-muted-foreground">
              401：检查密钥；403：检查模型权限；404：检查路径和对外模型名称；429：检查配额；上游错误：检查供应商配置和支持状态。视频任务按专用接口文档提交、查询和获取结果。
            </p>
            <ul className="space-y-2">
              {links.map(([label, value]) => {
                const url = marketDocUrl(value);
                return url ? (
                  <li key={String(label)}>
                    <a href={url} target="_blank" rel="noopener noreferrer" className="text-cyan-700 underline">
                      {String(label)}
                    </a>
                  </li>
                ) : null;
              })}
            </ul>
            <p className="text-xs text-muted-foreground">
              原厂文档用于核对能力和参数；内部地址与认证以实际部署信息为准。
            </p>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}

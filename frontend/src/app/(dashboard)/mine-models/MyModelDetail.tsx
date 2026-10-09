"use client";

import { useState } from "react";
import Link from "next/link";
import { Check, Copy, Terminal } from "lucide-react";
import { t } from "@/i18n";
import { getProxyBaseUrl } from "@/components/networking";
import { endpointLabel } from "@/components/llm_calls/model_endpoints";
import { Button, buttonVariants } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { uiHref } from "@/utils/uiHref";
import type { MyModelCard } from "./grantedModelCards";
import { invocationCurl, invocationURL } from "./modelInvocation";

export type ModelDetailTab = "pricing" | "access" | "api" | "docs";

/** 展示可复制字段；参数为文本和按钮名称，返回复制按钮。
 * 供模型接入详情调用；点击时写剪贴板，失败显示可观察提示，不自动读取或复制任何密钥。 */
function CopyValue({ value, label }: { value: string; label: string }) {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");
  /** 复制当前字段；无参数，返回异步完成状态；仅用户点击时写剪贴板，失败保留正文以便手动复制。 */
  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      setState("copied");
    } catch {
      setState("failed");
    }
  }
  return (
    <span className="inline-flex flex-wrap items-center gap-2">
      <Button size="sm" variant="outline" aria-label={label} onClick={() => void copy()}>
        {state === "copied" ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}
        {label}
      </Button>
      {state === "copied" && (
        <span role="status" className="text-xs text-emerald-700 dark:text-emerald-300">
          {t("myModels.copied")}
        </span>
      )}
      {state === "failed" && (
        <span role="alert" className="text-xs text-destructive">
          {t("myModels.copyFailed")}
        </span>
      )}
    </span>
  );
}

/** 我的模型调用详情；参数为授权模型、首次打开的标签及关闭回调，返回四标签弹窗。
 * 仅使用 /model/available 的公开绑定和参考费率，无额外管理接口请求；无绑定或错误配置不生成示例。
 * 专用媒体协议展示真实方法、路径和任务动作，已知 JSON 协议提供可复制 curl；关闭后由列表恢复焦点。 */
export function MyModelDetail({
  model,
  initialTab,
  onClose,
}: {
  model: MyModelCard;
  initialTab: ModelDetailTab;
  onClose: () => void;
}) {
  const [tab, setTab] = useState<ModelDetailTab>(initialTab);
  const endpoints = model.unavailable_reason ? [] : (model.endpoints ?? []);
  const [path, setPath] = useState(endpoints[0]?.path ?? "");
  const endpoint = endpoints.find((item) => item.path === path) ?? endpoints[0];
  const rawBase = getProxyBaseUrl();
  const base =
    typeof window === "undefined" ? rawBase : new URL(rawBase || "/", window.location.origin).href.replace(/\/$/, "");
  const example = endpoint ? invocationCurl(model.id, endpoint, base) : null;
  const schema = invocationURL(base, "/openapi.json");
  const unavailable = !endpoints.length;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-4xl">
        <DialogHeader className="pr-8">
          <div className="mb-2 flex items-center gap-2 text-xs text-muted-foreground">
            <Terminal className="size-4" aria-hidden="true" />
            XHub API<Badge variant="outline">{model.provider || t("myModels.unknownProvider")}</Badge>
          </div>
          <DialogTitle className="break-all text-xl leading-snug">{model.id}</DialogTitle>
          <DialogDescription>{t("myModels.detailDescription")}</DialogDescription>
        </DialogHeader>
        {/* 允许弹窗网格子项收缩，长命令只在代码区滚动；双行标签覆盖基础组件的固定高度。 */}
        <Tabs className="min-w-0" value={tab} onValueChange={(value) => setTab(value as ModelDetailTab)}>
          <TabsList className="mb-4 grid w-full grid-cols-2 group-data-horizontal/tabs:h-auto sm:grid-cols-4">
            {(["pricing", "access", "api", "docs"] as const).map((value) => (
              <TabsTrigger key={value} value={value} className="min-h-9">
                {t("myModels." + value)}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value="pricing" className="space-y-4">
            <p className="text-sm leading-6 text-muted-foreground">{t("myModels.priceNote")}</p>
            <div className="overflow-x-auto rounded-lg border">
              <table className="w-full text-left text-sm">
                <caption className="sr-only">{t("myModels.pricing")}</caption>
                <thead className="bg-muted/50">
                  <tr>
                    <th className="p-4">{t("myModels.item")}</th>
                    <th className="p-4">{t("myModels.rate")}</th>
                  </tr>
                </thead>
                <tbody>
                  {[
                    [t("Input"), model.input_price],
                    [t("Output"), model.output_price],
                    [t("Cache Read"), model.cache_read_price],
                    [t("Cache Write"), model.cache_write_price],
                  ].map(([label, value]) => (
                    <tr key={String(label)} className="border-t">
                      <td className="p-4">{label}</td>
                      <td className="p-4 font-mono">
                        {typeof value === "number"
                          ? "$" + value.toLocaleString("en-US", { maximumSignificantDigits: 8 })
                          : t("Price unavailable")}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </TabsContent>
          <TabsContent value="access" className="space-y-5">
            <dl className="grid gap-4 rounded-lg border bg-muted/20 p-4 sm:grid-cols-2">
              <div className="space-y-2">
                <dt className="text-xs text-muted-foreground">{t("myModels.modelName")}</dt>
                <dd className="break-all font-mono">{model.id}</dd>
                <CopyValue value={model.id} label={t("myModels.copyModel")} />
              </div>
              <div className="space-y-2">
                <dt className="text-xs text-muted-foreground">{t("myModels.gateway")}</dt>
                <dd className="break-all font-mono">{base}</dd>
                <CopyValue value={base} label={t("myModels.copyGateway")} />
              </div>
              <div className="space-y-2 sm:col-span-2">
                <dt className="text-xs text-muted-foreground">{t("myModels.authentication")}</dt>
                <dd>
                  <code>Authorization: Bearer &lt;XHUB_API_KEY&gt;</code>
                  <p className="mt-2 text-muted-foreground">{t("myModels.authHint")}</p>
                </dd>
              </div>
            </dl>
            <h3 className="font-medium">{t("myModels.endpoints")}</h3>
            {endpoints.map((item) => (
              <p key={item.method + item.path} className="break-all rounded-lg border p-3 font-mono text-xs">
                {item.method} {item.path}
              </p>
            ))}
            {unavailable && (
              <p role="alert" className="rounded-lg border border-amber-500/30 p-4">
                {t("myModels.noEndpoints")}
                {model.unavailable_reason && (
                  <span className="mt-2 block break-all text-xs text-muted-foreground">{model.unavailable_reason}</span>
                )}
              </p>
            )}
          </TabsContent>
          <TabsContent value="api" className="min-w-0 space-y-4">
            <p className="text-sm leading-6 text-muted-foreground">{t("myModels.exampleHint")}</p>
            {unavailable ? (
              <p role="alert" className="rounded-lg border border-amber-500/30 p-4">
                {t("myModels.noEndpoints")}
                {model.unavailable_reason && (
                  <span className="mt-2 block break-all text-xs">{model.unavailable_reason}</span>
                )}
              </p>
            ) : (
              <>
                <label className="block space-y-2 font-medium">
                  {t("myModels.selectEndpoint")}
                  <select
                    aria-label={t("myModels.selectEndpoint")}
                    className="block h-11 w-full min-w-0 rounded-md border bg-background px-3 font-normal"
                    value={endpoint?.path}
                    onChange={(event) => setPath(event.target.value)}
                  >
                    {endpoints.map((item) => (
                      <option key={item.method + item.path} value={item.path}>
                        {endpointLabel(item)}
                      </option>
                    ))}
                  </select>
                </label>
                <div className="min-w-0 overflow-hidden rounded-lg border">
                  <div className="flex flex-wrap items-center justify-between gap-3 border-b bg-muted/30 px-4 py-3">
                    <code className="min-w-0 break-all text-xs">
                      {endpoint?.method} {endpoint?.path}
                    </code>
                    {example && <CopyValue key={example} value={example} label={t("myModels.copyExample")} />}
                  </div>
                  {example ? (
                    <pre
                      aria-label={t("myModels.copyExample")}
                      className="max-h-80 overflow-auto bg-slate-950 p-5 text-xs leading-6 text-slate-100"
                    >
                      <code>{example}</code>
                    </pre>
                  ) : (
                    <p className="p-5 leading-6 text-muted-foreground">{t("myModels.specialized")}</p>
                  )}
                </div>
                {!!endpoint?.actions?.length && (
                  <div className="space-y-2">
                    <h3 className="font-medium">{t("myModels.actions")}</h3>
                    {endpoint.actions.map((action) => (
                      <p
                        key={action.name + action.public_path}
                        className="break-all rounded border p-3 font-mono text-xs"
                      >
                        {action.name} · {action.method} {action.public_path}
                        {action.task_query ? " · " + action.task_query : ""}
                      </p>
                    ))}
                  </div>
                )}
              </>
            )}
            <div className="flex flex-wrap gap-3">
              <Link href={uiHref("api-keys")} className={buttonVariants({ variant: "outline", size: "sm" })}>
                {t("myModels.keys")}
              </Link>
              {schema && (
                <a
                  href={schema}
                  target="_blank"
                  rel="noopener noreferrer"
                  className={buttonVariants({ variant: "ghost", size: "sm" })}
                >
                  {t("myModels.apiSchema")}
                </a>
              )}
            </div>
          </TabsContent>
          <TabsContent value="docs" className="space-y-4">
            <div className="rounded-lg border p-4">
              <h3 className="mb-2 font-medium">{t("myModels.stepKey")}</h3>
              <p className="leading-6 text-muted-foreground">{t("myModels.stepKeyHint")}</p>
              <pre className="my-3 overflow-x-auto rounded bg-muted p-3 text-xs">
                {"export XHUB_API_KEY='<YOUR_XHUB_API_KEY>'"}
              </pre>
              <Link href={uiHref("api-keys")} className="text-cyan-700 underline dark:text-cyan-300">
                {t("myModels.keys")}
              </Link>
            </div>
            <div className="rounded-lg border p-4">
              <h3 className="mb-2 font-medium">{t("myModels.stepCall")}</h3>
              <p className="leading-6 text-muted-foreground">{t("myModels.stepCallHint")}</p>
              <Button className="mt-3" variant="outline" size="sm" onClick={() => setTab("api")}>
                {t("myModels.api")}
              </Button>
            </div>
            <div className="rounded-lg border p-4">
              <h3 className="mb-2 font-medium">{t("myModels.stepResult")}</h3>
              <p className="leading-6 text-muted-foreground">{t("myModels.stepResultHint")}</p>
            </div>
            <div className="rounded-lg bg-muted/50 p-4">
              <h3 className="mb-2 font-medium">{t("myModels.errors")}</h3>
              <p className="text-xs leading-6 text-muted-foreground">{t("myModels.errorHint")}</p>
            </div>
          </TabsContent>
        </Tabs>
        <div className="flex justify-end border-t pt-4">
          <Link href={uiHref("playground")} className={buttonVariants({ variant: "outline" })}>
            {t("myModels.useInPlayground")}
          </Link>
        </div>
      </DialogContent>
    </Dialog>
  );
}

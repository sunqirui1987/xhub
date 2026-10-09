import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { Select, SelectContent, SelectGroup, SelectLabel, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { MountedFormField, type MountedFormValues } from "../common_components/MountedFormField";
import { apiClient } from "../networking";
import { provider_map } from "../provider_info_helpers";
import { catalogTransports, defaultTransport, compatibleEndpoint, protocolLabel, upstreamProtocolGroup, publicEndpointGroup, defaultEndpoints, type EndpointDescriptor, type EndpointTransport } from "./endpointCatalog";

/** 公开目录仅声明已实现的上游操作和固定模型绑定，不用定价信息猜测能力。 */
interface EndpointPayload { endpoint_types?: EndpointDescriptor[]; transports?: EndpointTransport[]; models?: Record<string, string>; catalogs?: Record<string, Record<string, string[]>> }

/** 模型编辑先选择上游实际支持的调用方式，再展示由执行能力生成的固定 XHub 接口。
 * 参数 selectedProvider 为连接协议，catalogId 为已保存供应商目录；返回上游选择与公开接口说明，供 ModelEditor 使用。
 * 普通型号按连接默认选择协议；菜单保留连接支持的全部实现，专用型号不匹配时显示原因，目录失败保留草稿。 */
const EndpointTypeField: React.FC<{ selectedProvider: string | null; catalogId?: string; modelCostMap?: Record<string, { endpoint_id?: string }> | null }> = ({ selectedProvider, catalogId = "" }) => {
  const { control, getValues, setValue } = useFormContext<MountedFormValues>();
  const lastModel = useRef<unknown>(undefined);
  const [payload, setPayload] = useState<EndpointPayload>({});
  const [state, setState] = useState<"loading" | "ready" | "failed">("loading");
  const transportId = useWatch({control, name:"transport"}) as string | undefined;
  const entries = useWatch({control, name:"endpoint_types"}) as string[] | undefined;
  const modelValue = useWatch({control, name:"model"});
  const picked = Array.isArray(modelValue) ? modelValue[0] : modelValue;
  const slug = selectedProvider ? (provider_map[selectedProvider] ?? selectedProvider.toLowerCase()) : "";
  useEffect(() => {
    let canceled = false;
    apiClient.get<EndpointPayload>("/public/endpoints").then(body => {
      if (!canceled && body) { setPayload(body); setState("ready"); }
    }).catch(() => { if (!canceled) setState("failed"); });
    return () => { canceled = true; };
  }, []);
  const transports = useMemo(() => catalogTransports(payload.transports ?? [], payload.catalogs ?? {}, catalogId,
    typeof picked === "string" ? picked : "", slug), [payload.transports, payload.catalogs, catalogId, picked, slug]);
  // 选择菜单展示连接可用的全部已实现协议；型号限制在选择后明确解释，并由保存接口再次校验。
  const choices = useMemo(() => (payload.transports ?? []).filter(item => !item.providers?.length || item.providers.includes(slug)), [payload.transports, slug]);
  const binding = choices.find(item => item.id === transportId);
  const supported = !binding || transports.some(item => item.id === binding.id);
  const bareModel = typeof picked === "string" && catalogId && picked.startsWith(catalogId + "/") ? picked.slice(catalogId.length + 1) : String(picked ?? "");
  const declaredTransports = payload.catalogs?.[catalogId]?.[bareModel];
  const unavailableReason = declaredTransports?.length
      ? `当前连接类型 ${slug} 不支持该模型登记的上游协议，暂时无法保存。请检查供应商连接类型。`
      : "目录未提供可用的上游接口协议，暂时无法保存该模型。请检查上游模型 ID 和供应商连接类型。";
  const available = (payload.endpoint_types ?? []).filter(entry => compatibleEndpoint(entry, binding));
  const standard = available.filter(entry => publicEndpointGroup(entry) !== "bypass");
  const bypass = available.filter(entry => publicEndpointGroup(entry) === "bypass");
  /** 写入上游执行及可用公开接口；参数为注册执行 ID，返回无。
   * 只开放后端已实现的标准接口，Bypass 须显式勾选；空值清空旧声明，供选择和模型切换调用。 */
  const selectTransport = useCallback((id: string, dirty = true) => {
    const execution = choices.find(item => item.id === id);
    const endpoints = defaultEndpoints(payload.endpoint_types ?? [], execution);
    if (getValues("transport") !== id) setValue("transport", id, {shouldDirty:dirty});
    if (JSON.stringify(getValues("endpoint_types")) !== JSON.stringify(endpoints)) setValue("endpoint_types", endpoints, {shouldDirty:dirty});
  }, [getValues, setValue, choices, payload.endpoint_types]);
  useEffect(() => {
    if (state !== "ready") return;
    const changed = lastModel.current !== undefined && lastModel.current !== picked;
    lastModel.current = picked;
    const current = getValues("transport") as string;
    if (picked && !changed && choices.some(item => item.id === current)) return;
    const bare = typeof picked === "string" && picked.startsWith(catalogId + "/") ? picked.slice(catalogId.length + 1) : picked;
    const declared = catalogId ? payload.catalogs?.[catalogId]?.[String(bare)]?.[0]
      : typeof picked === "string" ? payload.models?.[picked] ?? payload.models?.[slug + "/" + picked] : undefined;
    selectTransport(picked ? defaultTransport(transports, slug, declared) : "", false);
  }, [state, picked, slug, catalogId, transports, choices, payload.models, payload.catalogs, getValues, selectTransport]);
  return <>
    <MountedFormField label="上游接口协议" name="transport" className="mb-1">{field => <Select
      items={choices.map(item => ({value:item.id,label:protocolLabel(item)}))}
      value={transportId ?? ""} disabled={!picked || state !== "ready" || !choices.length}
      onValueChange={value => selectTransport(value ?? "")}
    >
      <SelectTrigger id={field.id} aria-label="上游接口协议"><SelectValue placeholder={state === "loading" ? "正在加载端点目录…" : "请选择上游实际支持的调用方式"}/></SelectTrigger>
      <SelectContent>{(["openai","vertex","claude","bypass"] as const).map(group => <SelectGroup key={group}>
        <SelectLabel>{{openai:"OpenAI",vertex:"Vertex / Gemini",claude:"Claude / Anthropic",bypass:"Bypass 转发（含 Fal）"}[group]}</SelectLabel>
        {choices.filter(item => upstreamProtocolGroup(item) === group).map(item => <SelectItem key={item.id} value={item.id}>{protocolLabel(item)}</SelectItem>)}
      </SelectGroup>)}</SelectContent>
    </Select>}</MountedFormField>
    <p className="text-sm text-muted-foreground mb-5">默认按提供商选择常规协议，可切换为上游实际支持的 OpenAI、Vertex / Gemini 或 Anthropic 协议。FAL、Ark 和 Bypass 直通使用已实现的操作路径。</p>
    {state === "failed" && <p role="alert">端点目录加载失败，请重新打开编辑页面。已保存的端点声明已保留。</p>}
    {state === "ready" && picked && !binding && <p role="status" className="mb-5">{choices.length ? "请选择上游实际支持的调用协议。常规模型无需登记专用能力。" : unavailableReason}</p>}
    {binding && !supported && <p role="alert" className="mb-5 text-destructive">所选 {protocolLabel(binding)} 尚未实现当前模型或目录的调用。请填写该协议支持的上游模型路径，或选择 OpenAI、Vertex / Gemini 等常规协议。保存时会校验此专用能力。</p>}
    {state === "ready" && (catalogId === "qiniu" || binding?.protocol === "fal") && <p className="text-sm text-muted-foreground mb-5">FAL 使用具体任务模型路径，例如 bytedance/seedance-2.0/text-to-video 或 byteplus/seedance-2.0/text-to-video；bytedance/doubao-seedance-2-0-260128 对应 Ark Video。修改对外模型名称不会改变上游协议，请在上游模型中选择对应路径。</p>}
    {binding && <section aria-label="XHub 对外接口" className="mb-5 min-w-0 space-y-4 rounded-lg border bg-muted/20 p-4">
      <div className="space-y-1">
        <p className="font-medium">XHub 对外接口</p>
        <p className="text-xs leading-relaxed text-muted-foreground">{standard.length ? "标准接口已自动开放，可使用同一个对外模型名称调用。" : "此模型使用供应商专用协议，以下直通接口已按注册能力开放。"}</p>
      </div>
      <ul aria-label="已开放接口" className="space-y-3">
        {standard.map(entry => <li key={entry.id} aria-label={entry.label} className="min-w-0 rounded-md border bg-background p-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-sm font-medium">{entry.label}</span>
            <span className="rounded bg-primary/10 px-2 py-0.5 text-xs text-primary">已开放</span>
          </div>
          <div className="mt-2 space-y-1.5">{entry.paths?.map(path => <code key={path} className="block break-all rounded bg-muted/60 px-2 py-1 font-mono text-xs leading-relaxed text-muted-foreground">{path}</code>)}</div>
        </li>)}
      </ul>
      {bypass.length > 0 && <div className="space-y-2 border-t pt-3">
        <p className="text-xs font-medium text-muted-foreground">可选直通接口</p>
        {bypass.map(entry => <label key={entry.id} className="flex cursor-pointer items-start gap-3 rounded-md border bg-background p-3 text-sm"><input type="checkbox" className="mt-0.5 size-4 shrink-0 accent-primary" aria-label={entry.label} checked={entries?.includes(entry.id) ?? false} onChange={event => {
          const next = event.target.checked ? [...new Set([...(entries ?? []), entry.id])] : (entries ?? []).filter(id => id !== entry.id);
          // 仅有 Bypass 的供应商必须保留入口，否则部署不能调用。
          if (next.length) setValue("endpoint_types", next, {shouldDirty:true});
        }}/><span className="min-w-0 flex-1 space-y-1"><span className="block font-medium">{entry.label}</span>{entry.paths?.map(path => <code key={path} className="block break-all font-mono text-xs leading-relaxed text-muted-foreground">{path}</code>)}</span></label>)}
      </div>}
    </section>}
  </>;
};
export default EndpointTypeField;

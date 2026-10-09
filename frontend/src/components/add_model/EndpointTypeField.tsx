import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { MountedFormField, type MountedFormValues } from "../common_components/MountedFormField";
import { apiClient } from "../networking";
import { provider_map } from "../provider_info_helpers";
import { t } from "@/i18n";

/** EndpointDescriptor 描述公开协议，和供应商执行传输分开选择。 */
interface EndpointDescriptor {
  id: string;
  label: string;
  kind: "adapted" | "bypass";
  protocol: string;
  family: string;
}
/** EndpointTransport 描述可用供应商、固定模型路径及其任务队列；不包含凭据。 */
interface EndpointTransport {
  id: string;
  label: string;
  kind: string;
  endpoint_id: string;
  protocol?: string;
  model_group?: string;
  strip_prefix?: string;
  providers?: string[];
  actions?: { name: string; public_path: string; model?: string }[];
}
/** EndpointPayload 是 /public/endpoints 的只读声明目录；models 映射已登记模型的默认传输。 */
interface EndpointPayload {
  endpoint_types?: EndpointDescriptor[];
  transports?: EndpointTransport[];
  models?: Record<string, string>;
}

/** EndpointTypeField 在选择上游模型后显示可配置的协议和执行传输。
 * 参数 selectedProvider：供应商标识。返回：端点与模型组表单字段。
 * 已登记模型受白名单约束；未知模型能力由管理员显式声明，不从价格目录推断。
 * endpoint_types 声明公开能力，transport 决定执行协议。调用：ModelEditor。
 */
const EndpointTypeField: React.FC<{
  selectedProvider: string | null;
  modelCostMap?: Record<string, { endpoint_id?: string }> | null;
}> = ({ selectedProvider }) => {
  const { control, getValues, setValue } = useFormContext<MountedFormValues>();
  const lastModel = useRef<unknown>(undefined);
  const [payload, setPayload] = useState<EndpointPayload>({});
  const [catalogState, setCatalogState] = useState<"loading" | "ready" | "failed">("loading");

  const transportId = useWatch({ control, name: "transport" }) as string | undefined;
  const enabledEntries = useWatch({ control, name: "endpoint_types" }) as string[] | undefined;
  const selectedId = enabledEntries?.[0];
  const modelValue = useWatch({ control, name: "model" });
  const slug = selectedProvider ? (provider_map[selectedProvider] ?? selectedProvider.toLowerCase()) : "";
  // 组件卸载后忽略目录响应；请求失败保持不可选，禁止补猜端点。
  useEffect(() => {
    let canceled = false;
    apiClient
      .get<EndpointPayload>("/public/endpoints")
      .then((body) => {
        if (!canceled && body) {
          setPayload(body);
          setCatalogState("ready");
        }
      })
      .catch(() => { if (!canceled) setCatalogState("failed"); });
    return () => {
      canceled = true;
    };
  }, []);
  const picked = Array.isArray(modelValue) ? modelValue[0] : modelValue;
  // 固定模型路径必须逐项匹配，不能因为同属视频协议就提供其他模型组的传输。
  const transports = useMemo(
    () =>
      (payload.transports ?? []).filter((item) => {
        if (!["openai-chat", "openai-responses", "anthropic-messages"].includes(item.protocol ?? "") && item.providers?.length && (!slug || !item.providers.includes(slug))) return false;
        const create = item.actions?.filter((action) => action.name === "create") ?? [];
        if (!create.some((action) => action.model)) return true;
        const prefix = item.strip_prefix ? item.strip_prefix + "/" : "";
        let model = typeof picked === "string" ? picked : "";
        if (prefix && model.startsWith(prefix)) model = model.slice(prefix.length);
        return create.some((action) => action.model === model);
      }),
    [payload.transports, slug, picked],
  );
  const declared =
    typeof picked === "string" ? payload.models?.[picked] || payload.models?.[slug + "/" + picked] : undefined;
  const declaredTransport = transports.find((item) => item.id === declared);
  const options = useMemo(
    () =>
      (payload.endpoint_types ?? []).filter((item) => {
        if (!picked) return false;
        if (declaredTransport && !["openai-chat", "openai-responses", "anthropic-messages"].includes(declaredTransport.protocol ?? "")) return item.id === declaredTransport.endpoint_id;
        return item.kind === "adapted" || transports.some((transport) => transport.endpoint_id === item.id);
      }),
    [payload.endpoint_types, transports, picked, declaredTransport],
  );
  const selected = options.find((item) => item.id === selectedId);
  const dialogue = selected?.family === "chat" && ["openai-chat", "openai-responses", "anthropic-messages"].includes(selected.protocol);
  const bindings = transports.filter((item) => dialogue
    ? ["openai-chat", "openai-responses", "anthropic-messages"].includes(item.protocol ?? "")
    : item.endpoint_id === selectedId);
  const binding = bindings.find((item) => item.id === transportId);
  // 用户只需辨认上游接口格式；内部传输标识仍作为保存值，不暴露给选择流程。
  const bindingLabels: Record<string, string> = {
    "openai-chat": "Chat Completions",
    "openai-responses": "Responses",
    "anthropic-messages": "Messages",
  };
  /** bindingLabel 获取可访问的协议名称。
   * 参数 item：目录传输；返回展示名称。调用：选择框；媒体传输保持模型组名称，无副作用。 */
  const bindingLabel = (item: EndpointTransport) => dialogue
    ? (bindingLabels[item.protocol ?? ""] ?? item.label)
    : (item.model_group ?? item.label);
  // 父表单订阅所有值；相同声明不得重复写入，否则会不断触发父组件渲染。
  const syncValues = useCallback(
    (id: string, transport: string, shouldDirty = true) => {
      // 用户入口数组是唯一声明，切换执行配置保留仍兼容的入口。
      const saved = getValues("endpoint_types");
      const endpointTypes = id ? [...new Set([id, ...(Array.isArray(saved) ? saved as string[] : [])])] : [];
      const current = getValues("endpoint_types");
      const sameEndpoints =
        Array.isArray(current) &&
        current.length === endpointTypes.length &&
        current.every((value, i) => value === endpointTypes[i]);
      if (!sameEndpoints) setValue("endpoint_types", endpointTypes, { shouldDirty });
      if (getValues("transport") !== transport) setValue("transport", transport, { shouldDirty });
    },
    [getValues, setValue],
  );
  /** writeValue 同步更新端点描述、公开能力和传输。
   * 参数 id：协议 ID；preferredTransport：模型默认传输。返回：无；清空时移除旧能力。
   */
  const writeValue = useCallback(
    (id: string, preferredTransport?: string) => {
      const descriptor = options.find((item) => item.id === id);
      const transport =
        descriptor?.kind === "adapted"
          ? (["chat", "responses", "messages"].includes(descriptor.id) ? (preferredTransport ?? "") : (preferredTransport ?? transports.find(item => item.protocol === descriptor.protocol)?.id ?? ""))
          : (preferredTransport ?? transports.find((item) => item.endpoint_id === id)?.id ?? "");
      syncValues(id, transport);
    },
    [options, transports, syncValues],
  );
  // 模型变化后重新验证端点与传输；无匹配项时清除三项声明，避免保留旧模型能力。
  useEffect(() => {
    // 加载中无法判断已有声明是否无效，保留未知模型的显式声明，直到目录成功返回。
    if (catalogState !== "ready") return;
    if (!options.length) {
      syncValues("", "", false);
      return;
    }
    const current = (getValues("endpoint_types") as string[] | undefined)?.[0] ?? "";
    const transport = getValues("transport") as string;
    const changed = lastModel.current !== undefined && lastModel.current !== picked;
    lastModel.current = picked;
    const validEndpoint = options.some((item) => item.id === current);
    const validTransport =
      transports.some((item) => item.id === transport && (item.endpoint_id === current ||
        (options.some(entry => entry.id === current && entry.family === "chat") &&
          ["openai-chat", "openai-responses", "anthropic-messages"].includes(item.protocol ?? ""))));
    if (!changed && validEndpoint && validTransport) return;
    const id = declaredTransport?.endpoint_id ?? declared;
    if (id && options.some((item) => item.id === id)) writeValue(id, declaredTransport?.id);
    else if (changed || !current || !validEndpoint) writeValue("");
    else if (!validTransport && transport) syncValues(current, "");
    // 用户选择后以表单为准；供应商变化由 ModelEditor 清空，模型变化在此校验。
  }, [
    options,
    catalogState,
    transports,
    picked,
    declared,
    declaredTransport,
    getValues,
    syncValues,
    writeValue,
    selectedId,
    transportId,
  ]);

  return (
    <>
      <MountedFormField label={t("Endpoint type")} name="endpoint_types" className="mb-1">
        {(control) => (
          <Select
            items={options.map((item) => ({ value: item.id, label: t(item.label) }))}
            disabled={!picked || catalogState !== "ready"}
            value={selectedId ?? ""}
            onValueChange={(value) => writeValue(value ?? "")}
          >
            <SelectTrigger id={control.id} className="w-full" aria-label={t("Endpoint type")}>
              <SelectValue placeholder={catalogState === "loading" ? "正在加载端点目录…" : "请选择端点类型"} />
            </SelectTrigger>
            <SelectContent>
              {options.map((item) => (
                <SelectItem key={item.id} value={item.id}>
                  {t(item.label)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </MountedFormField>
      <p className="text-sm text-muted-foreground mb-5 mt-1">{t("Choose the API used to call this model.")}</p>
      {catalogState === "failed" && <p role="alert" className="mb-5 text-sm text-destructive">端点目录加载失败，请重新打开编辑页面。已保存的端点声明已保留。</p>}
      {catalogState === "ready" && picked && !selectedId && <p role="status" className="mb-5 text-sm text-muted-foreground">该模型尚未声明调用端点，请选择实际支持的协议后保存。</p>}
      {(dialogue || (selected?.kind === "bypass" && bindings.length > 1)) && (
        <MountedFormField label={dialogue ? "上游接口协议" : t("Model group")} name="transport" className="mb-5">
          {(control) => (
            <Select
              items={bindings.map((item) => ({ value: item.id, label: bindingLabel(item) }))}
              disabled={!picked || !bindings.length}
              value={(control.value as string | undefined) ?? ""}
              onValueChange={(value) => {
                setValue("transport", value ?? "", { shouldDirty: true });
                const next = bindings.find(item => item.id === value);
                const entries = (getValues("endpoint_types") as string[] ?? []).filter(id => payload.endpoint_types?.some(entry => entry.id === id && ((entry.kind === "adapted" && entry.family === "chat") || (entry.kind === "bypass" && entry.protocol === next?.protocol))));
                setValue("endpoint_types", entries, { shouldDirty: true });
              }}
            >
              <SelectTrigger id={control.id} aria-label={dialogue ? "上游接口协议" : t("Model group")}>
                <SelectValue placeholder="请选择上游实际支持的协议" />
              </SelectTrigger>
              <SelectContent>
                {bindings.map((item) => (
                  <SelectItem key={item.id} value={item.id}>
                    {bindingLabel(item)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </MountedFormField>
      )}
      {dialogue && binding && <fieldset className="mb-5 space-y-2">
        <legend className="text-sm font-medium">开放的用户入口</legend>
        <p className="text-xs text-muted-foreground">统一入口支持协议转换及聊天护栏、完整响应缓存；Bypass 保留原生字段与事件，不执行聊天护栏或完整响应缓存。</p>
        {(payload.endpoint_types ?? []).filter(entry => (entry.kind === "adapted" && ["chat", "responses", "messages"].includes(entry.id)) || (entry.kind === "bypass" && entry.protocol === binding.protocol)).map(entry => <label key={entry.id} className="flex items-center gap-2 text-sm">
          <input type="checkbox" aria-label={t(entry.label)} checked={enabledEntries?.includes(entry.id) ?? false} onChange={event => {
            const values = enabledEntries ?? [];
            const next = event.target.checked ? [...values, entry.id] : values.filter(id => id !== entry.id);
            // 至少保留一个用户入口，避免主入口清空后协议配置区消失。
            if (!next.length) return;
            setValue("endpoint_types", next, { shouldDirty: true });
          }} />{t(entry.label)}
        </label>)}
      </fieldset>}
      {dialogue && !binding && <p role="status" className="mb-5 text-sm text-muted-foreground">
        {bindings.length ? "请选择上游实际支持的接口协议后保存。" : "目录未提供可用的上游接口协议，暂时无法保存该模型。"}
      </p>}
      {binding && (
        <p className="mb-5 break-all font-mono text-xs text-muted-foreground">
          {[...new Set(binding.actions?.map((action) => action.public_path) ?? [])].slice(0, 3).join(" · ")}
        </p>
      )}
    </>
  );
};
export default EndpointTypeField;

import React, { useEffect, useMemo, useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { MountedFormField, type MountedFormValues } from "../common_components/MountedFormField";
import { proxyBaseUrl } from "../networking";
import { provider_map } from "../provider_info_helpers";
import { t } from "@/i18n";

/**
 * 这条模型应答哪些入口（能力），以及网关怎么把请求送到上游（转发方式）。
 *
 * 这两件事原来压在一个下拉里，值写进 `model_info.mode`。压在一起的结果是
 * 「配置里没有这个选项」——一条只答对话的模型和一个原样转发的端点是两个独立的
 * 事实，用一格表达不了。
 *
 * 现在的写入：
 *   - `model_info.endpoint_types`：能力 id 列表。
 *   - `model_info.transport`：`adapted` 或一个已登记的内置转发 id。
 *   - `model_info.mode`：第一条能力，留着给还在读这个字符串的旧界面用。
 */
interface EndpointCapability {
  id: string;
  label: string;
  ops: string[];
  paths?: string[];
}

interface EndpointAction {
  name: string;
  method: string;
  public_path: string;
  upstream_path: string;
  task_query?: string;
}

interface EndpointTransport {
  id: string;
  label: string;
  kind: "adapted" | "bypass" | string;
  providers?: string[];
  api_base?: string;
  model_field?: string;
  task_id?: string;
  strip_prefix?: string;
  actions?: EndpointAction[];
}

interface EndpointPayload {
  capabilities?: EndpointCapability[];
  transports?: EndpointTransport[];
  models?: Record<string, string>;
}

/** 协议适配这一档的 id。它不在 transports 列表里，由前端补上作为选项之一。 */
const ADAPTED = "adapted";

function providerSlug(selected: string | null): string {
  if (!selected) return "";
  return provider_map[selected] ?? selected.toLowerCase();
}

const EndpointTypeField: React.FC<{
  selectedProvider: string | null;
  modelCostMap?: Record<string, { endpoint_type?: string }> | null;
}> = ({ selectedProvider, modelCostMap }) => {
  const form = useFormContext<MountedFormValues>();
  const [payload, setPayload] = useState<EndpointPayload>({});
  const mode = useWatch({ control: form.control, name: "mode" }) as string | undefined;
  const modelValue = useWatch({ control: form.control, name: "model" });
  const slug = providerSlug(selectedProvider);

  useEffect(() => {
    const url = proxyBaseUrl ? `${proxyBaseUrl}/public/endpoints` : "/public/endpoints";
    let cancel = false;
    fetch(url)
      .then((response) => (response.ok ? response.json() : null))
      .then((body: EndpointPayload | null) => {
        if (!cancel && body) setPayload(body);
      })
      .catch(() => {});
    return () => {
      cancel = true;
    };
  }, []);

  const capabilities = useMemo(() => payload.capabilities ?? [], [payload.capabilities]);

  // 内置转发方式按供应商过滤：方舟内容生成只对火山显示，七牛只对七牛显示。
  // 供应商不是列表里的任何一家时，这一档就没有可选项，只剩协议适配。
  const bypasses = useMemo(
    () =>
      (payload.transports ?? []).filter(
        (item) => !item.providers || item.providers.length === 0 || item.providers.includes(slug),
      ),
    [payload.transports, slug],
  );

  // 转发方式：协议适配永远可选，内置的那些按供应商来。
  const transportValue = mode && (mode === ADAPTED || bypasses.some((item) => item.id === mode)) ? mode : ADAPTED;
  const selectedBypass = bypasses.find((item) => item.id === transportValue) ?? null;

  // 能力：多选。mode 在这一栏不再表示转发方式，而表示第一条能力，
  // 所以旧行只写了 "chat" 时，这一栏预选 chat。
  const selectedCapabilities = useMemo(() => {
    const fromTypes = form.getValues("endpoint_types") as string[] | undefined;
    if (Array.isArray(fromTypes) && fromTypes.length > 0) return fromTypes;
    if (mode && capabilities.some((item) => item.id === mode)) return [mode];
    if (mode === ADAPTED || !mode) return ["chat"];
    return [];
    // mode 参与判定，但用一个稳定的字符串避免每次渲染都重算。
  }, [capabilities, mode, form]);

  // 能力这一栏默认预选 chat，并把它写进表单：一个新模型什么都不答是没意义的，
  // 而留空会让提交体里根本没有这个字段，后端只好自己猜。
  useEffect(() => {
    if (form.getValues("endpoint_types") !== undefined) return;
    if (form.getValues("mode")) return;
    if (capabilities.length === 0) return;
    writeCapabilities(["chat"]);
  }, [capabilities, form]);

  // 选中价目表的模型时预选：值是转发方式就选转发，否则选能力。
  useEffect(() => {
    const picked = Array.isArray(modelValue) ? modelValue[0] : modelValue;
    if (typeof picked !== "string" || picked === "") return;
    const declared = modelCostMap?.[picked]?.endpoint_type || payload.models?.[picked];
    if (!declared) return;
    const current = form.getValues("mode");
    if (current) return;
    if (declared === ADAPTED || (payload.transports ?? []).some((item) => item.id === declared)) {
      form.setValue("mode", declared);
      return;
    }
    if (capabilities.some((item) => item.id === declared)) {
      form.setValue("endpoint_types", [declared]);
      form.setValue("mode", declared);
    }
  }, [modelValue, modelCostMap, payload.models, payload.transports, capabilities, form]);

  /** 写能力：多选值进 endpoint_types，第一条也写进 mode 给旧界面读。 */
  const writeCapabilities = (next: string[]) => {
    form.setValue("endpoint_types", next);
    form.setValue("mode", next[0] ?? "");
  };

  /** 写转发：协议适配不写 endpoint 对象，内置的写上它的字段。 */
  const writeTransport = (id: string) => {
    form.setValue("mode", id);
    form.setValue("transport", id);
    const bypass = bypasses.find((item) => item.id === id);
    if (!bypass) {
      // 协议适配：上游由 (op, 供应商) 决定，这里没有要存的东西。
      form.setValue("endpoint", undefined);
      return;
    }
    form.setValue("endpoint", {
      kind: "bypass",
      api_base: bypass.api_base ?? "",
      model_field: bypass.model_field ?? "model",
      task_id: bypass.task_id ?? "",
      strip_prefix: bypass.strip_prefix ?? "",
      actions: bypass.actions ?? [],
    });
    if (bypass.api_base) form.setValue("api_base", bypass.api_base);
  };

  const adapted = transportValue === ADAPTED;

  return (
    <>
      <MountedFormField label={t("Transport")} name="transport" className="mb-1">
        {() => (
          <Select items={[]} value={transportValue} onValueChange={(value: string | null) => writeTransport(value ?? ADAPTED)}>
            <SelectTrigger id="model-transport" className="w-full" aria-label={t("Transport")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ADAPTED}>{t("Protocol adaptation")}</SelectItem>
              {bypasses.map((item) => (
                <SelectItem key={item.id} value={item.id}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </MountedFormField>
      <p className="text-sm text-muted-foreground mb-5 mt-1">
        {t(
          "Protocol adaptation lets the gateway compile the request from the provider and the endpoint. A registered pass-through forwards the provider's own API unchanged.",
        )}
      </p>

      {adapted && (
        <>
          <fieldset className="mb-5 space-y-3 rounded-md border border-border p-3">
            <legend className="px-1 text-sm font-medium">{t("Capabilities")}</legend>
            <div className="flex flex-wrap gap-3" role="group" aria-label={t("Capabilities")}>
              {capabilities.map((item) => (
                <label key={item.id} className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={selectedCapabilities.includes(item.id)}
                    onChange={(event) => {
                      const next = event.target.checked
                        ? [...selectedCapabilities, item.id]
                        : selectedCapabilities.filter((id) => id !== item.id);
                      writeCapabilities(next);
                    }}
                  />
                  <span>{t(item.label)}</span>
                </label>
              ))}
            </div>
            {selectedCapabilities.length === 0 && (
              <p className="text-xs text-destructive">
                {t("Pick at least one capability, or this model cannot be called.")}
              </p>
            )}
          </fieldset>
          {selectedCapabilities.length > 0 && (
            <p className="mb-5 -mt-3 text-xs text-muted-foreground">
              {selectedCapabilities
                .flatMap((id) => capabilities.find((item) => item.id === id)?.paths ?? [])
                .join(" · ")}
            </p>
          )}
        </>
      )}

      {!adapted && selectedBypass && (
        <div className="mb-5 space-y-2 rounded-md border border-border p-3 text-sm">
          {selectedBypass.api_base && (
            <p className="text-muted-foreground">
              {t("API Base")}: <span className="font-mono">{selectedBypass.api_base}</span>
            </p>
          )}
          <p className="text-muted-foreground">
            {t(
              "The paths and fields of this pass-through are registered with the gateway. Nothing to fill in here.",
            )}
          </p>
        </div>
      )}
    </>
  );
};

export default EndpointTypeField;

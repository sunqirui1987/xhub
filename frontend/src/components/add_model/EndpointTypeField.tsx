import React, { useEffect, useMemo, useRef, useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { MountedFormField, type MountedFormValues } from "../common_components/MountedFormField";
import { apiClient } from "../networking";
import { provider_map } from "../provider_info_helpers";
import { t } from "@/i18n";

/**
 * 这条模型怎么被调用。一个下拉，单选。
 *
 * 选项分两档，但在界面上是同一件事——"这条模型用什么形状被调用"：
 *
 *   - 协议适配（chat、completion、embedding、image、video、…）：上游路径由
 *     网关按 (操作, 供应商) 编译。
 *   - 直通（内置的那些）：上游路径和字段在网关里登记好了，请求原样转发。
 *
 * 直通不出现在这里作为"可以自己填的东西"：一份手填的路径表拿不到上游的结构化
 * 返回值，预选、日志、用量都无从谈起。所以它只能来自后台登记。
 *
 * 写入：
 *   - `model_info.endpoint_types`：选中能力时是它的 id；直通时为空。
 *   - `model_info.transport`：`adapted` 或一个登记过的直通 id。
 *   - `model_info.mode`：选中项的 id。旧的模型列表和 Playground 还在读这个字符串。
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

/** 协议适配这一档的 id。它不在 transports 列表里，由前端补上作为默认选项。 */
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
    let cancel = false;
    apiClient
      .get<EndpointPayload>("/public/endpoints")
      .then((body: EndpointPayload | null) => {
        if (!cancel && body) setPayload(body);
      })
      .catch(() => {});
    return () => {
      cancel = true;
    };
  }, []);

  const capabilities = useMemo(() => payload.capabilities ?? [], [payload.capabilities]);
  const transports = useMemo(() => payload.transports ?? [], [payload.transports]);

  /**
   * 下拉里的每一项：一个 id、一个显示名、一段说明，以及它是不是直通。
   *
   * 直通按供应商过滤：方舟内容生成的根地址是方舟的，七牛的是七牛的，拿另一家去
   * 接只会打错地址。供应商还没选时全都显示，免得看起来像"这家没有直通"。
   */
  const options = useMemo(() => {
    const items = capabilities.map((item) => ({
      value: item.id,
      label: t(item.label),
      detail: (item.paths ?? []).join(" · "),
      bypass: false,
    }));
    for (const item of transports) {
      const declared = item.providers ?? [];
      if (declared.length > 0 && slug !== "" && !declared.includes(slug)) continue;
      items.push({
        value: item.id,
        label: item.label,
        detail: item.api_base ?? "",
        bypass: true,
      });
    }
    return items;
  }, [capabilities, transports, slug]);

  const selected = options.find((item) => item.value === mode) ?? null;

  /**
   * 写入一个选项。
   *
   * 直通只写 transport 和 endpoint 对象，不写能力：它的入口是供应商自己的路径，
   * 不适配路径。协议适配相反，写能力，并清掉上一次留下的 endpoint 对象——
   * 否则换回 chat 之后那份旧路径表还挂在部署上，后端会当它不存在，但库里留着垃圾。
   */
  const writeValue = useMemo(
    () => (value: string) => {
      const item = options.find((option) => option.value === value);
      form.setValue("mode", value);
      if (!item?.bypass) {
        form.setValue("transport", ADAPTED);
        form.setValue("endpoint_types", value === "" ? [] : [value]);
        form.setValue("endpoint", undefined);
        return;
      }
      const transport = transports.find((entry) => entry.id === value);
      form.setValue("transport", value);
      form.setValue("endpoint_types", []);
      form.setValue("endpoint", {
        kind: "bypass",
        api_base: transport?.api_base ?? "",
        model_field: transport?.model_field ?? "model",
        task_id: transport?.task_id ?? "",
        strip_prefix: transport?.strip_prefix ?? "",
        actions: transport?.actions ?? [],
      });
      if (transport?.api_base) form.setValue("api_base", transport.api_base);
    },
    [form, options, transports],
  );

  // 目录到了之后做一次预选，只做一次。
  //
  // 顺序是"价目表里声明的那个"优先，没有才用 chat。放在一个 effect 里并且用
  // 一个 ref 标记做过，是因为目录是异步来的：早于目录的那次渲染里 options
  // 是空的，那时写值只会写下一个目录里不存在的 id。
  const seeded = useRef(false);
  useEffect(() => {
    if (seeded.current || options.length === 0) return;
    seeded.current = true;
    const picked = Array.isArray(modelValue) ? modelValue[0] : modelValue;
    const declared =
      typeof picked === "string" && picked !== ""
        ? modelCostMap?.[picked]?.endpoint_type || payload.models?.[picked]
        : undefined;
    if (declared && options.some((item) => item.value === declared) && !form.getValues("mode")) {
      writeValue(declared);
      return;
    }
    // 一个新模型什么都不答没有意义，所以默认 chat。留空会让提交体里根本没有
    // 这个字段，后端只好替它猜一个。
    if (!form.getValues("mode")) writeValue("chat");
  }, [options, modelValue, modelCostMap, payload.models, writeValue, form]);

  return (
    <>
      <MountedFormField label={t("Call type")} name="mode" className="mb-1">
        {(control) => (
          <Select
            items={options.map((item) => ({ value: item.value, label: item.label }))}
            value={(control.value as string | undefined) ?? ""}
            onValueChange={(value: string | null) => {
              control.onChange(value ?? "");
              writeValue(value ?? "");
            }}
          >
            <SelectTrigger id={control.id} className="w-full" aria-label={t("Call type")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {options.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </MountedFormField>
      <p className="text-sm text-muted-foreground mb-5 mt-1">{t("Choose the API used to call this model.")}</p>
      {selected?.detail && (
        <p className="mb-5 -mt-3 break-all font-mono text-xs text-muted-foreground">{selected.detail}</p>
      )}
    </>
  );
};

export default EndpointTypeField;

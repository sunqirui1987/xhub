import React, { useEffect, useMemo } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/shared/Alert";
import { Info, TriangleAlert } from "lucide-react";
import type { MountedFormValues } from "../common_components/MountedFormField";
import { t } from "@/i18n";

/**
 * 单价：这条模型按什么价计费。
 *
 * 这一栏存在的理由是一个静默故障。网关按模型名取单价（catalog.TokenRates）；
 * 名字对不上价目表时 Cost 返回 ok=false，这次调用**记一行零费用**。
 * 名字写错一个字符，模型照常能用，账单上是零——不报错，不告警。所以这不是
 * 可选的补充信息，而是一个必答的问题。
 *
 * 费率跟着调用方式走，因为计费读的字段不一样：
 *
 *   - 按 token 计费（对话、补全、向量、重排序、审核）：输入、输出、缓存读、缓存写。
 *     缓存的价通常比输入便宜得多，漏填会让一次带缓存的调用按输入价扣。
 *   - 按张计费（图像）：每张的价，输入图和输出图分开。
 *   - 按秒计费（视频、语音合成）：每秒的价。
 *   - 按次计费（联网搜索）：每次查询。
 *
 * 单位在界面上是每百万 token，存的是每 token；转换在 handle_add_model_submit
 * 里做，因为那一步是唯一动这个数的地方。
 */

/** 一个费率字段：表单名、标签、提示、单位换算。 */
type RateField = {
  name: string;
  label: string;
  hint?: string;
  /** 界面单位换算到存储单位的倍数。token 类是 1e6，其余是 1。 */
  scale: number;
  /** 目录里的同名字段，用来带出默认值。 */
  catalogKey: string;
};

const PER_TOKEN: RateField[] = [
  { name: "input_cost_per_token", label: t("Input"), scale: 1e6, catalogKey: "input_cost_per_token" },
  { name: "output_cost_per_token", label: t("Output"), scale: 1e6, catalogKey: "output_cost_per_token" },
  {
    name: "cache_read_input_token_cost",
    label: t("Cache read"),
    hint: t("What a cached prompt costs. Usually far below the input rate."),
    scale: 1e6,
    catalogKey: "cache_read_input_token_cost",
  },
  {
    name: "cache_creation_input_token_cost",
    label: t("Cache write"),
    hint: t("What writing the cache costs. Usually above the input rate."),
    scale: 1e6,
    catalogKey: "cache_creation_input_token_cost",
  },
];

const PER_IMAGE: RateField[] = [
  { name: "input_cost_per_image", label: t("Input image"), scale: 1, catalogKey: "input_cost_per_image" },
  { name: "output_cost_per_image", label: t("Output image"), scale: 1, catalogKey: "output_cost_per_image" },
];

const PER_SECOND: RateField[] = [
  { name: "input_cost_per_second", label: t("Input per second"), scale: 1, catalogKey: "input_cost_per_second" },
  { name: "output_cost_per_second", label: t("Output per second"), scale: 1, catalogKey: "output_cost_per_second" },
];

const PER_QUERY: RateField[] = [
  { name: "search_context_cost_per_query", label: t("Per search"), scale: 1, catalogKey: "search_context_cost_per_query" },
];

/** 一批费率加它的单位说明。 */
type RateGroup = { key: string; fields: RateField[]; unit: string };

/**
 * 调用方式到费率组的对应。
 *
 * 键是能力 id，取值来自 /public/endpoints 的能力表。没有列出的能力按 token
 * 计费：那是绝大多数模型，也是网关默认读的那一组字段。
 */
const GROUPS_BY_CAPABILITY: Record<string, RateGroup[]> = {
  image: [
    { key: "per_image", fields: PER_IMAGE, unit: t("USD per image") },
    { key: "per_token", fields: PER_TOKEN, unit: t("USD per 1M tokens") },
  ],
  video: [
    { key: "per_second", fields: PER_SECOND, unit: t("USD per second") },
    { key: "per_token", fields: PER_TOKEN, unit: t("USD per 1M tokens") },
  ],
  audio_speech: [
    { key: "per_second", fields: PER_SECOND, unit: t("USD per second") },
    { key: "per_token", fields: PER_TOKEN, unit: t("USD per 1M tokens") },
  ],
};

const DEFAULT_GROUPS: RateGroup[] = [{ key: "per_token", fields: PER_TOKEN, unit: t("USD per 1M tokens") }];

/** 每个模型都会读到的联网搜索费。它和上面几组并列，不互斥。 */
const ALWAYS_GROUPS: RateGroup[] = [
  { key: "per_query", fields: PER_QUERY, unit: t("USD per search") },
];

const ModelPriceFields: React.FC<{
  selectedProvider: string | null;
  modelCostMap?: Record<string, Record<string, unknown>> | null;
  /** 当前选中的调用方式 id，决定显示哪几组费率。 */
  capability?: string;
}> = ({ modelCostMap, capability }) => {
  const form = useFormContext<MountedFormValues>();
  const modelValue = useWatch({ control: form.control, name: "model" });

  const picked = Array.isArray(modelValue) ? modelValue[0] : modelValue;
  const modelID = typeof picked === "string" && picked !== "" ? picked : "";

  const groups = useMemo(
    () => [...(GROUPS_BY_CAPABILITY[capability ?? ""] ?? DEFAULT_GROUPS), ...ALWAYS_GROUPS],
    [capability],
  );

  // 所有费率字段的值放在一次订阅里。
  //
  // 每个格子各自 form.watch 会让表单每敲一个字符就重渲染整块；一次订阅拿一份
  // 快照，格子只读自己那一项。字段名在做完分组之后才知道，所以这里订阅整个表单
  // 的值，再从中挑出需要的那些。
  const allValues = useWatch({ control: form.control }) as MountedFormValues | undefined;
  const draft = useMemo(() => {
    const out: Record<string, string> = {};
    for (const group of groups) {
      for (const field of group.fields) {
        const value = allValues?.[field.name];
        out[field.name] = typeof value === "string" ? value : value === undefined || value === null ? "" : String(value);
      }
    }
    return out;
  }, [allValues, groups]);

  const setRate = (name: string, value: string) => form.setValue(name, value);

  const catalogRow = useMemo(() => {
    if (!modelID || !modelCostMap) return null;
    return modelCostMap[modelID] ?? null;
  }, [modelID, modelCostMap]);

  /**
   * 选中模型之后把目录里的价填上对应格子。
   *
   * 只填空着的格子，不然会把运维手改过的数字盖掉——从目录带价是省事，
   * 不是强制回到目录。
   */
  useEffect(() => {
    if (!catalogRow) return;
    for (const group of groups) {
      for (const field of group.fields) {
        const stored = catalogRow[field.catalogKey];
        if (typeof stored !== "number" || stored <= 0) continue;
        if (form.getValues(field.name) !== undefined) continue;
        form.setValue(field.name, String(Number((stored * field.scale).toPrecision(12))));
      }
    }
    // form 是稳定的；catalogRow 和 groups 变化才是触发条件。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [catalogRow, groups]);

  // 目录里有没有这条模型：没有就要手填，否则这次调用记零费用。
  const inCatalog = modelID !== "" && catalogRow !== null;

  return (
    <fieldset className="mb-5 space-y-4 rounded-md border border-border p-3">
      <legend className="px-1 text-sm font-medium">{t("Price")}</legend>

      {modelID === "" ? (
        <p className="text-sm text-muted-foreground">{t("Pick a model first; its price comes from the catalog.")}</p>
      ) : inCatalog ? (
        <Alert variant="info">
          <Info />
          <AlertDescription>
            {t("Price taken from the catalog. Edit it if this deployment is billed differently.")}
          </AlertDescription>
        </Alert>
      ) : (
        <Alert variant="destructive">
          <TriangleAlert />
          <AlertDescription>
            {t(
              "This model is not in the price catalog. Fill in its price below, or calls to it are recorded at zero cost with no error.",
            )}
          </AlertDescription>
        </Alert>
      )}

      {modelID !== "" &&
        groups.map((group) => (
          <div key={group.key} className="space-y-3">
            <div className="grid gap-4 sm:grid-cols-2">
              {group.fields.map((field) => (
                <div key={field.name} className="space-y-2">
                  <Label htmlFor={field.name}>{field.label}</Label>
                  <Input
                    id={field.name}
                    inputMode="decimal"
                    placeholder="0.00"
                    aria-describedby={field.hint ? `${field.name}-hint` : undefined}
                    value={draft[field.name] ?? ""}
                    onChange={(event) => setRate(field.name, event.target.value)}
                  />
                  {field.hint && (
                    <p id={`${field.name}-hint`} className="text-xs text-muted-foreground">
                      {field.hint}
                    </p>
                  )}
                </div>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">{group.unit}</p>
          </div>
        ))}

      {/* 目录里没有、又一个字都没填：说清后果。保存也会被挡住。 */}
      {!inCatalog && modelID !== "" && !anyRateFilled(form.getValues(), groups) && (
        <p role="alert" className="text-xs text-destructive">
          {t("Fill in at least one rate, or this model is billed at zero.")}
        </p>
      )}
    </fieldset>
  );
};

/**
 * 这些费率里有没有填过至少一个。
 * 参数 values（MountedFormValues）：表单当前值；groups（RateGroup[]）：当前显示的费率组。
 * 返回 bool（bool）：填过至少一个时为真。
 */
export function anyRateFilled(values: MountedFormValues, groups: RateGroup[]): boolean {
  for (const group of groups) {
    for (const field of group.fields) {
      const value = values[field.name];
      if (value !== undefined && value !== null && value !== "") return true;
    }
  }
  return false;
}

/**
 * 这条模型会被按什么价计费。价目表里有它，或者填过至少一个费率，都算。
 * 参数 values（MountedFormValues）：表单当前值；catalogRow（Record | null）：价目表里的这一行。
 * 返回 bool（bool）：这次调用会被计费时为真。
 */
export function modelPriced(values: MountedFormValues, catalogRow: Record<string, unknown> | null): boolean {
  if (catalogRow) {
    for (const key of Object.keys(catalogRow)) {
      if (key.includes("cost") && typeof catalogRow[key] === "number") return true;
    }
  }
  const filled = (value: unknown) => value !== undefined && value !== null && value !== "";
  return (
    filled(values.input_cost_per_token) ||
    filled(values.output_cost_per_token) ||
    filled(values.input_cost_per_image) ||
    filled(values.output_cost_per_image) ||
    filled(values.input_cost_per_second) ||
    filled(values.output_cost_per_second)
  );
}

export default ModelPriceFields;

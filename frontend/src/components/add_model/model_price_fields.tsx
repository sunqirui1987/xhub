import React, { useEffect, useMemo } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/shared/Alert";
import { Info, TriangleAlert } from "lucide-react";
import type { MountedFormValues } from "../common_components/MountedFormField";
import { t } from "@/i18n";
import { categoriesOf, rateFieldsOf } from "./billing_categories";

/**
 * 单价：这条模型按什么价计费。
 *
 * 这一栏存在的理由是一个静默故障。网关按模型名取单价；名字对不上价目表时
 * 取价失败，这次调用**记一行零费用**。名字写错一个字符，模型照常能用，
 * 账单上是零——不报错，不告警。所以这不是可选的补充信息，是必答的问题。
 *
 * 单价按**计费维度**分类，不是罗列一堆字段。分类表和"哪种调用方式显示哪几节"
 * 的对应在 billing_categories.ts，那边是纯数据，单测钉着它。这个组件只负责渲染。
 *
 * 单位在界面上是每百万 token（token 类）或原始单位（张/秒/次）；换算在
 * handle_add_model_submit 里做。
 */

/** 从目录的 rates[] 里找一条费率，同 side+window 有多条时取最便宜的。 */
type RatesEntry = { side: string; window: string; usd: number };

function rateFromRates(rates: unknown, side: string, window: string): number | null {
  if (!Array.isArray(rates)) return null;
  let best: number | null = null;
  for (const entry of rates as RatesEntry[]) {
    if (entry.side !== side || entry.window !== window) continue;
    if (typeof entry.usd !== "number" || entry.usd <= 0) continue;
    // usd 已经是每一个基础单位的价：buildRates 在 Go 那边除过 unit_size 了。
    if (best === null || entry.usd < best) best = entry.usd;
  }
  return best;
}

const ModelPriceFields: React.FC<{
  selectedProvider: string | null;
  modelCostMap?: Record<string, Record<string, unknown>> | null;
  capability?: string;
}> = ({ modelCostMap, capability }) => {
  const form = useFormContext<MountedFormValues>();
  const modelValue = useWatch({ control: form.control, name: "model" });

  const picked = Array.isArray(modelValue) ? modelValue[0] : modelValue;
  const modelID = typeof picked === "string" && picked !== "" ? picked : "";

  const categories = useMemo(() => categoriesOf(capability), [capability]);
  const allFields = useMemo(
    () => categories.flatMap((category) => category.fields),
    [categories],
  );

  // 所有费率字段的值放在一次订阅里。
  //
  // 每个格子各自 form.watch 会让表单每敲一个字符就重渲染整块；一次订阅拿一份
  // 快照，格子只读自己那一项。
  const allValues = useWatch({ control: form.control }) as MountedFormValues | undefined;
  const draft = useMemo(() => {
    const out: Record<string, string> = {};
    for (const field of allFields) {
      const value = allValues?.[field.name];
      out[field.name] = typeof value === "string" ? value : value === undefined || value === null ? "" : String(value);
    }
    return out;
  }, [allValues, allFields]);

  const setRate = (name: string, value: string) => form.setValue(name, value);

  const catalogRow = useMemo(() => {
    if (!modelID || !modelCostMap) return null;
    return modelCostMap[modelID] ?? null;
  }, [modelID, modelCostMap]);

  /**
   * 选中模型之后把目录里的价填上对应格子。
   *
   * 先读扁平字段（老形状），取不到再从 rates[] 里按 side+window 找——分时价
   * 只存在于 rates[] 里，扁平字段是"最便宜那一档"的派生值，拿它当高峰价会
   * 按空闲价收费。
   *
   * 只填空着的格子，不然会把运维手改过的数字盖掉。
   */
  useEffect(() => {
    if (!catalogRow) return;
    const rates = catalogRow["rates"];
    for (const field of allFields) {
      const existing = form.getValues(field.name);
      if (existing !== undefined && existing !== null && existing !== "") continue;

      const stored = catalogRow[field.catalogKey];
      if (typeof stored === "number" && stored > 0) {
        form.setValue(field.name, String(Number((stored * field.scale).toPrecision(12))));
        continue;
      }

      if (field.ratesFilter) {
        const usd = rateFromRates(rates, field.ratesFilter.side, field.ratesFilter.window);
        if (usd !== null) {
          form.setValue(field.name, String(Number((usd * field.scale).toPrecision(12))));
        }
      }
    }
    // form 是稳定的；catalogRow 和 allFields 变化才是触发条件。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [catalogRow, allFields]);

  // 目录里有没有这条模型：没有就要手填，否则这次调用记零费用。
  const inCatalog = modelID !== "" && catalogRow !== null;

  return (
    <fieldset className="mb-5 space-y-5 rounded-md border border-border p-3">
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
        categories.map((category) => (
          <div key={category.id} className="space-y-2">
            <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{category.title}</p>
            <div className="grid gap-3 sm:grid-cols-2">
              {category.fields.map((field) => (
                <div key={field.name} className="space-y-1">
                  <Label htmlFor={field.name}>{field.label}</Label>
                  <Input
                    id={field.name}
                    inputMode="decimal"
                    placeholder="—"
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
            <p className="text-xs text-muted-foreground">{category.unit}</p>
          </div>
        ))}

      {/* 目录里没有、又一个字都没填：说清后果。保存也会被挡住。 */}
      {!inCatalog && modelID !== "" && !anyRateFilled(form.getValues(), capability) && (
        <p role="alert" className="text-xs text-destructive">
          {t("Fill in at least one rate, or this model is billed at zero.")}
        </p>
      )}
    </fieldset>
  );
};

/**
 * 这个调用方式下有没有填过至少一个费率。
 * 参数 values（MountedFormValues）：表单当前值；capability（string | undefined）：当前调用方式。
 * 返回 bool（bool）：填过至少一个时为真。
 */
export function anyRateFilled(values: MountedFormValues, capability: string | undefined): boolean {
  for (const name of rateFieldsOf(capability)) {
    const value = values[name];
    if (value !== undefined && value !== null && value !== "") return true;
  }
  return false;
}

export default ModelPriceFields;

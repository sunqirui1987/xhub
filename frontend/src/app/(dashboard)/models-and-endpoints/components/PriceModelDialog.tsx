"use client";

import { useEffect, useMemo, useState } from "react";
import { Plus } from "lucide-react";
import { t } from "@/i18n";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { apiClient } from "@/components/networking";
import { ModelMode } from "@/components/chat_ui/mode_endpoint_mapping";
import { priceSelectOptions, type PriceSelectOption } from "./priceModelOptions";
import {
  EDITABLE_TOKEN_RATES,
  EDITABLE_UNIT_RATES,
  catalogFieldValue,
  invalidRateFields,
  priceModelPayload,
  rateToInputValue,
  unitRateToInputValue,
  type PriceCatalogRow,
  type PriceProviderRow,
} from "./priceCatalogRows";

/** 价格弹窗使用的原始目录行，以接口字段名为键。 */
export type PriceModelDraft = Record<string, unknown>;

/**
 * 新增或编辑价格目录模型；参数为打开状态、供应商目录、原始行、保存与关闭回调及保存状态，返回弹窗。
 * 价格页按编辑行重新挂载；Token 价格按百万填写，保存时转换为单 Token。
 * 三个分类字段使用目录下拉，旧目录值仍可回显；端点目录失败时保留草稿并提示降级，不修改部署协议。
 */
export function PriceModelDialog({
  open,
  onOpenChange,
  providers,
  editing,
  onSave,
  isSaving,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providers: PriceProviderRow[];
  /** 正在编辑的行；新增时为 null。 */
  editing: PriceCatalogRow | null;
  onSave: (body: Record<string, unknown>) => void;
  isSaving?: boolean;
}) {
  const draft: PriceModelDraft = editing?.raw ?? {};
  const [id, setId] = useState(editing?.id ?? "");
  const [provider, setProvider] = useState(editing?.provider ?? providers[0]?.slug ?? "");
  const [displayName, setDisplayName] = useState(editing?.displayName ?? "");
  const [mode, setMode] = useState(typeof draft.mode === "string" ? draft.mode : "");
  const initialEndpoint = draft.endpoint_id ?? draft.endpoint_type;
  const [endpointType, setEndpointType] = useState(typeof initialEndpoint === "string" ? initialEndpoint : "");
  const [endpoints, setEndpoints] = useState<PriceSelectOption[]>([]);
  const [endpointState, setEndpointState] = useState<"loading" | "ready" | "failed">("loading");

  useEffect(() => {
    if (!open) return;
    let canceled = false;
    // 价格 endpoint_id 记录执行类型（例如 Ark），选项读取 transports，不能误用 XHub 对外能力 ID。
    // 重开时沿用上次成功加载的选项，异步响应到达后再更新，避免闪烁和额外同步渲染。
    apiClient
      .get<{ transports?: PriceSelectOption[] }>("/public/endpoints")
      .then((body) => {
        if (canceled) return;
        if (!Array.isArray(body?.transports)) throw new Error("Missing endpoint catalog");
        setEndpoints(priceSelectOptions(body.transports));
        setEndpointState("ready");
      })
      .catch(() => {
        if (!canceled) setEndpointState("failed");
      });
    // 关闭或切换编辑行后忽略迟到请求，避免旧目录覆盖当前草稿。
    return () => {
      canceled = true;
    };
  }, [open]);
  const [tokenRates, setTokenRates] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      EDITABLE_TOKEN_RATES.map(({ field }) => [field, rateToInputValue(catalogFieldValue(draft, field))]),
    ),
  );
  const [unitRates, setUnitRates] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      EDITABLE_UNIT_RATES.map(({ field }) => [field, unitRateToInputValue(catalogFieldValue(draft, field))]),
    ),
  );

  const isEdit = Boolean(editing);
  const invalidRates = useMemo(
    () => new Set([...invalidRateFields(tokenRates), ...invalidRateFields(unitRates)]),
    [tokenRates, unitRates],
  );
  const canSave = id.trim() !== "" && provider.trim() !== "" && invalidRates.size === 0 && !isSaving;
  const knownProviders = priceSelectOptions(
    providers.map((p) => ({ id: p.slug, label: p.name })),
    provider,
  );
  const knownModes = priceSelectOptions(
    Object.values(ModelMode).map((id) => ({ id, label: id })),
    mode,
  );
  const knownEndpoints = priceSelectOptions(endpoints, endpointType);
  const selectClassName =
    "h-9 w-full min-w-0 rounded-md border border-input bg-background px-3 py-1 text-base shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 md:text-sm";

  /** 提交有效草稿；无参数及返回值，供保存按钮调用。缺少必填值或单价无效时不触发回调。 */
  function submit() {
    if (!canSave) return;
    onSave(
      priceModelPayload({
        id,
        provider,
        displayName,
        mode,
        endpointType,
        tokenRates,
        unitRates,
        original: editing?.raw,
      }),
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{isEdit ? t("priceData.editModel") : t("priceData.addModel")}</DialogTitle>
          <DialogDescription>{t("priceData.modelDialogHint")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="price-model-id">{t("priceData.modelId")}</Label>
              <Input
                id="price-model-id"
                value={id}
                readOnly={isEdit}
                placeholder="claude-4.1-opus"
                onChange={(event) => setId(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="price-model-provider">{t("Provider")}</Label>
              <select
                id="price-model-provider"
                className={selectClassName}
                value={provider}
                onChange={(event) => setProvider(event.target.value)}
              >
                <option value="" disabled>
                  {t("Select provider")}
                </option>
                {knownProviders.map(({ id, label }) => (
                  <option key={id} value={id}>
                    {label === id ? id : `${label} (${id})`}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="price-model-display">{t("priceData.displayName")}</Label>
              <Input
                id="price-model-display"
                value={displayName}
                onChange={(event) => setDisplayName(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="price-model-mode">{t("priceData.endpointMode")}</Label>
              <select
                id="price-model-mode"
                className={selectClassName}
                value={mode}
                onChange={(event) => setMode(event.target.value)}
              >
                <option value="">{t("Not set")}</option>
                {knownModes.map(({ id, label }) => (
                  <option key={id} value={id}>
                    {label}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="price-model-endpoint">{t("priceData.endpointType")}</Label>
              <select
                id="price-model-endpoint"
                className={selectClassName}
                value={endpointType}
                disabled={endpointState !== "ready" && !endpointType}
                onChange={(event) => setEndpointType(event.target.value)}
              >
                <option value="">{t("Not set")}</option>
                {knownEndpoints.map(({ id, label }) => (
                  <option key={id} value={id}>
                    {label === id ? id : `${label} (${id})`}
                  </option>
                ))}
              </select>
              {endpointState === "loading" && (
                <p role="status" className="text-xs text-muted-foreground">
                  正在加载端点目录…
                </p>
              )}
              {endpointState === "failed" && (
                <p role="alert" className="text-xs text-destructive">
                  端点目录加载失败，已有值已保留；请重新打开弹窗重试。
                </p>
              )}
              <p className="text-xs text-muted-foreground">{t("priceData.endpointTypeHint")}</p>
            </div>
          </div>

          <fieldset className="space-y-3 rounded-xl border border-border p-4">
            <legend className="px-1 text-sm font-medium">{t("priceData.tokenRates")}</legend>
            <p className="text-xs text-muted-foreground">{t("priceData.tokenRatesHint")}</p>
            <div className="grid gap-4 sm:grid-cols-2">
              {EDITABLE_TOKEN_RATES.map(({ field, labelKey }) => (
                <div key={field} className="space-y-2">
                  <Label htmlFor={`price-rate-${field}`}>{t(labelKey)}</Label>
                  <Input
                    id={`price-rate-${field}`}
                    inputMode="decimal"
                    value={tokenRates[field] ?? ""}
                    aria-invalid={invalidRates.has(field)}
                    onChange={(event) => setTokenRates((prev) => ({ ...prev, [field]: event.target.value }))}
                  />
                  {invalidRates.has(field) && <p className="text-xs text-destructive">{t("priceData.invalidRate")}</p>}
                </div>
              ))}
            </div>
          </fieldset>

          <fieldset className="space-y-3 rounded-xl border border-border p-4">
            <legend className="px-1 text-sm font-medium">{t("priceData.unitRates")}</legend>
            <p className="text-xs text-muted-foreground">{t("priceData.unitRatesHint")}</p>
            <div className="grid gap-4 sm:grid-cols-2">
              {EDITABLE_UNIT_RATES.map(({ field, labelKey }) => (
                <div key={field} className="space-y-2">
                  <Label htmlFor={`price-rate-${field}`}>{t(labelKey)}</Label>
                  <Input
                    id={`price-rate-${field}`}
                    inputMode="decimal"
                    value={unitRates[field] ?? ""}
                    aria-invalid={invalidRates.has(field)}
                    onChange={(event) => setUnitRates((prev) => ({ ...prev, [field]: event.target.value }))}
                  />
                  {invalidRates.has(field) && <p className="text-xs text-destructive">{t("priceData.invalidRate")}</p>}
                </div>
              ))}
            </div>
          </fieldset>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("Cancel")}
          </Button>
          <Button onClick={submit} disabled={!canSave} data-testid="price-model-save">
            <Plus aria-hidden="true" />
            {isEdit ? t("Save") : t("priceData.addModel")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

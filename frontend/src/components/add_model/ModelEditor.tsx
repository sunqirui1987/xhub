"use client";
import { useState } from "react";
import { FormProvider, useForm, useWatch } from "react-hook-form";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCredentials } from "@/app/(dashboard)/hooks/credentials/useCredentials";
import { usePriceCatalog, priceCatalogKeys } from "@/app/(dashboard)/hooks/models/usePriceCatalog";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { apiClient, modelCreateCall, modelPatchUpdateCall, type CredentialItem } from "@/components/networking";
import { MountedFormProvider, useMountRegistry, type MountedFormValues } from "../common_components/MountedFormField";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import EndpointTypeField from "./EndpointTypeField";
import { BILLING_CATEGORIES, type BillingMode } from "./billing_categories";
import {
  buildEditorModel,
  catalogFieldValue,
  editorDefaults,
  hasPrice,
  billingModeOf,
  PRICE_FIELDS,
  type EditorModel,
  type CatalogRow,
  catalogRows,
  pricingRates,
} from "./modelEditorPricing";
import PricingTable from "./PricingTable";
import RateEditor from "./RateEditor";
import { ProviderModelDialog } from "../model_add/ProviderModelDialog";
import type { ListedModel } from "../model_add/providerCatalogImport";
import ModelAutocomplete from "./ModelAutocomplete";
import { toast } from "@/lib/toast";

const selectClass = "h-10 w-full rounded-md border border-input bg-background px-3 text-sm";
export default function ModelEditor({
  model,
  initialSupplier,
  initialCatalogId,
  onSaved,
  onCancel,
}: {
  model?: EditorModel;
  initialSupplier?: CredentialItem;
  initialCatalogId?: string;
  onSaved?: (model: unknown) => void;
  onCancel?: () => void;
}) {
  const { accessToken } = useAuthorized();
  const queryClient = useQueryClient();
  const { data: credentials, isLoading: loadingSuppliers } = useCredentials();
  const { data: catalog, error: catalogError } = usePriceCatalog(accessToken);
  const form = useForm<MountedFormValues>({
    defaultValues: {
      ...editorDefaults(model),
      ...(!model && initialCatalogId ? { pricing_model: initialCatalogId } : {}),
      ...(initialSupplier
        ? {
            supplier: initialSupplier.credential_name,
            custom_llm_provider:
              initialSupplier.credential_info?.custom_llm_provider ??
              initialSupplier.credential_values?.custom_llm_provider,
          }
        : {}),
    },
  });
  const values = useWatch({ control: form.control });
  const registry = useMountRegistry();
  const [saving, setSaving] = useState(false);
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState("");
  const suppliers = credentials?.credentials ?? [];
  const supplier = suppliers.find((item) => item.credential_name === values.supplier);
  const rows = catalogRows(catalog?.models);
  const row = rows[String(values.pricing_model ?? "")];
  const provider = String(values.custom_llm_provider ?? "");
  const builtin = supplier?.credential_info?.builtin;
  const discovery = useQuery({
    queryKey: ["providerModelList", values.supplier],
    enabled: !!accessToken && !!supplier,
    queryFn: async ({ signal }) => {
      const body = await apiClient.post<{ models?: ListedModel[]; error?: string }>("/model/builtin/models", {
        accessToken: accessToken!,
        signal,
        body: { provider: builtin || provider, credential_name: supplier!.credential_name },
      });
      if (body.error) throw new Error(body.error);
      return body.models ?? [];
    },
    retry: false,
    staleTime: 60_000,
  });
  const modelOptions = (discovery.data ?? []).map((item) => ({ value: item.id, label: item.id }));
  const selectModel = (upstream: string) => {
    set("model", upstream);
    if (!values.model_name || values.model_name === String(values.model).replace(provider + "/", ""))
      set("model_name", upstream.replace(provider + "/", ""));
  };
  const changePricingSource = (event: React.ChangeEvent<HTMLSelectElement>) => {
    set("pricing_source", event.target.value);
    if (event.target.value === "manual" && values.pricing_source !== "manual") {
      seedPrices(row);
      set("manual_rates", pricingRates(row));
      set("pricing_format", Array.isArray(row?.rates) && row.rates.length ? "rates" : "flat");
    }
  };
  const changeSupplier = (name: string) => {
    if (name === values.supplier) return;
    if (name === "__existing__") {
      const existing = editorDefaults(model);
      for (const key of [
        "supplier",
        "custom_llm_provider",
        "model",
        "model_name",
        "mode",
        "transport",
        "endpoint_types",
      ])
        set(key, existing[key]);
      return;
    }
    const next = suppliers.find((item) => item.credential_name === name);
    set("supplier", name);
    set(
      "custom_llm_provider",
      next?.credential_info?.custom_llm_provider ?? next?.credential_values?.custom_llm_provider ?? "",
    );
    set("model", "");
    set("catalog_model", "");
    set("mode", "");
    set("transport", "adapted");
    set("endpoint_types", []);
    if (values.model_name === String(values.model).replace(provider + "/", "")) set("model_name", "");
  };

  const categories = BILLING_CATEGORIES.filter((category) =>
    category.modes.includes(values.billing_mode as BillingMode),
  );
  const set = (key: string, value: unknown) => form.setValue(key, value, { shouldDirty: true });
  const seedPrices = (selected: CatalogRow | undefined) => {
    set("billing_mode", billingModeOf(selected));
    PRICE_FIELDS.forEach((field) => set(field.name, catalogFieldValue(selected, field.name)));
  };
  const imported = async () => {
    await queryClient.invalidateQueries({ queryKey: priceCatalogKeys.all });
    await queryClient.invalidateQueries({ queryKey: ["modelCostMap"] });
    await discovery.refetch();
    setImporting(false);
  };
  const textField = (key: string, label: string, placeholder?: string) => (
    <div className="space-y-2">
      <Label htmlFor={"editor-" + key}>{label}</Label>
      <Input
        id={"editor-" + key}
        value={String(values[key] ?? "")}
        onChange={(event) => set(key, event.target.value)}
        placeholder={placeholder}
        required
      />
    </div>
  );
  let submitLabel = model ? "保存修改" : "添加模型";
  if (saving) submitLabel = "正在保存…";
  const save = form.handleSubmit(async (submitted) => {
    setError("");
    try {
      if (!accessToken) throw new Error("登录状态已失效，请重新登录。");
      if (submitted.pricing_source === "catalog" && !hasPrice(rows[String(submitted.pricing_model)]))
        throw new Error("该目录模型尚未定价，请选择有价格的模型或手动定价。");
      if (
        submitted.supplier !== "__existing__" &&
        !suppliers.some((item) => item.credential_name === submitted.supplier)
      )
        throw new Error("请选择已配置的供应商。");
      const payload = buildEditorModel(submitted, model);
      setSaving(true);
      const result = model
        ? await modelPatchUpdateCall(accessToken, payload, String(model.model_info.id))
        : await modelCreateCall(accessToken, payload);
      await queryClient.invalidateQueries({ queryKey: ["models"] });
      toast.success(model ? "模型已更新" : "模型已添加");
      if (!model) form.reset(editorDefaults());
      onSaved?.(result);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "保存失败，请重试。");
    } finally {
      setSaving(false);
    }
  });
  return (
    <FormProvider {...form}>
      {importing && accessToken && supplier && (
        <ProviderModelDialog
          provider={String(builtin || provider)}
          initialCredentialName={supplier.credential_name}
          credentials={suppliers}
          accessToken={accessToken}
          onClose={() => setImporting(false)}
          onAdded={imported}
        />
      )}
      <MountedFormProvider value={{ control: form.control, registry }}>
        <form onSubmit={save} className="space-y-6">
          <div>
            <h2 className="text-xl font-semibold">{model ? "编辑模型" : "添加模型"}</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              依次选择提供商、模型和计费规则。连接凭据在供应商中统一管理。
            </p>
          </div>
          <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)]">
            <div className="space-y-6">
              <section className="space-y-5 rounded-xl border bg-card p-5 sm:p-6">
                <div>
                  <h3 className="font-semibold">1. 选择模型提供商</h3>
                  <p className="mt-1 text-sm text-muted-foreground">
                    选择已配置的供应商，模型将使用它的连接地址与凭据。
                  </p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="editor-supplier">模型提供商 *</Label>
                  <select
                    id="editor-supplier"
                    className={selectClass}
                    value={String(values.supplier ?? "")}
                    required
                    disabled={loadingSuppliers || saving}
                    onChange={(event) => changeSupplier(event.target.value)}
                  >
                    <option value="">{loadingSuppliers ? "正在加载供应商…" : "请选择模型提供商"}</option>
                    {model && <option value="__existing__">保留当前提供商与连接</option>}
                    {model && values.supplier !== "__existing__" && values.supplier && !supplier && (
                      <option value={String(values.supplier)}>{String(values.supplier)}（原供应商）</option>
                    )}
                    {suppliers.map((item) => (
                      <option key={item.credential_name} value={item.credential_name}>
                        {item.credential_name} · {item.credential_info?.custom_llm_provider}
                      </option>
                    ))}
                  </select>
                </div>
                {supplier && (
                  <p className="break-all text-sm text-muted-foreground">
                    {supplier.credential_info?.api_base ?? supplier.credential_values?.api_base ?? "使用供应商默认地址"}
                  </p>
                )}
                {supplier && (
                  <div className="space-y-2">
                    <div className="flex flex-wrap gap-2">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={discovery.isFetching || saving}
                        onClick={() => void discovery.refetch()}
                      >
                        {discovery.isFetching ? "正在获取模型列表…" : "获取模型列表"}
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        disabled={saving}
                        onClick={() => setImporting(true)}
                      >
                        导入模型到广场
                      </Button>
                    </div>
                    {discovery.data && (
                      <p className="text-xs text-muted-foreground">
                        已获取 {discovery.data.length} 个上游模型，可在下方输入框中搜索选择。
                      </p>
                    )}
                    {discovery.error && (
                      <p role="alert" className="text-xs text-destructive">
                        获取模型列表失败：{discovery.error.message}。可重试或直接输入模型 ID。
                      </p>
                    )}
                  </div>
                )}
                {!loadingSuppliers && suppliers.length === 0 && (
                  <p className="text-sm text-muted-foreground">请先在“供应商”页签添加供应商及 API 凭据。</p>
                )}
              </section>
              <fieldset
                disabled={!values.supplier || saving}
                className="space-y-5 rounded-xl border bg-card p-5 sm:p-6 disabled:opacity-50"
              >
                <h3 className="font-semibold">2. 选择模型与调用方式</h3>
                {!values.supplier && <p className="text-sm text-muted-foreground">请先选择模型提供商。</p>}
                <div className="space-y-2">
                  <Label htmlFor="editor-model-select">上游模型 *</Label>
                  <ModelAutocomplete
                    id="editor-model-select"
                    value={String(values.model ?? "")}
                    onChange={(value) => selectModel(value)}
                    onSelect={selectModel}
                    options={modelOptions}
                    placeholder="输入上游模型 ID，或从联想列表中选择"
                    required
                  />
                  <p className="text-xs text-muted-foreground">
                    联想列表仅包含当前模型提供商返回的模型，也可以直接输入模型 ID。
                  </p>
                  {catalogError && <p className="text-xs text-destructive">模型广场加载失败，仍可手动填写模型 ID。</p>}
                </div>
                {textField("model_name", "对外模型名称 *", "客户端请求使用的模型名称")}
                <EndpointTypeField
                  key={String(values.supplier)}
                  selectedProvider={String(values.custom_llm_provider ?? "")}
                  modelCostMap={rows}
                />
                <div className="flex items-center justify-between gap-4 rounded-lg border p-4">
                  <div>
                    <Label htmlFor="editor-enabled">启用模型</Label>
                    <p className="mt-1 text-xs text-muted-foreground">停用后不接受新请求，可随时重新启用。</p>
                  </div>
                  <Switch
                    id="editor-enabled"
                    checked={values.disabled !== true}
                    onCheckedChange={(checked) => set("disabled", !checked)}
                  />
                </div>
              </fieldset>
            </div>
            <fieldset
              disabled={!values.supplier || !values.model || saving}
              className="space-y-5 rounded-xl border bg-card p-5 sm:p-6"
            >
              <div>
                <h3 className="font-semibold">3. 设置计价</h3>
                <p className="mt-1 text-sm text-muted-foreground">
                  独立选择价格目录模型或自定义费率。修改提供商或上游模型不会改变计价设置。
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="editor-pricing-source">价格来源</Label>
                <select
                  id="editor-pricing-source"
                  className={selectClass}
                  value={String(values.pricing_source)}
                  onChange={changePricingSource}
                >
                  <option value="catalog">使用价格目录</option>
                  <option value="manual">手动设置部署价格</option>
                </select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="editor-pricing-model">
                  目录定价模型{values.pricing_source === "catalog" ? " *" : "（参考）"}
                </Label>
                <select
                  className={selectClass}
                  id="editor-pricing-model"
                  value={String(values.pricing_model ?? "")}
                  onChange={(event) => {
                    set("pricing_model", event.target.value);
                  }}
                >
                  <option value="">请选择目录定价模型</option>
                  {Object.entries(rows)
                    .filter(([, item]) => hasPrice(item))
                    .map(([id]) => (
                      <option key={id} value={id}>
                        {id}
                      </option>
                    ))}
                </select>
              </div>
              {values.pricing_source === "catalog" ? (
                <>
                  {!hasPrice(row) ? (
                    <p className="text-sm text-amber-600">请选择有价格的目录模型；目录中没有的模型需要手动定价。</p>
                  ) : (
                    <PricingTable row={row} />
                  )}
                </>
              ) : (
                <>
                  <div className="space-y-2">
                    <Label htmlFor="editor-pricing-format">费率设置方式</Label>
                    <select
                      id="editor-pricing-format"
                      className={selectClass}
                      value={String(values.pricing_format ?? "flat")}
                      onChange={(event) => {
                        if (event.target.value === "rates" && values.pricing_format !== "rates") {
                          try {
                            const current = buildEditorModel({ ...values, pricing_format: "flat" });
                            set("manual_rates", pricingRates(current.litellm_params));
                          } catch {
                            set("manual_rates", []);
                          }
                        }
                        set("pricing_format", event.target.value);
                      }}
                    >
                      <option value="flat">基础单价</option>
                      <option value="rates">按项目、时段与规格设置费率</option>
                    </select>
                  </div>
                  {values.pricing_format === "rates" ? (
                    <RateEditor
                      rates={Array.isArray(values.manual_rates) ? values.manual_rates : []}
                      onChange={(rates) => set("manual_rates", rates)}
                    />
                  ) : (
                    <>
                      <div className="space-y-2">
                        <Label htmlFor="editor-billing-mode">计费方式</Label>
                        <select
                          id="editor-billing-mode"
                          className={selectClass}
                          value={String(values.billing_mode)}
                          onChange={(event) => set("billing_mode", event.target.value)}
                        >
                          <option value="token">按 Token（缓存 / 闲时 / 高峰）</option>
                          <option value="image">按图片张数</option>
                          <option value="second">按秒</option>
                        </select>
                      </div>
                      {categories.map((category) => (
                        <fieldset key={category.id} className="space-y-3 rounded-lg border p-4">
                          <legend className="px-1 text-sm font-medium">{category.title}</legend>
                          <div className="grid gap-4 sm:grid-cols-2">
                            {category.fields.map((field) => (
                              <div key={field.name} className="space-y-2">
                                <Label htmlFor={"editor-" + field.name}>{field.label}</Label>
                                <Input
                                  id={"editor-" + field.name}
                                  type="number"
                                  min="0"
                                  step="any"
                                  value={String(values[field.name] ?? "")}
                                  onChange={(event) => set(field.name, event.target.value)}
                                  placeholder="未设置"
                                />
                              </div>
                            ))}
                          </div>
                          <p className="text-xs text-muted-foreground">{category.unit}</p>
                        </fieldset>
                      ))}
                      <p className="text-xs text-muted-foreground">
                        留空表示未设置，0
                        表示免费。高峰价留空时使用基础价；缓存价留空时沿用输入价。修改单价将以当前表单的价格替换部署费率表。
                      </p>
                    </>
                  )}
                </>
              )}
            </fieldset>
          </div>
          {error && (
            <p
              role="alert"
              className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive"
            >
              {error}
            </p>
          )}
          <div className="flex justify-end gap-3 border-t pt-5">
            {onCancel && (
              <Button type="button" variant="outline" disabled={saving} onClick={onCancel}>
                取消
              </Button>
            )}
            <Button type="submit" disabled={saving || loadingSuppliers || !values.supplier || !values.model}>
              {submitLabel}
            </Button>
          </div>
        </form>
      </MountedFormProvider>
    </FormProvider>
  );
}

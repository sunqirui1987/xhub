import { BILLING_CATEGORIES, type BillingMode } from "./billing_categories";

export const PRICE_FIELDS = BILLING_CATEGORIES.flatMap((category) => category.fields);
export type CatalogRow = Record<string, unknown>;
export type PriceRate = {
  measure: string;
  side: string;
  window: string;
  variant?: string;
  usd: number;
  unit_size?: number;
};
export type EditorModel = {
  model_name: string;
  litellm_params: Record<string, unknown>;
  model_info: Record<string, unknown>;
};

export function catalogModelId(rows: Record<string, CatalogRow>, upstream: string, provider = ""): string {
  const bare = upstream.includes("/") ? upstream.slice(upstream.indexOf("/") + 1) : upstream;
  const candidates = [upstream, bare, provider + "/" + upstream];
  for (const [id, row] of Object.entries(rows)) {
    if (Array.isArray(row.model_alias) && row.model_alias.some((alias) => candidates.includes(String(alias))))
      candidates.push(id);
  }
  return candidates.find((id) => hasPrice(rows[id])) ?? candidates.find((id) => rows[id]) ?? "";
}

export function catalogRows(models?: unknown): Record<string, CatalogRow> {
  const entries = Array.isArray(models) ? (models as CatalogRow[]) : [];
  return Object.fromEntries(entries.filter((row) => !row.removed).map((row) => [String(row.id), row]));
}

// Deployment overrides have the same precedence as the gateway's cost lookup.
export function effectivePricing(model: EditorModel, rows: Record<string, CatalogRow>) {
  if (hasPrice(model.litellm_params)) return { row: model.litellm_params, source: "部署定价" };
  if (model.model_info.pricing_source === "manual") return { row: undefined, source: "部署定价（未设置）" };
  const explicit = String(model.model_info.base_model ?? "");
  if (explicit) {
    const resolved = catalogModelId(rows, explicit);
    return { row: rows[resolved], source: "价格目录 · " + (resolved || explicit) };
  }
  const upstream = catalogModelId(
    rows,
    String(model.litellm_params.model ?? ""),
    String(model.litellm_params.custom_llm_provider ?? ""),
  );
  const id = hasPrice(rows[upstream]) ? upstream : catalogModelId(rows, model.model_name) || upstream;
  return { row: rows[id], source: "价格目录 · " + id };
}

export function pricingRates(row?: CatalogRow): PriceRate[] {
  if (!row) return [];
  if (Array.isArray(row.rates) && row.rates.length) return row.rates as PriceRate[];
  return PRICE_FIELDS.filter((field) => typeof row[field.name] === "number").map((field) => {
    let window = field.ratesFilter?.window ?? "all";
    if (window === "offpeak" && row[field.name + "_peak"] == null) window = "all";
    return {
      measure: rateMeasure(field.name),
      side: field.ratesFilter?.side ?? "",
      window,
      usd: Number(row[field.name]),
    };
  });
}

export function hasPrice(row?: CatalogRow): boolean {
  return (
    !!row &&
    ((Array.isArray(row.rates) && row.rates.length > 0) ||
      PRICE_FIELDS.some((field) => typeof row[field.name] === "number"))
  );
}
export function billingModeOf(row?: CatalogRow): BillingMode {
  const rates = Array.isArray(row?.rates) ? (row.rates as PriceRate[]) : [];
  if (
    rates.some((rate) => rate.measure === "second") ||
    row?.output_cost_per_second != null ||
    row?.input_cost_per_second != null
  )
    return "second";
  if (
    rates.some((rate) => rate.measure === "picture") ||
    row?.output_cost_per_image != null ||
    row?.input_cost_per_image != null
  )
    return "image";
  return "token";
}
export function rateMeasure(name: string): string {
  if (name.includes("image")) return "picture";
  if (name.includes("second")) return "second";
  if (name.includes("query")) return "query";
  return "token";
}
export function catalogFieldValue(row: CatalogRow | undefined, name: string): string {
  if (!row) return "";
  const field = PRICE_FIELDS.find((entry) => entry.name === name)!;
  const candidates = (Array.isArray(row.rates) ? (row.rates as PriceRate[]) : []).filter(
    (rate) =>
      rate.measure === rateMeasure(name) &&
      rate.side === field.ratesFilter?.side &&
      (rate.window === field.ratesFilter?.window ||
        (field.ratesFilter?.window === "offpeak" && rate.window === "all")) &&
      Number.isFinite(rate.usd) &&
      rate.usd >= 0,
  );
  // Qualified catalog prices stay in the rate table; don't flatten them to the minimum.
  const exact = candidates.find((rate) => !rate.variant || rate.variant === "uncached");
  const value = exact?.usd ?? (candidates.length ? undefined : row[name]);
  return typeof value === "number" ? String(Number((value * field.scale).toPrecision(12))) : "";
}
export function editorDefaults(model?: EditorModel): Record<string, unknown> {
  const params = model?.litellm_params ?? {};
  const info = model?.model_info ?? {};
  return {
    supplier: params.litellm_credential_name ?? (model ? "__existing__" : ""),
    custom_llm_provider:
      params.custom_llm_provider ??
      (String(params.model ?? "").includes("/") ? String(params.model).split("/")[0] : "openai"),
    model: params.model ?? "",
    model_name: model?.model_name ?? "",
    mode: info.mode ?? "",
    transport: info.transport ?? "adapted",
    endpoint_types: info.endpoint_types ?? [],
    disabled: info.disabled === true,
    pricing_source: info.pricing_source ?? (hasPrice(params) ? "manual" : "catalog"),
    pricing_model: info.base_model ?? params.model ?? "",
    billing_mode: billingModeOf(params),
    pricing_format: Array.isArray(params.rates) && params.rates.length ? "rates" : "flat",
    manual_rates: pricingRates(params),
    ...Object.fromEntries(PRICE_FIELDS.map((field) => [field.name, catalogFieldValue(params, field.name)])),
  };
}
export function buildEditorModel(values: Record<string, unknown>, original?: EditorModel): EditorModel {
  const params: Record<string, unknown> = {
    model: String(values.model ?? "").trim(),
    custom_llm_provider: values.custom_llm_provider,
  };
  const info: Record<string, unknown> = {
    mode: values.mode,
    transport: values.transport,
    endpoint_types: values.endpoint_types,
    disabled: values.disabled === true,
    pricing_source: values.pricing_source,
    endpoint: values.endpoint ?? null,
  };
  const name = String(values.model_name ?? "").trim();
  if (!name || !params.model || !params.custom_llm_provider || !values.supplier)
    throw new Error("请选择供应商，并填写上游模型和对外模型名称。");
  if (!values.mode) throw new Error("请选择调用方式。");
  if (values.supplier === "__existing__" && !original) throw new Error("请选择已配置的模型提供商。");
  if (values.supplier !== "__existing__") {
    params.litellm_credential_name = values.supplier;
    for (const key of Object.keys(
      values.supplier === original?.litellm_params.litellm_credential_name ? {} : (original?.litellm_params ?? {}),
    )) {
      if (
        /api_base|api_key|api_version|organization|secret|password|access_key|credentials|region_name|token$|^aws_|^s3_|^vertex_|^azure_|^tenant_id$|^client_id$|^gcs_/.test(
          key,
        ) &&
        !PRICE_FIELDS.some((field) => field.name === key)
      )
        params[key] = null;
    }
  }
  for (const field of PRICE_FIELDS) params[field.name] = null;
  params.rates = null;
  if (values.pricing_source === "catalog") {
    info.base_model = values.pricing_model;
    if (!info.base_model) throw new Error("请选择目录中的定价模型。");
  } else if (values.pricing_format === "rates") {
    info.base_model = null;
    const rates = values.manual_rates as PriceRate[];
    if (!Array.isArray(rates) || !rates.length) throw new Error("请至少添加一个费率。");
    if (
      rates.some(
        (rate) =>
          !Number.isFinite(rate.usd) ||
          rate.usd < 0 ||
          !["token", "picture", "second", "query"].includes(rate.measure) ||
          !["input", "output", "cache_read", "cache_write", "batch_input", "batch_output"].includes(rate.side) ||
          !["all", "offpeak", "peak"].includes(rate.window),
      )
    )
      throw new Error("请检查费率单位、项目、时段与单价。");
    const keys = rates.map((rate) => [rate.measure, rate.side, rate.window, rate.variant || ""].join(":"));
    if (new Set(keys).size !== keys.length) throw new Error("相同单位、项目、时段与规格的费率不能重复。");
    params.rates = rates;
  } else {
    info.base_model = null;
    const mode = values.billing_mode as BillingMode;
    const fields = BILLING_CATEGORIES.filter((category) => category.modes.includes(mode)).flatMap(
      (category) => category.fields,
    );
    let filled = false;
    for (const field of fields) {
      const raw = values[field.name];
      if (raw === "" || raw == null) continue;
      const value = Number(raw);
      if (!Number.isFinite(value) || value < 0) throw new Error("价格必须是大于或等于 0 的有效数字。");
      params[field.name] = value / field.scale;
      filled = true;
    }
    const defaults = editorDefaults(original);
    if (
      original &&
      defaults.pricing_source === "manual" &&
      hasPrice(original.litellm_params) &&
      mode === defaults.billing_mode &&
      fields.every((field) => String(values[field.name] ?? "") === String(defaults[field.name] ?? ""))
    ) {
      params.rates = original.litellm_params.rates ?? null;
      for (const field of PRICE_FIELDS) params[field.name] = original.litellm_params[field.name] ?? null;
      filled = true;
    }
    if (!filled) throw new Error("请至少填写一个单价。免费模型请明确填写 0。");
  }
  return { model_name: name, litellm_params: params, model_info: info };
}

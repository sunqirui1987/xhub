/**
 * Turns the price catalog API response into the rows and suppliers the
 * /price-data page renders and edits.
 *
 * Rates arrive per token, which is the unit the gateway bills in. The console
 * shows per 1M tokens, so every token rate is scaled on the way out and scaled
 * back on the way in. A missing rate stays null: an unpriced side must render as
 * unavailable, not as $0.
 */

export const FALLBACK_CATEGORY = "other";

export type PriceCatalogRow = {
  id: string;
  category: string;
  provider: string | null;
  displayName: string | null;
  input: number | null;
  output: number | null;
  maxInput: number | null;
  maxOutput: number | null;
  capabilities: string[];
  extraPrices: { key: string; value: number; unit: string }[];
  /**
   * 变体费率：价目表里那些不属于通用输入/输出的价。
   *
   * 视频和图像模型按分辨率和输入方式分别定价（480p/720p/1080p、有无视频输入），
   * 一条模型能带十几档。它们不是"额外信息"，是这类模型**唯一**的价——只显示
   * 通用输入输出的话，这类模型的卡片上会写着"价格未提供"，而价目表里其实是有的。
   */
  priceUnits: { key: string; label: string; usd: number; unit: string; size: number }[];
  /** True when the row comes from the generated catalog rather than a console entry. */
  baseline: boolean;
  /** True when this id is stored in the database as an override of the baseline. */
  overridden: boolean;
  /** True when the operator deleted a baseline row; the id stays listed so it can be restored. */
  removed: boolean;
  /** The fields as the API sent them, so the edit form can seed every rate it does not show. */
  raw: Record<string, unknown>;
};

export type PriceProviderRow = {
  slug: string;
  name: string;
  apiBase: string | null;
  modelCount: number;
  baseline: boolean;
  overridden: boolean;
};

/** The catalog document the backend serves at GET /price/catalog. */
export type PriceCatalogDocument = {
  models?: unknown;
  providers?: unknown;
  source?: string | null;
  generated_at?: string | null;
  loaded_at?: string | null;
  count?: number | null;
};

const EXTRA_PRICES = [
  ["cache_read_input_token_cost", "cacheRead", "tokens"],
  ["cache_creation_input_token_cost", "cacheWrite", "tokens"],
  ["input_cost_per_image", "imageInput", "image"],
  ["output_cost_per_image", "imageOutput", "image"],
  ["input_cost_per_second", "secondInput", "second"],
  ["output_cost_per_second", "secondOutput", "second"],
] as const;

const CAPABILITIES = [
  ["supports_function_calling", "tools"],
  ["supports_response_schema", "structured"],
  ["supports_reasoning", "reasoning"],
  ["supports_vision", "vision"],
  ["supports_prompt_caching", "caching"],
] as const;

/** The token rates the edit form writes back, in the order it shows them. */
export const EDITABLE_TOKEN_RATES = [
  { field: "input_cost_per_token", labelKey: "Input" },
  { field: "output_cost_per_token", labelKey: "Output" },
  { field: "cache_read_input_token_cost", labelKey: "priceCatalog.cacheRead" },
  { field: "cache_creation_input_token_cost", labelKey: "priceCatalog.cacheWrite" },
] as const;

/** The per-unit rates the edit form writes back. */
export const EDITABLE_UNIT_RATES = [
  { field: "output_cost_per_image", labelKey: "priceCatalog.imageOutput" },
  { field: "output_cost_per_second", labelKey: "priceCatalog.secondOutput" },
] as const;

/**
 * Reads the models half of the catalog document. A malformed document yields an
 * empty list rather than throwing, so the page can show its empty state.
 */
export function priceCatalogRows(doc: PriceCatalogDocument | null | undefined): PriceCatalogRow[] {
  const models = doc?.models;
  let entries: Record<string, unknown>[] = [];
  if (Array.isArray(models)) {
    entries = models.filter(isObject);
  } else if (isObject(models)) {
    entries = Object.entries(models)
      .filter(([, value]) => isObject(value))
      .map(([id, value]) => ({ ...(value as Record<string, unknown>), id }));
  }
  const rows: PriceCatalogRow[] = [];
  for (const entry of entries) {
    const id = typeof entry.id === "string" ? entry.id.trim() : "";
    if (!id) continue;
    const mode = typeof entry.mode === "string" ? entry.mode.trim() : "";
    const row: PriceCatalogRow = {
      id,
      category: mode || FALLBACK_CATEGORY,
      provider: stringValue(entry.litellm_provider) ?? stringValue(entry.provider),
      displayName: stringValue(entry.display_name),
      input: perMillion(entry.input_cost_per_token),
      output: perMillion(entry.output_cost_per_token),
      maxInput: positiveNumber(entry.max_input_tokens),
      maxOutput: positiveNumber(entry.max_output_tokens),
      capabilities: CAPABILITIES.filter(([field]) => entry[field] === true).map(([, label]) => label),
      extraPrices: EXTRA_PRICES.flatMap(([field, key, unit]) => {
        const value = unit === "tokens" ? perMillion(entry[field]) : nonnegativeNumber(entry[field]);
        return value == null ? [] : [{ key, value, unit }];
      }),
      priceUnits: priceUnitsOf(entry.price_units),
      baseline: entry.baseline === true,
      overridden: entry.overridden === true,
      removed: entry.removed === true,
      raw: entry,
    };
    rows.push(row);
  }
  rows.sort((a, b) => a.category.localeCompare(b.category) || a.id.localeCompare(b.id));
  return rows;
}

/** Reads the providers half of the catalog document. */
export function priceCatalogProviders(doc: PriceCatalogDocument | null | undefined): PriceProviderRow[] {
  const providers = doc?.providers;
  if (!Array.isArray(providers)) return [];
  const rows: PriceProviderRow[] = [];
  for (const entry of providers.filter(isObject)) {
    const slug = stringValue(entry.litellm_provider);
    if (!slug) continue;
    rows.push({
      slug,
      name: stringValue(entry.provider_display_name) ?? stringValue(entry.provider) ?? slug,
      apiBase: stringValue(entry.default_api_base),
      modelCount: nonnegativeNumber(entry.model_count) ?? 0,
      baseline: entry.baseline === true,
      overridden: entry.overridden === true,
    });
  }
  return rows;
}

/**
 * Builds the request body for saving one model. Token rates are divided back to
 * per-token, and a blank field is sent as null so the backend clears that rate
 * instead of storing a zero.
 */
export function priceModelPayload(input: {
  id: string;
  provider: string;
  displayName?: string;
  mode?: string;
  endpointType?: string;
  tokenRates: Record<string, string>;
  unitRates: Record<string, string>;
}): Record<string, unknown> {
  const body: Record<string, unknown> = {
    id: input.id.trim(),
    litellm_provider: input.provider.trim(),
  };
  if (input.displayName?.trim()) body.display_name = input.displayName.trim();
  if (input.mode?.trim()) body.mode = input.mode.trim();
  if (input.endpointType?.trim()) body.endpoint_type = input.endpointType.trim();
  for (const [field, raw] of Object.entries(input.tokenRates)) {
    const value = parseRate(raw);
    body[field] = value == null ? null : value / 1_000_000;
  }
  for (const [field, raw] of Object.entries(input.unitRates)) {
    body[field] = parseRate(raw);
  }
  return body;
}

/**
 * Parses a rate the operator typed. A blank field returns null so the caller can
 * tell "cleared" from "zero", which bills differently.
 */
export function parseRate(raw: string | null | undefined): number | null {
  if (raw == null) return null;
  const trimmed = raw.trim();
  if (trimmed === "") return null;
  const value = Number(trimmed);
  return Number.isFinite(value) && value >= 0 ? value : null;
}

/** Formats a per-token rate for the edit form, which shows per 1M tokens. */
export function rateToInputValue(value: unknown): string {
  const rate = nonnegativeNumber(value);
  if (rate == null) return "";
  const scaled = rate * 1_000_000;
  // Keep small rates readable without exponent notation, which an <input> rejects.
  return Number.isFinite(scaled) ? String(Number(scaled.toPrecision(12))) : "";
}

/** Formats a per-unit rate for the edit form. */
export function unitRateToInputValue(value: unknown): string {
  const rate = nonnegativeNumber(value);
  return rate == null ? "" : String(Number(rate.toPrecision(12)));
}

/**
 * 读出变体费率。每一项是价目表里的一个键，带它自己的单位和说明。
 * 参数 raw（unknown）：price_units 的原始值。
 * 返回（{key,label,usd,unit,size}[]）：按美元价从低到高排好的变体。
 */
function priceUnitsOf(raw: unknown): { key: string; label: string; usd: number; unit: string; size: number }[] {
  if (!isObject(raw)) return [];
  const out: { key: string; label: string; usd: number; unit: string; size: number }[] = [];
  for (const [key, value] of Object.entries(raw)) {
    if (!isObject(value)) continue;
    const usd = nonnegativeNumber(value.usd);
    if (usd === null) continue;
    out.push({
      key,
      label: stringValue(value.label) ?? key,
      usd,
      unit: stringValue(value.unit) ?? "",
      size: nonnegativeNumber(value.size) ?? 1,
    });
  }
  out.sort((a, b) => a.usd - b.usd || a.key.localeCompare(b.key));
  return out;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function stringValue(value: unknown): string | null {
  if (typeof value !== "string") return null;
  const normalized = value.trim();
  return normalized || null;
}

function perMillion(value: unknown): number | null {
  const rate = nonnegativeNumber(value);
  if (rate == null) return null;
  // The stored rate is per token; the console shows per 1M. Multiplying back
  // reintroduces binary-float noise (1e-7 becomes 0.09999999999999999), so the
  // result is rounded to 12 significant digits — well beyond any real price,
  // and short of the noise.
  const scaled = rate * 1_000_000;
  if (!Number.isFinite(scaled)) return null;
  return Number(scaled.toPrecision(12));
}

function nonnegativeNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : null;
}

function positiveNumber(value: unknown): number | null {
  const number = nonnegativeNumber(value);
  return number != null && number > 0 ? number : null;
}

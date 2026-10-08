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

/** 一条费率，来自价目表的 rates[]。四个维度加单价，和 Go 侧的 catalog.Rate 同形。 */
export type CatalogRate = {
  /** token、second、picture、query。 */
  measure: string;
  /** 一个计价单位包多少个基础单位。token 类通常是 1000，其余是 1。 */
  unitSize: number;
  /** input、output、cache_read、cache_write、batch_input、batch_output。 */
  side: string;
  /** 同一侧内部的限定词，例如 uncached、cached、thinking、480p。空串表示无限定词。 */
  variant: string;
  /** peak、offpeak 或 all。 */
  window: string;
  /** 市场里原来的键名，例如 ncache_peak。运维排查时对得上市场。 */
  sourceKey: string;
  /** 市场给的中文说明。 */
  label: string;
  /** 每一个基础单位的美元价。 */
  usd: number;
};

/**
 * 一条模型的费率表，按计费维度分好组。
 *
 * 分组是渲染需要的形状：一个维度一行标题（"按 token"），组内按
 * 侧和变体成行、按时段成列。分时价并排显示——它们是一件事的两面，折起来
 * 看不出差一倍。
 */
export type RateGroup = {
  measure: string;
  /** 这个维度下一个计价单位包多少个基础单位。 */
  unitSize: number;
  rates: CatalogRate[];
  /** 这个维度里出现过的时段，用来决定表头有几列。 */
  windows: string[];
};

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
   * 费率表，按计费维度分组。
   *
   * 这是这条模型**能不能被计费**的完整答案：哪些维度、哪一侧、哪个时段、
   * 什么价。通用的输入/输出两格只是其中最常见的两行，视频和图像模型在
   * 那两格上是空的，价却在下面。
   */
  rateGroups: RateGroup[];
  /** 这条模型是不是分时计价。分时的话高峰价通常是空闲价的两倍。 */
  windowed: boolean;
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
  { field: "input_cost_per_token_peak", labelKey: "priceCatalog.inputPeak" },
  { field: "output_cost_per_token_peak", labelKey: "priceCatalog.outputPeak" },
  { field: "cache_read_input_token_cost", labelKey: "priceCatalog.cacheRead" },
  { field: "cache_creation_input_token_cost", labelKey: "priceCatalog.cacheWrite" },
] as const;

/** The per-unit rates the edit form writes back. */
export const EDITABLE_UNIT_RATES = [
  { field: "input_cost_per_image", labelKey: "priceCatalog.imageInput" },
  { field: "output_cost_per_image", labelKey: "priceCatalog.imageOutput" },
  { field: "input_cost_per_second", labelKey: "priceCatalog.secondInput" },
  { field: "output_cost_per_second", labelKey: "priceCatalog.secondOutput" },
  { field: "search_context_cost_per_query", labelKey: "priceCatalog.query" },
] as const;

export const EDITABLE_RATE_FIELDS = [...EDITABLE_TOKEN_RATES, ...EDITABLE_UNIT_RATES] as const;

type EditableRateField = (typeof EDITABLE_RATE_FIELDS)[number]["field"];

/**
 * The unqualified rates[] entry that can stand in for each legacy flat field.
 * Variant rates are deliberately excluded: editing a flat price must not turn
 * an uncached, thinking, resolution, or other qualified price into the base
 * price.
 */
const CATALOG_FIELD_RATE_SPECS: Record<
  EditableRateField,
  { measure: string; side: string; windows: readonly string[] }
> = {
  input_cost_per_token: { measure: "token", side: "input", windows: ["all", "offpeak"] },
  output_cost_per_token: { measure: "token", side: "output", windows: ["all", "offpeak"] },
  input_cost_per_token_peak: { measure: "token", side: "input", windows: ["peak"] },
  output_cost_per_token_peak: { measure: "token", side: "output", windows: ["peak"] },
  cache_read_input_token_cost: { measure: "token", side: "cache_read", windows: ["all"] },
  cache_creation_input_token_cost: { measure: "token", side: "cache_write", windows: ["all"] },
  input_cost_per_image: { measure: "picture", side: "input", windows: ["all"] },
  output_cost_per_image: { measure: "picture", side: "output", windows: ["all"] },
  input_cost_per_second: { measure: "second", side: "input", windows: ["all"] },
  output_cost_per_second: { measure: "second", side: "output", windows: ["all"] },
  search_context_cost_per_query: { measure: "query", side: "output", windows: ["all"] },
};

/**
 * Reads an editable flat catalog field, falling back to its matching base rate
 * in rates[]. A valid flat value always wins, including zero. Only an
 * unqualified rate is eligible so a no-op edit cannot flatten rate variants.
 */
export function catalogFieldValue(raw: Record<string, unknown>, field: EditableRateField): number | null {
  const flat = nonnegativeNumber(raw[field]);
  if (flat != null) return flat;

  const spec = CATALOG_FIELD_RATE_SPECS[field];
  if (!Array.isArray(raw.rates)) return null;
  for (const window of spec.windows) {
    for (const candidate of raw.rates) {
      if (!isObject(candidate)) continue;
      if ((stringValue(candidate.measure) ?? "") !== spec.measure) continue;
      if ((stringValue(candidate.side) ?? "") !== spec.side) continue;
      if ((stringValue(candidate.variant) ?? "") !== "") continue;
      if ((stringValue(candidate.window) ?? "all") !== window) continue;
      const value = nonnegativeNumber(candidate.usd);
      if (value != null) return value;
    }
  }
  return null;
}

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
    const rates = rateGroupsOf(entry.rates);
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
      rateGroups: rates,
      windowed: rates.some((group) => group.rates.some((rate) => rate.window === "peak" || rate.window === "offpeak")),
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
  /** Existing API row. When present, unchanged rates are omitted from the patch. */
  original?: Record<string, unknown>;
}): Record<string, unknown> {
  const body: Record<string, unknown> = {
    id: input.id.trim(),
    litellm_provider: input.provider.trim(),
  };
  if (input.displayName?.trim()) body.display_name = input.displayName.trim();
  if (input.mode?.trim()) body.mode = input.mode.trim();
  if (input.endpointType?.trim()) body.endpoint_type = input.endpointType.trim();
  for (const [field, raw] of Object.entries(input.tokenRates)) {
    if (
      input.original &&
      rateInputUnchanged(raw, rateToInputValue(catalogFieldValue(input.original, field as EditableRateField)))
    )
      continue;
    const value = parseRate(raw);
    body[field] = value == null ? null : value / 1_000_000;
  }
  for (const [field, raw] of Object.entries(input.unitRates)) {
    if (
      input.original &&
      rateInputUnchanged(raw, unitRateToInputValue(catalogFieldValue(input.original, field as EditableRateField)))
    )
      continue;
    body[field] = parseRate(raw);
  }
  return body;
}

/** Returns API field names whose nonblank values are not finite non-negative numbers. */
export function invalidRateFields(rates: Record<string, string>): string[] {
  return Object.entries(rates)
    .filter(([, raw]) => raw.trim() !== "" && parseRate(raw) == null)
    .map(([field]) => field);
}

function rateInputUnchanged(current: string, original: string): boolean {
  const left = current.trim();
  const right = original.trim();
  if (left === "" || right === "") return left === right;
  return Number(left) === Number(right);
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
 * 把价目表的 rates[] 读成按计费维度分好组的费率表。
 *
 * 参数 raw（unknown）：这一行价格里的 rates 字段。
 * 返回 RateGroup[]（RateGroup[]）：按维度分组的费率，组内和组间都有稳定的顺序。
 */
export function rateGroupsOf(raw: unknown): RateGroup[] {
  if (!Array.isArray(raw)) return [];
  const byMeasure = new Map<string, CatalogRate[]>();
  for (const item of raw) {
    if (!isObject(item)) continue;
    const measure = stringValue(item.measure);
    const usd = nonnegativeNumber(item.usd);
    if (!measure || usd === null) continue;
    const rate: CatalogRate = {
      measure,
      unitSize: positiveNumber(item.unit_size) ?? 1,
      side: stringValue(item.side) ?? "",
      variant: stringValue(item.variant) ?? "",
      window: stringValue(item.window) ?? "all",
      sourceKey: stringValue(item.source_key) ?? "",
      label: stringValue(item.label) ?? "",
      usd,
    };
    const bucket = byMeasure.get(measure);
    if (bucket) bucket.push(rate);
    else byMeasure.set(measure, [rate]);
  }

  // 维度的顺序是固定的：先 token（绝大多数模型），再按张、按秒，最后按次。
  // 用固定顺序而不是输入的先后，是因为两个模型对比时顺序应当一致。
  //
  // 认不出的维度接在最后，不丢掉：市场加一个新的计价维度时，运维要能看见它，
  // 而不是等到对账时才发现账单里有一档价格从来没显示过。
  const order = ["token", "picture", "second", "query"];
  const known = new Set(order);
  const remaining = [...byMeasure.keys()].filter((measure) => !known.has(measure)).sort();
  const groups: RateGroup[] = [];
  for (const measure of [...order, ...remaining]) {
    const rates = byMeasure.get(measure);
    if (!rates || rates.length === 0) continue;
    // 组内先按时段（空闲在前，高峰在后，和界面上并排的顺序一致），
    // 再按侧，最后按变体，这样同一个侧的两个时段总是相邻两行。
    const windowRank = (w: string) => {
      if (w === "offpeak") return 0;
      if (w === "peak") return 1;
      return 2;
    };
    rates.sort(
      (a, b) =>
        windowRank(a.window) - windowRank(b.window) ||
        a.side.localeCompare(b.side) ||
        a.variant.localeCompare(b.variant) ||
        a.sourceKey.localeCompare(b.sourceKey),
    );
    groups.push({
      measure,
      unitSize: rates[0].unitSize,
      rates,
      windows: [...new Set(rates.map((rate) => rate.window))],
    });
  }
  return groups;
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

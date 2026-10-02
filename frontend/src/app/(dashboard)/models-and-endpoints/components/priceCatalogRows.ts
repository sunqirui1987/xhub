export const FALLBACK_CATEGORY = "other";

export type PriceCatalogRow = {
  id: string;
  category: string;
  provider: string | null;
  input: number | null;
  output: number | null;
  maxInput: number | null;
  maxOutput: number | null;
  capabilities: string[];
  extraPrices: { key: string; value: number; unit: string }[];
};

const EXTRA_PRICES = [
  ["cache_read_input_token_cost", "cacheRead", "tokens"],
  ["cache_creation_input_token_cost", "cacheWrite", "tokens"],
  ["input_cost_per_image", "imageInput", "image"],
  ["output_cost_per_image", "imageOutput", "image"],
  ["input_cost_per_second", "secondInput", "second"],
  ["output_cost_per_second", "secondOutput", "second"],
  ["input_cost_per_audio_token", "audioInput", "tokens"],
  ["output_cost_per_audio_token", "audioOutput", "tokens"],
] as const;

const CAPABILITIES = [
  ["supports_function_calling", "tools"],
  ["supports_response_schema", "structured"],
  ["supports_reasoning", "reasoning"],
  ["supports_vision", "vision"],
  ["supports_prompt_caching", "caching"],
] as const;

// Every object entry except the sample_spec documentation row. Category is the entry's mode.
// Entries with no mode share one fallback category so they stay in the list.
// Rates are dollars per 1M tokens. A missing side stays null. A zero rate is kept.
export function priceCatalogRows(costMap: Record<string, unknown> | null | undefined): PriceCatalogRow[] {
  if (!costMap) return [];
  const rows: PriceCatalogRow[] = [];
  for (const [id, value] of Object.entries(costMap)) {
    if (id === "sample_spec") continue;
    if (!value || typeof value !== "object" || Array.isArray(value)) continue;
    const entry = value as Record<string, unknown>;
    const mode = typeof entry.mode === "string" ? entry.mode.trim() : "";
    const row: PriceCatalogRow = {
      id,
      category: mode || FALLBACK_CATEGORY,
      provider: stringValue(entry.litellm_provider) ?? stringValue(entry.provider),
      input: perMillion(entry.input_cost_per_token),
      output: perMillion(entry.output_cost_per_token),
      maxInput: positiveNumber(entry.max_input_tokens),
      maxOutput: positiveNumber(entry.max_output_tokens),
      capabilities: CAPABILITIES.filter(([field]) => entry[field] === true).map(([, label]) => label),
      extraPrices: EXTRA_PRICES.flatMap(([field, key, unit]) => {
        const value = unit === "tokens" ? perMillion(entry[field]) : nonnegativeNumber(entry[field]);
        return value == null ? [] : [{ key, value, unit }];
      }),
    };
    rows.push(row);
  }
  rows.sort((a, b) => a.category.localeCompare(b.category) || a.id.localeCompare(b.id));
  return rows;
}

function stringValue(value: unknown): string | null {
  if (typeof value !== "string") return null;
  const normalized = value.trim();
  return normalized || null;
}

function perMillion(value: unknown): number | null {
  const rate = nonnegativeNumber(value);
  return rate != null && Number.isFinite(rate * 1_000_000) ? rate * 1_000_000 : null;
}

function nonnegativeNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : null;
}

function positiveNumber(value: unknown): number | null {
  const number = nonnegativeNumber(value);
  return number != null && number > 0 ? number : null;
}

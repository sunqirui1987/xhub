export type MyModelCard = {
  id: string;
  provider: string | null;
  category: string;
  capabilities: string[];
  max_input_tokens: number | null;
  max_output_tokens: number | null;
  input_price: number | null;
  output_price: number | null;
  cache_read_price: number | null;
  cache_write_price: number | null;
};

export type ModelAccessRow = {
  model_name?: string | null;
  model_info?: Record<string, unknown> | null;
};

export type CostRow = {
  mode?: unknown;
  litellm_provider?: unknown;
  provider?: unknown;
  max_input_tokens?: unknown;
  max_output_tokens?: unknown;
  supports_function_calling?: unknown;
  supports_response_schema?: unknown;
  supports_reasoning?: unknown;
  supports_vision?: unknown;
  supports_prompt_caching?: unknown;
  input_cost_per_token?: unknown;
  output_cost_per_token?: unknown;
  cache_read_input_token_cost?: unknown;
  cache_creation_input_token_cost?: unknown;
};

// Cards the caller may open: granted ids joined to stored rows for category and price.
// Provider shells and ids that were not granted stay out, even when those rows are in the input.
// A missing stored price is filled from the price-data map (dollars per 1M tokens). A stored price wins.
export function grantedModelCards(
  grantedIds: string[],
  rows: ModelAccessRow[],
  costMap?: Record<string, CostRow> | null,
): MyModelCard[] {
  const rowsByName = new Map<string, ModelAccessRow>();
  for (const row of rows) {
    const name = (row.model_name ?? "").trim();
    if (name) rowsByName.set(name, row);
  }
  const seen = new Set<string>();
  const cards: MyModelCard[] = [];
  for (const raw of grantedIds) {
    const id = raw.trim();
    if (!id || seen.has(id)) continue;
    seen.add(id);
    const info = rowsByName.get(id)?.model_info ?? {};
    if (info.role === "provider") continue;
    const priced = costRow(costMap, id);
    const endpointTypes = Array.isArray(info.endpoint_types) ? info.endpoint_types : null;
    const category = stringValue(info.category)
      ?? (stringValue(info.transport) && info.transport !== "adapted" ? stringValue(info.transport) : null)
      ?? (endpointTypes ? endpointTypes.find((value): value is string => typeof value === "string" && value.trim() !== "") : null)
      ?? stringValue(priced?.mode)
      ?? "chat";
    cards.push({
      id,
      category,
      provider:
        stringValue(info.litellm_provider) ??
        stringValue(info.provider) ??
        stringValue(priced?.litellm_provider) ??
        stringValue(priced?.provider),
      capabilities: [
        ["supports_function_calling", "tools"],
        ["supports_response_schema", "structured"],
        ["supports_reasoning", "reasoning"],
        ["supports_vision", "vision"],
        ["supports_prompt_caching", "caching"],
      ]
        .filter(([field]) => info[field] === true || (info[field] == null && priced?.[field as keyof CostRow] === true))
        .map(([, label]) => label),
      max_input_tokens: positiveNumber(info.max_input_tokens) ?? positiveNumber(priced?.max_input_tokens),
      max_output_tokens: positiveNumber(info.max_output_tokens) ?? positiveNumber(priced?.max_output_tokens),
      input_price: storedRate(info.input_price) ?? perMillion(priced?.input_cost_per_token),
      output_price: storedRate(info.output_price) ?? perMillion(priced?.output_cost_per_token),
      cache_read_price: storedRate(info.cache_read_price) ?? perMillion(priced?.cache_read_input_token_cost),
      cache_write_price: storedRate(info.cache_write_price) ?? perMillion(priced?.cache_creation_input_token_cost),
    });
  }
  return cards;
}

function costRow(costMap: Record<string, CostRow> | null | undefined, id: string): CostRow | undefined {
  if (!costMap) return undefined;
  if (costMap[id]) return costMap[id];
  const slash = id.lastIndexOf("/");
  if (slash >= 0) return costMap[id.slice(slash + 1)];
  return undefined;
}

function storedRate(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : null;
}

function positiveNumber(value: unknown): number | null {
  const number = storedRate(value);
  return number != null && number > 0 ? number : null;
}

function stringValue(value: unknown): string | null {
  return typeof value === "string" && value.trim() ? value.trim() : null;
}

function perMillion(value: unknown): number | null {
  const rate = storedRate(value);
  return rate == null ? null : rate * 1_000_000;
}

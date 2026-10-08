/**
 * The template editor's document, kept separate from the table so a save can
 * round-trip through fields instead of a JSON textarea.
 *
 * The request path replaces a whole document rather than merging fields, so the
 * keys this form does not show are carried in `rest` and written back. Dropping
 * them on save would silently change routing for every scope that selected the
 * template.
 */

/** Strategy ids the editor offers. Labels live in the catalogs, not here. */
export const ROUTING_STRATEGIES = [
  "simple-shuffle",
  "least-busy",
  "latency-based-routing",
  "cost-based-routing",
  "usage-based-routing",
  "usage-based-routing-v2",
  "weighted-split",
] as const;

/** Failures that may switch to another model. The default is all three. */
export const FALLBACK_CAUSES = ["no_response", "status", "ambiguous"] as const;
export type FallbackCause = (typeof FALLBACK_CAUSES)[number];

/** One row of a fallback chain: a primary model and the models that follow it. */
export type ChainRow = { primary: string; targets: string };

/**
 * One deployment's share of traffic under weighted-split.
 *
 * The id the router matches is api_base|model. model_name is the public name
 * the operator groups by. A deployment left out of this list keeps its own weight.
 */
export type WeightRow = {
  model_name: string;
  api_base: string;
  model: string;
  weight: string;
};

/** A live deployment the weight table can offer a row for. */
export type SplitDeployment = {
  model_name: string;
  api_base: string;
  model: string;
};

export type TemplateRoutingGroup = {
  group_name: string;
  models: string[];
  routing_strategy: string;
  routing_strategy_args?: Record<string, unknown> | null;
};

/** Reads the routing_groups field for the visual editor without accepting malformed rows. */
export const parseRoutingGroups = (text: string): { ok: true; groups: TemplateRoutingGroup[] } | { ok: false } => {
  if (text.trim() === "") return { ok: true, groups: [] };
  try {
    const value = JSON.parse(text) as unknown;
    if (!Array.isArray(value)) return { ok: false };
    const groups: TemplateRoutingGroup[] = [];
    for (const item of value) {
      if (!isRecord(item)) return { ok: false };
      if (typeof item.group_name !== "string" || typeof item.routing_strategy !== "string") return { ok: false };
      if (!Array.isArray(item.models) || !item.models.every((model) => typeof model === "string")) return { ok: false };
      const args = item.routing_strategy_args;
      if (args !== undefined && args !== null && !isRecord(args)) return { ok: false };
      groups.push({
        group_name: item.group_name,
        models: item.models,
        routing_strategy: item.routing_strategy,
        ...(args !== undefined ? { routing_strategy_args: args as Record<string, unknown> | null } : {}),
      });
    }
    return { ok: true, groups };
  } catch {
    return { ok: false };
  }
};

export const prettyRoutingGroups = (groups: TemplateRoutingGroup[]): string =>
  groups.length > 0 ? JSON.stringify(groups, null, 2) : "";

export type TemplateFormState = {
  routing_strategy: string;
  num_retries: string;
  timeout: string;
  allowed_fails: string;
  cooldown_time: string;
  retry_after: string;
  max_fallbacks: string;
  enable_tag_filtering: boolean;
  fallback_causes: string[];
  fallbacks: ChainRow[];
  context_window_fallbacks: ChainRow[];
  content_policy_fallbacks: ChainRow[];
  /** LiteLLM default_fallbacks: models used when this name has no chain of its own. */
  default_fallbacks: string;
  weights: WeightRow[];
  /** Latency-based routing window, seconds. Empty omits it. */
  ttl: string;
  /** Latency-based routing buffer, percent. Empty omits it. */
  lowest_latency_buffer: string;
  stream_timeout: string;
  enable_pre_call_checks: boolean;
  /** JSON text of routing_groups. Empty means the key is omitted. */
  routing_groups: string;
  /** JSON text of retry_policy. Empty means the key is omitted. */
  retry_policy: string;
  /** JSON text of model_group_alias. Empty means the key is omitted. */
  model_group_alias: string;
  /**
   * JSON object of every other key. The form and the JSON tab both edit this,
   * so a key the named fields do not know is still visible and still saved.
   */
  extra: string;
};

const EDITED_KEYS = new Set([
  "routing_strategy",
  "num_retries",
  "timeout",
  "allowed_fails",
  "cooldown_time",
  "retry_after",
  "max_fallbacks",
  "enable_tag_filtering",
  "fallback_causes",
  "fallbacks",
  "context_window_fallbacks",
  "content_policy_fallbacks",
  "default_fallbacks",
  "routing_strategy_args",
  "stream_timeout",
  "enable_pre_call_checks",
  "routing_groups",
  "retry_policy",
  "model_group_alias",
]);

/** A new template starts as the same numbers the platform default uses. */
export const emptyForm = (): TemplateFormState => ({
  routing_strategy: "simple-shuffle",
  num_retries: "2",
  timeout: "60",
  allowed_fails: "3",
  cooldown_time: "0",
  retry_after: "0",
  max_fallbacks: "5",
  enable_tag_filtering: false,
  fallback_causes: [...FALLBACK_CAUSES],
  fallbacks: [],
  context_window_fallbacks: [],
  content_policy_fallbacks: [],
  default_fallbacks: "",
  weights: [],
  ttl: "",
  lowest_latency_buffer: "",
  stream_timeout: "",
  enable_pre_call_checks: false,
  routing_groups: "",
  retry_policy: "",
  model_group_alias: "",
  extra: "",
});

const numberText = (value: unknown): string => {
  if (typeof value === "number" && Number.isFinite(value)) return String(value);
  if (typeof value === "string" && value.trim() !== "" && Number.isFinite(Number(value))) return value.trim();
  return "";
};

const chainsFrom = (value: unknown): ChainRow[] => {
  if (!Array.isArray(value)) return [];
  const rows: ChainRow[] = [];
  for (const entry of value) {
    if (entry == null || typeof entry !== "object" || Array.isArray(entry)) continue;
    for (const [primary, targets] of Object.entries(entry as Record<string, unknown>)) {
      const list = Array.isArray(targets) ? targets.filter((item): item is string => typeof item === "string") : [];
      rows.push({ primary, targets: list.join(", ") });
    }
  }
  return rows;
};

const causesFrom = (value: unknown): string[] => {
  if (!Array.isArray(value)) return [...FALLBACK_CAUSES];
  return FALLBACK_CAUSES.filter((cause) => value.includes(cause));
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
  value != null && typeof value === "object" && !Array.isArray(value);

/** The live platform document from a router-settings response. Missing means empty. */
export const documentFromSettingsResponse = (data: unknown): Record<string, unknown> => {
  if (!isRecord(data)) return {};
  const document = data.router_settings ?? data.current_values;
  return isRecord(document) ? document : {};
};

/**
 * Drops routing_groups from a platform save that did not edit them.
 *
 * The routing-groups section writes that key on its own. Sending the copy this
 * editor opened with would put the older list back.
 */
export const omitUntouchedRoutingGroups = (
  body: Record<string, unknown>,
  current: string,
  opened: string,
): Record<string, unknown> => {
  if (current !== opened || !("routing_groups" in body)) return body;
  const next = { ...body };
  delete next.routing_groups;
  return next;
};

export const deploymentKey = (row: { api_base: string; model: string; model_name?: string }): string => {
  const model = row.model.trim() || row.model_name?.trim() || "";
  return `${row.api_base.trim()}|${model}`;
};

const weightsFrom = (value: unknown): WeightRow[] => {
  if (Array.isArray(value)) {
    return value.flatMap((item) => {
      if (!isRecord(item)) return [];
      const modelName = typeof item.model_name === "string" ? item.model_name : "";
      const model = typeof item.model === "string" ? item.model : modelName;
      const apiBase = typeof item.api_base === "string" ? item.api_base : "";
      return [{ model_name: modelName || model, api_base: apiBase, model, weight: numberText(item.weight) }];
    });
  }
  if (isRecord(value)) {
    return Object.entries(value).flatMap(([id, weight]) => {
      const splitAt = id.indexOf("|");
      const apiBase = splitAt >= 0 ? id.slice(0, splitAt) : "";
      const model = splitAt >= 0 ? id.slice(splitAt + 1) : id;
      return [{ model_name: model, api_base: apiBase, model, weight: numberText(weight) }];
    });
  }
  return [];
};

const weightsTo = (rows: WeightRow[]): Array<Record<string, unknown>> => {
  const out: Array<Record<string, unknown>> = [];
  for (const row of rows) {
    const model = row.model.trim() || row.model_name.trim();
    if (!model && !row.api_base.trim()) continue;
    const parsed = writeNumber(row.weight);
    if (!parsed.ok || parsed.value === undefined || parsed.value < 0) continue;
    const stored = {
      model_name: row.model_name.trim() || model,
      api_base: row.api_base.trim(),
      model,
      weight: parsed.value,
    };
    out.push(stored);
  }
  return out;
};

/**
 * Rows for the weight table: one per live deployment, plus any share the
 * document already names that is not in the catalog yet.
 *
 * A deployment with no saved share starts at 1, which is what the split does
 * when nobody set a weight. Saving that 1 makes the document say so.
 */
export const weightRowsForEditor = (saved: WeightRow[], deployments: SplitDeployment[]): WeightRow[] => {
  const byId = new Map(saved.map((row) => [deploymentKey(row), row]));
  const seen = new Set<string>();
  const rows: WeightRow[] = [];
  for (const deployment of deployments) {
    const key = deploymentKey(deployment);
    seen.add(key);
    const existing = byId.get(key);
    const row = {
      model_name: deployment.model_name,
      api_base: deployment.api_base,
      model: deployment.model,
      weight: existing?.weight || "1",
    };
    rows.push(row);
  }
  for (const row of saved) {
    if (!seen.has(deploymentKey(row)) && (row.model.trim() || row.model_name.trim())) rows.push(row);
  }
  return rows;
};

/** Pulls deployments out of the model info list. Auto routers are not endpoints. */
export const deploymentsFromInfo = (rows: unknown[]): SplitDeployment[] => {
  const out: SplitDeployment[] = [];
  for (const row of rows) {
    if (!isRecord(row)) continue;
    const params = isRecord(row.litellm_params) ? row.litellm_params : {};
    const modelName = typeof row.model_name === "string" ? row.model_name.trim() : "";
    const model = typeof params.model === "string" && params.model.trim() !== "" ? params.model.trim() : modelName;
    if (!model || model.startsWith("auto_router/")) continue;
    const apiBase = typeof params.api_base === "string" ? params.api_base.trim() : "";
    out.push({ model_name: modelName || model, api_base: apiBase, model });
  }
  return out;
};

const jsonText = (value: unknown): string => {
  if (value == null) return "";
  if (typeof value === "string") return value;
  return JSON.stringify(value, null, 2);
};

const listText = (value: unknown): string => {
  if (!Array.isArray(value)) return "";
  return value.filter((item): item is string => typeof item === "string").join(", ");
};

/** Reads a stored document into the fields the editor shows. Every key is visible. */
export const formFromBody = (body: Record<string, unknown> | undefined): TemplateFormState => {
  const source = body ?? {};
  const extra: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(source)) {
    if (!EDITED_KEYS.has(key)) extra[key] = value;
  }
  const strategy =
    typeof source.routing_strategy === "string" && source.routing_strategy.trim() !== ""
      ? source.routing_strategy.trim()
      : "simple-shuffle";
  const args = isRecord(source.routing_strategy_args) ? { ...source.routing_strategy_args } : {};
  const weights = weightsFrom(args.weights);
  const ttl = numberText(args.ttl);
  const lowestLatencyBuffer = numberText(args.lowest_latency_buffer);
  delete args.weights;
  delete args.ttl;
  delete args.lowest_latency_buffer;
  if (Object.keys(args).length > 0) extra.routing_strategy_args = args;
  return {
    routing_strategy: strategy,
    num_retries: numberText(source.num_retries),
    timeout: numberText(source.timeout),
    allowed_fails: numberText(source.allowed_fails),
    cooldown_time: numberText(source.cooldown_time),
    retry_after: numberText(source.retry_after),
    max_fallbacks: numberText(source.max_fallbacks),
    enable_tag_filtering: source.enable_tag_filtering === true,
    fallback_causes: causesFrom(source.fallback_causes),
    fallbacks: chainsFrom(source.fallbacks),
    context_window_fallbacks: chainsFrom(source.context_window_fallbacks),
    content_policy_fallbacks: chainsFrom(source.content_policy_fallbacks),
    default_fallbacks: listText(source.default_fallbacks),
    weights,
    ttl,
    lowest_latency_buffer: lowestLatencyBuffer,
    stream_timeout: numberText(source.stream_timeout),
    enable_pre_call_checks: source.enable_pre_call_checks === true,
    routing_groups: jsonText(source.routing_groups),
    retry_policy: jsonText(source.retry_policy),
    model_group_alias: jsonText(source.model_group_alias),
    extra: Object.keys(extra).length > 0 ? JSON.stringify(extra, null, 2) : "",
  };
};

/** A new template starts as a copy of the live platform document, every key included. */
export const prefillFromPlatform = (settings?: Record<string, unknown>): TemplateFormState => {
  if (!settings) return emptyForm();
  return formFromBody(settings);
};

const chainsTo = (rows: ChainRow[]): Array<Record<string, string[]>> => {
  const out: Array<Record<string, string[]>> = [];
  for (const row of rows) {
    const primary = row.primary.trim();
    if (!primary) continue;
    const targets = row.targets
      .split(/[,，\n]/)
      .map((item) => item.trim())
      .filter((item) => item !== "");
    out.push({ [primary]: targets });
  }
  return out;
};

const writeNumber = (raw: string): { ok: true; value?: number } | { ok: false } => {
  const trimmed = raw.trim();
  if (trimmed === "") return { ok: true };
  const value = Number(trimmed);
  if (!Number.isFinite(value)) return { ok: false };
  return { ok: true, value };
};

export type BodyResult = { ok: true; body: Record<string, unknown> } | { ok: false; field: string };

const parseJsonField = (raw: string, field: string): { ok: true; value?: unknown } | { ok: false; field: string } => {
  const trimmed = raw.trim();
  if (trimmed === "") return { ok: true };
  try {
    return { ok: true, value: JSON.parse(trimmed) as unknown };
  } catch {
    return { ok: false, field };
  }
};

/** Writes the editor back to a router settings document. An empty number omits that key. */
export const bodyFromForm = (form: TemplateFormState): BodyResult => {
  const numbers: Array<[string, string]> = [
    ["num_retries", form.num_retries],
    ["timeout", form.timeout],
    ["stream_timeout", form.stream_timeout],
    ["allowed_fails", form.allowed_fails],
    ["cooldown_time", form.cooldown_time],
    ["retry_after", form.retry_after],
    ["max_fallbacks", form.max_fallbacks],
  ];
  const extra = parseJsonField(form.extra, "extra");
  if (!extra.ok) return extra;
  if (extra.value !== undefined && !isRecord(extra.value)) return { ok: false, field: "extra" };
  const body: Record<string, unknown> = extra.value && isRecord(extra.value) ? { ...extra.value } : {};
  for (const [key, raw] of numbers) {
    const parsed = writeNumber(raw);
    if (!parsed.ok) return { ok: false, field: key };
    if (parsed.value === undefined) delete body[key];
    else body[key] = parsed.value;
  }
  body.routing_strategy = form.routing_strategy.trim() || "simple-shuffle";
  body.enable_tag_filtering = form.enable_tag_filtering;
  body.enable_pre_call_checks = form.enable_pre_call_checks;
  body.fallback_causes = FALLBACK_CAUSES.filter((cause) => form.fallback_causes.includes(cause));
  body.fallbacks = chainsTo(form.fallbacks);
  body.context_window_fallbacks = chainsTo(form.context_window_fallbacks);
  body.content_policy_fallbacks = chainsTo(form.content_policy_fallbacks);
  const defaults = form.default_fallbacks
    .split(/[,，\n]/)
    .map((item) => item.trim())
    .filter((item) => item !== "");
  if (defaults.length > 0) body.default_fallbacks = defaults;
  else delete body.default_fallbacks;
  for (const [key, raw] of [
    ["routing_groups", form.routing_groups],
    ["retry_policy", form.retry_policy],
    ["model_group_alias", form.model_group_alias],
  ] as const) {
    const parsed = parseJsonField(raw, key);
    if (!parsed.ok) return parsed;
    if (parsed.value === undefined) delete body[key];
    else body[key] = parsed.value;
  }
  const carried = isRecord(body.routing_strategy_args) ? { ...body.routing_strategy_args } : {};
  delete carried.weights;
  delete carried.ttl;
  delete carried.lowest_latency_buffer;
  const args: Record<string, unknown> = { ...carried };
  for (const row of form.weights) {
    if (!row.model.trim() && !row.model_name.trim() && !row.api_base.trim()) continue;
    const parsed = writeNumber(row.weight);
    if (!parsed.ok || (parsed.value !== undefined && parsed.value < 0)) return { ok: false, field: "weights" };
  }
  const weights = weightsTo(form.weights);
  if (weights.length > 0) args.weights = weights;
  const ttl = writeNumber(form.ttl);
  if (!ttl.ok) return { ok: false, field: "ttl" };
  if (ttl.value !== undefined) args.ttl = ttl.value;
  const buffer = writeNumber(form.lowest_latency_buffer);
  if (!buffer.ok) return { ok: false, field: "lowest_latency_buffer" };
  if (buffer.value !== undefined) args.lowest_latency_buffer = buffer.value;
  if (Object.keys(args).length > 0) body.routing_strategy_args = args;
  else delete body.routing_strategy_args;
  return { ok: true, body };
};

/** The document as the operator edits it: one JSON object, pretty printed. */
export const prettyDocument = (body: Record<string, unknown>): string => JSON.stringify(body, null, 2);

/** Parses a configuration file. Anything that is not one JSON object is refused. */
export const parseDocument = (text: string): { ok: true; body: Record<string, unknown> } | { ok: false } => {
  try {
    const parsed = JSON.parse(text) as unknown;
    if (!isRecord(parsed)) return { ok: false };
    return { ok: true, body: parsed };
  } catch {
    return { ok: false };
  }
};

/** The strategies the dropdown offers, plus the current value when it is an older name. */
export const strategyOptions = (current: string): string[] => {
  const known: string[] = [...ROUTING_STRATEGIES];
  if (current && !known.includes(current)) known.push(current);
  return known;
};

/**
 * A copy's name. The first free "copy" wins; a second copy of the same template
 * gets a number so two copies are not the same row.
 */
export const nextCopyName = (name: string, existing: readonly string[], copyOf: (name: string) => string): string => {
  const base = copyOf(name).trim() || name;
  if (!existing.includes(base)) return base;
  for (let n = 2; n < 100; n += 1) {
    const candidate = `${base} ${n}`;
    if (!existing.includes(candidate)) return candidate;
  }
  return `${base} ${existing.length + 1}`;
};

/** Local wall time for the updated column. Empty when the server sent nothing usable. */
export const formatUpdatedAt = (value: string | undefined): string => {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
};

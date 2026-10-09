import { validateSections, type TemplateSections } from "./templateSections";
import { policyError, strategyLabels, type Allocation } from "./allocationPolicy";

/** 部署目录投影用于模型与组的权重表单及预览；仅含展示信息，不包含凭据。 */
export type SplitDeployment = {
  deployment_id?: string;
  model_name: string;
  api_base: string;
  model: string;
  endpoint_types?: string[];
  transport?: string;
  unavailable_reason?: string;
  provider?: string;
  supplier?: string;
};

/** 模板模型规则支持独立权重；省略 allocations 继承模型默认，空数组表示均等权重。 */
export type ModelRoutingRule = { model: string; strategy: string; allocations?: Allocation[] };

/** 路由模板表单包含模型规则和模板级重试参数。 */
export type TemplateFormState = TemplateSections & {
  routing_strategy: string;
  max_attempts: string;
  timeout_seconds: string;
  failure_threshold: string;
  cooldown_seconds: string;
  model_routes: ModelRoutingRule[];
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === "object" && !Array.isArray(value);

/** emptyForm 为创建页返回模型负载均衡、组、回退和可靠性骨架；无参数和外部副作用。 */
export const emptyForm = (): TemplateFormState => ({
  routing_strategy: "",
  max_attempts: "1",
  timeout_seconds: "60",
  failure_threshold: "3",
  cooldown_seconds: "60",
  model_routes: [],
  routing_groups: [],
  fallbacks: [],
  context_window_fallbacks: [],
  content_policy_fallbacks: [],
});

/** formFromBody 将保存的文档转换为独立草稿；参数为正文，返回受控表单，无写入。
 * 旧模板缺少默认策略时保留继承语义，不在编辑或复制时自动改变实际请求行为。 */
export const formFromBody = (body: Record<string, unknown> = {}): TemplateFormState => {
  const retry = isRecord(body.retry_policy) ? body.retry_policy : {};
  return {
    ...emptyForm(),
    ...Object.fromEntries(
      ["routing_groups", "fallbacks", "context_window_fallbacks", "content_policy_fallbacks"].map((key) => [
        key,
        Object.hasOwn(body, key) ? structuredClone(body[key]) : undefined,
      ]),
    ),
    routing_strategy: typeof body.routing_strategy === "string" ? body.routing_strategy : "",
    ...Object.fromEntries(
      ["max_attempts", "timeout_seconds", "failure_threshold", "cooldown_seconds"]
        .filter((key) => retry[key] !== undefined)
        .map((key) => [key, String(retry[key])]),
    ),
    model_routes: Array.isArray(body.model_routes) ? structuredClone(body.model_routes) : [],
  };
};

/** bodyFromForm 写出唯一模板格式，拒绝无效数字、重复模型和未知策略。 */
export const bodyFromForm = (
  form: TemplateFormState,
): { ok: true; body: Record<string, unknown> } | { ok: false; field: string } => {
  const body: Record<string, unknown> = {};
  if (form.routing_strategy) {
    if (!Object.hasOwn(strategyLabels, form.routing_strategy)) return { ok: false, field: "routing_strategy" };
    body.routing_strategy = form.routing_strategy;
  }
  const retryPolicy: Record<string, number> = {};
  for (const key of ["max_attempts", "timeout_seconds", "failure_threshold", "cooldown_seconds"] as const) {
    const value = Number(form[key]);
    if (
      !form[key].trim() ||
      !Number.isFinite(value) ||
      value < 0 ||
      ((key === "max_attempts" || key === "timeout_seconds") && value === 0) ||
      ((key === "max_attempts" || key === "failure_threshold") && !Number.isInteger(value))
    )
      return { ok: false, field: key };
    retryPolicy[key] = value;
  }
  const seen = new Set<string>();
  for (const rule of form.model_routes) {
    const name = rule.model.trim();
    if (!name || seen.has(name) || !Object.hasOwn(strategyLabels, rule.strategy)) {
      return { ok: false, field: "model_routes" };
    }
    if (Object.hasOwn(rule, "allocations")) {
      if (rule.strategy !== "traffic-split" || !Array.isArray(rule.allocations))
        return { ok: false, field: "model_routes.allocations" };
      if (rule.allocations.length && policyError(rule)) return { ok: false, field: "model_routes.allocations" };
    }
    seen.add(name);
  }
  body.model_routes = form.model_routes.map((rule) => ({
    model: rule.model.trim(),
    strategy: rule.strategy,
    ...(Object.hasOwn(rule, "allocations") ? { allocations: structuredClone(rule.allocations) } : {}),
  }));
  body.retry_policy = retryPolicy;
  for (const key of ["routing_groups", "fallbacks", "context_window_fallbacks", "content_policy_fallbacks"] as const) {
    if (form[key] !== undefined) body[key] = structuredClone(form[key]);
  }
  const error = validateSections(body);
  if (error) return { ok: false, field: error };
  return { ok: true, body };
};

/** parseDocument 严格解析导入 JSON；入口级条件和未知字段失败；组内支持按部署ID配置权重。 */
export const parseDocument = (text: string): { ok: true; body: Record<string, unknown> } | { ok: false } => {
  try {
    const body: unknown = JSON.parse(text);
    const allowed = [
      "routing_strategy",
      "model_routes",
      "retry_policy",
      "routing_groups",
      "fallbacks",
      "context_window_fallbacks",
      "content_policy_fallbacks",
    ];
    if (!isRecord(body) || Object.keys(body).some((key) => !allowed.includes(key))) return { ok: false };
    if (
      Object.hasOwn(body, "routing_strategy") &&
      (typeof body.routing_strategy !== "string" || !Object.hasOwn(strategyLabels, body.routing_strategy))
    )
      return { ok: false };
    const retryFields = ["max_attempts", "timeout_seconds", "failure_threshold", "cooldown_seconds"];
    if (
      !isRecord(body.retry_policy) ||
      Object.keys(body.retry_policy).some((key) => !retryFields.includes(key)) ||
      retryFields.some((key) => typeof (body.retry_policy as Record<string, unknown>)[key] !== "number")
    )
      return { ok: false };
    if (
      !Array.isArray(body.model_routes) ||
      body.model_routes.some(
        (rule) =>
          !isRecord(rule) ||
          typeof rule.model !== "string" ||
          typeof rule.strategy !== "string" ||
          Object.keys(rule).some((key) => !["model", "strategy", "allocations"].includes(key)),
      )
    )
      return { ok: false };
    if (validateSections(body)) return { ok: false };
    const normalized = bodyFromForm(formFromBody(body));
    return normalized.ok ? { ok: true, body: normalized.body } : { ok: false };
  } catch {
    return { ok: false };
  }
};

/** prettyDocument 格式化模板 JSON，供查看、下载和编辑使用。 */
export const prettyDocument = (body: Record<string, unknown>): string => JSON.stringify(body, null, 2);

/** nextCopyName 为复制模板选择未占用名称，不写后台。 */
export const nextCopyName = (name: string, existing: readonly string[], copyOf: (name: string) => string): string => {
  const base = copyOf(name).trim() || name;
  let next = base;
  for (let n = 2; existing.includes(next); n++) next = base + " " + n;
  return next;
};

/** formatUpdatedAt 格式化有效时间；空值和错误时间返回空字符串。 */
export const formatUpdatedAt = (value: string | undefined): string => {
  if (!value) return "";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "" : date.toLocaleString();
};

/** deploymentsFromInfo 从模型接口投影公开模型目录，保持上游模型标识原样。 */
export const deploymentsFromInfo = (rows: unknown[]): SplitDeployment[] => {
  const out: SplitDeployment[] = [];
  for (const row of rows) {
    if (!isRecord(row)) continue;
    const params = isRecord(row.litellm_params) ? row.litellm_params : {};
    const modelName = typeof row.model_name === "string" ? row.model_name : "";
    const model = typeof params.model === "string" && params.model.trim() ? params.model : modelName;
    if (!model || model.startsWith("auto_router/")) continue;
    const info = isRecord(row.model_info) ? row.model_info : {};
    const deploymentId = [params.deployment_id, info.id].find((value) => typeof value === "string" && value.trim());
    if (!deploymentId) continue;
    out.push({
      deployment_id: deploymentId as string,
      model_name: modelName || model,
      api_base: typeof params.api_base === "string" ? params.api_base.trim() : "",
      model,
      endpoint_types: Array.isArray(info.endpoint_types)
        ? info.endpoint_types.filter((id): id is string => typeof id === "string")
        : [],
      transport: typeof info.transport === "string" ? info.transport : "",
      unavailable_reason: typeof info.unavailable_reason === "string" ? info.unavailable_reason : undefined,
      provider: typeof params.custom_llm_provider === "string" ? params.custom_llm_provider : undefined,
      supplier: typeof params.litellm_credential_name === "string" ? params.litellm_credential_name : undefined,
    });
  }
  return out;
};

import {
  createRouteTemplateCall,
  deleteRouteTemplateCall,
  getRouteTemplatesCall,
  getRouteTemplateUsageCall,
  updateRouteTemplateCall,
  type RouteTemplate,
  type RouteTemplateUsage,
} from "@/components/networking";

/**
 * What a template row looks like once it is on screen.
 *
 * The backend answers with the usage count inline, and the scope list only when
 * asked. Both are kept here so the delete confirmation can name the users without
 * a second round trip in the common case, and can still show them when the count
 * was stale.
 */
export type RouteTemplateRow = RouteTemplate & {
  usedBy: number;
  writable: boolean;
};

/**
 * Turns the list response into the rows the table renders.
 *
 * A row without an explicit writable flag is treated as read-only. Guessing the
 * other way would offer an edit button that fails, and the failure names nothing
 * the operator can act on.
 */
export const toTemplateRows = (templates: RouteTemplate[]): RouteTemplateRow[] =>
  templates.map((template) => ({
    ...template,
    usedBy: template.used_by ?? 0,
    writable: template.writable === true,
  }));

/**
 * summarizeTemplate 为模板库生成执行摘要；参数为正文和本地化标签函数，返回一行策略、例外数量及可靠性说明。
 * 优先显示模板默认策略；旧正文未设置默认时显示模型规则或继承，缺失可靠性字段使用展示默认值。
 * 仅转换展示数据，不补写配置或调用后台。
 */
export const summarizeTemplate = (
  body: Record<string, unknown>,
  labels: {
    strategy: (value: string) => string;
    retries: (value: number) => string;
    timeout: (seconds: number) => string;
    fallbacks: (value: number) => string;
    none: string;
    models?: (value: number) => string;
  },
): string => {
  const count = Array.isArray(body.model_routes) ? body.model_routes.length : 0;
  const retry =
    body.retry_policy && typeof body.retry_policy === "object" ? (body.retry_policy as Record<string, unknown>) : {};
  return [
    typeof body.routing_strategy === "string"
      ? labels.strategy(body.routing_strategy) + (count ? " · 模型例外 " + count + " 条" : "")
      : count
        ? "模型规则 " + count + " 条"
        : "全部使用模型默认",
    labels.retries(typeof retry.max_attempts === "number" ? retry.max_attempts : 1),
    labels.timeout(typeof retry.timeout_seconds === "number" ? retry.timeout_seconds : 60),
  ].join(" · ");
};

/** The scope kinds a template can be selected on, in inheritance order. */
export const SCOPE_KINDS = ["organization", "team", "key"] as const;
export type ScopeKind = (typeof SCOPE_KINDS)[number];

/**
 * Saves a template, creating it when there is no id.
 *
 * The body is sent as the object the form holds. An empty document is allowed
 * here and rejected on the server side only if the name is missing: a template
 * whose settings are not filled in yet is a name the operator is about to use,
 * and refusing to save it would lose the name they just typed.
 */
export const saveRouteTemplate = async (
  accessToken: string,
  template: {
    id?: string;
    name: string;
    body: Record<string, unknown>;
    /**
     * Ownership for a new template. Both absent means a platform template.
     *
     * Ignored on update on purpose: an organization administrator editing a
     * team's template must not quietly transfer it to the organization, and a
     * team editing its own must not promote it to the platform.
     */
    owner?: { organizationId?: string; teamId?: string };
  },
): Promise<void> => {
  const name = template.name.trim();
  if (!name) throw new Error("name is required");
  if (template.id) {
    await updateRouteTemplateCall(accessToken, template.id, { name, body: template.body });
    return;
  }
  const created = {
    name,
    body: template.body,
    organization_id: template.owner?.organizationId,
    team_id: template.owner?.teamId,
  };
  await createRouteTemplateCall(accessToken, created);
};

/**
 * Describes a delete refusal so the operator knows what stands in the way.
 *
 * The 409 body carries the scopes that select the template. When it does not -
 * an older gateway, or a failure that is not a refusal - the fallback text says
 * so instead of inventing a cause.
 */
export const deleteRefusalText = (
  error: unknown,
  labels: { inUse: (count: number) => string; failed: string },
): { message: string; usage: RouteTemplateUsage[] } => {
  const body = (error as { body?: unknown })?.body;
  if (body && typeof body === "object") {
    const parsed = body as { error?: { code?: string; used_by?: RouteTemplateUsage[] } };
    if (parsed.error?.code === "template_in_use") {
      const usage = parsed.error.used_by ?? [];
      return { message: labels.inUse(usage.length), usage };
    }
  }
  return { message: labels.failed, usage: [] };
};

/** Loads everything the page needs, in one place so the refresh path is one call. */
export const loadRouteTemplates = async (accessToken: string): Promise<RouteTemplateRow[]> =>
  toTemplateRows(await getRouteTemplatesCall(accessToken));

/** Loads the scopes selecting one template. */
export const loadTemplateUsage = async (accessToken: string, id: string): Promise<RouteTemplateUsage[]> =>
  getRouteTemplateUsageCall(accessToken, id);

/** Removes a template, surfacing the refusal rather than swallowing it. */
export const removeRouteTemplate = async (accessToken: string, id: string): Promise<void> => {
  await deleteRouteTemplateCall(accessToken, id);
};

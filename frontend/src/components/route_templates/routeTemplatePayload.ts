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
 * One line describing what a template does, from its settings document.
 *
 * It reads the same keys the request path reads, so the summary cannot claim a
 * strategy the data plane would ignore. Missing keys fall back to the platform
 * defaults rather than to blank, because a template seeded from the default has
 * no reason to spell them out.
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
  const strategy =
    typeof body.routing_strategy === "string" && body.routing_strategy !== ""
      ? labels.strategy(body.routing_strategy)
      : labels.strategy("simple-shuffle");
  const retries =
    typeof body.num_retries === "number" && Number.isFinite(body.num_retries) && body.num_retries >= 1
      ? Math.trunc(body.num_retries)
      : 1;
  // A missing or non-positive timeout is 60 on the request path. The summary
  // says that, so the row does not claim a timeout the call will not use.
  const timeout =
    typeof body.timeout === "number" && Number.isFinite(body.timeout) && body.timeout > 0 ? body.timeout : 60;
  const fallbacks = Array.isArray(body.fallbacks) ? body.fallbacks.length : 0;
  const parts = [strategy, labels.retries(retries), labels.timeout(timeout)];
  if (Array.isArray(body.model_routing) && body.model_routing.length && labels.models) {
    parts.push(labels.models(body.model_routing.length));
  }
  parts.push(fallbacks > 0 ? labels.fallbacks(fallbacks) : labels.none);
  return parts.join(" · ");
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

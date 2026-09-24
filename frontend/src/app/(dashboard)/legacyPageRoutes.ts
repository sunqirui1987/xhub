import { uiHref } from "@/utils/uiHref";

const LEGACY_PAGE_ROUTES: ReadonlyMap<string, string> = new Map(
  Object.entries({
    "api-keys": "api-keys",
    models: "models-and-endpoints",
    "llm-playground": "playground",
    projects: "projects",
    chat: "chat",
    "access-groups": "access-groups",
    budgets: "budgets",
    "guardrails-monitor": "guardrails-monitor",
    guardrails: "guardrails",
    "cost-tracking": "cost-tracking",
    "ui-theme": "ui-theme",
    logs: "logs",
    "admin-panel": "admin-panel",
    "logging-and-alerts": "logging-and-alerts",
    new_usage: "usage",
    "cost-optimization": "cost-optimization",
    "router-settings": "router-settings",
    users: "users",
    teams: "teams",
    organizations: "organizations",
  }),
);

export function legacyPageRedirectHref(searchParams: URLSearchParams): string | null {
  const page = searchParams.get("page");
  const route = page === null ? undefined : LEGACY_PAGE_ROUTES.get(page);
  if (route === undefined) return null;
  const rest = new URLSearchParams(searchParams);
  rest.delete("page");
  const query = rest.toString();
  return query ? `${uiHref(route)}?${query}` : uiHref(route);
}

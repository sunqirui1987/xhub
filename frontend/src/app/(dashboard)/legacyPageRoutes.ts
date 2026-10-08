import { uiHref } from "@/utils/uiHref";

const LEGACY_PAGE_ROUTES: ReadonlyMap<string, string> = new Map(
  Object.entries({
    "api-keys": "api-keys",
    "my-models": "mine-models",
    models: "models-and-endpoints",
    "price-data": "price-data",
    "llm-playground": "playground",
    projects: "projects",
    chat: "chat",
    "guardrails-monitor": "guardrails-monitor",
    guardrails: "guardrails",
    logs: "logs",
    "audit-logs": "audit-logs",
    new_usage: "usage",
    "route-templates": "route-templates",
    "router-settings": "route-templates",
    "admin-panel": "admin-panel",
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

import { NextResponse, type NextRequest } from "next/server";

const GATEWAY = process.env.XHUB_GATEWAY_ORIGIN || "http://127.0.0.1:4000";

/** Next App Router pages (LiteLLM dashboard). Everything else is the proxy API. */
const APP_PAGES = new Set([
  "/",
  "/login",
  "/onboarding",
  "/connect",
  "/api-keys",
  "/playground",
  "/models-and-endpoints",
  "/agents",
  "/workflows",
  "/memory",
  "/mcp-servers",
  "/skills",
  "/guardrails",
  "/policies",
  "/search-tools",
  "/vector-stores",
  "/tool-policies",
  "/usage",
  "/cost-optimization",
  "/logs",
  "/guardrails-monitor",
  "/teams",
  "/projects",
  "/users",
  "/organizations",
  "/access-groups",
  "/budgets",
  "/api-reference",
  "/model-hub-table",
  "/caching",
  "/prompts",
  "/transform-request",
  "/tag-management",
  "/router-settings",
  "/logging-and-alerts",
  "/admin-panel",
  "/cost-tracking",
  "/ui-theme",
  "/old-usage",
  "/chat",
  "/chat/api-keys",
  "/chat/credentials",
  "/chat/integrations",
  "/chat/logs",
  "/chat/usage",
  "/model_hub",
  "/model_hub_table",
  "/mcp/oauth/callback",
]);

function stripSlash(path: string): string {
  if (path.length > 1 && path.endsWith("/")) {
    return path.slice(0, -1);
  }
  return path || "/";
}

export function middleware(req: NextRequest) {
  const { pathname, search } = req.nextUrl;

  if (pathname.startsWith("/_next") || pathname.startsWith("/favicon") || pathname === "/robots.txt") {
    return NextResponse.next();
  }

  if (pathname === "/gw" || pathname.startsWith("/gw/")) {
    const rest = pathname === "/gw" || pathname === "/gw/" ? "/" : pathname.slice(3);
    return NextResponse.rewrite(new URL((stripSlash(rest) || "/") + search, GATEWAY));
  }

  // LiteLLM proxy mounts the static dashboard at /ui (see uiHref in production).
  if (pathname === "/ui" || pathname === "/ui/" || pathname.startsWith("/ui/")) {
    const rest = pathname === "/ui" || pathname === "/ui/" ? "/" : pathname.slice(3);
    const url = req.nextUrl.clone();
    url.pathname = stripSlash(rest) || "/";
    return NextResponse.rewrite(url);
  }

  if (APP_PAGES.has(stripSlash(pathname))) {
    return NextResponse.next();
  }

  const apiPath = stripSlash(pathname) || "/";
  return NextResponse.rewrite(new URL(apiPath + search, GATEWAY));
}

export const config = {
  matcher: ["/((?!_next/static|_next/image).*)"],
};

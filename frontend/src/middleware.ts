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
  "/guardrails",
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
  "/router-settings",
  "/logging-and-alerts",
  "/admin-panel",
  "/cost-tracking",
  "/ui-theme",
  "/chat",
  "/chat/api-keys",
  "/chat/credentials",
  "/chat/integrations",
  "/chat/logs",
  "/chat/usage",
]);

function stripSlash(path: string): string {
  if (path.length > 1 && path.endsWith("/")) {
    return path.slice(0, -1);
  }
  return path || "/";
}

// dashboardAppPath 是 Next 真正渲染的控制台路径。不在页面集合里的地址，包括 /ui 前缀，交给网关。
export function dashboardAppPath(pathname: string): string | null {
  let path = pathname;
  if (path === "/ui" || path === "/ui/" || path.startsWith("/ui/")) {
    const rest = path === "/ui" || path === "/ui/" ? "/" : path.slice(3);
    path = stripSlash(rest) || "/";
  } else {
    path = stripSlash(path) || "/";
  }
  return APP_PAGES.has(path) ? path : null;
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
    const appPath = dashboardAppPath(pathname);
    if (!appPath) {
      const rest = pathname === "/ui" || pathname === "/ui/" ? "/" : pathname.slice(3);
      return NextResponse.rewrite(new URL((stripSlash(rest) || "/") + search, GATEWAY));
    }
    const url = req.nextUrl.clone();
    url.pathname = appPath;
    return NextResponse.rewrite(url);
  }

  if (dashboardAppPath(pathname)) {
    return NextResponse.next();
  }

  const apiPath = stripSlash(pathname) || "/";
  return NextResponse.rewrite(new URL(apiPath + search, GATEWAY));
}

export const config = {
  matcher: ["/((?!_next/static|_next/image).*)"],
};

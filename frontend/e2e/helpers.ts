import { expect, type Page } from "@playwright/test";
import { translate } from "../src/i18n/translate";

export const t = (key: string) => translate("zh-CN", key);

export const MASTER = process.env.E2E_MASTER_KEY || "sk-e2e-master";

export function watchGateway(page: Page) {
  const bad: string[] = [];
  page.on("response", (res) => {
    const u = res.url();
    if (u.includes("/_next/")) return;
    if (!u.includes("/gw/") && !u.includes(":4000/") && !u.includes("127.0.0.1:3000/") && !u.includes("localhost:3000/")) return;
    if (res.status() >= 500) {
      bad.push(`${res.status()} ${res.request().method()} ${u}`);
    }
  });
  page.on("pageerror", (err) => {
    bad.push(`pageerror ${err.message}`);
  });
  return {
    assertOk() {
      expect(bad, bad.join("\n")).toEqual([]);
    },
  };
}

/** LiteLLM serves the dashboard under `/ui/` (cookie path, uiHref, login URL). */
export function uiPath(path: string): string {
  if (path.startsWith("/ui/") || path === "/ui" || path === "/ui/") {
    return path.endsWith("/") || path.includes("?") ? path : `${path}/`;
  }
  const qIndex = path.indexOf("?");
  const pathname = qIndex >= 0 ? path.slice(0, qIndex) : path;
  const query = qIndex >= 0 ? path.slice(qIndex) : "";
  if (pathname === "/") {
    return `/ui/${query}`;
  }
  const withSlash = pathname.endsWith("/") ? pathname : `${pathname}/`;
  return `/ui${withSlash}${query}`;
}

export async function gotoLogin(page: Page) {
  await page.goto(uiPath("/login"));
  await expect(page.getByRole("heading", { name: t("login.title") })).toBeVisible({ timeout: 20_000 });
}

export async function login(page: Page, username: string, password: string) {
  await gotoLogin(page);
  await page.getByPlaceholder(t("login.usernamePlaceholder")).fill(username);
  await page.getByPlaceholder(t("login.passwordPlaceholder")).fill(password);
  await page.getByRole("button", { name: t("login.submit"), exact: true }).click();
}

export async function loginAdmin(page: Page) {
  const guard = watchGateway(page);
  await login(page, "admin", MASTER);
  await expect(page.getByText(t("nav.apiKeys")).first()).toBeVisible({ timeout: 20_000 });
  guard.assertOk();
}

export const NAV_GROUPS = [
  t("nav.groups.gateway"),
  t("nav.groups.observability"),
  t("nav.groups.access"),
  t("nav.groups.developer"),
  t("nav.groups.settings"),
];

export const DASHBOARD_PAGES = [
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
];

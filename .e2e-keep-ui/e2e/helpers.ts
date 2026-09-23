import { expect, type Page } from "@playwright/test";

export const MASTER = process.env.E2E_MASTER_KEY || "sk-e2e-master";

export function watchGateway(page: Page) {
  const bad: string[] = [];
  page.on("response", (res) => {
    const u = res.url();
    if (!u.includes("/gw/")) return;
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

export async function gotoLogin(page: Page) {
  await page.goto("/login");
  await expect(page.getByRole("heading", { name: "XHub" })).toBeVisible();
}

export async function login(page: Page, username: string, password: string) {
  await gotoLogin(page);
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Login", exact: true }).click();
}

export async function loginAdmin(page: Page) {
  const guard = watchGateway(page);
  await login(page, "admin", MASTER);
  await expect(page.getByRole("heading", { name: "Virtual Keys" })).toBeVisible();
  guard.assertOk();
}

/** Seed session without the login form (page-crawl). */
export async function seedAdmin(page: Page) {
  const res = await page.request.post("http://127.0.0.1:4000/v2/login", {
    data: { username: "admin", password: MASTER },
    headers: { "Content-Type": "application/json" },
  });
  expect(res.ok()).toBeTruthy();
  const j = await res.json();
  await page.addInitScript(
    ({ token, role }) => {
      localStorage.setItem("xhub_token", token);
      localStorage.setItem("xhub_role", role);
    },
    { token: j.token as string, role: j.user_role as string },
  );
}

export async function loginViewer(page: Page) {
  const adminRes = await page.request.post("http://127.0.0.1:4000/v2/login", {
    data: { username: "admin", password: MASTER },
    headers: { "Content-Type": "application/json" },
  });
  expect(adminRes.ok()).toBeTruthy();
  const admin = await adminRes.json();
  const email = `viewer-${Date.now()}@local`;
  const password = "viewer-pass";
  const created = await page.request.post("http://127.0.0.1:4000/user/new", {
    headers: { Authorization: `Bearer ${admin.token}`, "Content-Type": "application/json" },
    data: { user_email: email, user_role: "proxy_admin_viewer", password },
  });
  expect(created.ok()).toBeTruthy();
  await login(page, email, password);
  await expect(page.getByRole("heading", { name: "Virtual Keys" })).toBeVisible();
}

export async function createVirtualKey(page: Page, alias: string) {
  await page.goto("/api-keys");
  await expect(page.getByRole("heading", { name: "Virtual Keys" })).toBeVisible();
  await page.getByRole("button", { name: "+ Create New Key" }).click();
  await page.locator(".modal input").fill(alias);
  await page.getByRole("button", { name: "Create", exact: true }).click();
  const secret = page.locator(".modal .mono");
  await expect(secret).toBeVisible();
  const text = (await secret.textContent()) || "";
  expect(text.startsWith("sk-")).toBeTruthy();
  await page.getByRole("button", { name: "Close" }).click();
  return text;
}

export const DASHBOARD_PAGES: { path: string; heading: string }[] = [
  { path: "/api-keys", heading: "Virtual Keys" },
  { path: "/playground", heading: "Playground" },
  { path: "/models-and-endpoints", heading: "Models + Endpoints" },
  { path: "/agents", heading: "Agents" },
  { path: "/workflows", heading: "Workflow Runs" },
  { path: "/memory", heading: "Memory" },
  { path: "/mcp-servers", heading: "MCP Servers" },
  { path: "/skills", heading: "Skills" },
  { path: "/guardrails", heading: "Guardrails" },
  { path: "/policies", heading: "Policies" },
  { path: "/search-tools", heading: "Search Tools" },
  { path: "/vector-stores", heading: "Vector Stores" },
  { path: "/tool-policies", heading: "Tool Policies" },
  { path: "/usage", heading: "Usage" },
  { path: "/cost-optimization", heading: "Cost Optimization" },
  { path: "/logs", heading: "Logs" },
  { path: "/guardrails-monitor", heading: "Guardrails Monitor" },
  { path: "/teams", heading: "Teams" },
  { path: "/projects", heading: "Projects" },
  { path: "/users", heading: "Internal Users" },
  { path: "/organizations", heading: "Organizations" },
  { path: "/access-groups", heading: "Access Groups" },
  { path: "/budgets", heading: "Budgets" },
  { path: "/api-reference", heading: "API Reference" },
  { path: "/model-hub-table", heading: "AI Hub" },
  { path: "/caching", heading: "Response Cache" },
  { path: "/prompts", heading: "Prompts" },
  { path: "/transform-request", heading: "API Playground" },
  { path: "/tag-management", heading: "Tags" },
  { path: "/router-settings", heading: "Router Settings" },
  { path: "/logging-and-alerts", heading: "Logging & Alerts" },
  { path: "/admin-panel", heading: "Admin Settings" },
  { path: "/cost-tracking", heading: "Cost Tracking" },
  { path: "/ui-theme", heading: "UI Theme" },
  { path: "/old-usage", heading: "Old Usage" },
];

export const CHAT_PAGES: { path: string; heading: string }[] = [
  { path: "/chat", heading: "Chat" },
  { path: "/chat/api-keys", heading: "Chat api-keys" },
  { path: "/chat/credentials", heading: "Chat credentials" },
  { path: "/chat/integrations", heading: "Chat integrations" },
  { path: "/chat/logs", heading: "Chat logs" },
  { path: "/chat/usage", heading: "Chat usage" },
];

export const NAV_GROUPS = [
  "AI GATEWAY",
  "OBSERVABILITY",
  "ACCESS CONTROL",
  "DEVELOPER TOOLS",
  "SETTINGS",
];

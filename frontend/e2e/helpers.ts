import { expect, type Page } from "@playwright/test";
import { translate } from "../src/i18n/translate";

export const t = (key: string) => translate("zh-CN", key);

export const GATEWAY = process.env.E2E_GATEWAY || "http://127.0.0.1:4100";
export const UPSTREAM = process.env.E2E_UPSTREAM || "http://127.0.0.1:4110";

export const MASTER = process.env.E2E_MASTER_KEY || "sk-e2e-master";

export function watchGateway(page: Page) {
  const bad: string[] = [];
  page.on("response", (res) => {
    const u = res.url();
    if (u.includes("/_next/")) return;
    if (!u.startsWith(GATEWAY) && !u.startsWith((page.url().startsWith("http") ? new URL(page.url()).origin : "about:blank"))) return;
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

/**
 * stableGoto 打开控制台路径并吸收开发代理偶发的导航中断或短暂 5xx。
 * 参数：page 为当前浏览器页面，path 为不带 /ui 前缀的控制台路径。
 * 返回：页面完成 DOM 加载后结束；调用方随后验证目标页面的可访问内容。
 * 异常与副作用：最多导航三次，持续 5xx、最终导航异常或始终被重定向到登录页时抛错。
 * 调用场景：长时间串行 E2E 在页面切换或重新登录前使用。
 */
export async function stableGoto(page: Page, path: string) {
  const target = uiPath(path);
  let last: unknown;
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      const response = await page.goto(target, { waitUntil: "commit" });
      await page.waitForLoadState("domcontentloaded");
      if (response && response.status() >= 500) {
        last = new Error(`navigation to ${path} returned HTTP ${response.status()}`);
        if (attempt === 2) throw last;
        await page.waitForTimeout(300);
        continue;
      }
      if (!page.url().includes("/login") || path === "/login") return;
    } catch (err) {
      last = err;
      await page.waitForLoadState("domcontentloaded").catch(() => undefined);
      if (page.url().includes(path) && !page.url().includes("/login")) return;
      const message = err instanceof Error ? err.message : String(err);
      if (!message.includes("interrupted") && attempt === 2) throw err;
    }
  }
  if (page.url().includes(path) && !page.url().includes("/login")) return;
  throw last ?? new Error(`navigation to ${path} ended on ${page.url()}`);
}

/**
 * clearConsoleSession drops the console's credentials so the next sign-in is a
 * real one.
 *
 * The login page redirects a visitor who already holds a session, so signing in
 * as somebody else without this lands back on the dashboard as the previous
 * user and the login form is never rendered.
 *
 * Cookies go through the browser context rather than document.cookie, which is
 * the same removal but works on a page that has not navigated anywhere yet.
 * sessionStorage still needs a document, so it is cleared on the login page.
 */
export async function clearConsoleSession(page: Page) {
  await page.context().clearCookies();
  if (!page.url().startsWith("http")) return;
  await page.evaluate(() => sessionStorage.clear()).catch(() => undefined);
}

export async function gotoLogin(page: Page) {
  await stableGoto(page, "/login");
  await page.evaluate(() => sessionStorage.clear()).catch(() => undefined);
  await expect(page.getByRole("heading", { name: t("login.title") })).toBeVisible({ timeout: 20_000 });
}

export async function login(page: Page, username: string, password: string) {
  await clearConsoleSession(page);
  await gotoLogin(page);
  await page.getByPlaceholder(t("login.usernamePlaceholder")).fill(username);
  const passwordInput = page.getByPlaceholder(t("login.passwordPlaceholder"));
  await passwordInput.fill(password);
  // 从密码框提交原生登录表单，避免长时间全量运行时按钮动画或重渲染打断指针点击。
  await passwordInput.press("Enter");
}

export async function loginAdmin(page: Page) {
  const guard = watchGateway(page);
  await login(page, "admin", MASTER);
  await expect(page.getByText(t("nav.apiKeys")).first()).toBeVisible({ timeout: 20_000 });
  await expect(page.locator('a[href="/ui/admin-panel"]')).toBeVisible({ timeout: 20_000 });
  await ensureFixtureTeam(page);
  guard.assertOk();
}

/** Hierarchy prerequisite; UI scenarios still perform their own writes. */
export async function ensureFixtureTeam(page: Page) {
  const headers = { Authorization: "Bearer " + await sessionBearer(page) };
  const listed = await page.request.get(GATEWAY + "/organization/list", { headers });
  expect(listed.ok()).toBeTruthy();
  const orgs = await listed.json();
  let org = orgs.find((item: { organization_alias: string }) => item.organization_alias === "e2e-fixture-org");
  if (!org) {
    const created = await page.request.post(GATEWAY + "/organization/new", { headers, data: { organization_alias: "e2e-fixture-org" } });
    expect(created.ok(), await created.text()).toBeTruthy();
    org = await created.json();
    const team = await page.request.post(GATEWAY + "/team/new", { headers, data: { organization_id: org.organization_id, team_alias: "e2e-fixture-team" } });
    expect(team.ok(), await team.text()).toBeTruthy();
    await page.reload();
    await expect(page.getByTestId("create-key-button")).toBeVisible();
  }
}

export async function chooseKeyTeam(page: Page) {
  // The selector mounts only after both membership and team queries resolve.
  // A one-shot count() can see zero controls and submit an empty team_id.
  const identity = await page.request.get(GATEWAY + "/auth/me", {
    headers: { Authorization: "Bearer " + await sessionBearer(page) },
  });
  expect(identity.ok(), "read team memberships before choosing a key team").toBeTruthy();
  const { teams } = await identity.json();
  expect(Array.isArray(teams), "session membership list").toBeTruthy();
  expect(teams.length, "key fixture has at least one team membership").toBeGreaterThan(0);
  const control = page.getByRole("dialog").getByRole("combobox", { name: t("Team"), exact: true });
  if (teams.length > 1) {
    await expect(control).toBeVisible();
    await control.click();
    await page.getByRole("option", { name: /e2e-fixture-team/ }).click();
    await expect(control).toContainText("e2e-fixture-team");
  }
}

export async function chooseOrganization(page: Page) {
  const control = page.getByRole("dialog").getByRole("combobox", { name: t("Organization"), exact: true });
  await control.click();
  await page.getByRole("option", { name: /e2e-fixture-org/ }).click();
}

export const NAV_GROUPS = [
  t("nav.groups.mine"),
  t("nav.groups.observability"),
  t("nav.groups.team"),
  t("nav.groups.platform"),
  t("nav.groups.settings"),
];

export const DASHBOARD_PAGES = [
  "/api-keys",
  "/playground",
  "/models-and-endpoints",
  "/guardrails",
  "/usage",
  "/logs",
  "/guardrails-monitor",
  "/teams",
  "/projects",
  "/users",
  "/organizations",
  "/route-templates",
  "/router-settings",
];

export async function sessionBearer(page: Page): Promise<string> {
  const token = (await page.context().cookies()).find((cookie) => cookie.name === "token")?.value;
  expect(token, "console session cookie").toBeTruthy();
  return JSON.parse(Buffer.from(token!.split(".")[1], "base64url").toString()).key;
}

import { execFileSync } from "child_process";
import path from "path";
import { expect, test, type Page } from "@playwright/test";
import { MASTER, loginAdmin, t, uiPath, watchGateway } from "./helpers";
import { recordChain, recordPage, reportDir } from "./report";
import { discoverPages } from "./routes";

const pages = discoverPages();

async function sessionBearer(page: Page): Promise<string> {
  const cookies = await page.context().cookies();
  const token = cookies.find((c) => c.name === "token")?.value;
  const payload = JSON.parse(Buffer.from(token!.split(".")[1], "base64url").toString());
  return payload.key as string;
}

async function noDashboardError(page: Page, route: string) {
  await expect(page.getByText("This page couldn’t load"), route).toHaveCount(0);
  await expect(page.getByText(/Dashboard error:/), route).toHaveCount(0);
}

test("discovers every page.tsx route", async () => {
  expect(pages.length).toBeGreaterThan(40);
  expect(pages.map((p) => p.route)).toContain("/login");
  expect(pages.map((p) => p.route)).toContain("/api-keys");
  expect(pages.map((p) => p.route)).toContain("/model_hub");
  expect(pages.map((p) => p.route)).toContain("/mcp/oauth/callback");
  console.log(`discovered pages=${pages.length}`);
});

test("every page route renders without a dashboard error", async ({ page }) => {
  test.setTimeout(300_000);
  const guard = watchGateway(page);
  const errors: string[] = [];
  const failedReads: string[] = [];
  let usageActivitySettled = false;
  page.on("pageerror", (err) => errors.push(`${page.url()}: ${err.message}`));
  page.on("response", (res) => {
    const url = res.url();
    if (url.includes("/user/daily/activity") && res.status() < 400) usageActivitySettled = true;
    if (!/\/user\/daily\/activity|\/guardrails\/usage\/overview|\/key\/list/.test(url)) return;
    if (res.status() >= 400) failedReads.push(`${res.status()} ${res.request().method()} ${url}`);
  });
  await loginAdmin(page);
  for (const item of pages) {
    if (item.route === "/login" || item.route === "/onboarding" || item.route === "/mcp/oauth/callback") {
      continue;
    }
    if (item.route === "/usage") usageActivitySettled = false;
    const response = await page.goto(uiPath(item.route));
    expect(response === null || response.status() < 500, item.route).toBeTruthy();
    await noDashboardError(page, item.route);
    await expect(
      page
        .getByRole("heading")
        .or(page.getByRole("table"))
        .or(page.getByRole("tab"))
        .or(page.getByText(t("pages.costTracking.calculator")))
        .first(),
      item.route,
    ).toBeVisible({ timeout: 15_000 });
    if (!item.removed) {
      await expect(page, item.route).not.toHaveURL(/\/login/);
    }
    if (item.route === "/usage") {
      await expect.poll(() => usageActivitySettled, { timeout: 15_000 }).toBe(true);
      const settled = page.getByTestId("daily-spend-settled");
      await expect(settled).toBeVisible({ timeout: 15_000 });
      await expect(settled.getByText(t("No data"))).toBeVisible();
      await expect(page.getByText(t("Loading chart data..."))).toHaveCount(0);
      await expect(settled.locator("svg")).toHaveCount(0);
    }
    if (item.route === "/old-usage") {
      await expect(page.getByTestId("old-usage-body")).toContainText(/"spend"|用量读取失败/, { timeout: 15_000 });
    }
    if (item.route === "/logs") {
      await expect(page.getByText(t("No requests yet")).first()).toBeVisible({ timeout: 15_000 });
    }
    if (item.route === "/guardrails-monitor") {
      await expect(page.getByText("No data for this period").first()).toBeVisible({ timeout: 15_000 });
      await expect(page.getByText(t("Failed to load data. Try again."))).toHaveCount(0);
    }
    if (item.route === "/chat/api-keys") {
      await expect(
        page.getByText(t("No keys found")).or(page.getByRole("cell", { name: /^\$/ })).first(),
      ).toBeVisible({ timeout: 15_000 });
    }
    if (item.route === "/chat/logs") {
      await expect(page.getByText(t("No logs for this period")).first()).toBeVisible({ timeout: 15_000 });
    }
    if (item.route === "/chat/usage") {
      await expect(page.getByText(t("No usage data for this period"))).toBeVisible({ timeout: 15_000 });
    }
    recordPage(item.route, "pass");
  }
  expect(errors, errors.join("\n")).toEqual([]);
  expect(failedReads, failedReads.join("\n")).toEqual([]);
  guard.assertOk();
});

test("public model hubs show model names without an admin session", async ({ page }) => {
  const guard = watchGateway(page);
  for (const route of ["/model_hub", "/model_hub_table"]) {
    await page.goto(uiPath(route));
    await expect(page).not.toHaveURL(/\/login/);
    await noDashboardError(page, route);
    await page.getByPlaceholder(t("Search model names...")).fill("gpt-4o-mini");
    await expect(page.getByText("gpt-4o-mini").first()).toBeVisible({ timeout: 20_000 });
    recordPage(route, "pass");
  }
  guard.assertOk();
});
test("oauth callback records the fixture query and exchanges the token", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  await page.addInitScript(() => {
    const encode = (value: string) =>
      btoa(encodeURIComponent(value).replace(/%([0-9A-F]{2})/g, (_, hex) => String.fromCharCode(parseInt(hex, 16))));
    const flow = {
      state: "e2e-state",
      codeVerifier: "e2e-verifier",
      serverId: "e2e-mcp",
      redirectUri: `${location.origin}/ui/mcp/oauth/callback`,
      flowSource: "e2e",
    };
    sessionStorage.setItem("litellm-mcp-oauth-flow-state", encode(JSON.stringify(flow)));
    sessionStorage.setItem("litellm-mcp-oauth-return-url", encode(`${location.origin}/ui/mcp-servers/`));
  });
  const tokenPost = page.waitForResponse(
    (res) => res.url().includes("/v1/mcp/server/oauth/e2e-mcp/token") && res.request().method() === "POST",
    { timeout: 20_000 },
  );
  await page.goto(uiPath("/mcp/oauth/callback?code=e2e-code&state=e2e-state"));
  const exchanged = await tokenPost;
  const exchangedBody = await exchanged.text();
  expect(exchanged.ok(), exchangedBody).toBeTruthy();
  expect(exchanged.request().postData() || "").toContain("code=e2e-code");
  expect(exchangedBody).toContain("access_token");
  await expect(page.getByText(t("OAuth token retrieved successfully")).first()).toBeVisible({ timeout: 15_000 });
  await expect(page.getByTestId("oauth-resume")).toContainText("access_token");
  recordPage("/mcp/oauth/callback", "pass");
  guard.assertOk();
});

test("connect fixture shows the authorize UI", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  await page.goto(uiPath("/connect"));
  await expect(page.getByText(/MCP servers|No MCP servers/i).first()).toBeVisible({ timeout: 15_000 });
  await page.goto(uiPath("/connect?connect_flow=e2e-fixture"));
  await expect(page.getByText("e2e-server").first()).toBeVisible({ timeout: 15_000 });
  await page.getByRole("button", { name: t("connect.finish") }).click();
  await expect(page.getByText("e2e-fixture").first()).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText(/"status"/).first()).toBeVisible();
  await expect(page.getByText("complete").first()).toBeVisible();
  guard.assertOk();
});

test("onboarding claims an invite link", async ({ page }) => {
  test.setTimeout(90_000);
  const guard = watchGateway(page);
  await loginAdmin(page);
  await page.goto(uiPath("/users"));
  await page.getByRole("button", { name: `+ ${t("pages.users.invite")}` }).click();
  await page.getByLabel(t("pages.users.userEmail")).fill("e2e-claim@example.com");
  await page.getByRole("dialog").getByRole("button", { name: t("pages.users.invite") }).click();
  await expect(page.getByRole("heading", { name: t("pages.users.invitationLink") })).toBeVisible({ timeout: 15_000 });
  const link = await page.getByRole("dialog").locator("p").filter({ hasText: "invitation_id=" }).innerText();
  const target = new URL(link.trim());
  await page.goto(target.pathname + target.search);
  await expect(page.getByRole("textbox", { name: t("onboarding.email") })).toHaveValue("e2e-claim@example.com", {
    timeout: 15_000,
  });
  await page.getByRole("textbox", { name: t("onboarding.password") }).fill("claimed-pass");
  await page.getByRole("button", { name: t("onboarding.signup") }).click();
  await expect(page).toHaveURL(/login=success/, { timeout: 20_000 });
  await page.waitForLoadState("domcontentloaded");
  await noDashboardError(page, "/onboarding");
  console.log("created resource name=e2e-claim@example.com");
  recordChain("created resource name=e2e-claim@example.com");
  recordPage("/onboarding", "pass");
  guard.assertOk();
});

test("chat shows the fake upstream assistant text", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  await page.goto(uiPath("/chat"));
  await expect(page.getByRole("button", { name: "gpt-4o-mini" })).toBeVisible({ timeout: 20_000 });
  await page.getByPlaceholder(t("How can I help you today?")).fill("hello from chat");
  await page.getByRole("button", { name: t("Send"), exact: true }).click();
  await expect(page.getByText("e2e-ok").first()).toBeVisible({ timeout: 20_000 });
  console.log("chat content=e2e-ok");
  recordChain("chat content=e2e-ok");
  recordPage("/chat", "pass");
  guard.assertOk();
});

test("ui create is visible from the live gateway and chat returns e2e-ok", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  const name = "e2e-roundtrip-key";
  await page.getByTestId("create-key-button").click();
  await page.getByLabel(t("Key Name")).fill(name);
  await page.getByRole("button", { name: t("pages.apiKeys.createSubmit"), exact: true }).click();
  await expect(page.getByText(t("pages.apiKeys.saveKey"))).toBeVisible({ timeout: 15_000 });
  await page.keyboard.press("Escape");
  await expect(page.getByText(name).first()).toBeVisible();
  const bearer = await sessionBearer(page);
  const listed = await page.request.get("http://127.0.0.1:4000/key/list", {
    headers: { Authorization: `Bearer ${bearer}` },
  });
  expect(listed.ok()).toBeTruthy();
  expect(await listed.text()).toContain(name);
  const minted = await page.request.post("http://127.0.0.1:4000/key/generate", {
    headers: { Authorization: `Bearer ${bearer}`, "Content-Type": "application/json" },
    data: { key_alias: "e2e-chat-key", key_type: "llm_api" },
  });
  expect(minted.ok()).toBeTruthy();
  const secret = (await minted.json()).key as string;
  const chat = await page.request.post("http://127.0.0.1:4000/v1/chat/completions", {
    headers: { Authorization: `Bearer ${secret}`, "Content-Type": "application/json" },
    data: { model: "gpt-4o-mini", messages: [{ role: "user", content: "ping" }] },
  });
  const chatBody = await chat.text();
  expect(chat.ok(), chatBody).toBeTruthy();
  expect(chatBody).toContain("e2e-ok");
  console.log(`created resource name=${name}`);
  console.log("chat content=e2e-ok");
  recordChain(`created resource name=${name}`);
  recordChain("chat content=e2e-ok");
  guard.assertOk();
});

test("live gateway catalog sweep", async () => {
  test.setTimeout(300_000);
  const bin = path.resolve(__dirname, "../../.e2e/livesweep");
  const catalog = path.resolve(__dirname, "../../docs/_inventory/catalog.json");
  let out = "";
  try {
    out = execFileSync(bin, {
      encoding: "utf8",
      timeout: 240_000,
      env: {
        ...process.env,
        E2E_GATEWAY: "http://127.0.0.1:4000",
        E2E_MASTER_KEY: MASTER,
        CATALOG_JSON: catalog,
        E2E_ROUTE_LINES: path.join(reportDir, "routes.txt"),
      },
    });
  } catch (err) {
    const failed = err as { stdout?: string; stderr?: string; message?: string };
    out = `${failed.stdout || ""}\n${failed.stderr || ""}\n${failed.message || ""}`;
    console.log(out);
    throw new Error(out);
  }
  console.log(out.trim());
  const match = out.match(/checked=(\d+) misaligned=0 unique_http_routes=(\d+)/);
  expect(match, out).not.toBeNull();
  expect(match![1]).toBe(match![2]);
});

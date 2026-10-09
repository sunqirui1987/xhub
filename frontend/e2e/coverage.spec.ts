import { chooseKeyTeam, chooseOrganization } from "./helpers";
import { GATEWAY } from "./helpers";
import { execFileSync } from "child_process";
import path from "path";
import { expect, test, type Page } from "@playwright/test";
import { MASTER, login, loginAdmin, t, uiPath, watchGateway } from "./helpers";
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
  expect(pages.length).toBeGreaterThan(0);
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
    if (item.route === "/login") {
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
        page
          .getByText(t("No keys found"))
          .or(page.getByRole("cell", { name: /^\$/ }))
          .first(),
      ).toBeVisible({ timeout: 15_000 });
    }
    if (item.route === "/chat/logs") {
      await expect(
        page.getByText(t("No logs for this period")).or(page.getByRole("table")).first(),
      ).toBeVisible({ timeout: 15_000 });
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

test("model hubs require login then allow searching the public price catalog", async ({ page }) => {
  const guard = watchGateway(page);
  for (const route of ["/model_hub", "/model_hub_table"]) {
    await page.goto(uiPath(route));
    await expect(page).toHaveURL(/\/login/);
    await loginAdmin(page);
    await page.goto(uiPath(route));
    await noDashboardError(page, route);
    const catalog = await page.request.get(`${GATEWAY}/public/v1/model_hub?page=1&page_size=1`);
    expect(catalog.ok()).toBeTruthy();
    const name = (await catalog.json()).data[0].model_group as string;
    await page.getByPlaceholder(t("Search model names...")).fill(name);
    await expect(page.getByRole("row").filter({ hasText: name }).first()).toBeVisible();
    await page.getByPlaceholder(t("Search model names...")).fill("e2e-model-that-does-not-exist");
    await expect(page.getByText(t("No matching models"))).toBeVisible();
    await page.context().clearCookies();
    await page.evaluate(() => sessionStorage.clear());
    recordPage(route, "pass");
  }
  guard.assertOk();
});
test("a created user can sign in with the initial password", async ({ page }) => {
  test.setTimeout(90_000);
  const guard = watchGateway(page);
  await loginAdmin(page);
  await page.goto(uiPath("/users"));
  await page.getByRole("button", { name: `+ ${t("pages.users.invite")}` }).click();
  await page.getByLabel(t("pages.users.userEmail")).fill("e2e-claim@example.com");
  await page.getByLabel(t("Initial password")).fill("claimed-pass");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: t("pages.users.invite") })
    .click();
  await expect(page.getByText("e2e-claim@example.com").first()).toBeVisible({ timeout: 15_000 });
  await login(page, "e2e-claim@example.com", "claimed-pass");
  await expect(page.getByText(t("nav.apiKeys")).first()).toBeVisible({ timeout: 20_000 });
  guard.assertOk();
  console.log("created resource name=e2e-claim@example.com");
  recordChain("created resource name=e2e-claim@example.com");
  guard.assertOk();
});

// The reset link is gone: it pointed at an onboarding page whose claim endpoint
// discarded the password, so a link could not change anything. An administrator
// now sets the new password directly, and the account signs in with it.
test("an administrator resets a password and the account signs in with it", async ({ page }) => {
  test.setTimeout(120_000);
  const guard = watchGateway(page);
  await loginAdmin(page);
  await page.goto(uiPath("/users"));

  // A fresh address per run, so a leftover account from an earlier attempt
  // cannot turn the create into a conflict that leaves the dialog open.
  const email = `e2e-reset-${Date.now()}@example.com`;
  await page.getByRole("button", { name: `+ ${t("pages.users.invite")}` }).click();
  await page.getByLabel(t("pages.users.userEmail")).fill(email);
  await page.getByLabel(t("Initial password")).fill("initial-pass");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: t("pages.users.invite") })
    .click();
  // The dialog closing is what says the create was accepted; the address also
  // appears inside the form while it is open.
  await expect(page.getByRole("dialog")).toHaveCount(0, { timeout: 15_000 });
  await expect(page.getByText(email).first()).toBeVisible({ timeout: 15_000 });

  // The row's own actions menu, so the click does not open whichever row is first.
  await page
    .getByRole("row", { name: new RegExp(email) })
    .locator('[data-testid^="user-actions-"]')
    .first()
    .click();
  await page.getByTestId("user-action-reset-password").click();
  // Exact, because "New password" is a substring of "Confirm the new password".
  await page.getByLabel(t("New password"), { exact: true }).fill("replaced-pass");
  await page.getByLabel(t("Confirm the new password")).fill("replaced-pass");
  await page.getByRole("button", { name: t("Save the new password") }).click();
  await expect(page.getByText(t("Password updated. Their existing sessions have been signed out."))).toBeVisible({
    timeout: 15_000,
  });

  // The whole point of the change: the password the administrator typed is the
  // one the account signs in with. The old reset link answered 200 and changed
  // nothing.
  await login(page, email, "replaced-pass");
  await expect(page.getByText(t("nav.apiKeys")).first()).toBeVisible({ timeout: 20_000 });
  guard.assertOk();
});

test("chat shows the fake upstream assistant text", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  await page.goto(uiPath("/chat"));
  const composer = page.getByPlaceholder(t("How can I help you today?")).locator("..");
  // Scope to the textarea's immediate container: the account menu is also a
  // popover, and its DOM order changes as the page hydrates.
  await composer.locator('[data-slot="popover-trigger"]').click();
  await page.getByRole("dialog").getByRole("button", { name: "gpt-4o-mini", exact: true }).click();
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
  await chooseKeyTeam(page);
  await page.getByLabel(t("Key Name")).fill(name);
  await page.getByRole("button", { name: t("pages.apiKeys.createSubmit"), exact: true }).click();
  await expect(page.getByText(t("pages.apiKeys.saveKey"))).toBeVisible({ timeout: 15_000 });
  await page.keyboard.press("Escape");
  await expect(page.getByText(name).first()).toBeVisible();
  const bearer = await sessionBearer(page);
  const listed = await page.request.get(`${GATEWAY}/key/list`, {
    headers: { Authorization: `Bearer ${bearer}` },
  });
  expect(listed.ok()).toBeTruthy();
  expect(await listed.text()).toContain(name);
  const minted = await page.request.post(`${GATEWAY}/key/generate`, {
    headers: { Authorization: `Bearer ${bearer}`, "Content-Type": "application/json" },
    data: {
      key_alias: "e2e-chat-key",
      key_type: "llm_api",
      team_id: (
        await (
          await page.request.get(GATEWAY + "/v2/team/list?page=1&page_size=500", {
            headers: { Authorization: "Bearer " + bearer },
          })
        ).json()
      ).teams.find((x: { team_alias: string }) => x.team_alias === "e2e-fixture-team").team_id,
      user_id: JSON.parse(
        Buffer.from(
          (await page.context().cookies()).find((x) => x.name === "token")!.value.split(".")[1],
          "base64url",
        ).toString(),
      ).user_id,
    },
  });
  expect(minted.ok()).toBeTruthy();
  const secret = (await minted.json()).key as string;
  const chat = await page.request.post(`${GATEWAY}/v1/chat/completions`, {
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
  const bin = path.join(process.env.E2E_RUN_DIR || path.resolve(__dirname, "../../.e2e"), "livesweep");
  const catalog = path.resolve(__dirname, "../../docs/testdata/catalog.json");
  let out = "";
  try {
    out = execFileSync(bin, {
      encoding: "utf8",
      timeout: 240_000,
      env: {
        ...process.env,
        E2E_GATEWAY: GATEWAY,
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

import { expect, test, type Page } from "@playwright/test";
import { loginAdmin, stableGoto, t, uiPath, watchGateway } from "./helpers";

async function sessionFrom(page: Page): Promise<string> {
  const cookies = await page.context().cookies();
  const token = cookies.find((c) => c.name === "token")?.value;
  return JSON.parse(Buffer.from(token!.split(".")[1], "base64url").toString()).key as string;
}

async function pickOption(page: Page, option: string | RegExp) {
  const loc = page.getByRole("option", { name: option });
  if (await loc.count()) {
    await loc.first().click();
    return;
  }
  await page.getByText(option).first().click();
}

test("virtual key update, regenerate, block, and delete", async ({ page }) => {
  test.setTimeout(120_000);
  const guard = watchGateway(page);
  await loginAdmin(page);
  const alias = "e2e-key-lifecycle";
  const renamed = "e2e-key-renamed";
  await page.getByTestId("create-key-button").click();
  await page.getByLabel(t("Key Name")).fill(alias);
  await page.getByRole("button", { name: t("pages.apiKeys.createSubmit"), exact: true }).click();
  await expect(page.getByRole("dialog").locator("pre").filter({ hasText: /sk-/ })).toBeVisible({ timeout: 15_000 });
  await page.keyboard.press("Escape");
  await page.getByText(alias, { exact: true }).first().click();
  await expect(page.getByRole("heading", { name: alias })).toBeVisible({ timeout: 15_000 });

  await page.getByRole("tab", { name: t("Settings") }).click();
  await page.getByRole("button", { name: t("Edit Settings") }).click();
  await page.getByRole("textbox", { name: t("Key Alias") }).fill(renamed);
  const saved = page.waitForResponse((res) => res.url().includes("/key/update") && res.request().method() === "POST");
  await page
    .getByRole("button", { name: t("Save Changes") })
    .last()
    .click();
  expect((await saved).ok()).toBeTruthy();
  await expect(page.getByText(renamed).first()).toBeVisible({ timeout: 15_000 });

  await page.getByRole("button", { name: t("Regenerate Key") }).click();
  await page.getByRole("button", { name: t("Regenerate"), exact: true }).click();
  await expect(page.getByRole("dialog").getByText(/sk-/).first()).toBeVisible({ timeout: 15_000 });
  await page
    .getByRole("dialog")
    .getByRole("button", { name: t("Close"), exact: true })
    .first()
    .click();

  await page.getByRole("button", { name: t("More key actions") }).click();
  await page.getByRole("menuitem", { name: t("Block Key") }).click();
  await page.getByRole("button", { name: t("Block"), exact: true }).click();
  await expect(page.getByText(t("Blocked")).first()).toBeVisible({ timeout: 15_000 });

  await page.getByRole("button", { name: t("More key actions") }).click();
  await page.getByRole("menuitem", { name: t("Delete Key") }).click();
  await page.getByPlaceholder(renamed).fill(renamed);
  await page.getByRole("button", { name: t("common.delete") }).click();
  await expect(page.getByText(renamed)).toHaveCount(0, { timeout: 15_000 });
  guard.assertOk();
});

test("model update, test connection, and delete", async ({ page }) => {
  test.setTimeout(120_000);
  const guard = watchGateway(page);
  await loginAdmin(page);
  const bearer = await (async () => {
    const cookies = await page.context().cookies();
    const token = cookies.find((c) => c.name === "token")?.value;
    return JSON.parse(Buffer.from(token!.split(".")[1], "base64url").toString()).key as string;
  })();
  const created = await page.request.post("http://127.0.0.1:4000/model/new", {
    headers: { Authorization: `Bearer ${bearer}`, "Content-Type": "application/json" },
    data: {
      model_name: "e2e-model-ops",
      litellm_params: { model: "openai/gpt-4o-mini", api_key: "sk-fake", api_base: "http://127.0.0.1:4010" },
    },
  });
  expect(created.ok(), await created.text()).toBeTruthy();
  await stableGoto(page, "/models-and-endpoints");
  if (page.url().includes("/login")) {
    await loginAdmin(page);
    await stableGoto(page, "/models-and-endpoints");
  }
  await page.getByRole("tab", { name: t("pages.models.all") }).click();
  await expect(page.getByText("e2e-model-ops").first()).toBeVisible({ timeout: 15_000 });
  await page
    .getByRole("row", { name: /e2e-model-ops/ })
    .getByRole("button")
    .first()
    .click();
  await expect(page.getByTestId("test-connection-button")).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("test-connection-button").click();
  await expect(
    page
      .getByText(t("Connection test successful!"))
      .or(page.getByText(t("Error testing connection: ")))
      .first(),
  ).toBeVisible({ timeout: 20_000 });
  await page.getByRole("button", { name: t("Edit Settings") }).click();
  await page.getByRole("textbox", { name: t("Enter model name") }).fill("e2e-model-renamed");
  await page.getByRole("button", { name: t("Save Changes") }).click();
  await expect(page.getByText(t("Model settings updated successfully"))).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText("e2e-model-renamed").first()).toBeVisible();
  await page.getByTestId("delete-model-button").click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: t("common.delete") })
    .click();
  await expect(page.getByText(t("Model deleted successfully"))).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("cell", { name: "e2e-model-renamed" })).toHaveCount(0);
  await expect(page.getByRole("cell", { name: "e2e-model-ops" })).toHaveCount(0);
  guard.assertOk();
});

test("team member add is listed", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  await page.goto(uiPath("/teams"));
  await page.getByTestId("create-team-button").click();
  await page.getByTestId("team-name-input").fill("e2e-member-team");
  await page.getByTestId("create-team-submit").click();
  await expect(page.getByText("e2e-member-team").first()).toBeVisible({ timeout: 15_000 });
  await page.getByText("e2e-member-team", { exact: true }).first().click();
  await page.getByRole("tab", { name: "Members" }).click();
  const member = await page.request.post("http://127.0.0.1:4000/user/new", {
    headers: { Authorization: `Bearer ${await sessionFrom(page)}`, "Content-Type": "application/json" },
    data: { user_id: "e2e-member", user_email: "e2e-member@example.com", user_role: "internal_user" },
  });
  expect(member.ok(), await member.text()).toBeTruthy();
  await page.getByRole("button", { name: t("Add Member") }).click();
  const memberDialog = page.getByRole("dialog");
  await expect(memberDialog.getByRole("heading", { name: /成员/ })).toBeVisible();
  const emailSearch = memberDialog.getByRole("combobox", { name: t("Email") });
  await emailSearch.click();
  await emailSearch.fill("e2e-member@example.com");
  await page.getByRole("option", { name: "e2e-member@example.com" }).click();
  await memberDialog.getByRole("button", { name: t("Add Member") }).click();
  await expect(page.getByText("e2e-member@example.com").first()).toBeVisible({ timeout: 15_000 });
  guard.assertOk();
});

test("router fallback update lists the mapping", async ({ page }) => {
  test.setTimeout(90_000);
  const guard = watchGateway(page);
  await loginAdmin(page);
  const bearer = await (async () => {
    const cookies = await page.context().cookies();
    const token = cookies.find((c) => c.name === "token")?.value;
    return JSON.parse(Buffer.from(token!.split(".")[1], "base64url").toString()).key as string;
  })();
  const created = await page.request.post("http://127.0.0.1:4000/model/new", {
    headers: { Authorization: `Bearer ${bearer}`, "Content-Type": "application/json" },
    data: {
      model_name: "e2e-fallback-model",
      litellm_params: { model: "openai/gpt-4o-mini", api_key: "sk-fake", api_base: "http://127.0.0.1:4010" },
    },
  });
  expect(created.ok(), await created.text()).toBeTruthy();
  await stableGoto(page, "/route-templates");
  if (page.url().includes("/login")) {
    await loginAdmin(page);
    await stableGoto(page, "/route-templates");
  }
  const platformRow = page.getByRole("row", { name: new RegExp(t("pages.routeTemplates.platformDefault")) });
  await platformRow.getByRole("button", { name: t("pages.routeTemplates.edit") }).click();
  await page.getByRole("tab", { name: t("pages.routeTemplates.fallbacksTab") }).click();
  await page.getByPlaceholder(t("Select primary model")).first().click();
  await pickOption(page, "gpt-4o-mini");
  await page.getByPlaceholder(t("Select fallback models to add...")).first().click();
  await page.getByRole("option", { name: "e2e-fallback-model" }).click();
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: t("Save"), exact: true }).click();
  await expect(page.getByText(t("pages.routeTemplates.platformSaved"))).toBeVisible({ timeout: 15_000 });
  await platformRow.getByRole("button", { name: t("pages.routeTemplates.edit") }).click();
  await page.getByRole("tab", { name: t("pages.routeTemplates.fallbacksTab") }).click();
  await expect(page.getByText("e2e-fallback-model")).toBeVisible();
  guard.assertOk();
});

test("admin panel saves prompt storage and hides the unused settings", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  await page.goto(uiPath("/admin-panel"));
  await expect(page.getByText(t("Logging Settings"))).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("tab", { name: t("SSO Settings") })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: t("Security Settings") })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: "SCIM" })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: t("Hashicorp Vault") })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: t("CyberArk Conjur") })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: t("Plugins") })).toHaveCount(0);
  const promptSwitch = page.getByRole("switch");
  await promptSwitch.click();
  await page.getByRole("button", { name: t("Save Settings") }).click();
  await expect(page.getByText(t("Spend logs settings updated successfully"))).toBeVisible({ timeout: 15_000 });
  guard.assertOk();
});

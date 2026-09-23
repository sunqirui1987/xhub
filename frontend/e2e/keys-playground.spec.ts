import { expect, test } from "@playwright/test";
import { loginAdmin, t, uiPath } from "./helpers";

test("Virtual Keys shows named table and Create Key", async ({ page }) => {
  await loginAdmin(page);
  await expect(page.getByRole("button", { name: t("pages.apiKeys.create") })).toBeVisible();
  await expect(page.getByPlaceholder(t("pages.apiKeys.searchPlaceholder"))).toBeVisible();
});

test("Create New Key issues a secret and lists the alias", async ({ page }) => {
  await loginAdmin(page);
  await page.getByTestId("create-key-button").click();
  await expect(page.getByRole("heading", { name: t("pages.apiKeys.create") })).toBeVisible();
  await page.getByLabel(/Key Name/).fill("e2e-virtual-key");
  await page.getByRole("button", { name: t("pages.apiKeys.createSubmit"), exact: true }).click();
  await expect(page.getByText(t("pages.apiKeys.saveKey"))).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("dialog").locator("pre").filter({ hasText: /sk-/ })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByText("e2e-virtual-key").first()).toBeVisible({ timeout: 15_000 });
});

test("Models page lists the configured model", async ({ page }) => {
  await loginAdmin(page);
  await page.goto(uiPath("/models-and-endpoints"));
  await expect(page.getByText("gpt-4o-mini").first()).toBeVisible({ timeout: 15_000 });
});

test("Playground shows a model picker", async ({ page }) => {
  await loginAdmin(page);
  await page.goto(uiPath("/playground"));
  await expect(page.locator(`input[placeholder="${t("Select a Model")}"]`)).toBeVisible({ timeout: 15_000 });
});

test("Internal Users lists the logged-in admin", async ({ page }) => {
  await loginAdmin(page);
  await page.goto(uiPath("/users"));
  await expect(page.getByText("admin").first()).toBeVisible({ timeout: 15_000 });
});

test("Create Team lists the new alias", async ({ page }) => {
  await loginAdmin(page);
  await page.goto(uiPath("/teams"));
  await page.getByTestId("create-team-button").click();
  await expect(page.getByRole("heading", { name: t("pages.teams.create") })).toBeVisible();
  await page.getByTestId("team-name-input").fill("e2e-team");
  await page.getByTestId("create-team-submit").click();
  await expect(page.getByText("e2e-team").first()).toBeVisible({ timeout: 15_000 });
});

test("Invite User opens an invitation link", async ({ page }) => {
  await loginAdmin(page);
  await page.goto(uiPath("/users"));
  await page.getByRole("button", { name: `+ ${t("pages.users.invite")}` }).click();
  await expect(page.getByRole("heading", { name: t("pages.users.invite") })).toBeVisible();
  await page.getByLabel(t("pages.users.userEmail")).fill("e2e-user@example.com");
  await page.getByRole("dialog").getByRole("button", { name: t("pages.users.invite") }).click();
  await expect(page.getByRole("heading", { name: t("pages.users.invitationLink") })).toBeVisible({ timeout: 15_000 });
  await page.keyboard.press("Escape");
  await expect(page.getByText("e2e-user@example.com").first()).toBeVisible({ timeout: 15_000 });
});

test("Playground sends a chat turn", async ({ page }) => {
  await loginAdmin(page);
  await page.goto(uiPath("/playground"));
  await page.locator(`input[placeholder="${t("Select a Model")}"]`).click();
  await page.getByRole("option", { name: /gpt-4o-mini/ }).click();
  await page.getByTestId("chat-composer-input").fill("hello from e2e");
  await page.getByTestId("chat-send-button").click();
  await expect(page.getByTestId("message-surface").filter({ hasText: "hello from e2e" })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByTestId("message-surface").filter({ hasText: /ok|e2e-ok/ })).toBeVisible({
    timeout: 15_000,
  });
});

test("Organizations create lists the new org", async ({ page }) => {
  await loginAdmin(page);
  await page.goto(uiPath("/organizations"));
  await page.getByRole("button", { name: t("pages.organizations.create") }).click();
  await expect(page.getByRole("heading", { name: t("pages.organizations.createTitle") })).toBeVisible();
  await page.getByLabel(t("Organization Name")).fill("e2e-org");
  await page.getByRole("button", { name: t("pages.organizations.createTitle"), exact: true }).click();
  await expect(page.getByText("e2e-org").first()).toBeVisible({ timeout: 15_000 });
});

test("Budgets create lists the new budget", async ({ page }) => {
  await loginAdmin(page);
  await page.goto(uiPath("/budgets"));
  await page.getByRole("button", { name: t("pages.budgets.create") }).click();
  await expect(page.getByRole("heading", { name: t("pages.budgets.create") })).toBeVisible();
  await page.getByLabel(t("Budget ID")).fill("e2e-budget");
  await page.getByRole("dialog").getByRole("button", { name: t("pages.budgets.create") }).click();
  await expect(page.getByText("e2e-budget").first()).toBeVisible({ timeout: 15_000 });
});

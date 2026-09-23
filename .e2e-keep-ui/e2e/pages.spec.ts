import { expect, test } from "@playwright/test";
import {
  CHAT_PAGES,
  DASHBOARD_PAGES,
  NAV_GROUPS,
  loginAdmin,
  seedAdmin,
  watchGateway,
} from "./helpers";

test.describe("console pages", () => {
  test("five nav groups and default Virtual Keys", async ({ page }) => {
    await loginAdmin(page);
    for (const g of NAV_GROUPS) {
      await expect(page.getByText(g, { exact: true })).toBeVisible();
    }
    await expect(page.getByRole("link", { name: "Virtual Keys" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Playground", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: /Agentic/ })).toBeVisible();
    await expect(page.getByRole("link", { name: "Agents" })).toBeVisible();
    await expect(page.locator(".topbar-title")).toHaveText("Virtual Keys");
    await expect(page.getByRole("columnheader", { name: "Key", exact: true })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Key ID" })).toBeVisible();
  });

  for (const { path, heading } of DASHBOARD_PAGES) {
    test(`dashboard ${path}`, async ({ page }) => {
      const guard = watchGateway(page);
      await seedAdmin(page);
      await page.goto(path);
      await expect(page.getByRole("heading", { name: heading })).toBeVisible();
      await expect(page.locator("body")).not.toContainText("Not implemented yet");
      guard.assertOk();
    });
  }

  for (const { path, heading } of CHAT_PAGES) {
    test(`chat ${path}`, async ({ page }) => {
      const guard = watchGateway(page);
      await seedAdmin(page);
      await page.goto(path);
      await expect(page.getByRole("heading", { name: heading })).toBeVisible();
      guard.assertOk();
    });
  }

  test("chat shell is not playground", async ({ page }) => {
    await seedAdmin(page);
    await page.goto("/chat");
    await expect(page.getByText("Not Playground")).toBeVisible();
    await expect(page.getByRole("button", { name: "Send" })).toBeVisible();
  });

  test("public hub has no write button", async ({ page }) => {
    await page.goto("/model_hub");
    await expect(page.getByRole("heading", { name: "Model Hub" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Create" })).toHaveCount(0);
    await page.goto("/model_hub_table");
    await expect(page.getByRole("heading", { name: "Model Hub" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Create" })).toHaveCount(0);
  });

  test("onboarding and connect render", async ({ page }) => {
    await page.goto("/onboarding");
    await expect(page.getByRole("heading", { name: "Onboarding" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Claim" })).toBeVisible();
    await page.goto("/connect");
    await expect(page.getByRole("heading", { name: "Connect" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Complete" })).toBeVisible();
  });

  test("mcp oauth callback mentions token path", async ({ page }) => {
    await page.goto("/mcp/oauth/callback");
    await expect(page.getByText(/oauth\/\{server_id\}\/token/)).toBeVisible();
  });

  test("Teams Create persists a row", async ({ page }) => {
    const guard = watchGateway(page);
    await seedAdmin(page);
    await page.goto("/teams");
    await page.getByRole("button", { name: "Create" }).click();
    await expect(page.locator("table .mono")).not.toHaveCount(0);
    await expect(page.locator(".err")).toHaveCount(0);
    guard.assertOk();
  });
});

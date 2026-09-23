import { expect, test } from "@playwright/test";
import { loginViewer, watchGateway } from "./helpers";

test.describe("view-only", () => {
  test("hides write CTAs and Playground", async ({ page }) => {
    const guard = watchGateway(page);
    await loginViewer(page);
    await expect(page.getByRole("button", { name: "+ Create New Key" })).toHaveCount(0);
    await expect(page.getByRole("link", { name: "Playground", exact: true })).toHaveCount(0);
    await page.goto("/teams");
    await expect(page.getByRole("heading", { name: "Teams" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Create" })).toHaveCount(0);
    guard.assertOk();
  });

  test("direct /playground redirects away", async ({ page }) => {
    await loginViewer(page);
    await page.goto("/playground");
    await expect(page.getByRole("heading", { name: "Virtual Keys" })).toBeVisible();
  });

  test("chat Send is hidden", async ({ page }) => {
    await loginViewer(page);
    await page.goto("/chat");
    await expect(page.getByRole("heading", { name: "Chat" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Send" })).toHaveCount(0);
    await expect(page.getByText("View-only: sending is disabled.")).toBeVisible();
  });

  test("write API is 403", async ({ page }) => {
    await loginViewer(page);
    const status = await page.evaluate(async () => {
      const token = localStorage.getItem("xhub_token") || "";
      const res = await fetch("/gw/key/generate", {
        method: "POST",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify({ key_alias: "nope" }),
      });
      return res.status;
    });
    expect(status).toBe(403);
  });
});

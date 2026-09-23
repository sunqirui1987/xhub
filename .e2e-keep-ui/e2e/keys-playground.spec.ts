import { expect, test } from "@playwright/test";
import { MASTER, createVirtualKey, loginAdmin, watchGateway } from "./helpers";

test.describe.serial("virtual keys and playground", () => {
  test("create key shows plaintext once and lists alias", async ({ page }) => {
    const guard = watchGateway(page);
    await loginAdmin(page);
    const sk = await createVirtualKey(page, "e2e-key");
    await expect(page.getByRole("cell", { name: "e2e-key" })).toBeVisible();
    await expect(page.locator("table")).not.toContainText(sk);
    guard.assertOk();
  });

  test("playground sends chat with virtual key", async ({ page }) => {
    const guard = watchGateway(page);
    await loginAdmin(page);
    await createVirtualKey(page, "pg-key");
    await page.goto("/playground");
    await expect(page.getByRole("heading", { name: "Playground" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Send" })).toBeVisible();
    const keyBox = page.getByPlaceholder("sk-…");
    await expect(keyBox).toHaveValue(/^sk-/);
    await page.locator("textarea").fill("hello from e2e");
    const chat = page.waitForResponse(
      (r) => r.url().includes("/v1/chat/completions") && r.request().method() === "POST",
    );
    await page.getByRole("button", { name: "Send" }).click();
    const res = await chat;
    expect(res.status()).toBe(200);
    await expect(page.locator(".bubble")).toContainText("e2e-ok");
    guard.assertOk();
  });

  test("playground with master key cannot chat", async ({ page }) => {
    const guard = watchGateway(page);
    await loginAdmin(page);
    await page.goto("/playground");
    await page.getByPlaceholder("sk-…").fill(MASTER);
    await page.getByRole("button", { name: "Send" }).click();
    await expect(page.locator(".err")).toBeVisible();
    await expect(page.locator(".bubble")).toHaveCount(0);
    guard.assertOk();
  });

  test("chat shell Send uses virtual key", async ({ page }) => {
    const guard = watchGateway(page);
    await loginAdmin(page);
    await createVirtualKey(page, "chat-key");
    await page.goto("/chat");
    await expect(page.getByText("Not Playground")).toBeVisible();
    await page.getByRole("button", { name: "Send" }).click();
    await expect(page.locator("p").filter({ hasText: "e2e-ok" })).toBeVisible();
    guard.assertOk();
  });

  test("logs page shows spend after chat", async ({ page }) => {
    await loginAdmin(page);
    await createVirtualKey(page, "log-key");
    await page.goto("/playground");
    await page.getByRole("button", { name: "Send" }).click();
    await expect(page.locator(".bubble")).toContainText("e2e-ok");
    await page.goto("/logs");
    await expect(page.getByRole("heading", { name: "Logs" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Call Type" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Model" })).toBeVisible();
    await expect(page.locator("table")).toContainText("chat");
  });
});

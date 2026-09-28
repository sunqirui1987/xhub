import { expect, test, type Page } from "@playwright/test";
import { loginAdmin, stableGoto, t, uiPath } from "./helpers";

async function pickOption(page: Page, option: string | RegExp) {
  const loc = page.getByRole("option", { name: option });
  if (await loc.count()) {
    await loc.first().click();
    return;
  }
  await page.getByText(option).first().click();
}

test.describe("write wizards create then list", () => {
  test("Add Model creates a deployment that lists", async ({ page }) => {
    test.setTimeout(90_000);
    await loginAdmin(page);
    await stableGoto(page, "/models-and-endpoints");
    if (page.url().includes("/login")) {
      await loginAdmin(page);
      await stableGoto(page, "/models-and-endpoints");
    }
    await page.getByRole("button", { name: t("pages.models.add") }).click();
    const add = page.locator("form").filter({ has: page.getByTestId("add-model-btn") });
    const provider = add.getByRole("combobox", { name: t("Provider") });
    await provider.click();
    await provider.fill("OpenAI");
    await page.getByRole("option", { name: "OpenAI 标志 OpenAI" }).click();
    await add.locator("#model").click();
    await add.locator("#model").fill("自定义");
    await page.getByRole("option", { name: t("Custom Model Name (Enter below)") }).click();
    await page.keyboard.press("Escape");
    await add.locator("#custom_model_name").fill("openai/gpt-4o-mini");
    const publicName = add.getByTestId("public-model-name-input");
    await expect(publicName).toBeVisible({ timeout: 15_000 });
    await publicName.fill("e2e-added-model");
    await add.locator("#api_key").fill("sk-fake");
    await add.locator("#api_base").fill("http://127.0.0.1:4010");
    await add.getByTestId("add-model-btn").click();
    await page.getByRole("tab", { name: t("pages.models.all") }).click();
    await expect(page.getByText("e2e-added-model").first()).toBeVisible({ timeout: 15_000 });
  });

  test("Create Project lists after a team exists", async ({ page }) => {
    await loginAdmin(page);
    await page.goto(uiPath("/teams"));
    await page.getByTestId("create-team-button").click();
    await page.getByTestId("team-name-input").fill("e2e-proj-team");
    await page.getByTestId("create-team-submit").click();
    await expect(page.getByText("e2e-proj-team").first()).toBeVisible({ timeout: 15_000 });

    await page.goto(uiPath("/projects"));
    await page.getByRole("button", { name: t("pages.projects.create") }).click();
    await expect(page.getByRole("dialog").getByRole("heading", { name: /项目/ })).toBeVisible();
    await page.getByLabel(t("Project Name")).fill("e2e-project");
    await page.getByRole("dialog").getByRole("combobox").first().click();
    await pickOption(page, /e2e-proj-team/);
    await page.getByRole("dialog").getByRole("button", { name: t("Create Project"), exact: true }).click();
    await expect(page.getByText("e2e-project").first()).toBeVisible({ timeout: 15_000 });
  });

  test("Create Access Group lists the group", async ({ page }) => {
    await loginAdmin(page);
    await page.goto(uiPath("/access-groups"));
    await page.getByRole("button", { name: t("pages.accessGroups.create") }).click();
    await expect(page.getByRole("heading", { name: t("pages.accessGroups.create") })).toBeVisible();
    await page.getByLabel(t("Group Name")).fill("e2e-ag");
    await page.getByRole("dialog").getByRole("button", { name: "创建组" }).click();
    await expect(page.getByText("e2e-ag").first()).toBeVisible({ timeout: 15_000 });
  });

  test("Add Guardrail lists the guardrail", async ({ page }) => {
    test.setTimeout(90_000);
    await loginAdmin(page);
    await stableGoto(page, "/guardrails");
    if (page.url().includes("/login")) {
      await loginAdmin(page);
      await stableGoto(page, "/guardrails");
    }
    await page.getByRole("tab", { name: t("pages.guardrails.title"), exact: true }).click();
    await page.getByRole("button", { name: t("pages.guardrails.create") }).click();
    await page.getByRole("menuitem", { name: t("Add Provider Guardrail") }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel(t("Guardrail Name")).fill("e2e-gr");
    await dialog.getByRole("combobox", { name: t("Guardrail Provider") }).click();
    await page.getByRole("option", { name: "Lakera" }).click();
    await dialog.getByRole("button", { name: t("Next") }).click();
    await dialog.getByRole("button", { name: t("Create Guardrail") }).click();
    await expect(page.getByText("e2e-gr").first()).toBeVisible({ timeout: 15_000 });
  });

});

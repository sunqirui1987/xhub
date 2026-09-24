import { expect, test, type Page } from "@playwright/test";
import { loginAdmin, t, uiPath } from "./helpers";

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
    await loginAdmin(page);
    await page.goto(uiPath("/models-and-endpoints"));
    await page.getByRole("tab", { name: t("pages.models.add") }).click();
    await expect(page.getByRole("heading", { name: t("pages.models.add") })).toBeVisible();
    const modelInput = page.locator("#model, input[name='model']").first();
    if (await page.getByPlaceholder(t("Select models")).isVisible().catch(() => false)) {
      await page.getByPlaceholder(t("Select models")).click();
      await pickOption(page, /Custom Model Name/i);
      await page.getByPlaceholder(/Enter custom model name/i).fill("e2e-added-model");
    } else if (await modelInput.isVisible().catch(() => false)) {
      await modelInput.fill("e2e-added-model");
    } else {
      await page.getByRole("textbox").nth(1).fill("e2e-added-model");
    }
    const apiKey = page.getByLabel(/^API Key$/i);
    if (await apiKey.count()) {
      await apiKey.first().fill("sk-fake");
    }
    const apiBase = page.getByLabel(/^API Base$/i);
    if (await apiBase.count()) {
      await apiBase.first().fill("http://127.0.0.1:4010");
    }
    await page.getByTestId("add-model-btn").click();
    await page.getByRole("tab", { name: /All Models/i }).click();
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
    await expect(page.getByRole("heading", { name: t("Create Project") })).toBeVisible();
    await page.getByLabel(t("Project Name")).fill("e2e-project");
    await page.getByRole("combobox", { name: "Team" }).click();
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
    await page.getByRole("dialog").getByRole("button", { name: t("Create Group") }).click();
    await expect(page.getByText("e2e-ag").first()).toBeVisible({ timeout: 15_000 });
  });

  test("Add Guardrail lists the guardrail", async ({ page }) => {
    await loginAdmin(page);
    await page.goto(uiPath("/guardrails"));
    await page.getByRole("tab", { name: t("nav.guardrails"), exact: true }).click();
    await page.getByRole("button", { name: t("pages.guardrails.create") }).click();
    await page.getByRole("menuitem", { name: t("Add Provider Guardrail") }).click();
    await expect(page.getByText(t("Create guardrail"))).toBeVisible();
    const name = page.getByLabel(/Guardrail Name/i);
    if (await name.count()) {
      await name.first().fill("e2e-gr");
    } else {
      await page.getByRole("textbox").first().fill("e2e-gr");
    }
    const providerBox = page.getByPlaceholder(/Select|Search|provider/i);
    if (await providerBox.count()) {
      await providerBox.first().click();
      await page.keyboard.press("Enter");
    }
    for (let i = 0; i < 6; i++) {
      const create = page.getByRole("button", { name: t("Create Guardrail") });
      if (await create.count()) {
        await create.click();
        break;
      }
      await page.getByRole("button", { name: "Next" }).click();
    }
    await expect(page.getByText("e2e-gr").first()).toBeVisible({ timeout: 15_000 });
  });

});

import { chooseKeyTeam, chooseOrganization } from "./helpers";
import { GATEWAY, UPSTREAM } from "./helpers";
import { expect, test, type Page } from "@playwright/test";
import { sessionBearer, loginAdmin, stableGoto, t, uiPath } from "./helpers";

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
    const bearer = await sessionBearer(page);
    const supplier = await page.request.post(GATEWAY + "/credentials", {
      headers: { Authorization: "Bearer " + bearer },
      data: { credential_name: "e2e-wizard-provider", credential_info: { custom_llm_provider: "openai" },
        credential_values: { custom_llm_provider: "openai", api_key: "sk-fake", api_base: UPSTREAM } },
    });
    expect(supplier.ok(), await supplier.text()).toBeTruthy();
    await page.reload();
    await page.getByRole("button", { name: t("pages.models.add") }).click();
    const add = page.locator("form").filter({ has: page.getByLabel("模型提供商 *") });
    await add.getByLabel("模型提供商 *").selectOption("e2e-wizard-provider");
    await add.getByLabel("上游模型 *").fill("gpt-4o-mini");
    await add.getByLabel("对外模型名称 *").fill("e2e-added-model");
    await add.getByLabel("价格来源").selectOption("manual");
    await add.locator("#editor-input_cost_per_token").fill("0.15");
    await add.locator("#editor-output_cost_per_token").fill("0.60");
    const submitted = page.waitForResponse((res) => res.url().includes("/model/new") && res.request().method() === "POST");
    await add.getByRole("button", { name: "添加模型", exact: true }).click();
    expect((await submitted).ok()).toBeTruthy();
    await page.getByRole("tab", { name: t("pages.models.all") }).click();
    await expect(page.getByText("e2e-added-model").first()).toBeVisible({ timeout: 15_000 });
  });

  test("Create Project lists after a team exists", async ({ page }) => {
    await loginAdmin(page);
    await page.goto(uiPath("/teams"));
    await page.getByTestId("create-team-button").click();
  await chooseOrganization(page);
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

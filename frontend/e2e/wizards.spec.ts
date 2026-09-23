import { expect, test, type Page } from "@playwright/test";
import { writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
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

  test("Add New Agent lists the agent", async ({ page }) => {
    await loginAdmin(page);
    await page.goto(uiPath("/agents"));
    await page.getByRole("button", { name: t("pages.agents.create") }).click();
    await expect(page.getByText(t("pages.agents.create")).first()).toBeVisible();
    await page.getByRole("combobox").first().click();
    await page.getByText(t("Custom / Other")).click();
    await page.getByPlaceholder(t("e.g. my-custom-agent")).fill("e2e-agent");
    await page.getByRole("button", { name: /Next/ }).click();
    await page.getByRole("button", { name: /Next/ }).click();
    await page.getByRole("button", { name: /Next/ }).click();
    await page.getByRole("button", { name: /Create Agent/ }).click();
    await page.getByRole("button", { name: "Done" }).click();
    await expect(page.getByText("e2e-agent").first()).toBeVisible({ timeout: 15_000 });
  });

  test("Add MCP Server lists the server", async ({ page }) => {
    await loginAdmin(page);
    await page.goto(uiPath("/mcp-servers"));
    await page.getByRole("button", { name: t("pages.mcpServers.add") }).click();
    await page.getByRole("button", { name: t("+ Custom Server") }).click();
    await page.getByPlaceholder(/GitHub_MCP/).first().fill("E2E_MCP");
    const url = page.getByPlaceholder(/https:\/\/|mcp/i);
    if (await url.count()) {
      await url.last().fill("http://127.0.0.1:4010/mcp");
    }
    await page.getByRole("button", { name: t("Add MCP Server"), exact: true }).click();
    await expect(page.getByText("E2E_MCP").first()).toBeVisible({ timeout: 15_000 });
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

  test("Add Policy lists the policy", async ({ page }) => {
    await loginAdmin(page);
    await page.goto(uiPath("/policies"));
    await page.getByRole("tab", { name: t("nav.policies") }).click();
    await page.getByRole("button", { name: t("pages.policies.add") }).click();
    await expect(page.getByRole("heading", { name: t("Create New Policy") })).toBeVisible();
    await page.getByRole("dialog").getByRole("button", { name: t("Create Policy") }).click();
    await page.getByLabel(t("Policy Name")).fill("e2e-pol");
    await page.getByRole("dialog").getByRole("button", { name: t("Create Policy") }).click();
    await expect(page.getByText("e2e-pol").first()).toBeVisible({ timeout: 15_000 });
  });

  test("Add Prompt lists the prompt", async ({ page }) => {
    await loginAdmin(page);
    await page.goto(uiPath("/prompts"));
    await page.getByRole("button", { name: t("pages.prompts.add") }).click();
    await page.getByRole("button", { name: /^Save$/ }).click();
    const back = page.getByRole("button", { name: /Back/i });
    if (await back.count()) {
      await back.click();
    }
    await expect(page.getByText(/New prompt/i).first()).toBeVisible({ timeout: 15_000 });
  });

  test("Create Tag lists the tag", async ({ page }) => {
    await loginAdmin(page);
    await page.goto(uiPath("/tag-management"));
    await page.getByRole("button", { name: t("Create New Tag") }).click();
    await page.getByLabel(/Tag Name/i).fill("e2e-tag");
    await page.getByRole("button", { name: t("Create Tag") }).click();
    await expect(page.getByText("e2e-tag").first()).toBeVisible({ timeout: 15_000 });
  });

  test("Add Search Tool lists the tool", async ({ page }) => {
    await loginAdmin(page);
    await page.goto(uiPath("/search-tools"));
    await page.getByRole("button", { name: t("Add New Search Tool") }).click();
    await page.getByLabel(/Search Tool Name/i).fill("e2e-search");
    await page.getByRole("combobox", { name: /Search Provider|Provider/i }).click();
    await pickOption(page, /Tavily/i);
    await page.getByRole("button", { name: t("Add Search Tool") }).click();
    await expect(page.getByText("e2e-search").first()).toBeVisible({ timeout: 15_000 });
  });

  test("Add Skill lists the skill", async ({ page }) => {
    await loginAdmin(page);
    await page.goto(uiPath("/skills"));
    await page.getByRole("button", { name: t("Add Skill") }).click();
    await page.getByLabel(/Source URL/i).fill("https://github.com/example/e2e-skill");
    await page.getByLabel(/Skill Name|^Name$/i).first().fill("e2e-skill");
    await page.getByRole("button", { name: t("Add Skill"), exact: true }).click();
    await expect(page.getByText("e2e-skill").first()).toBeVisible({ timeout: 15_000 });
  });

  test("Create Vector Store lists the store", async ({ page }) => {
    await loginAdmin(page);
    await page.goto(uiPath("/vector-stores"));
    await page.getByRole("tab", { name: t("Create Vector Store") }).click();
    const file = join(tmpdir(), "e2e-vs.txt");
    writeFileSync(file, "hello vector store");
    await page.locator('input[type="file"]').setInputFiles(file);
    await page.getByLabel(/Vector Store Name|Name/i).first().fill("e2e-vs");
    await page.getByRole("button", { name: t("Create Vector Store") }).click();
    await expect(page.getByText("e2e-vs").first()).toBeVisible({ timeout: 15_000 });
  });

  test("Cache settings Save Changes persists", async ({ page }) => {
    await loginAdmin(page);
    await page.goto(uiPath("/caching"));
    await page.getByRole("tab", { name: t("Cache Settings") }).click();
    const save = page.getByRole("button", { name: t("Save Changes") });
    await expect(save.first()).toBeVisible({ timeout: 15_000 });
    await save.first().click();
    await expect(page.getByText(/saved|updated|success/i).first()).toBeVisible({ timeout: 15_000 });
  });
});

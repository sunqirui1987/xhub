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
      data: {
        credential_name: "e2e-wizard-provider",
        credential_info: { custom_llm_provider: "openai" },
        credential_values: { custom_llm_provider: "openai", api_key: "sk-fake", api_base: UPSTREAM },
      },
    });
    expect(supplier.ok(), await supplier.text()).toBeTruthy();
    await page.reload();
    await page.getByRole("button", { name: t("pages.models.add") }).click();
    const add = page.locator("form").filter({ has: page.getByLabel("模型提供商 *") });
    await add.getByLabel("模型提供商 *").selectOption("e2e-wizard-provider");
    await add.getByLabel("上游模型 *").fill("gpt-4o-mini");
    await add.getByLabel("对外模型名称 *").fill("e2e-added-model");
    await add.getByRole("combobox", { name: t("Endpoint type"), exact: true }).click();
    await page.getByRole("option", { name: t("Chat"), exact: true }).click();
    await add.getByRole("combobox", { name: "上游接口协议", exact: true }).click();
    await page.getByRole("option", { name: "Chat Completions", exact: true }).click();
    await add.getByLabel("价格来源").selectOption("manual");
    await add.locator("#editor-input_cost_per_token").fill("0.15");
    await add.locator("#editor-output_cost_per_token").fill("0.60");
    const submitted = page.waitForResponse(
      (res) => res.url().includes("/model/new") && res.request().method() === "POST",
    );
    await add.getByRole("button", { name: "添加模型", exact: true }).click();
    const created = await submitted;
    expect(created.ok(), await created.text()).toBeTruthy();
    expect(created.request().postDataJSON().model_info).toMatchObject({
      transport: "bypass_openai_chat",
      endpoint_types: ["chat"],
    });
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
    await page
      .getByRole("dialog")
      .getByRole("button", { name: t("Create Project"), exact: true })
      .click();
    await expect(page.getByText("e2e-project").first()).toBeVisible({ timeout: 15_000 });
  });

  test("invalid guardrail is rejected without saving", async ({ page }) => {
    await loginAdmin(page);
    await stableGoto(page, "/guardrails");
    await page.getByRole("tab", { name: t("pages.guardrails.title"), exact: true }).click();
    await page.getByRole("button", { name: t("pages.guardrails.create") }).click();
    await expect(page.getByRole("menuitem", { name: t("Add Provider Guardrail") })).toHaveCount(0);
    await page.getByRole("menuitem", { name: "关键词 / 正则护栏", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("护栏名称", { exact: true }).fill("e2e-invalid-gr");
    await dialog.getByLabel("正则表达式（每行一个，可选）", { exact: true }).fill("[");
    const submitted = page.waitForResponse(
      (res) => new URL(res.url()).pathname === "/guardrails" && res.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: "保存护栏", exact: true }).click();
    const rejected = await submitted;
    expect(rejected.status()).toBe(400);
    expect(await rejected.text()).toContain("invalid regular expression");
    await expect(dialog.getByRole("alert")).toBeVisible();
    await dialog.getByRole("button", { name: t("Cancel"), exact: true }).click();
    await page.reload();
    await expect(page.getByText("e2e-invalid-gr", { exact: true })).toHaveCount(0);
  });

  test("Add Guardrail persists, blocks, and redacts after editing", async ({ page }) => {
    await loginAdmin(page);
    await stableGoto(page, "/guardrails");
    await page.getByRole("tab", { name: t("pages.guardrails.title"), exact: true }).click();
    await page.getByRole("button", { name: t("pages.guardrails.create") }).click();
    await page.getByRole("menuitem", { name: "关键词 / 正则护栏", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("护栏名称", { exact: true }).fill("e2e-gr");
    await dialog.getByLabel("关键词（每行一个）", { exact: true }).fill("e2e-forbidden-word");
    await dialog.getByRole("combobox", { name: "命中后的动作", exact: true }).selectOption("block");
    const submitted = page.waitForResponse(
      (res) => new URL(res.url()).pathname === "/guardrails" && res.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: "保存护栏", exact: true }).click();
    const created = await submitted;
    expect(created.ok(), await created.text()).toBeTruthy();
    expect(created.request().postDataJSON().guardrail.litellm_params.blocked_words).toEqual(["e2e-forbidden-word"]);
    await expect(dialog).not.toBeVisible();
    await expect(page.getByRole("row").filter({ has: page.getByText("e2e-gr", { exact: true }) })).toBeVisible();
    await page.reload();
    const row = page.getByRole("row").filter({ has: page.getByText("e2e-gr", { exact: true }) });
    await row.getByRole("button").first().click();
    await expect(page.getByRole("textbox", { name: "关键词（每行一个）", exact: true })).toHaveValue(
      "e2e-forbidden-word",
    );
    const trialInput = page.getByRole("textbox", { name: "测试文本", exact: true });
    await trialInput.fill("say e2e-forbidden-word please");
    const blocked = page.waitForResponse((res) => new URL(res.url()).pathname === "/guardrails/apply_guardrail");
    await page.getByRole("button", { name: "测试当前规则", exact: true }).click();
    expect((await (await blocked).json()).action).toBe("block");
    await page.getByRole("combobox", { name: "命中后的动作", exact: true }).selectOption("redact");
    const updated = page.waitForResponse(
      (res) => res.request().method() === "PATCH" && new URL(res.url()).pathname.startsWith("/guardrails/"),
    );
    await page.getByRole("button", { name: "保存护栏", exact: true }).click();
    expect((await updated).ok()).toBeTruthy();
    await row.getByRole("button").first().click();
    await expect(page.getByRole("combobox", { name: "命中后的动作", exact: true })).toHaveValue("redact");
    await trialInput.fill("say e2e-forbidden-word please");
    const redacted = page.waitForResponse((res) => new URL(res.url()).pathname === "/guardrails/apply_guardrail");
    await page.getByRole("button", { name: "测试当前规则", exact: true }).click();
    const result = await (await redacted).json();
    expect(result.action).toBe("redact");
    expect(result.response_text).toBe("say [REDACTED] please");
  });
});

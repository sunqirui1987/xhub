import { expect, test } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t, watchGateway } from "./helpers";

// Distinguish identical connections in the editor; Go checks actual traffic.
test("identical deployments: edit 7:3 weights, persist, reload, and distinguish IDs", async ({ page }) => {
  const guard = watchGateway(page);
  await loginAdmin(page);
  const headers = { Authorization: "Bearer " + await sessionBearer(page) };
  const model = "e2e-identical-weight-model";
  for (const id of ["e2e-weight-a", "e2e-weight-b"]) {
    const created = await page.request.post(GATEWAY + "/model/new", { headers, data: {
      model_name: model,
      litellm_params: { model: "openai/gpt-4o-mini", api_key: "sk-fake", api_base: UPSTREAM, deployment_id: id, weight: 1 },
      model_info: { id, transport: "adapted", endpoint_types: ["chat"] },
    } });
    expect(created.ok(), await created.text()).toBeTruthy();
  }
  await stableGoto(page, "/route-templates");
  await page.getByRole("button", { name: t("pages.routeTemplates.create"), exact: true }).first().click();
  await page.getByLabel(t("pages.routeTemplates.name"), { exact: true }).fill("e2e-identical-weights");
  await page.getByRole("combobox", { name: t("pages.routeTemplates.strategy"), exact: true }).click();
  await page.getByRole("option", { name: t("Weighted Split"), exact: true }).click();
  await page.getByRole("combobox", { name: t("pages.routeTemplates.weights.publicModelLabel"), exact: true }).click();
  await page.getByRole("option", { name: model, exact: true }).click();
  const overrides = page.getByRole("button", { name: t("pages.routeTemplates.weights.override"), exact: true });
  await expect(overrides).toHaveCount(2);
  await overrides.first().click();
  await overrides.first().click();
  const weights = page.getByRole("spinbutton", { name: new RegExp(model) });
  await expect(weights).toHaveCount(2);
  await weights.nth(0).fill("7");
  await weights.nth(1).fill("3");
  await expect(page.getByText(t("pages.routeTemplates.weights.estimatedShare").replace("{percent}", "70"))).toBeVisible();
  const saved = page.waitForResponse(r => r.url().includes("/route_template/new") && r.request().method() === "POST");
  await page.getByRole("region").filter({ has: weights.first() }).getByRole("button", { name: t("pages.routeTemplates.create"), exact: true }).click();
  expect((await saved).ok()).toBeTruthy();
  await page.reload();
  const row = page.getByRole("row").filter({ has: page.getByText("e2e-identical-weights", { exact: true }) });
  await row.getByRole("button", { name: t("pages.routeTemplates.edit"), exact: true }).click();
  await expect(weights.nth(0)).toHaveValue("7");
  await expect(weights.nth(1)).toHaveValue("3");
  const library = await page.request.get(GATEWAY + "/route_template/list", { headers });
  const payload = await library.json();
  const documents = Array.isArray(payload) ? payload : payload.data;
  const body = documents.find((r: any) => r.name === "e2e-identical-weights").body;
  expect(body.routing_strategy).toBe("weighted-split");
  expect(body.routing_strategy_args.weights).toEqual([
    { model_name: model, model: "openai/gpt-4o-mini", api_base: UPSTREAM, deployment_id: "e2e-weight-a", weight: 7 },
    { model_name: model, model: "openai/gpt-4o-mini", api_base: UPSTREAM, deployment_id: "e2e-weight-b", weight: 3 },
  ]);
  guard.assertOk();
});

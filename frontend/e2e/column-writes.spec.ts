import { expect, test, type Page } from "@playwright/test";
import { loginAdmin, stableGoto, watchGateway } from "./helpers";
const valueVerbs = new Set(["创建", "更新", "保存", "上传", "转换"]);
const checkVerbs = new Set(["测试", "测试连接", "清空"]);
async function writeColumn(page: Page, route: string, name: string, verbs: string[]) {
  await stableGoto(page, route);
  if (page.url().includes("/login")) { await loginAdmin(page); await stableGoto(page, route); }
  await expect(page.getByRole("heading").first()).toBeVisible({ timeout: 15_000 });
  const input = page.getByLabel("名称");
  const list = page.getByTestId("column-list");
  const result = page.getByTestId("column-result");
  await input.fill(name);
  let listed = name;
  for (const verb of verbs) {
    let expected = listed;
    if (verb !== "创建" && verb !== "删除" && verb !== "测试" && verb !== "测试连接" && verb !== "清空") {
      expected = `${name}-${verb}`;
      await input.fill(expected);
      await expect(list, `${route} ${verb} before`).not.toContainText(expected);
    }
    const beforeDelete = await list.innerText();
    if (verb === "删除") expect(beforeDelete, `${route} delete before`).toContain(listed);
    await page.getByRole("button", { name: verb, exact: true }).click();
    if (valueVerbs.has(verb)) { await expect(list, `${route} ${verb}`).toContainText(expected, { timeout: 15_000 }); listed = expected; }
    else if (verb === "删除") await expect(list, `${route} ${verb}`).not.toContainText(listed, { timeout: 15_000 });
    else if (checkVerbs.has(verb)) await expect(result, `${route} ${verb}`).toContainText(/succeeded|failed|flushed/, { timeout: 15_000 });
  }
}
test("column write pages create, update, and delete in the UI", async ({ page }) => {
  test.setTimeout(240_000);
  const guard = watchGateway(page);
  await loginAdmin(page);
  const cases: Array<[string, string, string[]]> = [
    ["/agents", "e2e-agent", ["创建", "更新", "删除"]],
    ["/workflows", "e2e-run", ["创建", "更新"]],
    ["/memory", "e2e-mem", ["创建", "删除"]],
    ["/mcp-servers", "e2e-mcp", ["创建", "更新", "删除"]],
    ["/skills", "e2e-skill", ["创建", "删除"]],
    ["/policies", "e2e-policy", ["创建"]],
    ["/search-tools", "e2e-search", ["创建", "更新", "测试连接", "删除"]],
    ["/vector-stores", "e2e-vector", ["创建"]],
    ["/tool-policies", "e2e-tool", ["创建"]],
    ["/budgets", "e2e-budget", ["创建", "更新"]],
    ["/caching", "e2e-cache", ["保存", "清空"]],
    ["/prompts", "e2e-prompt", ["创建", "更新", "测试", "删除"]],
    ["/transform-request", "e2e-transform", ["转换"]],
    ["/tag-management", "e2e-tag", ["创建", "更新", "删除"]],
    ["/ui-theme", "e2e-theme", ["保存", "上传"]],
  ];
  for (const [route, name, verbs] of cases) await writeColumn(page, route, name, verbs);
  guard.assertOk();
});

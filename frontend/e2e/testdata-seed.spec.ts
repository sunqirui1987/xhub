import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { expect, test } from "@playwright/test";
import { GATEWAY, MASTER, loginAdmin, sessionBearer, stableGoto, watchGateway } from "./helpers";

/**
 * 目的：验证 testdata 建数后管理员能在真实控制台查看组织和模型，且没有会话、图片或视频调用。
 * 前置条件：独立网关、PostgreSQL schema 和生产控制台，无供应商凭据；执行同一 seed 方法及页面导航，
 * 核对 81 把成员密钥、1 把管理员个人密钥及空模型回执；临时报告在 finally 删除，全部资源由浏览器服务的 schema 清理。
 */
test("testdata 仅构造资源，控制台可见且不执行 Codex 或媒体模型", async ({ page }) => {
  // 建数需要空租户环境，必须单独选择本文件；完整浏览器套件的其他用例会先创建组织。
  test.skip(process.env.E2E_TESTDATA_SEED !== "1", "请使用 E2E_TESTDATA_SEED=1 单独运行 testdata-seed.spec.ts");
  test.setTimeout(120_000);
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "xhub-testdata-"));
  try {
    try {
      execFileSync("python3", [path.resolve(__dirname, "../../e2e/real_dataset.py"),
      "--gateway", GATEWAY, "--directory", directory, "--seed-only"], {
      env: { ...process.env, E2E_DATASET_ADMIN: "admin", E2E_DATASET_PASSWORD: MASTER,
        FENNO_AI_API_KEY: "", QINIU_API_KEY: "" }, timeout: 90_000,
      });
    } catch (error) {
      const reportPath = path.join(directory, "report.json");
      const detail = fs.existsSync(reportPath) ? JSON.parse(fs.readFileSync(reportPath, "utf8")).error : String(error);
      throw new Error("testdata 建数失败：" + detail);
    }
    const access = JSON.parse(fs.readFileSync(path.join(directory, "access.json"), "utf8"));
    const report = JSON.parse(fs.readFileSync(path.join(directory, "report.json"), "utf8"));
    expect(access.keys, "应构造 81 把成员个人密钥").toHaveLength(81);
    expect(access.admin_key).toMatchObject({ owner_type: "personal", key_alias: "管理员个人密钥", team_id: null });
    expect(report.total_key_count).toBe(82);
    expect(report.status).toBe("seeded");
    expect(report.calls).toEqual([]);
    expect(report.probe_attempts).toEqual([]);
    expect(report.agent_conversations).toBeUndefined();
    expect(report.media_tasks).toBeUndefined();
    const guard = watchGateway(page);
    await loginAdmin(page);
    await stableGoto(page, "/api-keys");
    await expect(page.getByText("管理员个人密钥", { exact: true }).first()).toBeVisible();
    const logs = await page.request.get(GATEWAY + "/spend/logs/ui?page_size=200", {
      headers: { Authorization: "Bearer " + await sessionBearer(page) },
    });
    expect(logs.status()).toBe(200);
    expect((await logs.json()).data, "建数不应产生任何数据面调用日志").toEqual([]);
    await stableGoto(page, "/organizations");
    for (const organization of access.organizations) {
      await expect(page.getByText(organization.name, { exact: true }).first()).toBeVisible();
    }
    await stableGoto(page, "/models-and-endpoints");
    await page.getByRole("tab", { name: "全部模型", exact: true }).click();
    await expect(page.getByText("gpt-5.6-sol", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("z-ai/glm-5", { exact: true }).first()).toBeVisible();
    guard.assertOk();
  } finally {
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

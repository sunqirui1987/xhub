import { describe, expect, it } from "vitest";
import { orgSettingsSchema } from "./schema";

/** 组织创建/编辑分钟字段边界：真实 schema 验证 null 由表单空白表示，数字必须为 INT；纯函数无数据清理。 */
describe("organization rate allocations", () => {
  const base = { organization_alias: "rates", models: [], max_budget: "", budget_duration: "", metadata: "", route_template_id: "" };
  for (const value of ["", "0", "1", "2147483647", "-1", "1.5", "2147483648", "wrong"]) {
    it("validates " + JSON.stringify(value), () => {
      const result = orgSettingsSchema.safeParse({ ...base, rpm_limit: value, tpm_limit: value });
      expect(result.success).toBe(["", "0", "1", "2147483647"].includes(value));
    });
  }
});

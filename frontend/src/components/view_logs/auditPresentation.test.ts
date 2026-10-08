import { afterEach, describe, expect, it } from "vitest";
import { setActiveLocale } from "@/i18n/runtime";
import { auditAction, auditOperator, auditResourceName } from "./auditPresentation";
import type { AuditLogEntry } from "./AuditLogsTableColumns";

const row = (action: string, table_name: string): AuditLogEntry => ({
  id: "1",
  updated_at: "2026-10-08T13:44:37Z",
  changed_by: "admin-id",
  changed_by_api_key: "",
  action,
  table_name,
  object_id: "resource-id",
  before_value: {},
  updated_values: {},
});
afterEach(() => setActiveLocale("en"));

describe("Readable Chinese audit events", () => {
  it.each([
    ["route_template.create", "route_template", "创建路由模板"],
    ["log.read", "request_log", "查看请求日志"],
    ["route_template.delete", "route_template", "删除路由模板"],
  ])("translates %s", (action, type, expected) => {
    setActiveLocale("zh-CN");
    expect(auditAction(row(action, type))).toBe(expected);
  });

  it("labels missing historical names explicitly and uses available names", () => {
    setActiveLocale("zh-CN");
    const entry = row("route_template.delete", "route_template");
    expect(auditResourceName(entry)).toBe("路由模板（名称未记录）");
    expect(auditOperator(entry)).toBe("用户（名称未记录）");
    expect(auditResourceName({ ...entry, detail: { object_name: "生产路由" } })).toBe("生产路由");
    expect(auditOperator({ ...entry, actor_name: "管理员" })).toBe("管理员");
  });
});

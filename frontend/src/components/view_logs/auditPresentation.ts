import { getActiveLocale, t } from "@/i18n";
import type { StatusTone } from "@/components/shared/table_cells";
import type { AuditLogEntry } from "./AuditLogsTableColumns";

export const AUDIT_RESOURCE_TYPES: Record<string, string> = {
  key: "key",
  api_keys: "key",
  LiteLLM_VerificationToken: "key",
  team: "team",
  teams: "team",
  LiteLLM_TeamTable: "team",
  user: "user",
  LiteLLM_UserTable: "user",
  organization: "organization",
  organizations: "organization",
  LiteLLM_OrganizationTable: "organization",
  model: "model",
  LiteLLM_ProxyModelTable: "model",
  project: "project",
  team_member: "team_member",
  route_template: "route_template",
  request_log: "request_log",
  log: "request_log",
};
const SPECIAL_ACTIONS: Record<string, string> = {
  "log.read": "readLog",
  "log.read_content": "readLogContent",
  "route_template.bind": "bindTemplate",
  "key.status": "keyStatus",
  "key.revoke_team": "revokeTeamKeys",
  "key.reset_spend": "resetSpend",
  "team.move": "moveTeam",
  "user.password": "changePassword",
  "member.add": "addMember",
  "member.role": "changeMemberRole",
  "member.remove": "removeMember",
  "org.admin_add": "addAdmin",
  "org.admin_remove": "removeAdmin",
  "org.member_set": "setMembers",
  bootstrap: "bootstrap",
  "admin.seed": "seedAdmin",
};
export function auditResourceType(log: Pick<AuditLogEntry, "table_name">): string {
  return t("audit.resources." + (AUDIT_RESOURCE_TYPES[log.table_name] ?? "unknown"));
}
export function auditAction(log: AuditLogEntry): string {
  const special = SPECIAL_ACTIONS[log.action];
  if (special) return t("audit.actions." + special);
  const suffix = log.action.split(".").at(-1) ?? "";
  const verb = (
    {
      create: "created",
      update: "updated",
      profile: "updated",
      admin_update: "updated",
      delete: "deleted",
      rotate: "rotated",
      created: "created",
      updated: "updated",
      deleted: "deleted",
      rotated: "rotated",
    } as Record<string, string>
  )[suffix];
  return verb ? t("audit.actions." + verb, { resource: auditResourceType(log) }) : t("audit.actions.unknown");
}
export function auditTone(log: AuditLogEntry): StatusTone {
  const suffix = log.action.split(".").at(-1) ?? "";
  if (["create", "created", "add", "admin_add"].includes(suffix)) return "success";
  if (["delete", "deleted", "remove", "admin_remove", "revoke_team"].includes(suffix)) return "error";
  if (["rotate", "rotated", "password", "status"].includes(suffix)) return "warning";
  return "info";
}
export function shortAuditID(value: string): string {
  return value.length > 12 ? value.slice(0, 8) + "…" + value.slice(-4) : value;
}
export function auditResourceName(log: AuditLogEntry): string {
  if (log.object_name) return log.object_name;
  if (typeof log.detail?.object_name === "string" && log.detail.object_name) return log.detail.object_name;
  const type = auditResourceType(log);
  if (["request_log", "log"].includes(log.table_name)) return type;
  return t("audit.unnamed", { resource: type });
}
export function auditOperator(log: AuditLogEntry): string {
  if (log.actor_kind === "master") return t("audit.master");
  if (log.actor_kind === "system") return t("audit.system");
  if (log.actor_name || log.actor_email) return log.actor_name || log.actor_email || "";
  if (log.changed_by === "default_user_id") return t("audit.defaultAdmin");
  if (log.actor_kind === "key") return t("audit.keyOperator");
  return log.changed_by ? t("audit.unidentifiedUser") : t("audit.unidentifiedOperator");
}
export function auditDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return new Intl.DateTimeFormat(getActiveLocale(), {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(date);
}

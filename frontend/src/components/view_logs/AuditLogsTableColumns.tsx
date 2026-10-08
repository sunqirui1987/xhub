"use client";
import { ColumnDef } from "@tanstack/react-table";
import { IdentityCell, StatusBadge } from "@/components/shared/table_cells";
import { t } from "@/i18n";
import {
  auditAction,
  auditDate,
  auditOperator,
  auditResourceName,
  auditResourceType,
  auditTone,
  shortAuditID,
} from "./auditPresentation";
export type AuditLogEntry = {
  id: string;
  updated_at: string;
  changed_by: string;
  changed_by_api_key: string;
  actor_kind?: string;
  actor_name?: string;
  actor_email?: string;
  object_name?: string;
  team_id?: string | null;
  action: string;
  table_name: string;
  object_id: string;
  before_value: Record<string, unknown>;
  updated_values: Record<string, unknown>;
  detail?: Record<string, unknown>;
};
export const getAuditLogsTableColumns = ({
  onViewLog,
}: {
  onViewLog: (log: AuditLogEntry) => void;
}): ColumnDef<AuditLogEntry>[] => [
  {
    id: "updated_at",
    accessorKey: "updated_at",
    header: t("audit.time"),
    size: 190,
    enableSorting: false,
    cell: ({ row }) => (
      <time dateTime={row.original.updated_at} className="whitespace-nowrap text-xs text-muted-foreground">
        {auditDate(row.original.updated_at)}
      </time>
    ),
  },
  {
    id: "action",
    accessorKey: "action",
    header: t("audit.action"),
    size: 180,
    enableSorting: false,
    cell: ({ row }) => <StatusBadge tone={auditTone(row.original)} label={auditAction(row.original)} />,
  },
  {
    id: "object_id",
    accessorKey: "object_id",
    header: t("audit.resource"),
    minSize: 240,
    enableSorting: false,
    cell: ({ row }) => (
      <IdentityCell
        title={auditResourceName(row.original)}
        subtitle={auditResourceType(row.original) + " · " + shortAuditID(row.original.object_id)}
        className="max-w-80"
        onClick={() => onViewLog(row.original)}
      />
    ),
  },
  {
    id: "changed_by",
    accessorKey: "changed_by",
    header: t("audit.operator"),
    size: 200,
    enableSorting: false,
    cell: ({ row }) => (
      <IdentityCell
        title={auditOperator(row.original)}
        subtitle={
          row.original.actor_email && row.original.actor_name
            ? row.original.actor_email
            : shortAuditID(row.original.changed_by)
        }
        className="max-w-64"
      />
    ),
  },
];

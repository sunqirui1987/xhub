"use client";

import { SortingState } from "@tanstack/react-table";
import { Inbox } from "lucide-react";
import React, { useMemo, useState } from "react";

import { DataTable } from "@/components/shared/DataTable";
import { Guardrail } from "@/components/guardrails/types";

import { getGuardrailTableColumns } from "./guardrailTableColumns";
import { t } from "@/i18n";

interface GuardrailTableProps {
  guardrailsList: Guardrail[];
  isLoading: boolean;
  onDeleteClick: (guardrailId: string, guardrailName: string) => void;
  onGuardrailClick: (id: string) => void;
}

const DEFAULT_SORTING: SortingState = [{ id: "created_at", desc: true }];

/**
 * 用途：展示空规则目录，并引导用户添加可执行规则。
 * 参数：无。
 * 返回：React 空态。
 * 调用：GuardrailTable 的 noDataMessage。
 * 测试：guardrail_table.test.tsx。
 */
function EmptyState() {
  return (
    <div className="flex flex-col items-center gap-1 py-6">
      <div className="mb-1 flex size-10 items-center justify-center rounded-lg bg-muted">
        <Inbox className="size-5 text-muted-foreground" />
      </div>
      <div className="text-sm font-medium text-foreground">{t("No guardrails yet")}</div>
      <div className="text-sm text-muted-foreground">{t("添加护栏以开始检查请求文本。")}</div>
    </div>
  );
}

/**
 * 用途：展示规则列表，维护客户端排序并转发详情/删除操作。
 * 参数：属性：规则数组、加载状态和详情/删除回调。
 * 返回：React 数据表；使用规则 ID 作为稳定行标识。
 * 调用：GuardrailsPanel。
 * 测试：guardrail_table.test.tsx。
 */
const GuardrailTable: React.FC<GuardrailTableProps> = ({
  guardrailsList,
  isLoading,
  onDeleteClick,
  onGuardrailClick,
}) => {
  const [sorting, setSorting] = useState<SortingState>(DEFAULT_SORTING);

  const columns = useMemo(
    () => getGuardrailTableColumns({ onGuardrailClick, onDeleteClick }),
    [onGuardrailClick, onDeleteClick],
  );

  return (
    <div className="overflow-hidden rounded-xl border bg-card">
      <DataTable
        data={guardrailsList}
        paginationMode="client"
        columns={columns}
        getRowId={(guardrail, index) => guardrail.guardrail_id || String(index)}
        sortingMode="client"
        sorting={sorting}
        onSortingChange={setSorting}
        isLoading={isLoading}
        loadingMessage={t("Loading guardrails…")}
        noDataMessage={<EmptyState />}
        size="compact"
      />
    </div>
  );
};

export default GuardrailTable;

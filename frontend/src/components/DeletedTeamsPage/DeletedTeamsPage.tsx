"use client";
import { PaginationState } from "@tanstack/react-table";
import { Info } from "lucide-react";
import { useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "@/components/shared/Alert";
import { DEFAULT_PAGE_SIZE_OPTIONS } from "@/components/shared/DataTable";
import { useDeletedTeams } from "@/app/(dashboard)/hooks/teams/useTeams";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { DeletedTeamsTable } from "./DeletedTeamsTable/DeletedTeamsTable";
import { t } from "@/i18n";

/** 展示服务端分页记录，无参数，返回列表；路由调用，占位旧页期间禁用分页，查询失败由列表状态处理，无数据库写入。 */
export default function DeletedTeamsPage() {
  const { premiumUser } = useAuthorized();
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: DEFAULT_PAGE_SIZE_OPTIONS[0],
  });
  const { data: teamsData, isLoading, isPlaceholderData } = useDeletedTeams(pagination.pageIndex + 1, pagination.pageSize);

  return (
    <div className="flex flex-col gap-4">
      {!premiumUser && (
        <Alert>
          <Info />
          <AlertTitle>{t("Coming soon to Enterprise")}</AlertTitle>
          <AlertDescription>
            {t("Deleted team auditing is graduating from beta into our Enterprise audit & compliance suite.")}
          </AlertDescription>
        </Alert>
      )}
      <DeletedTeamsTable
        teams={teamsData?.teams ?? []}
        isLoading={isLoading || isPlaceholderData}
        pagination={pagination}
        onPaginationChange={setPagination}
        rowCount={teamsData?.total ?? 0}
      />
    </div>
  );
}

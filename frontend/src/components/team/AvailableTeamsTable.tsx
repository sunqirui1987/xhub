"use client";

import { SortingState } from "@tanstack/react-table";
import { Users } from "lucide-react";
import React, { useMemo, useState } from "react";

import { DataTable } from "@/components/shared/DataTable";

import { AvailableTeam, getAvailableTeamsTableColumns } from "./AvailableTeamsTableColumns";
import { t } from "@/i18n";

interface AvailableTeamsTableProps {
  teams: AvailableTeam[];
  isLoading: boolean;
  onJoinTeam: (teamId: string) => void;
}

const DEFAULT_SORTING: SortingState = [{ id: "team_alias", desc: false }];

function EmptyState() {
  return (
    <div className="flex flex-col items-center gap-1 py-6">
      <div className="mb-1 flex size-10 items-center justify-center rounded-lg bg-muted">
        <Users className="size-5 text-muted-foreground" />
      </div>
      <div className="text-sm font-medium text-foreground">{t("No available teams to join")}</div>
      <div className="text-sm text-muted-foreground">
        {t("See how to set available teams")}{" "}
        
          {t("here")}
        
      </div>
    </div>
  );
}

const AvailableTeamsTable: React.FC<AvailableTeamsTableProps> = ({ teams, isLoading, onJoinTeam }) => {
  const [sorting, setSorting] = useState<SortingState>(DEFAULT_SORTING);

  const columns = useMemo(() => getAvailableTeamsTableColumns({ onJoinTeam }), [onJoinTeam]);

  return (
    <DataTable
      data={teams}
      paginationMode="client"
      columns={columns}
      getRowId={(team, index) => team.team_id || String(index)}
      sortingMode="client"
      sorting={sorting}
      onSortingChange={setSorting}
      isLoading={isLoading}
      loadingMessage={t("Loading available teams…")}
      noDataMessage={<EmptyState />}
      size="compact"
    />
  );
};

export default AvailableTeamsTable;

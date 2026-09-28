"use client";

import { ColumnWriteSurface } from "@/components/column_write_surface";

export default function Page() {
  return (
    <ColumnWriteSurface title="预算" emptyText="还没有预算" listPath="/budget/list" actions={[
      { label: "创建", method: "POST", path: "/budget/new", body: ({ name }) => ({ budget_id: name }) },
      { label: "更新", method: "POST", needsId: true, path: "/budget/update", body: ({ name, id }) => ({ budget_id: id || name, max_budget: 1, budget_duration: name }) },
    ]} />
  );
}

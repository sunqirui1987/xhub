"use client";

import { ColumnWriteSurface } from "@/components/column_write_surface";

export default function Page() {
  return (
    <ColumnWriteSurface title="策略" emptyText="还没有策略" listPath="/policies/list" actions={[
      { label: "创建", method: "POST", path: "/policies", body: ({ name }) => ({ policy_name: name, name }) },
    ]} />
  );
}

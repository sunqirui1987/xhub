"use client";

import { ColumnWriteSurface } from "@/components/column_write_surface";

export default function Page() {
  return (
    <ColumnWriteSurface title="工具策略" emptyText="还没有工具策略" listPath="/v1/tool/list" actions={[
      { label: "创建", method: "POST", path: "/v1/tool/policy", body: ({ name }) => ({ tool_name: name, name }) },
    ]} />
  );
}

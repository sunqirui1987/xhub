"use client";

import { ColumnWriteSurface } from "@/components/column_write_surface";

export default function Page() {
  return (
    <ColumnWriteSurface title="工作流" emptyText="还没有工作流运行" listPath="/v1/workflows/runs" actions={[
      { label: "创建", method: "POST", path: "/v1/workflows/runs", body: ({ name }) => ({ name }) },
      { label: "更新", method: "PATCH", needsId: true, path: ({ id }) => `/v1/workflows/runs/${id}`, body: ({ name }) => ({ name }) },
    ]} />
  );
}

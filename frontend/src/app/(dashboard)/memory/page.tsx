"use client";

import { ColumnWriteSurface } from "@/components/column_write_surface";

export default function Page() {
  return (
    <ColumnWriteSurface title="记忆" emptyText="还没有记忆" listPath="/v1/memory" actions={[
      { label: "创建", method: "POST", path: "/v1/memory", body: ({ name }) => ({ key: name, name }) },
      { label: "删除", method: "DELETE", needsId: true, path: ({ id }) => `/v1/memory/${id}` },
    ]} />
  );
}

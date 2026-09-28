"use client";

import { ColumnWriteSurface } from "@/components/column_write_surface";

export default function Page() {
  return (
    <ColumnWriteSurface title="代理" emptyText="还没有代理" listPath="/v1/agents" actions={[
      { label: "创建", method: "POST", path: "/v1/agents", body: ({ name }) => ({ agent_name: name, name }) },
      { label: "更新", method: "PATCH", needsId: true, path: ({ id }) => `/v1/agents/${id}`, body: ({ name }) => ({ agent_name: name, name }) },
      { label: "删除", method: "DELETE", needsId: true, path: ({ id }) => `/v1/agents/${id}` },
    ]} />
  );
}

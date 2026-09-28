"use client";

import { ColumnWriteSurface } from "@/components/column_write_surface";

export default function Page() {
  return (
    <ColumnWriteSurface title="技能" emptyText="还没有技能" listPath="/v1/skills" actions={[
      { label: "创建", method: "POST", path: "/v1/skills", body: ({ name }) => ({ skill_name: name, name }) },
      { label: "删除", method: "DELETE", needsId: true, path: ({ id }) => `/v1/skills/${id}` },
    ]} />
  );
}

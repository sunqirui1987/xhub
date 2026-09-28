"use client";

import { ColumnWriteSurface } from "@/components/column_write_surface";

export default function Page() {
  return (
    <ColumnWriteSurface title="标签" emptyText="还没有标签" listPath="/tag/list" actions={[
      { label: "创建", method: "POST", path: "/tag/new", body: ({ name }) => ({ tag_name: name, name }) },
      { label: "更新", method: "POST", needsId: true, path: "/tag/update", body: ({ name, id }) => ({ name: id, tag_name: id, description: name }) },
      { label: "删除", method: "POST", needsId: true, path: "/tag/delete", body: ({ id, name }) => ({ name: id || name, tag_name: id || name }) },
    ]} />
  );
}

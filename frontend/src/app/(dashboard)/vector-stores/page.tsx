"use client";

import { ColumnWriteSurface } from "@/components/column_write_surface";

export default function Page() {
  return (
    <ColumnWriteSurface title="向量存储" emptyText="还没有向量存储" listPath="/vector_store/list" actions={[
      { label: "创建", method: "POST", path: "/vector_store/new", body: ({ name }) => ({ vector_store_name: name, name }) },
    ]} />
  );
}

"use client";

import { ColumnWriteSurface } from "@/components/column_write_surface";

export default function Page() {
  return (
    <ColumnWriteSurface title="缓存" emptyText="还没有缓存设置" listPath="/cache/settings" actions={[
      { label: "保存", method: "POST", path: "/cache/settings", body: ({ name }) => ({ type: "local", name }) },
      { label: "清空", method: "POST", path: "/flushall", body: () => ({}) },
    ]} />
  );
}

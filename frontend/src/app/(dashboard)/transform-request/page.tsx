"use client";

import { ColumnWriteSurface } from "@/components/column_write_surface";

export default function Page() {
  return (
    <ColumnWriteSurface title="转换请求" emptyText="还没有转换结果" listPath="/utils/supported_openai_params?model=gpt-4o-mini" actions={[
      { label: "转换", method: "POST", path: "/utils/transform_request", body: ({ name }) => ({ model: "gpt-4o-mini", name }) },
    ]} />
  );
}

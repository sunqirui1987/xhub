"use client";

import { ColumnWriteSurface } from "@/components/column_write_surface";

export default function Page() {
  return (
    <ColumnWriteSurface title="提示词" emptyText="还没有提示词" listPath="/prompts/list" actions={[
      { label: "创建", method: "POST", path: "/prompts", body: ({ name }) => ({ prompt_id: name, name }) },
      { label: "更新", method: "PATCH", needsId: true, path: ({ id }) => `/prompts/${id}`, body: ({ name, id }) => ({ prompt_id: id, name }) },
      { label: "测试", method: "POST", path: "/prompts/test", body: ({ id, name }) => ({ prompt_id: id || name }) },
      { label: "删除", method: "DELETE", needsId: true, path: ({ id }) => `/prompts/${id}` },
    ]} />
  );
}

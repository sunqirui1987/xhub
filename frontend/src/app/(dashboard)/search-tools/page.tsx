"use client";

import { ColumnWriteSurface } from "@/components/column_write_surface";

export default function Page() {
  return (
    <ColumnWriteSurface title="搜索工具" emptyText="还没有搜索工具" listPath="/search_tools/list" actions={[
      { label: "创建", method: "POST", path: "/search_tools", body: ({ name }) => ({ search_tool_name: name, name }) },
      { label: "更新", method: "PUT", needsId: true, path: ({ id }) => `/search_tools/${id}`, body: ({ name, id }) => ({ search_tool_id: id, search_tool_name: name, name }) },
      { label: "测试连接", method: "POST", path: "/search_tools/test_connection", body: ({ name, id }) => ({ search_tool_id: id, search_tool_name: name }) },
      { label: "删除", method: "DELETE", needsId: true, path: ({ id }) => `/search_tools/${id}` },
    ]} />
  );
}

"use client";

import { ColumnWriteSurface } from "@/components/column_write_surface";

export default function Page() {
  return (
    <ColumnWriteSurface title="界面主题" emptyText="还没有主题设置" listPath="/get/ui_theme_settings" actions={[
      { label: "保存", method: "PATCH", path: "/update/ui_theme_settings", body: ({ name }) => ({ primary_color: name || "#112233" }) },
      { label: "上传", method: "POST", path: "/upload/logo", body: ({ name }) => ({ logo_url: name || "e2e-logo" }) },
    ]} />
  );
}

"use client";

import { useT } from "@/i18n";

/** 显示额度帮助；参数 scope 指定层级，返回随语言切换的说明区域。调用：管理页面；无写入副作用。 */
export function QuotaGuide({ scope }: { scope: "organization" | "team" | "person" | "key" }) {
  const t = useT();
  return (
    <aside aria-label={t("quotaGuide.title")} className="rounded-lg border bg-muted/30 p-4 text-sm">
      <p className="font-medium">{t("quotaGuide.title")}</p>
      {["chain", scope, "shared", "singleTeam", "business", "rates"].map((key) => (
        <p key={key} className="mt-1 text-muted-foreground">
          {t("quotaGuide." + key)}
        </p>
      ))}
    </aside>
  );
}

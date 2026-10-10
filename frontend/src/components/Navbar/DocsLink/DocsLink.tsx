"use client";
import Link from "next/link";
import { BookOpen } from "lucide-react";
import { useI18n } from "@/i18n";

export const DOCS_URL = "/docs";

/** 右上角文档入口；无参数，返回同站双语链接。各导航栏调用，不依赖权限、不向外部站点传递信息。 */
export function DocsLink() {
  const { t } = useI18n();
  return (
    <Link
      href={DOCS_URL}
      aria-label={t("docs.center")}
      title={t("docs.center")}
      className="inline-flex h-8 items-center gap-2 rounded-md px-2 text-sm text-muted-foreground hover:bg-muted hover:text-foreground"
    >
      <BookOpen size={16} />
      <span className="hidden sm:inline">{t("docs.center")}</span>
    </Link>
  );
}

export default DocsLink;

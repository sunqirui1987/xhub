"use client";

import { useRouter } from "next/navigation";
import { useI18n, type Locale } from "@/i18n";
import { Button } from "@/components/ui/button";

const ORDER: Locale[] = ["zh-CN", "en"];

export default function LanguageSwitcher() {
  const router = useRouter();
  const { locale, setLocale, t } = useI18n();

  return (
    <div className="flex items-center gap-1" role="group" aria-label={t("language.label")}>
      {ORDER.map((code) => {
        const label = code === "zh-CN" ? t("language.zhCN") : t("language.en");
        const active = locale === code;
        return (
          <Button
            key={code}
            type="button"
            variant={active ? "secondary" : "ghost"}
            size="sm"
            aria-pressed={active}
            aria-label={label}
            className="h-7 min-w-8 px-2 text-xs"
            onClick={() => {
              setLocale(code);
              // Server components read the locale cookie on the request. Refresh them
              // after the cookie is written so their text matches the remounted client tree.
              if (typeof router.refresh === "function") router.refresh();
            }}
          >
            {code === "zh-CN" ? "中" : "EN"}
          </Button>
        );
      })}
    </div>
  );
}

"use client";

import { useRouter } from "next/navigation";
import { useI18n, type Locale } from "@/i18n";
import { Button } from "@/components/ui/button";

const ORDER: Locale[] = ["zh-CN", "en"];

/** 切换中英文界面；无参数，返回语言按钮。全局导航调用，保存语言并刷新服务端文案；
 * 客户端文案订阅 i18n，刷新不应重挂载页面或丢弃用户草稿。 */
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
              // 服务端文案读取 cookie；写入后刷新，客户端通过 i18n 订阅更新并保留原状态。
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

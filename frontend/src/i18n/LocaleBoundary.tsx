"use client";

import React, { useSyncExternalStore } from "react";
import { usePathname } from "next/navigation";
import { getActiveLocale, subscribeLocale } from "./runtime";

/** 保持已订阅语言的调试/模型页面组件身份；参数 children 为页面内容，返回语言边界。
 * 根布局调用；文档、调试、模型详情与供应商页面订阅语言更新，保留请求草稿和任务结果。
 * 尚未迁移的旧页面保留原重挂载行为，避免 memo 容器阻止运行时翻译更新；只对旧页面重置状态。 */
export function LocaleBoundary({ children }: { children: React.ReactNode }) {
  const locale = useSyncExternalStore(subscribeLocale, getActiveLocale, getActiveLocale);
  const pathname = usePathname() ?? "";
  const subscribedPage =
    /\/docs(?:\/|$)/.test(pathname) ||
    /\/(?:playground|mine-models|model-providers|models-and-endpoints\/mine)\/?$/.test(pathname);
  return <React.Fragment key={subscribedPage ? "subscribed-page" : locale}>{children}</React.Fragment>;
}

"use client";

import { useState, useEffect } from "react";
import ChatUI from "@/app/(dashboard)/playground/components/chat_ui/ChatUI";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { fetchProxySettings } from "@/utils/proxyUtils";
import { canAccessManagement } from "@/utils/roles";
import { useT } from "@/i18n";

interface ProxySettings {
  PROXY_BASE_URL?: string;
  LITELLM_UI_API_DOC_BASE_URL?: string | null;
}

/** 按权限直接提供单一调试工作区；无参数，返回页面；只读角色被拦截，代理配置仅向管理员加载。 */
export default function PlaygroundPage() {
  const t = useT();
  const { accessToken, userRole, userId, disabledPersonalKeyCreation, token, isViewOnly } = useAuthorized();
  const [proxySettings, setProxySettings] = useState<ProxySettings | undefined>(undefined);

  useEffect(() => {
    const initializeProxySettings = async () => {
      if (accessToken && canAccessManagement(userRole)) {
        const settings = await fetchProxySettings(accessToken);
        if (settings) {
          setProxySettings({
            PROXY_BASE_URL: settings.PROXY_BASE_URL,
            LITELLM_UI_API_DOC_BASE_URL: settings.LITELLM_UI_API_DOC_BASE_URL,
          });
        }
      }
    };

    initializeProxySettings();
  }, [accessToken, userRole]);

  if (isViewOnly) {
    return (
      <div className="flex h-full w-full flex-col items-center justify-center gap-2 p-8 text-center">
        <h1 className="text-2xl font-semibold">{t("pages.playground.accessDenied")}</h1>
        <p className="text-muted-foreground">{t("pages.playground.accessDeniedBody")}</p>
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 w-full min-w-0 flex-col overflow-hidden">
      <ChatUI
        accessToken={accessToken}
        token={token}
        userRole={userRole}
        userID={userId}
        disabledPersonalKeyCreation={disabledPersonalKeyCreation}
        proxySettings={proxySettings}
      />
    </div>
  );
}

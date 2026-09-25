"use client";

import { useState, useEffect } from "react";
import ChatUI from "@/app/(dashboard)/playground/components/chat_ui/ChatUI";
import CompareUI from "@/app/(dashboard)/playground/components/compareUI/CompareUI";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { fetchProxySettings } from "@/utils/proxyUtils";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { t } from "@/i18n";

interface ProxySettings {
  PROXY_BASE_URL?: string;
  LITELLM_UI_API_DOC_BASE_URL?: string | null;
}

export default function PlaygroundPage() {
  const { accessToken, userRole, userId, disabledPersonalKeyCreation, token, isViewOnly } = useAuthorized();
  const [proxySettings, setProxySettings] = useState<ProxySettings | undefined>(undefined);

  useEffect(() => {
    const initializeProxySettings = async () => {
      if (accessToken) {
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
  }, [accessToken]);

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
      <Tabs defaultValue="chat" className="flex min-h-0 min-w-0 flex-1 flex-col gap-0 overflow-hidden">
        <TabsList variant="line" className="h-12 w-full shrink-0 justify-start gap-2 overflow-x-auto border-b border-border bg-card px-2">
          <TabsTrigger value="chat" className="h-10 flex-none px-4 text-base">
            {t("pages.playground.chat")}
          </TabsTrigger>
          <TabsTrigger value="compare" className="h-10 flex-none px-4 text-base">
            {t("pages.playground.compare")}
          </TabsTrigger>
        </TabsList>
        <TabsContent
          value="chat"
          className="mt-0 h-full min-h-0 min-w-0 overflow-hidden data-hidden:hidden"
          keepMounted
        >
          <ChatUI
            accessToken={accessToken}
            token={token}
            userRole={userRole}
            userID={userId}
            disabledPersonalKeyCreation={disabledPersonalKeyCreation}
            proxySettings={proxySettings}
          />
        </TabsContent>
        <TabsContent value="compare" className="mt-0 h-full data-hidden:hidden" keepMounted>
          <CompareUI accessToken={accessToken} disabledPersonalKeyCreation={disabledPersonalKeyCreation} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

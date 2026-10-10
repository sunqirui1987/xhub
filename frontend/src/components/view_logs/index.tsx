"use client";

import { useState } from "react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import RequestLogsPanel from "./RequestLogsPanel";
import { UiLoadingSpinner } from "@/components/ui/ui-loading-spinner";
import { t } from "@/i18n";

interface SpendLogsTableProps {
  accessToken: string | null;
  token: string | null;
  userRole: string | null;
  userID: string | null;
  premiumUser: boolean;
}
/** 日志页入口：接收登录凭据并提供非失败请求和失败请求两个独立视图；未登录完成时显示加载状态，查询权限由后台校验。 */
export default function SpendLogsTable({ accessToken, token, userRole, userID }: SpendLogsTableProps) {
  const [activeTab, setActiveTab] = useState("logs");
  if (!accessToken || !token || !userRole || !userID) {
    return (
      <div role="status" aria-busy="true" aria-label={t("Loading")} className="flex h-64 items-center justify-center">
        <UiLoadingSpinner className="size-8 text-primary" />
      </div>
    );
  }
  return (
    <Tabs value={activeTab} onValueChange={(value) => setActiveTab(String(value))} className="h-full w-full">
      <TabsList aria-label={t("Logs")}>
        <TabsTrigger value="logs">{t("Logs")}</TabsTrigger>
        <TabsTrigger value="errors">{t("Error Logs")}</TabsTrigger>
      </TabsList>
      <TabsContent value="logs">
        <RequestLogsPanel accessToken={accessToken} token={token} userRole={userRole} userID={userID} isActive={activeTab === "logs"} />
      </TabsContent>
      <TabsContent value="errors">
        <RequestLogsPanel accessToken={accessToken} token={token} userRole={userRole} userID={userID} isActive={activeTab === "errors"} errorsOnly />
      </TabsContent>
    </Tabs>
  );
}

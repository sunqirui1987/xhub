"use client";

import { useState } from "react";
import { useSearchParams } from "next/navigation";
import { Plus, RefreshCw } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { all_admin_roles } from "@/utils/roles";
import ModelInfoView from "@/components/model_info_view";
import { useModelDetailRouting } from "@/app/(dashboard)/models-and-endpoints/detailNavigation";
import { useModelDashboardData } from "@/app/(dashboard)/models-and-endpoints/useModelDashboardData";
import AllModelsPanel from "@/app/(dashboard)/models-and-endpoints/panels/AllModelsPanel";
import AddModelPanel from "@/app/(dashboard)/models-and-endpoints/panels/AddModelPanel";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { t } from "@/i18n";

const BASE_TAB_KEY = "all-models";

export default function ModelsAndEndpointsPage() {
  const { accessToken, userRole, userId: userID, premiumUser, isViewOnly } = useAuthorized();
  const queryClient = useQueryClient();
  const { modelId, close } = useModelDetailRouting();
  const { availableModelAccessGroups, allModelsOnProxy } = useModelDashboardData();

  const catalogId = useSearchParams().get("catalog") ?? undefined;
  const [activeKey, setActiveKey] = useState<string>(catalogId ? "add" : BASE_TAB_KEY);
  const [lastRefreshed, setLastRefreshed] = useState("");

  const isAdmin = all_admin_roles.includes(userRole);
  const canCreate = isAdmin && !isViewOnly;
  const allModelsLabel = isAdmin ? t("pages.models.all") : t("pages.models.yours");

  const handleRefreshClick = () => {
    setLastRefreshed(new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }));
    queryClient.invalidateQueries({ queryKey: ["models", "list"] });
  };

  const invalidateModels = () => queryClient.invalidateQueries({ queryKey: ["models", "list"] });

  return (
    <div className="mx-auto w-full max-w-[1600px] space-y-6">
      <div className="flex w-full flex-col gap-2">
        {!modelId && (
          <div className="flex flex-wrap items-center justify-between gap-4 rounded-xl border bg-card p-6">
            <div>
              <h2 className="text-2xl font-semibold tracking-tight">{t("Model Management")}</h2>
              {isAdmin ? (
                <p className="text-sm text-muted-foreground">{t("按公开模型管理部署、默认分配、用户入口与计费规则")}</p>
              ) : (
                <p className="text-sm text-muted-foreground">{t("查看可用模型及调用配置。")}</p>
              )}
            </div>
          </div>
        )}

        {modelId ? (
          <ModelInfoView
            key={modelId}
            modelId={modelId}
            onClose={close}
            accessToken={accessToken}
            userID={userID}
            userRole={userRole}
            isViewOnly={isViewOnly}
            onModelUpdate={invalidateModels}
            modelAccessGroups={availableModelAccessGroups}
          />
        ) : (
          <>
            <Tabs
              value={activeKey === BASE_TAB_KEY ? activeKey : ""}
              onValueChange={setActiveKey}
            >
              <div className="flex min-w-0 flex-nowrap items-center gap-3 border-b">
                <div className="no-scrollbar scroll-fade-e -mb-1.5 min-w-0 flex-1 overflow-x-auto pb-1.5">
                  <TabsList variant="line" className="w-max justify-start">
                    <TabsTrigger value={BASE_TAB_KEY} className="flex-none">
                      {allModelsLabel}
                    </TabsTrigger>

                  </TabsList>
                </div>
                <div className="flex shrink-0 items-center gap-2 pb-1">
                  {canCreate && (
                    <Button size="sm" onClick={() => setActiveKey("add")}>
                      <Plus className="size-4" />
                      {t("pages.models.add")}
                    </Button>
                  )}
                  {lastRefreshed && (
                    <span className="text-xs text-muted-foreground">
                      {t("Last Refreshed: {lastRefreshed}", { lastRefreshed })}
                    </span>
                  )}
                  <Button variant="ghost" size="icon-sm" onClick={handleRefreshClick} aria-label={t("Refresh models")}>
                    <RefreshCw />
                  </Button>
                </div>
              </div>
            </Tabs>
            <div className="pt-4">
              {activeKey !== BASE_TAB_KEY && (
                <Button variant="ghost" size="sm" className="mb-3" onClick={() => setActiveKey(BASE_TAB_KEY)}>
                  {t("pages.models.backToModels")}
                </Button>
              )}
              {activeKey === BASE_TAB_KEY && <AllModelsPanel />}
              {activeKey === "add" && canCreate && (
                <AddModelPanel
                  initialCatalogId={catalogId}
                  onCancel={() => setActiveKey(BASE_TAB_KEY)}
                  onSaved={() => setActiveKey(BASE_TAB_KEY)}
                />
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}

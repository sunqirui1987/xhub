"use client";

import { useMemo, useState } from "react";
import { RefreshCw } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { useTeams } from "@/app/(dashboard)/hooks/teams/useTeams";
import { useUISettings } from "@/app/(dashboard)/hooks/uiSettings/useUISettings";
import { all_admin_roles, internalUserRoles } from "@/utils/roles";
import { canCreateModels } from "@/utils/modelPermissions";
import BetaBadge from "@/components/BetaBadge";
import ModelInfoView from "@/components/model_info_view";
import TeamInfoView from "@/components/team/TeamInfo";
import { useModelDetailRouting } from "@/app/(dashboard)/models-and-endpoints/detailNavigation";
import { useModelDashboardData } from "@/app/(dashboard)/models-and-endpoints/useModelDashboardData";
import AllModelsPanel from "@/app/(dashboard)/models-and-endpoints/panels/AllModelsPanel";
import AutoRoutersTabPanel from "@/app/(dashboard)/models-and-endpoints/panels/AutoRoutersTabPanel";
import AddModelPanel from "@/app/(dashboard)/models-and-endpoints/panels/AddModelPanel";
import LlmCredentialsPanel from "@/app/(dashboard)/models-and-endpoints/panels/LlmCredentialsPanel";
import PassThroughPanel from "@/app/(dashboard)/models-and-endpoints/panels/PassThroughPanel";
import HealthStatusPanel from "@/app/(dashboard)/models-and-endpoints/panels/HealthStatusPanel";
import ModelRetrySettingsPanel from "@/app/(dashboard)/models-and-endpoints/panels/ModelRetrySettingsPanel";
import ModelGroupAliasPanel from "@/app/(dashboard)/models-and-endpoints/panels/ModelGroupAliasPanel";
import AccessGroupBudgetsPanel from "@/app/(dashboard)/models-and-endpoints/panels/AccessGroupBudgetsPanel";
import PriceDataPanel from "@/app/(dashboard)/models-and-endpoints/panels/PriceDataPanel";
import { Button, buttonVariants } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn } from "@/lib/cva.config";
import { t } from "@/i18n";


type ModelTabSlug =
  | "add"
  | "auto-routers"
  | "llm-credentials"
  | "pass-through"
  | "health"
  | "retry-settings"
  | "model-group-alias"
  | "access-group-budgets"
  | "price-data";

const BASE_TAB_KEY = "all-models";

const TAB_LABELS: Record<ModelTabSlug, string> = {
  add: "pages.models.add",
  "auto-routers": "pages.models.autoRouters",
  "llm-credentials": "pages.models.llmCredentials",
  "pass-through": "pages.models.passThrough",
  health: "pages.models.health",
  "retry-settings": "pages.models.retrySettings",
  "model-group-alias": "pages.models.modelGroupAlias",
  "access-group-budgets": "pages.models.accessGroupBudgets",
  "price-data": "pages.models.priceData",
};

const renderPanel = (key: string) => {
  switch (key) {
    case BASE_TAB_KEY:
      return <AllModelsPanel />;
    case "auto-routers":
      return <AutoRoutersTabPanel />;
    case "add":
      return <AddModelPanel />;
    case "llm-credentials":
      return <LlmCredentialsPanel />;
    case "pass-through":
      return <PassThroughPanel />;
    case "health":
      return <HealthStatusPanel />;
    case "retry-settings":
      return <ModelRetrySettingsPanel />;
    case "model-group-alias":
      return <ModelGroupAliasPanel />;
    case "access-group-budgets":
      return <AccessGroupBudgetsPanel />;
    case "price-data":
      return <PriceDataPanel />;
    default:
      return null;
  }
};

export default function ModelsAndEndpointsPage() {
  const { accessToken, userRole, userId: userID, premiumUser, isViewOnly } = useAuthorized();
  const { data: teams } = useTeams();
  const { data: uiSettings } = useUISettings();
  const queryClient = useQueryClient();
  const { modelId, teamId, close } = useModelDetailRouting();
  const { availableModelAccessGroups, allModelsOnProxy } = useModelDashboardData();

  const [activeKey, setActiveKey] = useState<string>(BASE_TAB_KEY);
  const [lastRefreshed, setLastRefreshed] = useState("");

  const isInternalUser = userRole && internalUserRoles.includes(userRole);
  const canCreate = canCreateModels(
    { userRole, userID, isViewOnly },
    {
      teams: teams ?? null,
      disabledForInternalUsers:
        isInternalUser === true && uiSettings?.values?.disable_model_add_for_internal_users === true,
    },
  );
  const isAdmin = all_admin_roles.includes(userRole);

  // The model list and saved providers are the only peer tabs. Write-only tools stay
  // behind the same role gates, but they open from More tools instead of that strip.
  const showProviders = isAdmin && !isViewOnly;
  const secondarySlugs = useMemo<ModelTabSlug[]>(
    () => [
      ...(isAdmin || canCreate ? (["auto-routers"] as const) : []),
      ...(isAdmin && !isViewOnly ? (["pass-through"] as const) : []),
      ...(isAdmin ? (["health"] as const) : []),
      ...(isAdmin && !isViewOnly
        ? (["retry-settings", "model-group-alias", "access-group-budgets", "price-data"] as const)
        : []),
    ],
    [canCreate, isAdmin, isViewOnly],
  );

  const allModelsLabel = isAdmin ? t("pages.models.all") : t("pages.models.yours");
  const tabLabel = (slug: "" | ModelTabSlug): React.ReactNode => {
    if (!slug) return allModelsLabel;
    if (slug === "auto-routers" || slug === "access-group-budgets") {
      return (
        <span className="flex items-center gap-2">
          {t(TAB_LABELS[slug])} <BetaBadge />
        </span>
      );
    }
    return t(TAB_LABELS[slug]);
  };

  const handleRefreshClick = () => {
    setLastRefreshed(new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }));
    queryClient.invalidateQueries({ queryKey: ["models", "list"] });
  };

  const invalidateModels = () => queryClient.invalidateQueries({ queryKey: ["models", "list"] });

  if (teamId) {
    return (
      <div className="w-full h-full">
        <TeamInfoView
          teamId={teamId}
          onClose={close}
          accessToken={accessToken}
          is_team_admin={userRole === "Admin" && !isViewOnly}
          is_proxy_admin={userRole === "Proxy Admin"}
          userModels={allModelsOnProxy}
          editTeam={false}
          onUpdate={invalidateModels}
          premiumUser={premiumUser}
        />
      </div>
    );
  }

  return (
    <div className="w-full">
      <div className="flex w-full flex-col gap-2">
        <div className="mb-4 flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold">{t("Model Management")}</h2>
            {isAdmin ? (
              <p className="text-sm text-muted-foreground">{t("Add and manage models for the proxy")}</p>
            ) : (
              <p className="text-sm text-muted-foreground">{t("Add models for teams you are an admin for.")}</p>
            )}
          </div>
        </div>

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
            value={activeKey === "llm-credentials" || activeKey === BASE_TAB_KEY ? activeKey : ""}
            onValueChange={setActiveKey}
          >
            <div className="flex min-w-0 flex-nowrap items-center gap-3 border-b">
              <div className="no-scrollbar scroll-fade-e -mb-1.5 min-w-0 flex-1 overflow-x-auto pb-1.5">
                <TabsList variant="line" className="w-max justify-start">
                  <TabsTrigger value={BASE_TAB_KEY} className="flex-none">
                    {allModelsLabel}
                  </TabsTrigger>
                  {showProviders && (
                    <TabsTrigger value="llm-credentials" className="flex-none">
                      {t("pages.models.llmCredentials")}
                    </TabsTrigger>
                  )}
                </TabsList>
              </div>
              <div className="flex shrink-0 items-center gap-2 pb-1">
                {canCreate && (
                  <Button size="sm" onClick={() => setActiveKey("add")}>
                    {t("pages.models.add")}
                  </Button>
                )}
                {secondarySlugs.length > 0 && (
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      className={cn(buttonVariants({ variant: "outline", size: "sm" }))}
                    >
                      {t("pages.models.moreTools")}
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-64">
                      {secondarySlugs.map((slug) => (
                        <DropdownMenuItem key={slug} onClick={() => setActiveKey(slug)}>
                          {tabLabel(slug)}
                        </DropdownMenuItem>
                      ))}
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
                {lastRefreshed && (
                  <span className="text-xs text-muted-foreground">{t("Last Refreshed: {lastRefreshed}", { lastRefreshed })}</span>
                )}
                <Button variant="ghost" size="icon-sm" onClick={handleRefreshClick} aria-label={t("Refresh models")}>
                  <RefreshCw />
                </Button>
              </div>
            </div>
          </Tabs>
          <div className="pt-4">
            {activeKey !== BASE_TAB_KEY && activeKey !== "llm-credentials" && (
              <Button variant="ghost" size="sm" className="mb-3" onClick={() => setActiveKey(BASE_TAB_KEY)}>
                {t("pages.models.backToModels")}
              </Button>
            )}
            {renderPanel(activeKey)}
          </div>
          </>
        )}
      </div>
    </div>
  );
}

"use client";

import { useState } from "react";
import { Settings2 } from "lucide-react";
import PriceDataReload from "@/components/price_data_reload";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { useModelCostMap } from "../../hooks/models/useModelCostMap";
import { t } from "@/i18n";
import { PriceCatalog } from "./PriceCatalog";

const PriceDataManagementTab = () => {
  const { accessToken } = useAuthorized();
  const { data: costMap, refetch: refetchModelCostMap, isPending, isError } = useModelCostMap();
  const [managementOpen, setManagementOpen] = useState(false);

  return (
    <div className="min-h-full bg-background text-foreground">
      <div className="mx-auto max-w-7xl px-4 py-8 sm:px-8 sm:py-12 lg:px-10">
        <header className="mb-9 flex flex-col items-start justify-between gap-6 lg:flex-row lg:items-end">
          <div className="max-w-2xl">
            <p className="mb-4 flex items-center gap-2 text-xs font-semibold tracking-[0.18em] text-cyan-700 dark:text-cyan-300">
              <span className="size-1.5 rounded-full bg-cyan-500" aria-hidden="true" />
              AI INFERENCE MODELS
            </p>
            <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl lg:text-5xl">{t("priceCatalog.title")}</h1>
            <p className="mt-4 text-sm leading-7 text-muted-foreground sm:text-base">{t("priceCatalog.description")}</p>
          </div>
          <Dialog open={managementOpen} onOpenChange={setManagementOpen}>
            <DialogTrigger render={<Button variant="outline" className="shrink-0" />}>
              <Settings2 aria-hidden="true" />
              {t("Price Data Management")}
            </DialogTrigger>
            <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-2xl">
              <DialogHeader>
                <DialogTitle>{t("Price Data Management")}</DialogTitle>
                <DialogDescription>
                  {t("Manage model pricing data and configure automatic reload schedules")}
                </DialogDescription>
              </DialogHeader>
              <PriceDataReload
                accessToken={accessToken}
                onReloadSuccess={() => {
                  void refetchModelCostMap();
                }}
                buttonText={t("Reload Price Data")}
                size="middle"
                type="primary"
                className="w-full"
              />
            </DialogContent>
          </Dialog>
        </header>
        <PriceCatalog
          costMap={costMap}
          isLoading={isPending}
          isError={isError}
          onRetry={() => {
            void refetchModelCostMap();
          }}
        />
      </div>
    </div>
  );
};

export default PriceDataManagementTab;

"use client";

import { useMemo, useState } from "react";
import { Settings2 } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import PriceDataReload from "@/components/price_data_reload";
import { toast } from "@/lib/toast";
import { t } from "@/i18n";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { usePriceCatalog, priceCatalogKeys } from "../../hooks/models/usePriceCatalog";
import { deletePriceModel, deletePriceProvider, resetPriceModel, upsertPriceModel, upsertPriceProvider } from "@/components/networking";
import { PriceCatalog } from "./PriceCatalog";
import { PriceModelDialog } from "./PriceModelDialog";
import { PriceProviderDialog } from "./PriceProviderDialog";
import { PriceProviderPanel } from "./PriceProviderPanel";
import { priceCatalogProviders, priceCatalogRows, type PriceCatalogRow, type PriceProviderRow } from "./priceCatalogRows";

const PriceDataManagementTab = () => {
  const { accessToken } = useAuthorized();
  const { data, isPending, isError, refetch } = usePriceCatalog(accessToken);
  const queryClient = useQueryClient();
  const [managementOpen, setManagementOpen] = useState(false);
  const [modelDialogOpen, setModelDialogOpen] = useState(false);
  const [providerDialogOpen, setProviderDialogOpen] = useState(false);
  const [editingModel, setEditingModel] = useState<PriceCatalogRow | null>(null);
  const [editingProvider, setEditingProvider] = useState<PriceProviderRow | null>(null);
  const [saving, setSaving] = useState(false);

  const rows = useMemo(() => priceCatalogRows(data), [data]);
  const providers = useMemo(() => priceCatalogProviders(data), [data]);

  // Every write changes which rows are overrides, so the whole catalog is
  // refetched rather than patched locally.
  async function afterWrite(message: string) {
    await queryClient.invalidateQueries({ queryKey: priceCatalogKeys.all });
    await queryClient.invalidateQueries({ queryKey: ["modelCostMap"] });
    toast.success(message);
  }

  async function saveModel(body: Record<string, unknown>) {
    setSaving(true);
    try {
      await upsertPriceModel(accessToken, body);
      setModelDialogOpen(false);
      setEditingModel(null);
      await afterWrite(t("priceData.modelSaved"));
    } catch (error) {
      console.error("Failed to save the model price:", error);
    } finally {
      setSaving(false);
    }
  }

  async function removeModel(row: PriceCatalogRow) {
    if (!window.confirm(t("priceData.confirmDeleteModel", { id: row.id }))) return;
    await deletePriceModel(accessToken, row.id);
    await afterWrite(t("priceData.modelDeleted"));
  }

  async function revertModel(row: PriceCatalogRow) {
    await resetPriceModel(accessToken, row.id);
    await afterWrite(t("priceData.modelReset"));
  }

  async function saveProvider(body: Record<string, unknown>) {
    setSaving(true);
    try {
      await upsertPriceProvider(accessToken, body);
      setProviderDialogOpen(false);
      setEditingProvider(null);
      await afterWrite(t("priceData.providerSaved"));
    } catch (error) {
      console.error("Failed to save the provider:", error);
    } finally {
      setSaving(false);
    }
  }

  async function removeProvider(provider: PriceProviderRow) {
    if (!window.confirm(t("priceData.confirmDeleteProvider", { id: provider.name }))) return;
    await deletePriceProvider(accessToken, provider.slug);
    await afterWrite(t("priceData.providerDeleted"));
  }

  return (
    <div className="min-h-full bg-background text-foreground">
      <div className="mx-auto max-w-7xl px-4 py-8 sm:px-8 sm:py-12 lg:px-10">
        <header className="mb-9 flex flex-col items-start justify-between gap-6 lg:flex-row lg:items-end">
          <div className="max-w-2xl">
            <p className="mb-4 flex items-center gap-2 text-xs font-semibold tracking-[0.18em] text-cyan-700 dark:text-cyan-300">
              <span className="size-1.5 rounded-full bg-cyan-500" aria-hidden="true" />
              {t("priceData.eyebrow")}
            </p>
            <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl lg:text-5xl">{t("priceCatalog.title")}</h1>
            <p className="mt-4 text-sm leading-7 text-muted-foreground sm:text-base">
              {t("priceData.description")}
            </p>
            {data?.generated_at && (
              <p className="mt-3 text-xs text-muted-foreground">
                {t("priceData.generatedAt", { value0: data.generated_at })}
              </p>
            )}
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
                  void refetch();
                }}
                buttonText={t("Reload Price Data")}
                size="middle"
                type="primary"
                className="w-full"
              />
            </DialogContent>
          </Dialog>
        </header>

        <Tabs defaultValue="models">
          <TabsList className="mb-6">
            <TabsTrigger value="models">{t("priceData.models")}</TabsTrigger>
            <TabsTrigger value="providers">{t("priceData.providers")}</TabsTrigger>
          </TabsList>
          <TabsContent value="models">
            <PriceCatalog
              rows={rows}
              isLoading={isPending}
              isError={isError}
              onRetry={() => {
                void refetch();
              }}
              onAdd={() => {
                setEditingModel(null);
                setModelDialogOpen(true);
              }}
              onEdit={(row) => {
                setEditingModel(row);
                setModelDialogOpen(true);
              }}
              onDelete={(row) => {
                void removeModel(row);
              }}
              onReset={(row) => {
                void revertModel(row);
              }}
            />
          </TabsContent>
          <TabsContent value="providers">
            <PriceProviderPanel
              providers={providers}
              onAdd={() => {
                setEditingProvider(null);
                setProviderDialogOpen(true);
              }}
              onEdit={(provider) => {
                setEditingProvider(provider);
                setProviderDialogOpen(true);
              }}
              onDelete={(provider) => {
                void removeProvider(provider);
              }}
            />
          </TabsContent>
        </Tabs>

        <PriceModelDialog
          key={editingModel?.id ?? "new-model"}
          open={modelDialogOpen}
          onOpenChange={(open) => {
            setModelDialogOpen(open);
            if (!open) setEditingModel(null);
          }}
          providers={providers}
          editing={editingModel}
          onSave={(body) => {
            void saveModel(body);
          }}
          isSaving={saving}
        />
        <PriceProviderDialog
          key={editingProvider?.slug ?? "new-provider"}
          open={providerDialogOpen}
          onOpenChange={(open) => {
            setProviderDialogOpen(open);
            if (!open) setEditingProvider(null);
          }}
          editing={editingProvider}
          onSave={(body) => {
            void saveProvider(body);
          }}
          isSaving={saving}
        />
      </div>
    </div>
  );
};

export default PriceDataManagementTab;

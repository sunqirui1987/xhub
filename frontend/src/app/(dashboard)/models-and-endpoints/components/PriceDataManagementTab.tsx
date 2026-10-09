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
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { usePriceCatalog, priceCatalogKeys } from "../../hooks/models/usePriceCatalog";
import { apiClient, resetPriceModel, upsertPriceModel } from "@/components/networking";
import Link from "next/link";
import { MarketModelDetail, loadMarketAccess } from "./MarketModelDetail";
import { modelDeployments } from "./modelAccess";
import { marketSale } from "./marketCatalog";
import { MarketPriceCatalog } from "./MarketPriceCatalog";
import { PriceModelDialog } from "./PriceModelDialog";
import { LocalModelList } from "./LocalModelList";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { priceCatalogProviders, priceCatalogRows, type PriceCatalogRow } from "./priceCatalogRows";

/** 管理模型广场及本地快照；无参数，返回页面。刷新只同步目录并提示配置，保存操作需要管理会话。 */
const PriceDataManagementTab = () => {
  const { accessToken, userId, userRole } = useAuthorized();
  const { data, isPending, isError, refetch } = usePriceCatalog(accessToken);
  const queryClient = useQueryClient();
  const [managementOpen, setManagementOpen] = useState(false);
  const [modelDialogOpen, setModelDialogOpen] = useState(false);
  const [editingModel, setEditingModel] = useState<PriceCatalogRow | null>(null);
  const [missingModels, setMissingModels] = useState<PriceCatalogRow[] | null>(null);
  const [discoveryError, setDiscoveryError] = useState(false);
  const [detail, setDetail] = useState<PriceCatalogRow | null>(null);
  const [saving, setSaving] = useState(false);

  /** 刷新成功后检查在售公开价格模型是否已部署；参数无，返回异步结果。
   * 供价格重载回调使用；只提示待添加模型，不自动创建部署，读取失败显示可重试错误。 */
  async function discoverMissingModels() {
    setManagementOpen(false);
    setDiscoveryError(false);
    try {
      const refreshed = await refetch();
      if (refreshed.isError || !refreshed.data) throw new Error("价格目录读取失败");
      if (!accessToken) throw new Error("未登录");
      const access = await loadMarketAccess(accessToken, userId ?? "", userRole);
      setMissingModels(
        priceCatalogRows(refreshed.data).filter(
          (row) => marketSale(row) && !modelDeployments(row, access.deployments, true).length,
        ),
      );
    } catch {
      setDiscoveryError(true);
      setMissingModels([]);
    }
  }

  const rows = useMemo(() => priceCatalogRows(data), [data]);
  const providers = useMemo(() => priceCatalogProviders(data), [data]);

  /** 写入后刷新目录和计费缓存；参数为提示文案，返回异步结果；保存成功回调使用。 */
  async function afterWrite(message: string) {
    await queryClient.invalidateQueries({ queryKey: priceCatalogKeys.all });
    await queryClient.invalidateQueries({ queryKey: ["modelCostMap"] });
    toast.success(message);
  }

  /** 保存价格编辑；参数为表单正文，返回异步结果；弹窗使用，失败保留表单。 */
  async function saveModel(body: Record<string, unknown>) {
    setSaving(true);
    try {
      if (!accessToken) return;
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

  /** 保存本地上下架状态；参数为价格行及目标状态，返回异步结果。
   * 管理列表与广场调用；保留完整价格和已有部署，失败显示错误，不执行删除。 */
  async function setListing(row: PriceCatalogRow, delisted: boolean) {
    if (!accessToken) return;
    try {
      await apiClient.post("/price/model/listing", { accessToken, body: { id: row.id, delisted } });
      await afterWrite(delisted ? "模型已下架，本地记录已保留" : "模型已上架");
    } catch {
      toast.error("上下架保存失败，请重试");
    }
  }

  /** 恢复市场价格；参数为目录行，返回异步结果；保持独立的上下架状态。 */
  async function revertModel(row: PriceCatalogRow) {
    if (!accessToken) return;
    await resetPriceModel(accessToken, row.id);
    await afterWrite(t("priceData.modelReset"));
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
              在售公开模型的价格与能力来自 Modelink 市场目录。点击模型查看完整价格及内部接入文档。
            </p>
            <p className="mt-3 break-all text-xs text-muted-foreground">
              价格来源：{data?.source || "https://api.modelink.ai/v1/market/models"}
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
                  void discoverMissingModels();
                }}
                buttonText={t("Reload Price Data")}
                size="middle"
                type="primary"
                className="w-full"
              />
            </DialogContent>
          </Dialog>
        </header>

        <Tabs defaultValue="market">
          <TabsList className="mb-6">
            <TabsTrigger value="market">模型广场</TabsTrigger>
            <TabsTrigger value="local">本地模型列表</TabsTrigger>
          </TabsList>
          <TabsContent value="market">
            <MarketPriceCatalog
              rows={rows}
              onDetail={setDetail}
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
                void setListing(row, true);
              }}
              onReset={(row) => {
                void revertModel(row);
              }}
            />
          </TabsContent>
          <TabsContent value="local">
            <LocalModelList
              rows={rows}
              onDetail={setDetail}
              onListing={(row, delisted) => void setListing(row, delisted)}
            />
          </TabsContent>
        </Tabs>

        <Dialog
          open={missingModels !== null}
          onOpenChange={(open) => {
            if (!open) setMissingModels(null);
          }}
        >
          <DialogContent className="max-h-[80dvh] overflow-y-auto sm:max-w-2xl">
            <DialogHeader>
              <DialogTitle>是否添加价格表中的模型？</DialogTitle>
              <DialogDescription>选择模型后配置供应商和调用方式，确认保存后才会成为可调用模型。</DialogDescription>
            </DialogHeader>
            {discoveryError ? (
              <p role="alert">
                待添加模型读取失败。
                <Button variant="outline" onClick={() => void discoverMissingModels()}>
                  重试
                </Button>
              </p>
            ) : (
              <>
                <p role="status">发现 {missingModels?.length ?? 0} 个尚未部署的在售公开模型。</p>
                <ul className="max-h-96 space-y-2 overflow-y-auto">
                  {missingModels?.map((row) => (
                    <li key={row.id} className="flex items-center justify-between gap-3 rounded border p-3">
                      <span className="min-w-0 text-sm">
                        {row.displayName || row.id}
                        <code className="block break-all text-xs text-muted-foreground">{row.id}</code>
                      </span>
                      <Link
                        className="shrink-0 text-sm text-cyan-700 underline"
                        href={"/models-and-endpoints?catalog=" + encodeURIComponent(row.id)}
                      >
                        添加此模型
                      </Link>
                    </li>
                  ))}
                </ul>
              </>
            )}
            <Button variant="outline" onClick={() => setMissingModels(null)}>
              暂不添加
            </Button>
          </DialogContent>
        </Dialog>
        <MarketModelDetail
          row={detail}
          accessToken={accessToken}
          userId={userId}
          userRole={userRole}
          onClose={() => setDetail(null)}
        />
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
      </div>
    </div>
  );
};

export default PriceDataManagementTab;

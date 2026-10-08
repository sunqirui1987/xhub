"use client";

import { useState } from "react";
import { ArrowUpRight, Boxes, Search } from "lucide-react";
import { useModelsInfo } from "@/app/(dashboard)/hooks/models/useModels";
import { useModelCostMap } from "@/app/(dashboard)/hooks/models/useModelCostMap";
import { useModelDetailRouting } from "../detailNavigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import ModelStatusToggle from "@/components/add_model/ModelStatusToggle";
import { Badge } from "@/components/ui/badge";
import {
  effectivePricing,
  hasPrice,
  pricingRates,
  type EditorModel,
  type CatalogRow,
} from "@/components/add_model/modelEditorPricing";

function priceSummary(model: EditorModel, rows: Record<string, CatalogRow>) {
  const price = effectivePricing(model, rows);
  if (!hasPrice(price.row)) return "尚未定价";
  const rates = pricingRates(price.row);
  const measures = new Set(rates.map((rate) => rate.measure));
  const units: Record<string, string> = { token: "按 Token", picture: "按张", second: "按秒", query: "按次" };
  return Array.from(measures)
    .map((measure) => units[measure] ?? measure)
    .join(" · ");
}

export default function AllModelsPanel() {
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const models = useModelsInfo(page, 20, search || undefined);
  const { data: costMap } = useModelCostMap();
  const { openModel } = useModelDetailRouting();
  const rows = (costMap ?? {}) as Record<string, CatalogRow>;
  const data = (models.data?.data ?? []) as EditorModel[];
  const pages = models.data?.total_pages ?? 1;
  return (
    <section className="overflow-hidden rounded-xl border bg-card">
      <div className="flex flex-wrap items-center justify-between gap-4 border-b p-5">
        <div>
          <h2 className="flex items-center gap-2 font-semibold">
            <Boxes className="size-4" />
            模型部署<span className="text-sm font-normal text-muted-foreground">{models.data?.total_count ?? "—"}</span>
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">查看提供商、上游模型、调用方式与实际计费来源。</p>
        </div>
        <div className="relative w-full sm:w-80">
          <Search className="absolute left-3 top-3 size-4 text-muted-foreground" />
          <Input
            aria-label="搜索模型"
            placeholder="搜索名称、提供商或上游模型…"
            className="pl-9"
            value={search}
            onChange={(event) => {
              setSearch(event.target.value);
              setPage(1);
            }}
          />
        </div>
      </div>
      {models.error ? (
        <div className="space-y-3 p-8" role="alert">
          <p className="text-sm">模型加载失败。</p>
          <Button variant="outline" onClick={() => models.refetch()}>
            重新加载
          </Button>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="bg-muted/30 text-xs text-muted-foreground">
              <tr>
                <th className="px-5 py-3 font-medium">模型名称</th>
                <th className="px-5 py-3 font-medium">模型提供商 / 上游模型</th>
                <th className="px-5 py-3 font-medium">调用方式</th>
                <th className="px-5 py-3 font-medium">计价</th>
                <th className="px-5 py-3 font-medium">状态</th>
                <th className="px-5 py-3">
                  <span className="sr-only">操作</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {data.map((model) => (
                <tr key={String(model.model_info.id)} className="border-t hover:bg-muted/20">
                  <td className="min-w-48 px-5 py-4">
                    <button
                      type="button"
                      onClick={() => openModel(String(model.model_info.id))}
                      className="break-all text-left font-semibold text-primary hover:underline"
                    >
                      {model.model_name}
                    </button>
                    <p className="mt-1 font-mono text-xs text-muted-foreground">{String(model.model_info.id)}</p>
                  </td>
                  <td className="max-w-80 px-5 py-4">
                    <p className="font-medium">
                      {String(
                        model.litellm_params.litellm_credential_name ||
                          model.litellm_params.custom_llm_provider ||
                          "独立连接",
                      )}
                    </p>
                    <p className="mt-1 break-all text-xs text-muted-foreground">
                      {String(model.litellm_params.model ?? "—")}
                    </p>
                  </td>
                  <td className="px-5 py-4">
                    <Badge variant="outline">{String(model.model_info.mode ?? "chat")}</Badge>
                  </td>
                  <td className="px-5 py-4">
                    <p className="whitespace-nowrap">{priceSummary(model, rows)}</p>
                    <p className="mt-1 text-xs text-muted-foreground">{effectivePricing(model, rows).source}</p>
                  </td>
                  <td className="px-5 py-4">
                    <ModelStatusToggle model={model} />
                  </td>
                  <td className="px-5 py-4">
                    <Button variant="ghost" size="sm" onClick={() => openModel(String(model.model_info.id))}>
                      详情
                      <ArrowUpRight className="size-3.5" />
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {models.isLoading && (
            <p role="status" className="p-12 text-center text-sm text-muted-foreground">
              正在加载模型…
            </p>
          )}
          {!models.isLoading && !data.length && (
            <div className="space-y-2 p-12 text-center">
              <Boxes className="mx-auto size-8 text-muted-foreground" />
              <p className="text-sm font-medium">{search ? "没有找到匹配的模型" : "尚未添加模型"}</p>
              <p className="text-xs text-muted-foreground">
                {search ? "尝试其他模型名称或提供商。" : "先配置模型提供商，再添加模型部署。"}
              </p>
            </div>
          )}
        </div>
      )}
      <div className="flex items-center justify-between border-t px-5 py-3 text-xs text-muted-foreground">
        <span>
          第 {page} / {pages} 页
        </span>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1 || models.isFetching}
            onClick={() => setPage(page - 1)}
          >
            上一页
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= pages || models.isFetching}
            onClick={() => setPage(page + 1)}
          >
            下一页
          </Button>
        </div>
      </div>
    </section>
  );
}

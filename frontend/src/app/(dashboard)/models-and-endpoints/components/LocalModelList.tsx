"use client";
import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import type { PriceCatalogRow } from "./priceCatalogRows";
import { marketRetired } from "./marketCatalog";

/** 本地模型档案列表；参数含完整目录及详情、上下架回调，返回可搜索表格。
 * 价格页管理标签调用；全部记录保留，只允许改变广场展示，退役、非公开和上游撤回条目不能强制上架。 */
export function LocalModelList({
  rows,
  onDetail,
  onListing,
}: {
  rows: PriceCatalogRow[];
  onDetail: (row: PriceCatalogRow) => void;
  onListing: (row: PriceCatalogRow, delisted: boolean) => void;
}) {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("all");
  const visible = rows.filter(
    (row) =>
      [row.id, row.displayName, row.provider].join(" ").toLowerCase().includes(query.trim().toLowerCase()) &&
      (status === "all" || (status === "down" ? !!row.raw.delisted : !row.raw.delisted)),
  );
  return (
    <section aria-label="本地模型列表" className="space-y-4">
      <p className="text-sm text-muted-foreground">
        本地保存模型资料与完整价格。下架只隐藏广场展示，保留记录与已配置的调用接口；同步后保留上下架设置。
      </p>
      <div className="flex gap-3">
        <Input
          aria-label="搜索本地模型"
          placeholder="搜索名称、ID 或供应商"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <select
          className="rounded border bg-background px-3"
          aria-label="本地模型状态"
          value={status}
          onChange={(e) => setStatus(e.target.value)}
        >
          <option value="all">全部</option>
          <option value="up">未下架</option>
          <option value="down">已下架</option>
        </select>
      </div>
      <p role="status">
        本地共 {rows.length} 个模型，当前匹配 {visible.length} 个
      </p>
      <div className="overflow-x-auto rounded-xl border">
        <table className="w-full text-left text-sm">
          <caption className="sr-only">本地模型档案</caption>
          <thead>
            <tr className="border-b bg-muted/40">
              <th className="p-3">模型</th>
              <th className="p-3">供应商</th>
              <th className="p-3">状态</th>
              <th className="p-3">操作</th>
            </tr>
          </thead>
          <tbody>
            {visible.map((row) => {
              const unavailable =
                row.raw.private === true ||
                marketRetired(row) ||
                row.raw.feed_unavailable === true ||
                row.removed ||
                row.raw.market_catalog !== true;
              return (
                <tr key={row.id} className="border-b last:border-0" data-testid={"local-model-" + row.id}>
                  <td className="p-3">
                    <button className="text-left font-medium hover:underline" onClick={() => onDetail(row)}>
                      {row.displayName || row.id}
                    </button>
                    <code className="block text-xs text-muted-foreground">{row.id}</code>
                  </td>
                  <td className="p-3">{String(row.raw.issuer || row.provider || "自定义")}</td>
                  <td className="p-3">
                    {row.raw.delisted ? "已下架" : "未下架"}
                    {row.raw.private === true
                      ? " · 非公开"
                      : marketRetired(row)
                        ? " · 已退役"
                        : row.raw.feed_unavailable
                          ? " · 上游已撤回"
                          : row.raw.market_catalog !== true
                            ? " · 本地自定义"
                            : ""}
                  </td>
                  <td className="p-3">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={!!row.raw.delisted && unavailable}
                      onClick={() => onListing(row, !row.raw.delisted)}
                    >
                      {row.raw.delisted ? "上架" : "下架"}
                    </Button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {!visible.length && <p>没有匹配的本地模型。</p>}
    </section>
  );
}

"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { ArrowDownUp, Boxes, Pencil, Plus, Rocket, RotateCcw, Search, SlidersHorizontal, Trash2 } from "lucide-react";
import { t } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FALLBACK_CATEGORY, type PriceCatalogRow } from "./priceCatalogRows";
import { measureLabel, rateLabel, rateText, windowLabel } from "@/lib/rateDisplay";

import { MarketPriceCatalog } from "./MarketPriceCatalog";

type SortOrder = "name" | "input" | "output";
type Modality = "text" | "image" | "audio" | "video";

const MODALITIES: { value: Modality; label: string }[] = [
  { value: "text", label: "priceCatalog.textModality" },
  { value: "image", label: "priceCatalog.imageModality" },
  { value: "audio", label: "priceCatalog.audioModality" },
  { value: "video", label: "priceCatalog.videoModality" },
];

const CAPABILITY_FILTERS = [
  { value: "longContext", label: "priceCatalog.longContext" },
  { value: "tools", label: "priceCatalog.tools" },
  { value: "reasoning", label: "priceCatalog.reasoning" },
  { value: "vision", label: "priceCatalog.vision" },
  { value: "caching", label: "priceCatalog.caching" },
] as const;

function priceText(value: number | null): string {
  if (value == null) return t("Price unavailable");
  return "$" + new Intl.NumberFormat("en-US", { maximumSignificantDigits: 8 }).format(value);
}

function categoryLabel(category: string): string {
  return category === FALLBACK_CATEGORY ? t("Other") : category;
}

function rowModalities(row: PriceCatalogRow): Set<Modality> {
  const text = [row.id, row.category, row.displayName ?? ""].join(" ").toLowerCase();
  const measures = new Set(row.rateGroups.map((group) => group.measure));
  const result = new Set<Modality>();
  if (measures.has("picture") || row.capabilities.includes("vision") || /image|vision/.test(text)) result.add("image");
  if (/audio|speech|transcri/.test(text)) result.add("audio");
  if (
    /video/.test(text) ||
    row.rateGroups.some((group) => group.rates.some((rate) => /video|t2v|i2v|r2v/.test(rate.variant)))
  )
    result.add("video");
  if (measures.has("token") || (!result.has("audio") && !result.has("video"))) result.add("text");
  return result;
}

function ModelCard({
  row,
  onEdit,
  onDelete,
  onReset,
}: {
  row: PriceCatalogRow;
  onEdit?: (row: PriceCatalogRow) => void;
  onDelete?: (row: PriceCatalogRow) => void;
  onReset?: (row: PriceCatalogRow) => void;
}) {
  const editable = Boolean(onEdit || onDelete || onReset);
  return (
    <article
      data-testid={"price-row-" + row.id}
      className="min-w-0 rounded-xl border border-border bg-card p-5 transition-colors hover:border-cyan-500/50 sm:p-6"
    >
      <div className="flex items-start gap-3">
        <div className="flex size-11 shrink-0 items-center justify-center rounded-xl border border-cyan-500/20 bg-cyan-500/10 text-cyan-700 dark:text-cyan-300">
          <Boxes className="size-5" aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1">
          <h3 className="break-all text-lg font-semibold leading-snug tracking-tight">{row.id}</h3>
          <p className="mt-1 break-all text-sm text-muted-foreground">
            {row.provider ?? t("priceCatalog.unknownProvider")}
          </p>
        </div>
        {editable && (
          <div className="flex shrink-0 gap-1">
            {onReset && row.overridden && (
              <Button
                size="sm"
                variant="ghost"
                aria-label={t("priceData.resetModel")}
                title={t("priceData.resetModel")}
                onClick={() => onReset(row)}
              >
                <RotateCcw aria-hidden="true" />
              </Button>
            )}
            {onEdit && (
              <Button size="sm" variant="ghost" aria-label={t("Edit")} title={t("Edit")} onClick={() => onEdit(row)}>
                <Pencil aria-hidden="true" />
              </Button>
            )}
            {onDelete && (
              <Button
                size="sm"
                variant="ghost"
                aria-label={t("Delete")}
                title={t("Delete")}
                onClick={() => onDelete(row)}
              >
                <Trash2 aria-hidden="true" />
              </Button>
            )}
          </div>
        )}
      </div>
      <div className="mt-4 flex flex-wrap gap-2">
        <Badge variant="outline" className="font-mono text-xs">
          {categoryLabel(row.category)}
        </Badge>
        {row.overridden && <Badge variant="secondary">{t("priceData.edited")}</Badge>}
        {row.removed && <Badge variant="destructive">{t("priceData.removed")}</Badge>}
        {row.capabilities.map((capability) => (
          <Badge key={capability} variant="secondary">
            {t("priceCatalog." + capability)}
          </Badge>
        ))}
      </div>
      <Link
        className={buttonVariants({ variant: "outline", className: "mt-4 w-full" })}
        href={`/models-and-endpoints?catalog=${encodeURIComponent(row.id)}`}
      >
        <Rocket aria-hidden="true" />
        {t("priceCatalog.deployModel")}
      </Link>
      {(row.maxInput || row.maxOutput) && (
        <div className="mt-4 flex flex-wrap gap-x-5 gap-y-1 text-xs text-muted-foreground">
          {row.maxInput != null && (
            <span>
              {t("priceCatalog.maxInput")}{" "}
              <strong className="font-medium text-foreground">{row.maxInput.toLocaleString()} tokens</strong>
            </span>
          )}
          {row.maxOutput != null && (
            <span>
              {t("priceCatalog.maxOutput")}{" "}
              <strong className="font-medium text-foreground">{row.maxOutput.toLocaleString()} tokens</strong>
            </span>
          )}
        </div>
      )}
      <dl className="mt-5 grid grid-cols-2 gap-3 border-t border-border pt-4">
        {[
          { label: t("Input"), value: row.input },
          { label: t("Output"), value: row.output },
        ].map(({ label, value }) => (
          <div key={label}>
            <dt className="text-xs text-muted-foreground">
              {label}
              {/* 分时模型的通用两格取的是最便宜那一档，也就是空闲价。
                  不标出来的话，上面的数字和下面表里的高峰价对不上。 */}
              {row.windowed && value != null && <span className="ml-1 text-muted-foreground">({t("Off-peak")})</span>}
            </dt>
            <dd className="mt-1 flex flex-wrap items-baseline gap-1.5">
              <span
                className={value == null ? "text-sm text-muted-foreground" : "text-2xl font-semibold tracking-tight"}
              >
                {priceText(value)}
              </span>
              {value != null && <span className="text-xs text-muted-foreground">/ 1M tokens</span>}
            </dd>
          </div>
        ))}
      </dl>
      {/* 费率表。按计费维度分组：一个维度一行标题，组内按时段并排列出。
          视频和图像模型在通用输入输出那两格上是空的，价全在这里，所以这一块
          对它们是唯一的价格展示，不是"更多信息"。 */}
      {row.rateGroups.length > 0 && (
        <div className="mt-4 space-y-4 border-t border-border pt-3">
          {row.rateGroups.map((group) => (
            <div key={group.measure}>
              <div className="mb-2 flex items-center gap-2">
                <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {t("priceCatalog.byMeasure", { measure: measureLabel(group.measure) })}
                </span>
                {group.measure !== "token" && group.unitSize > 1 && (
                  <span className="text-xs text-muted-foreground">
                    {t("priceCatalog.perUnits", { count: group.unitSize })}
                  </span>
                )}
              </div>
              <table className="w-full text-xs">
                <thead>
                  <tr className="text-muted-foreground">
                    <th className="py-1 text-left font-normal">{t("priceCatalog.rateSide")}</th>
                    {group.windows.map((window) => (
                      <th key={window} className="py-1 text-right font-normal">
                        {windowLabel(window)}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {group.rates
                    .filter(
                      (rate, index, rates) =>
                        rates.findIndex(
                          (candidate) => candidate.side === rate.side && candidate.variant === rate.variant,
                        ) === index,
                    )
                    .map((rate) => (
                      <tr key={rate.sourceKey || `${rate.side}-${rate.variant}-${rate.window}`}>
                        <td className="py-1">
                          {rateLabel(rate.side, rate.variant)}
                          {/* 市场的中文说明（"含视频输入（1080p）"）和上面那一段
                            说的是同一件事，重复显示只会让人以为有两档价。
                            只在它带来新信息时才附上，而且用 hover 收起来。 */}
                          {rate.label && rate.label !== rateLabel(rate.side, rate.variant) && (
                            <span className="ml-1 text-muted-foreground" title={rate.label}>
                              · {rate.label}
                            </span>
                          )}
                        </td>
                        {group.windows.map((window) => {
                          const cell = group.rates.find(
                            (candidate) =>
                              candidate.side === rate.side &&
                              candidate.variant === rate.variant &&
                              candidate.window === window,
                          );
                          return (
                            <td key={window} className="py-1 text-right font-medium tabular-nums">
                              {cell ? rateText(cell.usd, group.measure) : "—"}
                            </td>
                          );
                        })}
                      </tr>
                    ))}
                </tbody>
              </table>
            </div>
          ))}
        </div>
      )}
      {/* 费率表已经按维度把每一条都列出来了。这里只补它没有的东西：
          缓存读写这类不在"输入/输出"两格里的价，以及**没有费率表**的旧行
          （扁平字段时代的条目）才需要兜底展示。两者都显示会让同一条价出现两次。 */}
      {row.extraPrices.length > 0 && row.rateGroups.length === 0 && (
        <details className="mt-4 border-t border-border pt-3 text-sm">
          <summary className="cursor-pointer text-muted-foreground hover:text-foreground">
            {t("priceCatalog.morePrices", { count: row.extraPrices.length })}
          </summary>
          <dl className="mt-3 space-y-2">
            {row.extraPrices.map((price) => (
              <div key={price.key} className="flex flex-wrap justify-between gap-2 text-xs">
                <dt>{t("priceCatalog." + price.key)}</dt>
                <dd className="font-medium">
                  {priceText(price.value)}{" "}
                  <span className="font-normal text-muted-foreground">
                    / {price.unit === "tokens" ? "1M tokens" : t("priceCatalog." + price.unit)}
                  </span>
                </dd>
              </div>
            ))}
          </dl>
        </details>
      )}
    </article>
  );
}

/** The search, sort, filter and add controls above the model list. */
function CatalogToolbar({
  query,
  onQuery,
  sort,
  onSort,
  showFilters,
  onToggleFilters,
  activeFilters,
  onAdd,
}: {
  query: string;
  onQuery: (value: string) => void;
  sort: SortOrder;
  onSort: (value: SortOrder) => void;
  showFilters: boolean;
  onToggleFilters: () => void;
  activeFilters: number;
  onAdd?: () => void;
}) {
  return (
    <div className="flex flex-col gap-3 rounded-xl border border-border bg-card p-3 sm:flex-row">
      <div className="relative min-w-0 flex-1">
        <Search
          aria-hidden="true"
          className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          aria-label={t("Search")}
          placeholder={t("priceCatalog.search")}
          className="h-11 border-0 bg-transparent pl-10 shadow-none"
          value={query}
          onChange={(event) => onQuery(event.target.value)}
        />
      </div>
      <label className="flex h-11 items-center gap-2 rounded-lg border border-input bg-background px-3 text-sm sm:w-56">
        <ArrowDownUp aria-hidden="true" className="size-4 text-muted-foreground" />
        <select
          aria-label={t("priceCatalog.sort")}
          className="min-w-0 flex-1 bg-background outline-none focus-visible:ring-2 focus-visible:ring-ring"
          value={sort}
          onChange={(event) => onSort(event.target.value as SortOrder)}
        >
          <option value="name">{t("priceCatalog.sortName")}</option>
          <option value="input">{t("priceCatalog.sortInput")}</option>
          <option value="output">{t("priceCatalog.sortOutput")}</option>
        </select>
      </label>
      <Button
        type="button"
        variant="outline"
        className="h-11 px-4"
        aria-expanded={showFilters}
        aria-controls="price-catalog-filters"
        onClick={onToggleFilters}
      >
        <SlidersHorizontal aria-hidden="true" />
        {t("priceCatalog.filters")}
        {activeFilters > 0 && <Badge variant="secondary">{activeFilters}</Badge>}
      </Button>
      {onAdd && (
        <Button type="button" className="h-11 px-4" onClick={onAdd} data-testid="price-add-model">
          <Plus aria-hidden="true" />
          {t("priceData.addModel")}
        </Button>
      )}
    </div>
  );
}

export type PriceCatalogProps = {

  costMap?: Record<string, unknown> | null;
  rows?: PriceCatalogRow[];
  isLoading?: boolean;
  isError?: boolean;
  onRetry?: () => void;
  onAdd?: () => void;
  onEdit?: (row: PriceCatalogRow) => void;
  onDelete?: (row: PriceCatalogRow) => void;
  onReset?: (row: PriceCatalogRow) => void;
  onDetail?: (row: PriceCatalogRow) => void;
};

/** 价格目录入口；参数为价格行及操作回调，返回对应目录布局；市场元数据存在时使用完整筛选，旧手工目录继续兼容原布局。 */
export function PriceCatalog(props: PriceCatalogProps) {
  return props.rows?.some(row => row.raw.market_catalog === true) ? <MarketPriceCatalog {...props} /> : <LegacyPriceCatalog {...props} />;
}

/** 旧手工目录布局；参数为价格行与操作回调，返回可编辑列表，保持无市场元数据目录的展示兼容性。 */
function LegacyPriceCatalog({costMap, rows: providedRows, isLoading, isError, onRetry, onAdd, onEdit, onDelete, onReset}: PriceCatalogProps) {
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("all");
  const [provider, setProvider] = useState("all");
  const [modality, setModality] = useState<Modality | "all">("all");
  const [capability, setCapability] = useState("all");
  const [sort, setSort] = useState<SortOrder>("name");
  const [showFilters, setShowFilters] = useState(false);
  const [limit, setLimit] = useState(60);
  const rows = useMemo(() => providedRows ?? [], [providedRows]);
  const categories = useMemo(() => ["all", ...Array.from(new Set(rows.map((row) => row.category))).sort()], [rows]);
  const providers = useMemo(
    () =>
      Array.from(new Set(rows.map((row) => row.provider).filter((value): value is string => Boolean(value)))).sort(),
    [rows],
  );
  const activeFilters =
    Number(category !== "all") + Number(provider !== "all") + Number(modality !== "all") + Number(capability !== "all");
  const visible = useMemo(() => {
    const search = query.trim().toLowerCase();
    return rows
      .filter((row) => {
        if (category !== "all" && row.category !== category) return false;
        if (provider !== "all" && (row.provider ?? "unknown") !== provider) return false;
        if (modality !== "all" && !rowModalities(row).has(modality)) return false;
        if (capability === "longContext" && (row.maxInput ?? 0) < 128_000) return false;
        if (capability !== "all" && capability !== "longContext" && !row.capabilities.includes(capability))
          return false;
        return !search || [row.id, row.provider ?? "", row.category].join(" ").toLowerCase().includes(search);
      })
      .sort((a, b) => {
        if (sort === "input" || sort === "output") {
          return (a[sort] ?? Infinity) - (b[sort] ?? Infinity) || a.id.localeCompare(b.id);
        }
        return a.id.localeCompare(b.id);
      });
  }, [capability, category, modality, provider, query, rows, sort]);

  function resetFilters() {
    setQuery("");
    setCategory("all");
    setProvider("all");
    setModality("all");
    setCapability("all");
    setLimit(60);
  }

  const emptyResult = !isLoading && !isError && visible.length === 0;

  const emptyCatalog = visible.length === 0;
  const catalogIsEmpty = rows.length === 0;

  return (
    <section aria-label={t("Price catalog")}>
      <CatalogToolbar
        query={query}
        onQuery={(value) => {
          setQuery(value);
          setLimit(60);
        }}
        sort={sort}
        onSort={(value) => {
          setSort(value);
          setLimit(60);
        }}
        showFilters={showFilters}
        onToggleFilters={() => setShowFilters(!showFilters)}
        activeFilters={activeFilters}
        onAdd={onAdd}
      />

      <div className="mt-4 space-y-4 rounded-xl border border-border bg-card p-4 sm:p-5">
        <div>
          <p className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {t("priceCatalog.manufacturers")}
          </p>
          <div className="flex gap-2 overflow-x-auto pb-1" role="group" aria-label={t("priceCatalog.manufacturers")}>
            {["all", ...providers].map((item) => (
              <Button
                key={item}
                type="button"
                size="sm"
                className="shrink-0"
                variant={provider === item ? "default" : "outline"}
                aria-pressed={provider === item}
                onClick={() => {
                  setProvider(item);
                  setLimit(60);
                }}
              >
                {item === "all" ? t("priceCatalog.allProviders") : item}
              </Button>
            ))}
            {rows.some((row) => row.provider == null) && (
              <Button
                type="button"
                size="sm"
                className="shrink-0"
                variant={provider === "unknown" ? "default" : "outline"}
                aria-pressed={provider === "unknown"}
                onClick={() => setProvider("unknown")}
              >
                {t("priceCatalog.unknownProvider")}
              </Button>
            )}
          </div>
        </div>
        <div className="grid gap-4 lg:grid-cols-2">
          <div>
            <p className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
              {t("priceCatalog.modalities")}
            </p>
            <div className="flex flex-wrap gap-2" role="group" aria-label={t("priceCatalog.modalities")}>
              <Button
                type="button"
                size="sm"
                variant={modality === "all" ? "default" : "outline"}
                aria-pressed={modality === "all"}
                onClick={() => setModality("all")}
              >
                {t("All")}
              </Button>
              {MODALITIES.map((item) => (
                <Button
                  key={item.value}
                  type="button"
                  size="sm"
                  variant={modality === item.value ? "default" : "outline"}
                  aria-pressed={modality === item.value}
                  onClick={() => setModality(item.value)}
                >
                  {t(item.label)}
                </Button>
              ))}
            </div>
          </div>
          <div>
            <p className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
              {t("priceCatalog.contexts")}
            </p>
            <div className="flex flex-wrap gap-2" role="group" aria-label={t("priceCatalog.contexts")}>
              <Button
                type="button"
                size="sm"
                variant={capability === "all" ? "default" : "outline"}
                aria-pressed={capability === "all"}
                onClick={() => setCapability("all")}
              >
                {t("All")}
              </Button>
              {CAPABILITY_FILTERS.map((item) => (
                <Button
                  key={item.value}
                  type="button"
                  size="sm"
                  variant={capability === item.value ? "default" : "outline"}
                  aria-pressed={capability === item.value}
                  onClick={() => setCapability(item.value)}
                >
                  {t(item.label)}
                </Button>
              ))}
            </div>
          </div>
        </div>
      </div>

      {showFilters && (
        <div id="price-catalog-filters" className="mt-3 space-y-5 rounded-xl border border-border bg-card p-5">
          <div>
            <p className="mb-3 text-sm font-medium">{t("Category")}</p>
            <div className="flex flex-wrap gap-2" role="group" aria-label={t("Category")}>
              {categories.map((item) => (
                <Button
                  key={item}
                  type="button"
                  size="sm"
                  variant={category === item ? "default" : "outline"}
                  aria-pressed={category === item}
                  onClick={() => {
                    setCategory(item);
                    setLimit(60);
                  }}
                >
                  {item === "all" ? t("All") : categoryLabel(item)}
                </Button>
              ))}
            </div>
          </div>
          <label className="flex flex-wrap items-center gap-3 text-sm">
            {t("Provider")}
            <select
              aria-label={t("Provider")}
              className="h-9 max-w-full rounded-md border border-input bg-background px-3"
              value={provider}
              onChange={(event) => {
                setProvider(event.target.value);
                setLimit(60);
              }}
            >
              <option value="all">{t("priceCatalog.allProviders")}</option>
              {providers.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
              {rows.some((row) => row.provider == null) && (
                <option value="unknown">{t("priceCatalog.unknownProvider")}</option>
              )}
            </select>
          </label>
        </div>
      )}

      <div className="my-5 flex flex-wrap items-center justify-between gap-3 text-sm">
        <p role="status" className="text-muted-foreground">
          {t("priceCatalog.results", { count: visible.length, total: rows.length })}
        </p>
        {(query || activeFilters > 0) && (
          <Button type="button" size="sm" variant="ghost" onClick={resetFilters}>
            {t("priceCatalog.reset")}
          </Button>
        )}
      </div>
      <p className="mb-5 border-l-2 border-cyan-500/40 pl-3 text-xs leading-relaxed text-muted-foreground">
        {t("priceCatalog.units")}
      </p>

      {isError && (
        <div
          role="alert"
          className="mb-5 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-destructive/30 p-5 text-sm"
        >
          <span>{t("priceCatalog.loadError")}</span>
          <Button variant="outline" onClick={onRetry}>
            {t("priceCatalog.retry")}
          </Button>
        </div>
      )}
      {isLoading && (
        <div role="status" className="rounded-xl border border-border p-12 text-center text-muted-foreground">
          {t("priceCatalog.loading")}
        </div>
      )}
      {!isLoading && visible.length > 0 && (
        <>
          <div className="grid items-start gap-4 md:grid-cols-2 xl:grid-cols-3">
            {visible.slice(0, limit).map((row) => (
              <ModelCard key={row.id} row={row} onEdit={onEdit} onDelete={onDelete} onReset={onReset} />
            ))}
          </div>
          {visible.length > limit && (
            <div className="mt-6 text-center">
              <Button variant="outline" onClick={() => setLimit(limit + 60)}>
                {t("priceCatalog.loadMore", { count: visible.length - limit })}
              </Button>
            </div>
          )}
        </>
      )}
      {emptyResult && (
        <div className="rounded-xl border border-dashed border-border p-12 text-center">
          <Search aria-hidden="true" className="mx-auto mb-4 size-7 text-muted-foreground" />
          <p className="font-medium">{t(catalogIsEmpty ? "priceCatalog.empty" : "priceCatalog.noMatches")}</p>
          <p className="mt-2 text-sm text-muted-foreground">
            {t(catalogIsEmpty ? "priceData.emptyHint" : "priceCatalog.tryFilters")}
          </p>
          {onAdd && (
            <Button className="mt-5" onClick={onAdd}>
              <Plus aria-hidden="true" />
              {t("priceData.addModel")}
            </Button>
          )}
        </div>
      )}
    </section>
  );
}

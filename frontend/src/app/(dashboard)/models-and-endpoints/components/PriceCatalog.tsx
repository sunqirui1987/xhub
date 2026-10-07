"use client";

import { useMemo, useState } from "react";
import { ArrowDownUp, Boxes, Pencil, Plus, RotateCcw, Search, SlidersHorizontal, Trash2 } from "lucide-react";
import { t } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FALLBACK_CATEGORY, type PriceCatalogRow } from "./priceCatalogRows";

type SortOrder = "name" | "input" | "output";

function priceText(value: number | null): string {
  if (value == null) return t("Price unavailable");
  return "$" + new Intl.NumberFormat("en-US", { maximumSignificantDigits: 8 }).format(value);
}

function categoryLabel(category: string): string {
  return category === FALLBACK_CATEGORY ? t("Other") : category;
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
              <Button
                size="sm"
                variant="ghost"
                aria-label={t("Edit")}
                title={t("Edit")}
                onClick={() => onEdit(row)}
              >
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
            <dt className="text-xs text-muted-foreground">{label}</dt>
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
      {row.extraPrices.length > 0 && (
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

export function PriceCatalog({
  costMap,
  rows: providedRows,
  isLoading,
  isError,
  onRetry,
  onAdd,
  onEdit,
  onDelete,
  onReset,
}: {
  costMap?: Record<string, unknown> | null;
  rows?: PriceCatalogRow[];
  isLoading?: boolean;
  isError?: boolean;
  onRetry?: () => void;
  onAdd?: () => void;
  onEdit?: (row: PriceCatalogRow) => void;
  onDelete?: (row: PriceCatalogRow) => void;
  onReset?: (row: PriceCatalogRow) => void;
}) {
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("all");
  const [provider, setProvider] = useState("all");
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
  const activeFilters = Number(category !== "all") + Number(provider !== "all");
  const visible = useMemo(() => {
    const search = query.trim().toLowerCase();
    return rows
      .filter((row) => {
        if (category !== "all" && row.category !== category) return false;
        if (provider !== "all" && (row.provider ?? "unknown") !== provider) return false;
        return !search || [row.id, row.provider ?? "", row.category].join(" ").toLowerCase().includes(search);
      })
      .sort((a, b) => {
        if (sort === "input" || sort === "output") {
          return (a[sort] ?? Infinity) - (b[sort] ?? Infinity) || a.id.localeCompare(b.id);
        }
        return a.id.localeCompare(b.id);
      });
  }, [category, provider, query, rows, sort]);

  function resetFilters() {
    setQuery("");
    setCategory("all");
    setProvider("all");
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
          <div className="grid items-start gap-4 md:grid-cols-2">
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

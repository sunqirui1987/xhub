"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { ArrowDownUp, ArrowRight, Boxes, Search, SlidersHorizontal } from "lucide-react";
import { t } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/cva.config";
import { uiHref } from "@/utils/uiHref";
import type { MyModelCard } from "./grantedModelCards";

export type { MyModelCard };

type SortOrder = "name" | "input" | "output";

function priceText(value: number | null): string {
  if (value == null) return t("Price unavailable");
  return "$" + new Intl.NumberFormat("en-US", { maximumSignificantDigits: 8 }).format(value);
}

function categoryLabel(category: string): string {
  return category === "other" ? t("Other") : category;
}

function ModelCard({ model }: { model: MyModelCard }) {
  return (
    <article
      data-testid={"my-model-" + model.id}
      className="flex min-w-0 flex-col rounded-lg border border-border bg-card p-5 transition-colors hover:border-cyan-500/50 sm:p-6"
    >
      <div className="flex items-start gap-3">
        <div className="flex size-11 shrink-0 items-center justify-center rounded-lg border border-cyan-500/20 bg-cyan-500/10 text-cyan-700 dark:text-cyan-300">
          <Boxes className="size-5" aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1">
          <h2 className="break-all text-lg font-semibold leading-snug">{model.id}</h2>
          <p className="mt-1 break-all text-sm text-muted-foreground">
            {model.provider ?? t("myModels.unknownProvider")}
          </p>
        </div>
        <Badge className="shrink-0 border-emerald-500/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300">
          {t("myModels.available")}
        </Badge>
      </div>
      <div className="mt-4 flex flex-wrap gap-2">
        <Badge variant="outline" className="font-mono text-xs">
          {categoryLabel(model.category)}
        </Badge>
        {model.capabilities.map((capability) => (
          <Badge key={capability} variant="secondary">
            {t("priceCatalog." + capability)}
          </Badge>
        ))}
      </div>
      {(model.max_input_tokens || model.max_output_tokens) && (
        <div className="mt-4 flex flex-wrap gap-x-5 gap-y-1 text-xs text-muted-foreground">
          {model.max_input_tokens != null && (
            <span>
              {t("priceCatalog.maxInput")}{" "}
              <strong className="font-medium text-foreground">{model.max_input_tokens.toLocaleString()} tokens</strong>
            </span>
          )}
          {model.max_output_tokens != null && (
            <span>
              {t("priceCatalog.maxOutput")}{" "}
              <strong className="font-medium text-foreground">{model.max_output_tokens.toLocaleString()} tokens</strong>
            </span>
          )}
        </div>
      )}
      <dl className="mt-5 grid grid-cols-2 gap-3 border-t border-border pt-4">
        {[
          { label: t("Input"), value: model.input_price },
          { label: t("Output"), value: model.output_price },
        ].map(({ label, value }) => (
          <div key={label} className="min-w-0">
            <dt className="text-xs text-muted-foreground">{label}</dt>
            <dd className="mt-1 flex flex-wrap items-baseline gap-1.5">
              <span className={value == null ? "text-sm text-muted-foreground" : "text-xl font-semibold sm:text-2xl"}>
                {priceText(value)}
              </span>
              {value != null && <span className="text-xs text-muted-foreground">/ 1M tokens</span>}
            </dd>
          </div>
        ))}
      </dl>
      {(model.cache_read_price != null || model.cache_write_price != null) && (
        <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
          {model.cache_read_price != null && (
            <span>
              {t("Cache Read")} {priceText(model.cache_read_price)}
            </span>
          )}
          {model.cache_write_price != null && (
            <span>
              {t("Cache Write")} {priceText(model.cache_write_price)}
            </span>
          )}
        </div>
      )}
      <div className="mt-auto pt-5">
        <Link href={uiHref("playground")} className={cn(buttonVariants({ variant: "outline" }), "w-full")}>
          {t("myModels.useInPlayground")} <ArrowRight aria-hidden="true" />
        </Link>
      </div>
    </article>
  );
}

export function MyModels({
  models,
  isLoading = false,
  isError = false,
  onRetry,
}: {
  models: MyModelCard[];
  isLoading?: boolean;
  isError?: boolean;
  onRetry?: () => void;
}) {
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("all");
  const [provider, setProvider] = useState("all");
  const [sort, setSort] = useState<SortOrder>("name");
  const [showFilters, setShowFilters] = useState(false);
  const categories = useMemo(
    () => ["all", ...Array.from(new Set(models.map((model) => model.category))).sort()],
    [models],
  );
  const providers = useMemo(
    () =>
      Array.from(
        new Set(models.map((model) => model.provider).filter((value): value is string => Boolean(value))),
      ).sort(),
    [models],
  );
  const activeFilters = Number(category !== "all") + Number(provider !== "all");
  const visible = useMemo(() => {
    const search = query.trim().toLowerCase();
    return models
      .filter((model) => {
        if (category !== "all" && model.category !== category) return false;
        if (provider !== "all" && (model.provider ?? "unknown") !== provider) return false;
        return (
          !search ||
          [model.id, model.provider ?? "", model.category, ...model.capabilities]
            .join(" ")
            .toLowerCase()
            .includes(search)
        );
      })
      .sort((a, b) => {
        if (sort === "input")
          return (a.input_price ?? Infinity) - (b.input_price ?? Infinity) || a.id.localeCompare(b.id);
        if (sort === "output")
          return (a.output_price ?? Infinity) - (b.output_price ?? Infinity) || a.id.localeCompare(b.id);
        return a.id.localeCompare(b.id);
      });
  }, [category, models, provider, query, sort]);

  function resetFilters() {
    setQuery("");
    setCategory("all");
    setProvider("all");
  }

  return (
    <div className="min-h-full bg-background text-foreground">
      <div className="mx-auto max-w-7xl px-4 py-8 sm:px-8 sm:py-12 lg:px-10">
        <header className="mb-9 max-w-2xl">
          <p className="mb-4 flex items-center gap-2 text-xs font-semibold tracking-[0.18em] text-cyan-700 dark:text-cyan-300">
            <span className="size-1.5 rounded-full bg-cyan-500" aria-hidden="true" />
            {t("myModels.eyebrow")}
          </p>
          <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl lg:text-5xl">{t("myModels.title")}</h1>
          <p className="mt-4 text-sm leading-7 text-muted-foreground sm:text-base">{t("myModels.description")}</p>
        </header>
        <section aria-label={t("myModels.title")}>
          <div className="flex flex-col gap-3 rounded-lg border border-border bg-card p-3 sm:flex-row">
            <div className="relative min-w-0 flex-1">
              <Search
                aria-hidden="true"
                className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
              />
              <Input
                aria-label={t("Search")}
                placeholder={t("myModels.search")}
                className="h-11 border-0 bg-transparent pl-10 shadow-none"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
              />
            </div>
            <label className="flex h-11 items-center gap-2 rounded-lg border border-input bg-background px-3 text-sm sm:w-56">
              <ArrowDownUp aria-hidden="true" className="size-4 text-muted-foreground" />
              <select
                aria-label={t("myModels.sort")}
                className="min-w-0 flex-1 bg-background outline-none focus-visible:ring-2 focus-visible:ring-ring"
                value={sort}
                onChange={(event) => setSort(event.target.value as SortOrder)}
              >
                <option value="name">{t("myModels.sortName")}</option>
                <option value="input">{t("myModels.sortInput")}</option>
                <option value="output">{t("myModels.sortOutput")}</option>
              </select>
            </label>
            <Button
              type="button"
              variant="outline"
              className="h-11 px-4"
              aria-expanded={showFilters}
              aria-controls="my-model-filters"
              onClick={() => setShowFilters(!showFilters)}
            >
              <SlidersHorizontal aria-hidden="true" />
              {t("myModels.filters")}
              {activeFilters > 0 && <Badge variant="secondary">{activeFilters}</Badge>}
            </Button>
          </div>
          {showFilters && (
            <div id="my-model-filters" className="mt-3 space-y-5 rounded-lg border border-border bg-card p-5">
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
                      onClick={() => setCategory(item)}
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
                  onChange={(event) => setProvider(event.target.value)}
                >
                  <option value="all">{t("myModels.allProviders")}</option>
                  {providers.map((item) => (
                    <option key={item} value={item}>
                      {item}
                    </option>
                  ))}
                  {models.some((model) => model.provider == null) && (
                    <option value="unknown">{t("myModels.unknownProvider")}</option>
                  )}
                </select>
              </label>
            </div>
          )}
          <div className="my-5 flex flex-wrap items-center justify-between gap-3 text-sm">
            <p role="status" className="text-muted-foreground">
              {t("myModels.results", { count: visible.length, total: models.length })}
            </p>
            {(query || activeFilters > 0) && (
              <Button type="button" size="sm" variant="ghost" onClick={resetFilters}>
                {t("myModels.reset")}
              </Button>
            )}
          </div>
          <p className="mb-5 border-l-2 border-cyan-500/40 pl-3 text-xs leading-relaxed text-muted-foreground">
            {t("myModels.units")}
          </p>
          {isError && (
            <div
              role="alert"
              className="mb-5 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/30 p-5 text-sm"
            >
              <span>{t("myModels.loadError")}</span>
              <Button variant="outline" onClick={onRetry}>
                {t("myModels.retry")}
              </Button>
            </div>
          )}
          {isLoading && (
            <div role="status" className="rounded-lg border border-border p-12 text-center text-muted-foreground">
              {t("myModels.loading")}
            </div>
          )}
          {!isLoading && !isError && visible.length > 0 && (
            <div className="grid items-stretch gap-4 md:grid-cols-2">
              {visible.map((model) => (
                <ModelCard key={model.id} model={model} />
              ))}
            </div>
          )}
          {!isLoading && !isError && visible.length === 0 && (
            <div className="rounded-lg border border-dashed border-border p-12 text-center">
              <Search aria-hidden="true" className="mx-auto mb-4 size-7 text-muted-foreground" />
              <p className="font-medium">{t(models.length ? "myModels.noMatches" : "myModels.empty")}</p>
              <p className="mt-2 text-sm text-muted-foreground">
                {t(models.length ? "myModels.tryFilters" : "myModels.emptyHint")}
              </p>
            </div>
          )}
        </section>
      </div>
    </div>
  );
}

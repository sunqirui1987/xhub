"use client";

import { useState } from "react";
import { Pencil, Plus, RotateCcw, Trash2 } from "lucide-react";
import { t } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import type { PriceProviderRow } from "./priceCatalogRows";

/**
 * The suppliers a model can be filed under. These are the same entries the
 * add-model form offers, so a supplier added here is immediately selectable
 * there.
 */
export function PriceProviderPanel({
  providers,
  onAdd,
  onEdit,
  onDelete,
  onReset,
}: {
  providers: PriceProviderRow[];
  onAdd?: () => void;
  onEdit?: (provider: PriceProviderRow) => void;
  onDelete?: (provider: PriceProviderRow) => void;
  onReset?: (provider: PriceProviderRow) => void;
}) {
  const [query, setQuery] = useState("");
  const search = query.trim().toLowerCase();
  const visible = providers.filter(
    (provider) => !search || [provider.slug, provider.name].join(" ").toLowerCase().includes(search),
  );

  return (
    <section aria-label={t("priceData.providers")} className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-sm text-muted-foreground" role="status">
          {t("priceData.providerCount", { count: visible.length, total: providers.length })}
        </p>
        <div className="flex gap-2">
          <input
            aria-label={t("Search")}
            className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm sm:w-56"
            placeholder={t("priceData.searchProviders")}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          {onAdd && (
            <Button onClick={onAdd} data-testid="price-add-provider">
              <Plus aria-hidden="true" />
              {t("priceData.addProvider")}
            </Button>
          )}
        </div>
      </div>

      <div className="grid items-start gap-4 md:grid-cols-2">
        {visible.map((provider) => (
          <Card key={provider.slug} className="min-w-0 p-5">
            <div className="flex items-start gap-3">
              <div className="min-w-0 flex-1">
                <h3 className="break-all text-base font-semibold tracking-tight">{provider.name}</h3>
                <p className="mt-1 break-all font-mono text-xs text-muted-foreground">{provider.slug}</p>
              </div>
              <div className="flex shrink-0 gap-1">
                {onReset && provider.overridden && (
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={t("priceData.resetProvider")}
                    title={t("priceData.resetProvider")}
                    onClick={() => onReset(provider)}
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
                    onClick={() => onEdit(provider)}
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
                    onClick={() => onDelete(provider)}
                  >
                    <Trash2 aria-hidden="true" />
                  </Button>
                )}
              </div>
            </div>
            <div className="mt-3 flex flex-wrap gap-2">
              {provider.overridden && <Badge variant="secondary">{t("priceData.edited")}</Badge>}
              <Badge variant="outline">{t("priceData.modelCount", { count: provider.modelCount })}</Badge>
            </div>
            {provider.apiBase && (
              <p className="mt-3 break-all text-xs text-muted-foreground">{provider.apiBase}</p>
            )}
          </Card>
        ))}
      </div>

      {visible.length === 0 && (
        <div className="rounded-xl border border-dashed border-border p-12 text-center text-sm text-muted-foreground">
          {t("priceData.noProviders")}
        </div>
      )}
    </section>
  );
}

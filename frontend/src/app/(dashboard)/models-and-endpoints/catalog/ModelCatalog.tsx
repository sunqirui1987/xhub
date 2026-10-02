"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { getProxyBaseUrl } from "@/components/networking";
import { Button } from "@/components/ui/button";
import { t } from "@/i18n";
import { uiHref } from "@/utils/uiHref";

export type CatalogCard = {
  id: string;
  provider: "fennoai" | "qiniu";
  category: string;
  input_price: number | null;
  output_price: number | null;
  added: boolean;
};

const PROVIDERS = ["fennoai", "qiniu"] as const;

export function priceLabel(value: number | null): string {
  if (value == null || Number.isNaN(value)) return t("Price unavailable");
  const rounded = Math.round(value * 1_000_000) / 1_000_000;
  const text = rounded.toFixed(6).replace(/\.?0+$/, "");
  return `$${text}`;
}

export function ModelCatalog({
  provider,
  accessToken,
}: {
  provider: "fennoai" | "qiniu";
  accessToken: string;
}) {
  const [models, setModels] = useState<CatalogCard[]>([]);
  const [category, setCategory] = useState("all");
  const [providerFilter, setProviderFilter] = useState<"all" | "fennoai" | "qiniu">(provider);

  const load = async () => {
    const lists = await Promise.all(
      PROVIDERS.map(async (name) => {
        const response = await fetch(`${getProxyBaseUrl()}/model/builtin/models`, {
          method: "POST",
          headers: {
            Authorization: `Bearer ${accessToken}`,
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ provider: name, credential_name: name }),
        });
        if (!response.ok) return [];
        const body = (await response.json()) as { models?: Omit<CatalogCard, "provider">[] };
        return (body.models ?? []).map((model) => ({ ...model, provider: name }));
      }),
    );
    setModels(lists.flat());
  };

  useEffect(() => {
    void load();
  }, [accessToken]);

  const categories = useMemo(() => ["all", ...Array.from(new Set(models.map((model) => model.category)))], [models]);
  const visible = models.filter((model) => {
    if (providerFilter !== "all" && model.provider !== providerFilter) return false;
    if (category !== "all" && model.category !== category) return false;
    return true;
  });

  const addOne = async (model: CatalogCard) => {
    const response = await fetch(`${getProxyBaseUrl()}/model/builtin/add`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${accessToken}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        provider: model.provider,
        model_ids: [model.id],
        credential_name: model.provider,
        models: [
          {
            id: model.id,
            category: model.category,
            input_price: model.input_price,
            output_price: model.output_price,
          },
        ],
      }),
    });
    if (!response.ok) return;
    await load();
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">{t("Model catalog")}</h1>
          <p className="text-sm text-muted-foreground">{provider}</p>
        </div>
        <Link className="text-sm text-primary" href={uiHref("mine-models")}>
          {t("My models")}
        </Link>
      </div>
      <div className="flex flex-wrap gap-2" role="group" aria-label={t("Provider")}>
        {(["all", ...PROVIDERS] as const).map((item) => (
          <button
            key={item}
            type="button"
            aria-pressed={providerFilter === item}
            className={providerFilter === item ? "rounded-full bg-foreground px-3 py-1 text-sm text-background" : "rounded-full border px-3 py-1 text-sm"}
            onClick={() => setProviderFilter(item)}
          >
            {item === "all" ? t("All") : item}
          </button>
        ))}
      </div>
      <div className="flex flex-wrap gap-2" role="group" aria-label={t("Category")}>
        {categories.map((item) => (
          <button
            key={item || "uncategorized"}
            type="button"
            aria-pressed={category === item}
            className={category === item ? "rounded-full bg-foreground px-3 py-1 text-sm text-background" : "rounded-full border px-3 py-1 text-sm"}
            onClick={() => setCategory(item)}
          >
            {item === "all" ? t("All") : item}
          </button>
        ))}
      </div>
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {visible.map((model) => (
          <article key={`${model.provider}/${model.id}`} data-testid={`catalog-card-${model.provider}-${model.id}`} className="flex min-h-40 flex-col justify-between rounded-xl border bg-card p-4 shadow-sm">
            <div>
              <h2 className="text-lg font-medium">{model.id}</h2>
              <p className="text-xs text-muted-foreground">{model.category}</p>
              <p className="mt-3 text-sm">{t("Input")} {priceLabel(model.input_price)}</p>
              <p className="text-sm">{t("Output")} {priceLabel(model.output_price)}</p>
            </div>
            {model.added ? (
              <span className="mt-4 text-sm text-muted-foreground">{t("Already added")}</span>
            ) : (
              <Button className="mt-4" size="sm" onClick={() => void addOne(model)}>
                {t("Add this model")}
              </Button>
            )}
          </article>
        ))}
      </div>
    </div>
  );
}

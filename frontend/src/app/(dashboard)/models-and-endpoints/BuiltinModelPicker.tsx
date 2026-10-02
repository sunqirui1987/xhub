"use client";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { getProxyBaseUrl } from "@/components/networking";
import { t } from "@/i18n";

type ProviderID = "fennoai" | "qiniu";

type ListedModel = { id: string; added: boolean };

type ProviderList = {
  provider: ProviderID;
  api_base: string;
  models: ListedModel[];
  error?: string;
};

const PROVIDERS: { id: ProviderID; title: string; fetchLabel: string }[] = [
  { id: "fennoai", title: "fennoai", fetchLabel: "Fetch fennoai models" },
  { id: "qiniu", title: "qiniu", fetchLabel: "Fetch qiniu models" },
];

export function selectedNewIds(models: ListedModel[], checked: Record<string, boolean>): string[] {
  return models.filter((model) => !model.added && checked[model.id]).map((model) => model.id);
}

export function BuiltinModelPicker({
  accessToken,
  onAdded,
}: {
  accessToken: string;
  onAdded: () => void;
}) {
  const [lists, setLists] = useState<ProviderList[]>([]);
  const [checked, setChecked] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState<ProviderID | null>(null);

  const load = async (provider: ProviderID) => {
    const response = await fetch(`${getProxyBaseUrl()}/model/builtin/models`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${accessToken}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ provider }),
    });
    if (!response.ok) {
      return { provider, api_base: "", models: [], error: t("Fetch failed") };
    }
    const body = (await response.json()) as { api_base?: string; models?: ListedModel[] };
    return { provider, api_base: body.api_base ?? "", models: body.models ?? [] };
  };

  useEffect(() => {
    let cancelled = false;
    void Promise.all(PROVIDERS.map((item) => load(item.id))).then((next) => {
      if (!cancelled) setLists(next);
    });
    return () => {
      cancelled = true;
    };
  }, [accessToken]);

  const refreshProvider = async (provider: ProviderID) => {
    setBusy(provider);
    const response = await fetch(`${getProxyBaseUrl()}/model/builtin/refresh`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${accessToken}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ provider }),
    });
    setBusy(null);
    if (!response.ok) return;
    const fresh = await load(provider);
    setLists((current) => {
      const rest = current.filter((item) => item.provider !== provider);
      return [...rest, fresh];
    });
    onAdded();
  };

  const addSelected = async (provider: ProviderID) => {
    const list = lists.find((item) => item.provider === provider);
    const modelIDs = selectedNewIds(list?.models ?? [], checked);
    if (modelIDs.length === 0) return;
    setBusy(provider);
    const response = await fetch(`${getProxyBaseUrl()}/model/builtin/add`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${accessToken}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ provider, model_ids: modelIDs }),
    });
    setBusy(null);
    if (!response.ok) return;
    const fresh = await load(provider);
    setLists((current) => current.map((item) => (item.provider === provider ? fresh : item)));
    setChecked({});
    onAdded();
  };

  return (
    <div className="mb-4 grid gap-3 md:grid-cols-2">
      {PROVIDERS.map((meta) => {
        const list = lists.find((item) => item.provider === meta.id) ?? {
          provider: meta.id,
          api_base: "",
          models: [],
        };
        return (
        <section key={list.provider} className="rounded-md border p-3">
          <div className="mb-2 flex items-center justify-between gap-2">
            <div>
              <h3 className="text-sm font-medium">{meta.title}</h3>
              <p className="text-xs text-muted-foreground">{list.api_base}</p>
            </div>
            <div className="flex gap-2">
              <Button size="sm" variant="outline" disabled={busy === list.provider} onClick={() => void refreshProvider(list.provider)}>
                {t(meta.fetchLabel)}
              </Button>
              <Button size="sm" disabled={busy === list.provider} onClick={() => void addSelected(list.provider)}>
                {t("Add selected models")}
              </Button>
            </div>
          </div>
          {list.error ? <p className="text-xs text-destructive">{list.error}</p> : null}
          <ul className="max-h-64 space-y-1 overflow-auto">
            {list.models.map((model) => (
              <li key={model.id} className={model.added ? "flex items-center gap-2 text-sm text-muted-foreground opacity-60" : "flex items-center gap-2 text-sm"}>
                <Checkbox
                  checked={model.added || Boolean(checked[model.id])}
                  disabled={model.added}
                  aria-label={model.id}
                  onCheckedChange={(value) =>
                    setChecked((current) => ({ ...current, [model.id]: value === true }))
                  }
                />
                <span>{model.id}</span>
                {model.added ? <span className="text-xs">{t("Already added")}</span> : null}
              </li>
            ))}
          </ul>
        </section>
        );
      })}
    </div>
  );
}

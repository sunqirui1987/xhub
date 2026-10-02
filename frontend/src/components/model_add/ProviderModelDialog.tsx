"use client";

import { useEffect, useMemo, useState } from "react";
import { CredentialItem, getProxyBaseUrl } from "@/components/networking";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { priceLabel } from "@/app/(dashboard)/models-and-endpoints/catalog/ModelCatalog";
import { t } from "@/i18n";

type ListedModel = {
  id: string;
  added: boolean;
  category?: string;
  input_price?: number | null;
  output_price?: number | null;
};

export function ProviderModelDialog({
  provider,
  initialCredentialName,
  credentials,
  accessToken,
  onClose,
  onAdded,
}: {
  provider: "fennoai" | "qiniu";
  initialCredentialName?: string;
  credentials: CredentialItem[];
  accessToken: string;
  onClose: () => void;
  onAdded: () => void;
}) {
  const [models, setModels] = useState<ListedModel[]>([]);
  const [picked, setPicked] = useState<string[]>([]);
  const [category, setCategory] = useState("all");
  const [credentialName, setCredentialName] = useState<string>(initialCredentialName ?? provider);
  const matching = credentials.filter(
    (item) => item.credential_info?.builtin === provider || item.credential_name === provider,
  );
  const options = matching.length > 0 ? matching : [{ credential_name: provider } as CredentialItem];

  const load = async (name: string) => {
    const response = await fetch(`${getProxyBaseUrl()}/model/builtin/models`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${accessToken}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ provider, credential_name: name }),
    });
    if (!response.ok) return;
    const body = (await response.json()) as { models?: ListedModel[] };
    setModels(body.models ?? []);
    setPicked([]);
  };

  useEffect(() => {
    void load(credentialName);
  }, [provider, accessToken, credentialName]);

  const categories = useMemo(
    () => ["all", ...Array.from(new Set(models.map((model) => model.category).filter((item): item is string => Boolean(item))))],
    [models],
  );
  const visible = category === "all" ? models : models.filter((model) => model.category === category);

  const toggle = (model: ListedModel) => {
    if (model.added) return;
    setPicked((current) => (current.includes(model.id) ? current.filter((id) => id !== model.id) : [...current, model.id]));
  };

  const addSelected = async () => {
    const chosen = models.filter((model) => !model.added && picked.includes(model.id));
    if (chosen.length === 0) return;
    const response = await fetch(`${getProxyBaseUrl()}/model/builtin/add`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${accessToken}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        provider,
        model_ids: chosen.map((model) => model.id),
        credential_name: credentialName,
        models: chosen.map((model) => ({
          id: model.id,
          category: model.category,
          input_price: model.input_price,
          output_price: model.output_price,
        })),
      }),
    });
    if (!response.ok) return;
    await load(credentialName);
    onAdded();
  };

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-xl" aria-label={t("Fetch model list")}>
        <DialogHeader>
          <DialogTitle>{t("Fetch model list")}</DialogTitle>
        </DialogHeader>
        <label className="flex items-center gap-2 text-sm">
          {t("Credential")}
          <select
            aria-label={t("Credential")}
            className="rounded-md border bg-background px-2 py-1"
            value={credentialName}
            onChange={(event) => setCredentialName(event.target.value)}
          >
            {options.map((item) => (
              <option key={item.credential_name} value={item.credential_name}>
                {item.credential_name}
              </option>
            ))}
          </select>
        </label>
        <div className="flex flex-wrap gap-2" role="group" aria-label={t("Category")}>
          {categories.map((item) => (
            <button
              key={item}
              type="button"
              aria-pressed={category === item}
              className={category === item ? "rounded-full bg-foreground px-3 py-1 text-sm text-background" : "rounded-full border px-3 py-1 text-sm"}
              onClick={() => setCategory(item)}
            >
              {item === "all" ? t("All") : item}
            </button>
          ))}
        </div>
        <ul className="max-h-80 space-y-1 overflow-auto">
          {visible.map((model) => (
            <li key={model.id}>
              <label className={model.added ? "flex items-center gap-2 text-sm text-muted-foreground opacity-60" : "flex items-center gap-2 text-sm"}>
                <input
                  type="checkbox"
                  aria-label={model.id}
                  disabled={model.added}
                  checked={!model.added && picked.includes(model.id)}
                  onChange={() => toggle(model)}
                />
                <span className="min-w-0 flex-1">
                  <span className="font-medium">{model.id}</span>
                  <span className="ml-2 text-xs text-muted-foreground">
                    {model.category ?? ""} {t("Input")} {priceLabel(model.input_price ?? null)} {t("Output")} {priceLabel(model.output_price ?? null)}
                  </span>
                </span>
                {model.added ? <span className="text-xs">{t("Already added")}</span> : null}
              </label>
            </li>
          ))}
        </ul>
        <Button type="button" disabled={picked.length === 0} onClick={() => void addSelected()}>
          {t("Add selected models")}
        </Button>
      </DialogContent>
    </Dialog>
  );
}

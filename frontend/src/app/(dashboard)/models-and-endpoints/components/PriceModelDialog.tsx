"use client";

import { useMemo, useState } from "react";
import { Plus } from "lucide-react";
import { t } from "@/i18n";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  EDITABLE_TOKEN_RATES,
  EDITABLE_UNIT_RATES,
  catalogFieldValue,
  invalidRateFields,
  priceModelPayload,
  rateToInputValue,
  unitRateToInputValue,
  type PriceCatalogRow,
  type PriceProviderRow,
} from "./priceCatalogRows";

/** The raw row a form field is initialized from, keyed by the API field name. */
export type PriceModelDraft = Record<string, unknown>;

/**
 * Adds a model to the price catalog, or edits one that is already there. Prices
 * are entered per 1M tokens and converted to per-token on save, because the
 * gateway bills per token.
 *
 * The caller remounts this on the row it is editing, so the fields below start
 * from that row and a reopened dialog never carries the last one's values.
 */
export function PriceModelDialog({
  open,
  onOpenChange,
  providers,
  editing,
  onSave,
  isSaving,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providers: PriceProviderRow[];
  /** The row being edited, or null when adding a model. */
  editing: PriceCatalogRow | null;
  onSave: (body: Record<string, unknown>) => void;
  isSaving?: boolean;
}) {
  const draft: PriceModelDraft = editing?.raw ?? {};
  const [id, setId] = useState(editing?.id ?? "");
  const [provider, setProvider] = useState(editing?.provider ?? providers[0]?.slug ?? "");
  const [displayName, setDisplayName] = useState(editing?.displayName ?? "");
  const [mode, setMode] = useState(typeof draft.mode === "string" ? draft.mode : "");
  const [endpointType, setEndpointType] = useState(typeof draft.endpoint_id === "string" ? draft.endpoint_id : "");
  const [tokenRates, setTokenRates] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      EDITABLE_TOKEN_RATES.map(({ field }) => [field, rateToInputValue(catalogFieldValue(draft, field))]),
    ),
  );
  const [unitRates, setUnitRates] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      EDITABLE_UNIT_RATES.map(({ field }) => [field, unitRateToInputValue(catalogFieldValue(draft, field))]),
    ),
  );

  const isEdit = Boolean(editing);
  const invalidRates = useMemo(
    () => new Set([...invalidRateFields(tokenRates), ...invalidRateFields(unitRates)]),
    [tokenRates, unitRates],
  );
  const canSave = id.trim() !== "" && provider.trim() !== "" && invalidRates.size === 0 && !isSaving;
  const knownProviders = useMemo(() => providers.map((p) => p.slug), [providers]);

  function submit() {
    if (!canSave) return;
    onSave(
      priceModelPayload({
        id,
        provider,
        displayName,
        mode,
        endpointType,
        tokenRates,
        unitRates,
        original: editing?.raw,
      }),
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{isEdit ? t("priceData.editModel") : t("priceData.addModel")}</DialogTitle>
          <DialogDescription>{t("priceData.modelDialogHint")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="price-model-id">{t("priceData.modelId")}</Label>
              <Input
                id="price-model-id"
                value={id}
                readOnly={isEdit}
                placeholder="claude-4.1-opus"
                onChange={(event) => setId(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="price-model-provider">{t("Provider")}</Label>
              <Input
                id="price-model-provider"
                list="price-model-providers"
                value={provider}
                onChange={(event) => setProvider(event.target.value)}
              />
              <datalist id="price-model-providers">
                {knownProviders.map((slug) => (
                  <option key={slug} value={slug} />
                ))}
              </datalist>
            </div>
            <div className="space-y-2">
              <Label htmlFor="price-model-display">{t("priceData.displayName")}</Label>
              <Input
                id="price-model-display"
                value={displayName}
                onChange={(event) => setDisplayName(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="price-model-mode">{t("priceData.endpointMode")}</Label>
              <Input
                id="price-model-mode"
                value={mode}
                placeholder="chat"
                onChange={(event) => setMode(event.target.value)}
              />
            </div>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="price-model-endpoint">{t("priceData.endpointType")}</Label>
              <Input
                id="price-model-endpoint"
                value={endpointType}
                placeholder="ark_contents_generation"
                onChange={(event) => setEndpointType(event.target.value)}
              />
              <p className="text-xs text-muted-foreground">{t("priceData.endpointTypeHint")}</p>
            </div>
          </div>

          <fieldset className="space-y-3 rounded-xl border border-border p-4">
            <legend className="px-1 text-sm font-medium">{t("priceData.tokenRates")}</legend>
            <p className="text-xs text-muted-foreground">{t("priceData.tokenRatesHint")}</p>
            <div className="grid gap-4 sm:grid-cols-2">
              {EDITABLE_TOKEN_RATES.map(({ field, labelKey }) => (
                <div key={field} className="space-y-2">
                  <Label htmlFor={`price-rate-${field}`}>{t(labelKey)}</Label>
                  <Input
                    id={`price-rate-${field}`}
                    inputMode="decimal"
                    value={tokenRates[field] ?? ""}
                    aria-invalid={invalidRates.has(field)}
                    onChange={(event) => setTokenRates((prev) => ({ ...prev, [field]: event.target.value }))}
                  />
                  {invalidRates.has(field) && <p className="text-xs text-destructive">{t("priceData.invalidRate")}</p>}
                </div>
              ))}
            </div>
          </fieldset>

          <fieldset className="space-y-3 rounded-xl border border-border p-4">
            <legend className="px-1 text-sm font-medium">{t("priceData.unitRates")}</legend>
            <p className="text-xs text-muted-foreground">{t("priceData.unitRatesHint")}</p>
            <div className="grid gap-4 sm:grid-cols-2">
              {EDITABLE_UNIT_RATES.map(({ field, labelKey }) => (
                <div key={field} className="space-y-2">
                  <Label htmlFor={`price-rate-${field}`}>{t(labelKey)}</Label>
                  <Input
                    id={`price-rate-${field}`}
                    inputMode="decimal"
                    value={unitRates[field] ?? ""}
                    aria-invalid={invalidRates.has(field)}
                    onChange={(event) => setUnitRates((prev) => ({ ...prev, [field]: event.target.value }))}
                  />
                  {invalidRates.has(field) && <p className="text-xs text-destructive">{t("priceData.invalidRate")}</p>}
                </div>
              ))}
            </div>
          </fieldset>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("Cancel")}
          </Button>
          <Button onClick={submit} disabled={!canSave} data-testid="price-model-save">
            <Plus aria-hidden="true" />
            {isEdit ? t("Save") : t("priceData.addModel")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

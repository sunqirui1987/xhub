"use client";

import { useState } from "react";
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
import type { PriceProviderRow } from "./priceCatalogRows";

/**
 * Adds a supplier to the add-model dropdown, or edits one. The slug is the
 * litellm_provider value deployments match on, so it is fixed once saved.
 *
 * The caller remounts this on the record it is editing, so the fields below
 * start from that record.
 */
export function PriceProviderDialog({
  open,
  onOpenChange,
  editing,
  onSave,
  isSaving,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  editing: PriceProviderRow | null;
  onSave: (body: Record<string, unknown>) => void;
  isSaving?: boolean;
}) {
  const [slug, setSlug] = useState(editing?.slug ?? "");
  const [name, setName] = useState(editing?.name ?? "");
  const [apiBase, setApiBase] = useState(editing?.apiBase ?? "");

  const isEdit = Boolean(editing);
  const canSave = slug.trim() !== "" && !isSaving;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEdit ? t("priceData.editProvider") : t("priceData.addProvider")}</DialogTitle>
          <DialogDescription>{t("priceData.providerDialogHint")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="price-provider-slug">{t("priceData.providerSlug")}</Label>
            <Input
              id="price-provider-slug"
              value={slug}
              readOnly={isEdit}
              placeholder="acme"
              onChange={(event) => setSlug(event.target.value)}
            />
            <p className="text-xs text-muted-foreground">{t("priceData.providerSlugHint")}</p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="price-provider-name">{t("priceData.providerName")}</Label>
            <Input id="price-provider-name" value={name} onChange={(event) => setName(event.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="price-provider-base">{t("priceData.apiBase")}</Label>
            <Input
              id="price-provider-base"
              value={apiBase}
              placeholder="https://api.example.com"
              onChange={(event) => setApiBase(event.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("Cancel")}
          </Button>
          <Button
            data-testid="price-provider-save"
            disabled={!canSave}
            onClick={() => {
              if (!canSave) return;
              onSave({
                litellm_provider: slug.trim(),
                provider_display_name: name.trim() || slug.trim(),
                provider: name.trim() || slug.trim(),
                default_api_base: apiBase.trim(),
              });
            }}
          >
            <Plus aria-hidden="true" />
            {isEdit ? t("Save") : t("priceData.addProvider")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

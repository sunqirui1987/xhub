"use client";

import React, { useEffect, useState } from "react";
import { t } from "@/i18n";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  getRouteTemplateBindingCall,
  getRouteTemplatesCall,
  type EffectiveRouteTemplate,
} from "@/components/networking";

const INHERIT = "inherit";

const scopeName = (scope: EffectiveRouteTemplate["scope_type"]) => {
  if (scope === "organization") return t("pages.routeTemplates.scopeOrganization");
  if (scope === "team") return t("pages.routeTemplates.scopeTeam");
  if (scope === "key") return t("pages.routeTemplates.scopeKey");
  return "";
};

/**
 * Picks one named template, or inherits.
 *
 * The dropdown is the choice. The line under it is what this scope will
 * actually route by, and which level that came from. A team that selected
 * nothing and a team whose organization selected a template look the same
 * without that line.
 */
const RouteTemplateSelect: React.FC<{
  accessToken: string | null;
  value: string;
  onChange: (templateId: string) => void;
  scope?: "organization" | "team" | "key";
  scopeId?: string;
  disabled?: boolean;
}> = ({ accessToken, value, onChange, scope, scopeId, disabled }) => {
  const [options, setOptions] = useState<Array<{ id: string; name: string }>>([]);
  const [optionsLoaded, setOptionsLoaded] = useState(false);
  const [effective, setEffective] = useState<EffectiveRouteTemplate | null>(null);
  const [inherited, setInherited] = useState<EffectiveRouteTemplate | null>(null);

  useEffect(() => {
    setOptions([]);
    setOptionsLoaded(false);
    if (!accessToken) return;
    let cancelled = false;
    void getRouteTemplatesCall(accessToken)
      .then((rows) => {
        if (cancelled) return;
        setOptions(rows.map((row) => ({ id: row.id, name: row.name })));
        setOptionsLoaded(true);
      })
      .catch(() => {
        if (!cancelled) setOptionsLoaded(true);
      });
    return () => {
      cancelled = true;
    };
  }, [accessToken]);

  useEffect(() => {
    setEffective(null);
    setInherited(null);
    if (!accessToken || !scope || !scopeId) return;
    let cancelled = false;
    void getRouteTemplateBindingCall(accessToken, scope, scopeId)
      .then((binding) => {
        if (cancelled) return;
        setEffective(binding.effective);
        setInherited(binding.inherited ?? (!binding.route_template_id ? binding.effective : null));
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [accessToken, scope, scopeId, value]);

  const selected = value || INHERIT;
  const selectedOption = options.find((option) => option.id === value);
  const selectedLabel =
    selected === INHERIT
      ? t("pages.routeTemplates.notApplied")
      : selectedOption?.name ||
        (effective?.template_id === value ? effective.name : undefined) ||
        t(optionsLoaded ? "pages.routeTemplates.unknownTemplate" : "pages.routeTemplates.loadingTemplates");
  let status = t("pages.routeTemplates.inheritHint");
  const preview = value
    ? { template_id: value, name: selectedOption?.name ?? (effective?.template_id === value ? effective.name : undefined), scope_type: scope ?? "key" }
    : inherited;
  if (preview) {
    status =
      preview.scope_type === "platform"
        ? t("pages.routeTemplates.effectivePlatform")
        : preview.name
          ? t("pages.routeTemplates.effectiveFrom", { name: preview.name, scope: scopeName(preview.scope_type) })
          : t("pages.routeTemplates.inheritHint");
  }

  return (
    <div className="space-y-1">
      <Select
        value={selected}
        disabled={disabled}
        onValueChange={(next) => {
          if (!next) return;
          onChange(next === INHERIT ? "" : next);
        }}
      >
        <SelectTrigger className="w-full" aria-label={t("pages.routeTemplates.title")}>
          <SelectValue>{selectedLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={INHERIT}>{t("pages.routeTemplates.notApplied")}</SelectItem>
          {options.map((option) => (
            <SelectItem key={option.id} value={option.id}>
              {option.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{status}</p>
    </div>
  );
};

export default RouteTemplateSelect;

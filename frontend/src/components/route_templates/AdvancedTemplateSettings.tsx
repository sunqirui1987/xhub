"use client";

import React, { useMemo, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { t } from "@/i18n";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import ModelFallbackEditor from "./ModelFallbackEditor";
import TemplateRoutingGroups from "./TemplateRoutingGroups";
import { FALLBACK_CAUSES, type TemplateFormState } from "./templateForm";

export type AdvancedTemplateSettingsProps = {
  form: TemplateFormState;
  modelNames: string[];
  availableStrategies: string[];
  onChange: (patch: Partial<TemplateFormState>) => void;
};

type ModuleId =
  | "routingGroups"
  | "failureFallbacks"
  | "contextFallbacks"
  | "policyFallbacks"
  | "defaultFallbacks"
  | "maxFallbacks"
  | "fallbackCauses"
  | "streamTimeout"
  | "retryAfter"
  | "tagFiltering"
  | "preCallChecks";

const MODULES: ModuleId[] = [
  "routingGroups",
  "failureFallbacks",
  "contextFallbacks",
  "policyFallbacks",
  "defaultFallbacks",
  "maxFallbacks",
  "fallbackCauses",
  "streamTimeout",
  "retryAfter",
  "tagFiltering",
  "preCallChecks",
];

const moduleLabel = (id: ModuleId) => t(`pages.routeTemplates.advanced.modules.${id}`);

const configuredModules = (form: TemplateFormState): Set<ModuleId> => {
  const configured = new Set<ModuleId>();
  if (form.routing_groups.trim()) configured.add("routingGroups");
  if (form.fallbacks.length) configured.add("failureFallbacks");
  if (form.context_window_fallbacks.length) configured.add("contextFallbacks");
  if (form.content_policy_fallbacks.length) configured.add("policyFallbacks");
  if (form.default_fallbacks.trim()) configured.add("defaultFallbacks");
  if (form.max_fallbacks.trim() !== "" && form.max_fallbacks.trim() !== "5") {
    configured.add("maxFallbacks");
  }
  if (FALLBACK_CAUSES.some((cause) => !form.fallback_causes.includes(cause))) configured.add("fallbackCauses");
  if (form.stream_timeout.trim()) configured.add("streamTimeout");
  if (form.retry_after.trim() && form.retry_after !== "0") configured.add("retryAfter");
  if (form.enable_tag_filtering) configured.add("tagFiltering");
  if (form.enable_pre_call_checks) configured.add("preCallChecks");
  return configured;
};

const Section: React.FC<{ title: string; onRemove: () => void; children: React.ReactNode }> = ({
  title,
  onRemove,
  children,
}) => (
  <section className="space-y-4 rounded-lg border border-border p-4">
    <div className="flex items-center justify-between gap-3">
      <h3 className="text-sm font-medium text-foreground">{title}</h3>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        onClick={onRemove}
        aria-label={`${t("pages.routeTemplates.advanced.remove")} ${title}`}
      >
        <Trash2 className="size-4 text-destructive" />
      </Button>
    </div>
    {children}
  </section>
);

const NumberOption: React.FC<{
  label: string;
  hint: string;
  value: string;
  onChange: (value: string) => void;
}> = ({ label, hint, value, onChange }) => (
  <label className="block space-y-1">
    <span className="text-xs font-medium uppercase tracking-wide text-foreground">{label}</span>
    <p className="text-xs text-muted-foreground">{hint}</p>
    <Input
      type="number"
      inputMode="decimal"
      aria-label={label}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      className="font-mono"
    />
  </label>
);

const ToggleOption: React.FC<{
  label: string;
  hint: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}> = ({ label, hint, checked, onChange }) => (
  <label className="flex items-start gap-2">
    <Switch
      className="mt-0.5"
      checked={checked}
      onCheckedChange={(next) => onChange(next === true)}
      aria-label={label}
    />
    <span>
      <span className="block text-sm">{label}</span>
      <span className="block text-xs text-muted-foreground">{hint}</span>
    </span>
  </label>
);

const AdvancedTemplateSettings: React.FC<AdvancedTemplateSettingsProps> = ({
  form,
  modelNames,
  availableStrategies,
  onChange,
}) => {
  const [added, setAdded] = useState<Set<ModuleId>>(() => new Set());
  const [choice, setChoice] = useState<ModuleId | "">("");
  const configured = useMemo(() => configuredModules(form), [form]);
  const visible = useMemo(() => new Set([...configured, ...added]), [configured, added]);
  const available = MODULES.filter((id) => !visible.has(id));

  const add = () => {
    if (!choice) return;
    setAdded((current) => new Set(current).add(choice));
    setChoice("");
  };
  const remove = (id: ModuleId, patch: Partial<TemplateFormState>) => {
    setAdded((current) => {
      const next = new Set(current);
      next.delete(id);
      return next;
    });
    onChange(patch);
  };
  const hasRawJson = Boolean(form.retry_policy.trim() || form.model_group_alias.trim() || form.extra.trim());

  return (
    <div className="space-y-4">
      <Alert>
        <AlertTitle>{t("pages.routeTemplates.advanced.title")}</AlertTitle>
        <AlertDescription>{t("pages.routeTemplates.advancedHint")}</AlertDescription>
      </Alert>

      {visible.size === 0 && (
        <div className="rounded-lg border border-dashed border-border px-5 py-8 text-center">
          <p className="text-sm font-medium text-foreground">{t("pages.routeTemplates.advanced.emptyTitle")}</p>
          <p className="mt-1 text-xs text-muted-foreground">{t("pages.routeTemplates.advanced.emptyHint")}</p>
        </div>
      )}

      {visible.has("routingGroups") && (
        <Section title={moduleLabel("routingGroups")} onRemove={() => remove("routingGroups", { routing_groups: "" })}>
          <TemplateRoutingGroups
            value={form.routing_groups}
            modelOptions={modelNames}
            availableStrategies={availableStrategies}
            onChange={(routing_groups) => onChange({ routing_groups })}
          />
        </Section>
      )}

      {visible.has("failureFallbacks") && (
        <Section title={moduleLabel("failureFallbacks")} onRemove={() => remove("failureFallbacks", { fallbacks: [] })}>
          <ModelFallbackEditor
            title={moduleLabel("failureFallbacks")}
            hint={t("pages.routeTemplates.advanced.failureFallbacksHint")}
            rows={form.fallbacks}
            modelNames={modelNames}
            onChange={(fallbacks) => onChange({ fallbacks })}
          />
        </Section>
      )}

      {visible.has("contextFallbacks") && (
        <Section
          title={moduleLabel("contextFallbacks")}
          onRemove={() => remove("contextFallbacks", { context_window_fallbacks: [] })}
        >
          <ModelFallbackEditor
            title={moduleLabel("contextFallbacks")}
            hint={t("pages.routeTemplates.advanced.contextFallbacksHint")}
            rows={form.context_window_fallbacks}
            modelNames={modelNames}
            onChange={(context_window_fallbacks) => onChange({ context_window_fallbacks })}
          />
        </Section>
      )}

      {visible.has("policyFallbacks") && (
        <Section
          title={moduleLabel("policyFallbacks")}
          onRemove={() => remove("policyFallbacks", { content_policy_fallbacks: [] })}
        >
          <ModelFallbackEditor
            title={moduleLabel("policyFallbacks")}
            hint={t("pages.routeTemplates.advanced.policyFallbacksHint")}
            rows={form.content_policy_fallbacks}
            modelNames={modelNames}
            onChange={(content_policy_fallbacks) => onChange({ content_policy_fallbacks })}
          />
        </Section>
      )}

      {visible.has("defaultFallbacks") && (
        <Section
          title={moduleLabel("defaultFallbacks")}
          onRemove={() => remove("defaultFallbacks", { default_fallbacks: "" })}
        >
          <label className="block space-y-1">
            <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.advanced.defaultFallbacksHint")}</p>
            <Input
              aria-label={moduleLabel("defaultFallbacks")}
              value={form.default_fallbacks}
              onChange={(event) => onChange({ default_fallbacks: event.target.value })}
            />
          </label>
        </Section>
      )}

      {visible.has("maxFallbacks") && (
        <Section title={moduleLabel("maxFallbacks")} onRemove={() => remove("maxFallbacks", { max_fallbacks: "" })}>
          <NumberOption
            label={t("pages.routeTemplates.advanced.maxFallbacks")}
            hint={t("pages.routeTemplates.advanced.maxFallbacksHint")}
            value={form.max_fallbacks}
            onChange={(max_fallbacks) => onChange({ max_fallbacks })}
          />
        </Section>
      )}

      {visible.has("fallbackCauses") && (
        <Section
          title={moduleLabel("fallbackCauses")}
          onRemove={() => remove("fallbackCauses", { fallback_causes: [...FALLBACK_CAUSES] })}
        >
          <div className="space-y-2">
            <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.advanced.fallbackCausesHint")}</p>
            {FALLBACK_CAUSES.map((cause) => (
              <label key={cause} className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={form.fallback_causes.includes(cause)}
                  onCheckedChange={(next) =>
                    onChange({
                      fallback_causes:
                        next === true
                          ? [...form.fallback_causes, cause]
                          : form.fallback_causes.filter((item) => item !== cause),
                    })
                  }
                />
                {t(`pages.routeTemplates.advanced.causes.${cause}`)}
              </label>
            ))}
          </div>
        </Section>
      )}

      {visible.has("streamTimeout") && (
        <Section title={moduleLabel("streamTimeout")} onRemove={() => remove("streamTimeout", { stream_timeout: "" })}>
          <NumberOption
            label={moduleLabel("streamTimeout")}
            hint={t("pages.routeTemplates.advanced.streamTimeoutHint")}
            value={form.stream_timeout}
            onChange={(stream_timeout) => onChange({ stream_timeout })}
          />
        </Section>
      )}

      {visible.has("retryAfter") && (
        <Section title={moduleLabel("retryAfter")} onRemove={() => remove("retryAfter", { retry_after: "" })}>
          <NumberOption
            label={moduleLabel("retryAfter")}
            hint={t("pages.routeTemplates.advanced.retryAfterHint")}
            value={form.retry_after}
            onChange={(retry_after) => onChange({ retry_after })}
          />
        </Section>
      )}

      {visible.has("tagFiltering") && (
        <Section
          title={moduleLabel("tagFiltering")}
          onRemove={() => remove("tagFiltering", { enable_tag_filtering: false })}
        >
          <ToggleOption
            label={moduleLabel("tagFiltering")}
            hint={t("pages.routeTemplates.advanced.tagFilteringHint")}
            checked={form.enable_tag_filtering}
            onChange={(enable_tag_filtering) => onChange({ enable_tag_filtering })}
          />
        </Section>
      )}

      {visible.has("preCallChecks") && (
        <Section
          title={moduleLabel("preCallChecks")}
          onRemove={() => remove("preCallChecks", { enable_pre_call_checks: false })}
        >
          <ToggleOption
            label={moduleLabel("preCallChecks")}
            hint={t("pages.routeTemplates.advanced.preCallChecksHint")}
            checked={form.enable_pre_call_checks}
            onChange={(enable_pre_call_checks) => onChange({ enable_pre_call_checks })}
          />
        </Section>
      )}

      {hasRawJson && (
        <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
          {t("pages.routeTemplates.advanced.jsonManagedHint")}
        </p>
      )}

      {available.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <label className="sr-only" htmlFor="advanced-template-module">
            {t("pages.routeTemplates.advanced.addConfiguration")}
          </label>
          <select
            id="advanced-template-module"
            className="h-9 w-full min-w-0 rounded-md border border-input bg-background px-3 text-sm"
            value={choice}
            onChange={(event) => setChoice(event.target.value as ModuleId | "")}
          >
            <option value="">{t("pages.routeTemplates.advanced.chooseConfiguration")}</option>
            {available.map((id) => (
              <option key={id} value={id}>
                {moduleLabel(id)}
              </option>
            ))}
          </select>
          <Button type="button" variant="outline" onClick={add} disabled={!choice}>
            <Plus className="size-4" />
            {t("pages.routeTemplates.advanced.addConfiguration")}
          </Button>
        </div>
      )}
    </div>
  );
};

export default AdvancedTemplateSettings;

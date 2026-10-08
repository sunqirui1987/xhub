"use client";

import React, { useEffect, useMemo, useState } from "react";
import { t } from "@/i18n";
import { fetchAvailableModels } from "@/components/llm_calls/fetch_models";
import { formatStrategyLabel } from "@/components/routing_groups/strategy";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import ModelFallbackEditor from "./ModelFallbackEditor";
import {
  bodyFromForm,
  FALLBACK_CAUSES,
  formFromBody,
  parseDocument,
  prettyDocument,
  strategyOptions,
  weightRowsForEditor,
  type SplitDeployment,
  type TemplateFormState,
} from "./templateForm";
import WeightedSplitFields from "./WeightedSplitFields";
import TemplateRoutingGroups from "./TemplateRoutingGroups";
import RouteTemplateJsonGuide from "./RouteTemplateJsonGuide";

const CAUSE_COPY: Record<(typeof FALLBACK_CAUSES)[number], { label: string; hint: string }> = {
  no_response: {
    label: "pages.routeTemplates.causeNoResponse",
    hint: "pages.routeTemplates.causeNoResponseHint",
  },
  status: {
    label: "pages.routeTemplates.causeStatus",
    hint: "pages.routeTemplates.causeStatusHint",
  },
  ambiguous: {
    label: "pages.routeTemplates.causeAmbiguous",
    hint: "pages.routeTemplates.causeAmbiguousHint",
  },
};

const NumberField: React.FC<{
  label: string;
  hint: string;
  value: string;
  onChange: (value: string) => void;
}> = ({ label, hint, value, onChange }) => (
  <label className="space-y-1">
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

const StrategyOption: React.FC<{ option: string }> = ({ option }) => (
  <SelectItem value={option}>{t(formatStrategyLabel(option))}</SelectItem>
);

const JsonArea: React.FC<{ label: string; hint: string; value: string; onChange: (value: string) => void }> = ({
  label,
  hint,
  value,
  onChange,
}) => (
  <label className="block space-y-1">
    <span className="text-xs font-medium uppercase tracking-wide text-foreground">{label}</span>
    <p className="text-xs text-muted-foreground">{hint}</p>
    <textarea
      aria-label={label}
      className="h-28 w-full rounded-md border border-border bg-transparent p-3 font-mono text-xs"
      value={value}
      spellCheck={false}
      onChange={(event) => onChange(event.target.value)}
    />
  </label>
);

/**
 * Edits one template: how traffic is split, which model to try after a failure,
 * and the same document as JSON.
 *
 * The parent owns the draft. This component only reports the next form, so a
 * cancel throws the draft away without a second copy of the document.
 */
const TemplateEditor: React.FC<{
  name: string;
  nameLocked?: boolean;
  form: TemplateFormState;
  deployments: SplitDeployment[];
  accessToken: string | null;
  onName: (name: string) => void;
  onChange: (form: TemplateFormState) => void;
  onJsonValid: (valid: boolean) => void;
}> = ({ name, nameLocked = false, form, deployments, accessToken, onName, onChange, onJsonValid }) => {
  const [jsonOverride, setJsonOverride] = useState<string | null>(null);
  const [jsonError, setJsonError] = useState("");
  const [catalog, setCatalog] = useState<string[]>([]);
  const [activeTab, setActiveTab] = useState("loadbalancing");
  const patch = (next: Partial<TemplateFormState>) => {
    setJsonOverride(null);
    setJsonError("");
    onJsonValid(true);
    onChange({ ...form, ...next });
  };
  useEffect(() => {
    if (!accessToken) return;
    let cancelled = false;
    void fetchAvailableModels(accessToken)
      .then((list) => {
        if (!cancelled) setCatalog(list.map((item) => item.model_group));
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [accessToken]);

  const modelNames = useMemo(() => {
    const names = new Set(catalog);
    for (const deployment of deployments) {
      if (deployment.model_name) names.add(deployment.model_name);
    }
    return [...names];
  }, [catalog, deployments]);

  const options = strategyOptions(form.routing_strategy);
  const selectedKey = `pages.routeTemplates.strategyDescriptions.${form.routing_strategy}`;
  const selectedDescription = t(selectedKey);
  const written = bodyFromForm(form);
  const generatedJson = written.ok ? prettyDocument(written.body) : "";
  const jsonText = jsonOverride ?? generatedJson;

  const chooseStrategy = (strategy: string) => {
    if (strategy === "weighted-split" && form.weights.length === 0 && deployments.length > 0) {
      patch({ routing_strategy: strategy, weights: weightRowsForEditor([], deployments) });
      return;
    }
    patch({ routing_strategy: strategy });
  };

  const editJson = (text: string) => {
    setJsonOverride(text);
    const parsed = parseDocument(text);
    if (!parsed.ok) {
      setJsonError(t("pages.routeTemplates.jsonInvalid"));
      onJsonValid(false);
      return;
    }
    setJsonError("");
    onJsonValid(true);
    onChange(formFromBody(parsed.body));
  };

  const uploadJson = (file: File | undefined) => {
    if (!file) return;
    void file.text().then((text) => editJson(text));
  };

  const downloadJson = () => {
    const blob = new Blob([jsonText], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${name.trim() || "router-settings"}.json`;
    link.click();
    URL.revokeObjectURL(url);
  };
  const toggleCause = (cause: string, on: boolean) => {
    const fallback_causes = on
      ? [...form.fallback_causes, cause]
      : form.fallback_causes.filter((item) => item !== cause);
    patch({ fallback_causes });
  };

  return (
    <div className="space-y-4">
      <label className="block space-y-1">
        <span className="text-xs font-medium uppercase tracking-wide text-foreground">
          {t("pages.routeTemplates.name")}
        </span>
        <Input
          aria-label={t("pages.routeTemplates.name")}
          value={name}
          readOnly={nameLocked}
          onChange={(event) => onName(event.target.value)}
        />
        {nameLocked && <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.platformNameLocked")}</p>}
      </label>

      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList variant="line" className="h-auto w-full justify-start overflow-x-auto rounded-none border-b p-0">
          <TabsTrigger value="loadbalancing" className="flex-none rounded-none px-4 py-2">
            {t("pages.routeTemplates.loadBalancing")}
          </TabsTrigger>
          <TabsTrigger value="routing-groups" className="flex-none rounded-none px-4 py-2">
            {t("pages.routeTemplates.routingGroups")}
          </TabsTrigger>
          <TabsTrigger value="fallbacks" className="flex-none rounded-none px-4 py-2">
            {t("pages.routeTemplates.fallbacksTab")}
          </TabsTrigger>
          <TabsTrigger value="json" className="flex-none rounded-none px-4 py-2">
            {t("pages.routeTemplates.jsonTab")}
          </TabsTrigger>
          <TabsTrigger value="guide" className="flex-none rounded-none px-4 py-2">
            {t("pages.routeTemplates.jsonGuide.tab")}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="loadbalancing" className="space-y-6 pt-4">
          <div className="max-w-xl space-y-1">
            <span className="text-xs font-medium uppercase tracking-wide text-foreground">
              {t("pages.routeTemplates.strategy")}
            </span>
            <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.strategyHint")}</p>
            <Select value={form.routing_strategy} onValueChange={(value) => value && chooseStrategy(value)}>
              <SelectTrigger className="w-full" aria-label={t("pages.routeTemplates.strategy")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {options.map((option) => (
                  <StrategyOption key={option} option={option} />
                ))}
              </SelectContent>
            </Select>
            {selectedDescription !== selectedKey && (
              <p className="text-xs text-muted-foreground">{selectedDescription}</p>
            )}
          </div>

          {form.routing_strategy === "weighted-split" && (
            <WeightedSplitFields
              saved={form.weights}
              deployments={deployments}
              onChange={(weights) => patch({ weights })}
            />
          )}

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <NumberField
              label={t("pages.routeTemplates.retries")}
              hint={t("pages.routeTemplates.retriesHint")}
              value={form.num_retries}
              onChange={(num_retries) => patch({ num_retries })}
            />
            <NumberField
              label={t("pages.routeTemplates.timeout")}
              hint={t("pages.routeTemplates.timeoutHint")}
              value={form.timeout}
              onChange={(timeout) => patch({ timeout })}
            />
            <NumberField
              label={t("pages.routeTemplates.streamTimeout")}
              hint={t("pages.routeTemplates.streamTimeoutHint")}
              value={form.stream_timeout}
              onChange={(stream_timeout) => patch({ stream_timeout })}
            />
            <NumberField
              label={t("pages.routeTemplates.allowedFails")}
              hint={t("pages.routeTemplates.allowedFailsHint")}
              value={form.allowed_fails}
              onChange={(allowed_fails) => patch({ allowed_fails })}
            />
            <NumberField
              label={t("pages.routeTemplates.cooldown")}
              hint={t("pages.routeTemplates.cooldownHint")}
              value={form.cooldown_time}
              onChange={(cooldown_time) => patch({ cooldown_time })}
            />
            <NumberField
              label={t("pages.routeTemplates.retryAfter")}
              hint={t("pages.routeTemplates.retryAfterHint")}
              value={form.retry_after}
              onChange={(retry_after) => patch({ retry_after })}
            />
          </div>

          {form.routing_strategy === "latency-based-routing" && (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <NumberField
                label={t("pages.routeTemplates.ttl")}
                hint={t("pages.routeTemplates.ttlHint")}
                value={form.ttl}
                onChange={(ttl) => patch({ ttl })}
              />
              <NumberField
                label={t("pages.routeTemplates.latencyBuffer")}
                hint={t("pages.routeTemplates.latencyBufferHint")}
                value={form.lowest_latency_buffer}
                onChange={(lowest_latency_buffer) => patch({ lowest_latency_buffer })}
              />
            </div>
          )}

          <label className="flex items-start gap-2">
            <Switch
              className="mt-0.5"
              checked={form.enable_tag_filtering}
              onCheckedChange={(checked) => patch({ enable_tag_filtering: checked === true })}
              aria-label={t("pages.routeTemplates.tagFiltering")}
            />
            <span>
              <span className="block text-sm">{t("pages.routeTemplates.tagFiltering")}</span>
              <span className="block text-xs text-muted-foreground">{t("pages.routeTemplates.tagFilteringHint")}</span>
            </span>
          </label>
          <label className="flex items-start gap-2">
            <Switch
              className="mt-0.5"
              checked={form.enable_pre_call_checks}
              onCheckedChange={(checked) => patch({ enable_pre_call_checks: checked === true })}
              aria-label={t("pages.routeTemplates.preCallChecks")}
            />
            <span>
              <span className="block text-sm">{t("pages.routeTemplates.preCallChecks")}</span>
              <span className="block text-xs text-muted-foreground">{t("pages.routeTemplates.preCallChecksHint")}</span>
            </span>
          </label>
        </TabsContent>

        <TabsContent value="routing-groups" className="pt-4">
          <TemplateRoutingGroups
            value={form.routing_groups}
            modelOptions={modelNames}
            availableStrategies={options}
            onChange={(routing_groups) => patch({ routing_groups })}
          />
        </TabsContent>

        <TabsContent value="fallbacks" className="space-y-6 pt-4">
          <Alert>
            <AlertTitle>{t("pages.routeTemplates.fallbackIntroTitle")}</AlertTitle>
            <AlertDescription className="space-y-2">
              <p>{t("pages.routeTemplates.fallbackIntro")}</p>
              <Button type="button" size="sm" variant="outline" onClick={() => setActiveTab("guide")}>
                {t("pages.routeTemplates.openJsonGuide")}
              </Button>
            </AlertDescription>
          </Alert>
          <NumberField
            label={t("pages.routeTemplates.maxFallbacks")}
            hint={t("pages.routeTemplates.maxFallbacksHint")}
            value={form.max_fallbacks}
            onChange={(max_fallbacks) => patch({ max_fallbacks })}
          />

          <div className="space-y-2">
            <p className="text-sm font-medium text-foreground">{t("pages.routeTemplates.causes")}</p>
            <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.causesHint")}</p>
            {FALLBACK_CAUSES.map((cause) => {
              const copy = CAUSE_COPY[cause];
              const checked = form.fallback_causes.includes(cause);
              return (
                <label key={cause} className="flex items-start gap-2">
                  <Checkbox
                    className="mt-0.5"
                    checked={checked}
                    aria-label={t(copy.label)}
                    onCheckedChange={(next) => toggleCause(cause, next === true)}
                  />
                  <span>
                    <span className="block text-sm">{t(copy.label)}</span>
                    <span className="block text-xs text-muted-foreground">{t(copy.hint)}</span>
                  </span>
                </label>
              );
            })}
          </div>

          <ModelFallbackEditor
            title={t("Configure Model Fallbacks")}
            hint={t("Manage multiple fallback chains for different models (up to 5 groups at a time)")}
            rows={form.fallbacks}
            modelNames={modelNames}
            onChange={(fallbacks) => patch({ fallbacks })}
          />
          <ModelFallbackEditor
            title={t("pages.routeTemplates.contextFallbacks")}
            hint={t("pages.routeTemplates.contextHint")}
            rows={form.context_window_fallbacks}
            modelNames={modelNames}
            onChange={(context_window_fallbacks) => patch({ context_window_fallbacks })}
          />
          <ModelFallbackEditor
            title={t("pages.routeTemplates.contentFallbacks")}
            hint={t("pages.routeTemplates.contentHint")}
            rows={form.content_policy_fallbacks}
            modelNames={modelNames}
            onChange={(content_policy_fallbacks) => patch({ content_policy_fallbacks })}
          />
          <label className="block space-y-1">
            <span className="text-xs font-medium uppercase tracking-wide text-foreground">
              {t("pages.routeTemplates.defaultFallbacks")}
            </span>
            <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.defaultFallbacksHint")}</p>
            <Input
              aria-label={t("pages.routeTemplates.defaultFallbacks")}
              value={form.default_fallbacks}
              onChange={(event) => patch({ default_fallbacks: event.target.value })}
            />
          </label>
          <JsonArea
            label={t("pages.routeTemplates.retryPolicy")}
            hint={t("pages.routeTemplates.retryPolicyHint")}
            value={form.retry_policy}
            onChange={(retry_policy) => patch({ retry_policy })}
          />
          <JsonArea
            label={t("pages.routeTemplates.modelGroupAlias")}
            hint={t("pages.routeTemplates.modelGroupAliasHint")}
            value={form.model_group_alias}
            onChange={(model_group_alias) => patch({ model_group_alias })}
          />
          <JsonArea
            label={t("pages.routeTemplates.extra")}
            hint={t("pages.routeTemplates.extraHint")}
            value={form.extra}
            onChange={(extra) => patch({ extra })}
          />
        </TabsContent>

        <TabsContent value="json" className="space-y-3 pt-4">
          <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.jsonHint")}</p>
          <textarea
            aria-label={t("pages.routeTemplates.jsonTab")}
            className="h-96 w-full rounded-md border border-border bg-transparent p-3 font-mono text-xs"
            value={jsonText}
            spellCheck={false}
            onChange={(event) => editJson(event.target.value)}
          />
          {jsonError && <p className="text-xs text-destructive">{jsonError}</p>}
          <div className="flex flex-wrap gap-2">
            <Button type="button" size="sm" variant="outline" onClick={downloadJson}>
              {t("pages.routeTemplates.downloadJson")}
            </Button>
            <label className="inline-flex cursor-pointer items-center">
              <input
                type="file"
                accept="application/json,.json"
                className="sr-only"
                onChange={(event) => {
                  uploadJson(event.target.files?.[0]);
                  event.target.value = "";
                }}
              />
              <span className="inline-flex h-8 items-center rounded-md border border-border px-3 text-sm">
                {t("pages.routeTemplates.importJson")}
              </span>
            </label>
          </div>
        </TabsContent>

        <TabsContent value="guide" className="pt-4">
          <RouteTemplateJsonGuide />
        </TabsContent>
      </Tabs>
    </div>
  );
};

export default TemplateEditor;

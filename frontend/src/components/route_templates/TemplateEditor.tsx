"use client";

import React, { useEffect, useMemo, useState } from "react";
import { t } from "@/i18n";
import { fetchAvailableModels } from "@/components/llm_calls/fetch_models";
import { formatTemplateStrategyLabel } from "./strategyLabel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import AdvancedTemplateSettings from "./AdvancedTemplateSettings";
import ModelRoutingFields from "./ModelRoutingFields";
import {
  bodyFromForm,
  formFromBody,
  parseDocument,
  prettyDocument,
  strategyOptions,
  type SplitDeployment,
  type TemplateFormState,
} from "./templateForm";
import WeightedSplitFields from "./WeightedSplitFields";
import RoutePreview from "./RoutePreview";

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
  <SelectItem value={option}>{t(formatTemplateStrategyLabel(option))}</SelectItem>
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
  templateId?: string;
  organizationId?: string;
  teamId?: string;
  onName: (name: string) => void;
  onChange: (form: TemplateFormState) => void;
  onJsonValid: (valid: boolean) => void;
}> = ({ name, nameLocked = false, form, deployments, accessToken, templateId, organizationId, teamId, onName, onChange, onJsonValid }) => {
  const [jsonOverride, setJsonOverride] = useState<string | null>(null);
  const [jsonError, setJsonError] = useState("");
  const [catalog, setCatalog] = useState<string[]>([]);
  const [activeTab, setActiveTab] = useState("basic");
  const patch = (next: Partial<TemplateFormState>) => {
    setJsonOverride(null);
    setJsonError("");
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
  useEffect(() => {
    onJsonValid(written.ok && !jsonError);
  }, [written.ok, jsonError, onJsonValid]);
  const generatedJson = written.ok ? prettyDocument(written.body) : "";
  const jsonText = jsonOverride ?? generatedJson;

  const chooseStrategy = (strategy: string) => patch({ routing_strategy: strategy });

  const editJson = (text: string) => {
    setJsonOverride(text);
    const parsed = parseDocument(text);
    if (!parsed.ok) {
      setJsonError(t("pages.routeTemplates.jsonInvalid"));
      onJsonValid(false);
      return;
    }
    setJsonError("");
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

  return (
    <div className="space-y-4">
      <label className="block max-w-xl space-y-1">
        <span className="text-xs font-medium uppercase tracking-wide text-foreground">
          {t("pages.routeTemplates.name")}
        </span>
        <Input
          aria-label={t("pages.routeTemplates.name")}
          value={name}
          readOnly={nameLocked}
          placeholder={t("pages.routeTemplates.namePlaceholder")}
          required
          autoFocus={!nameLocked}
          onChange={(event) => onName(event.target.value)}
        />
        {nameLocked && <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.platformNameLocked")}</p>}
      </label>

      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList variant="line" className="h-auto w-full justify-start rounded-none border-b p-0">
          <TabsTrigger value="basic" disabled={!!jsonError} className="px-4 py-2">
            {t("pages.routeTemplates.basicSettings")}
          </TabsTrigger>
          <TabsTrigger value="advanced" disabled={!!jsonError} className="px-4 py-2">
            {t("pages.routeTemplates.advancedSettings")}
          </TabsTrigger>
          <TabsTrigger value="json" className="px-4 py-2">
            {t("pages.routeTemplates.jsonTab")}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="basic" className="space-y-6 pt-4">
          <div className="max-w-xl space-y-1">
            <span className="text-xs font-medium uppercase tracking-wide text-foreground">
              {t("pages.routeTemplates.modelRouting.defaultTitle")}
            </span>
            <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.modelRouting.defaultHint")}</p>
            <Select value={form.routing_strategy} onValueChange={(value) => value && chooseStrategy(value)}>
              <SelectTrigger className="w-full" aria-label={t("pages.routeTemplates.strategy")}>
                <SelectValue>{t(formatTemplateStrategyLabel(form.routing_strategy))}</SelectValue>
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

          <ModelRoutingFields
            rules={form.model_routing}
            deployments={deployments}
            defaultStrategy={form.routing_strategy}
            onChange={(model_routing) => patch({ model_routing })}
          />

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
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
          </div>

          {form.routing_strategy === "latency-based-routing" && (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
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
        </TabsContent>

        <TabsContent value="advanced" className="space-y-4 pt-4">
          <AdvancedTemplateSettings
            form={form}
            modelNames={modelNames}
            availableStrategies={options}
            onChange={patch}
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
          {jsonError && (
            <p role="alert" className="text-xs text-destructive">
              {jsonError} · {t("pages.routeTemplates.jsonFixHint")}
            </p>
          )}
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
      </Tabs>
      <RoutePreview accessToken={accessToken} models={modelNames} body={written.ok && !jsonError ? written.body : null} templateId={templateId} organizationId={organizationId} teamId={teamId} />
      {!written.ok && !jsonError && (
        <p role="alert" className="text-xs text-destructive">
          {t("pages.routeTemplates.invalidField", { field: written.field })}
        </p>
      )}
    </div>
  );
};

export default TemplateEditor;

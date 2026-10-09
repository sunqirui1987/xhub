"use client";

import React, { useEffect, useMemo, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { apiClient } from "../networking";
import { t } from "@/i18n";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { formatTemplateStrategyLabel } from "./strategyLabel";
import WeightedSplitFields from "./WeightedSplitFields";
import {
  bodyFromForm,
  formFromBody,
  strategyOptions,
  type ModelRoutingRule,
  type SplitDeployment,
  type WeightRow,
} from "./templateForm";

const isRecord = (value: unknown): value is Record<string, unknown> =>
  Boolean(value) && typeof value === "object" && !Array.isArray(value);
const textValue = (value: unknown) => {
  if (typeof value === "string") return value;
  if (typeof value === "number") return String(value);
  return "";
};
const hasWeightIdentity = (item: Record<string, unknown>) =>
  [item.deployment_id, item.pricing_id].some((value) => typeof value === "string" && value.trim() !== "");
const weightsFromRule = (rule: ModelRoutingRule): WeightRow[] => {
  const parsed = formFromBody({
    routing_strategy: rule.routing_strategy,
    routing_strategy_args: rule.routing_strategy_args,
  }).weights;
  const raw = rule.routing_strategy_args?.weights;
  if (!Array.isArray(raw)) return parsed;
  return raw.flatMap((item) => {
    if (!isRecord(item)) return [];
    const modelName = textValue(item.model_name);
    const model = textValue(item.model) || modelName;
    const apiBase = textValue(item.api_base);
    if (!hasWeightIdentity(item)) return [];
    return [
      {
        ...(typeof item.deployment_id === "string" ? { deployment_id: item.deployment_id } : {}),
        ...(typeof item.pricing_id === "string" ? { pricing_id: item.pricing_id } : {}),
        model_name: modelName || model,
        api_base: apiBase,
        model,
        weight: textValue(item.weight),
      },
    ];
  });
};
const nonEmptyRecord = (value: unknown): value is Record<string, unknown> => {
  if (!value || typeof value !== "object") return false;
  if (Array.isArray(value)) return false;
  return Object.keys(value).length > 0;
};

const ModelRoutingFields: React.FC<{
  rules: ModelRoutingRule[];
  deployments: SplitDeployment[];
  defaultStrategy: string;
  onChange: (rules: ModelRoutingRule[]) => void;
}> = ({ rules, deployments, defaultStrategy, onChange }) => {
  const [endpoint, setEndpoint] = useState("");
  const [entries, setEntries] = useState<{id:string;label:string}[]>([]);
  // 入口来自后端公共目录；模板不接受任意 URL 作为匹配条件。
  useEffect(() => { let canceled = false; apiClient.get<{endpoint_types?:{id:string;label:string}[]}>("/public/endpoints").then(body => { if (!canceled) setEntries(body?.endpoint_types ?? []); }).catch(() => {}); return () => { canceled = true }; }, []);
  const [selectedModel, setSelectedModel] = useState("");
  const publicModels = useMemo(
    () => [...new Set(deployments.map((row) => row.model_name).filter(Boolean))],
    [deployments],
  );
  const availableModels = publicModels.filter((name) => !rules.some((rule) => rule.model_name === name && (rule.endpoint_id ?? "") === endpoint));
  const selectedAvailable = availableModels.includes(selectedModel);

  const addRule = () => {
    if (!selectedAvailable) return;
    onChange([...rules, { model_name: selectedModel, ...(endpoint ? {endpoint_id:endpoint} : {}), routing_strategy: rules.find(rule => rule.model_name === selectedModel && !rule.endpoint_id)?.routing_strategy ?? defaultStrategy }]);
    setSelectedModel("");
  };
  const patch = (index: number, next: ModelRoutingRule) =>
    onChange(rules.map((rule, i) => (i === index ? next : rule)));
  const remove = (index: number) => onChange(rules.filter((_, i) => i !== index));
  const changeStrategy = (index: number, strategy: string) =>
    patch(index, { ...rules[index], routing_strategy: strategy });
  const changeWeights = (index: number, weights: WeightRow[]) => {
    const rule = rules[index];
    const parsed = formFromBody({
      routing_strategy: rule.routing_strategy,
      routing_strategy_args: rule.routing_strategy_args,
    });
    const written = bodyFromForm({ ...parsed, weights });
    if (!written.ok) {
      patch(index, {
        ...rule,
        routing_strategy_args: {
          ...(rule.routing_strategy_args ?? {}),
          weights: weights.map((row) => ({
            ...(row.deployment_id ? { deployment_id: row.deployment_id } : {}),
            ...(row.pricing_id ? { pricing_id: row.pricing_id } : {}),
            model_name: row.model_name,
            api_base: row.api_base,
            model: row.model,
            weight:
              row.weight.trim() !== "" && Number.isFinite(Number(row.weight)) && Number(row.weight) >= 0
                ? Number(row.weight)
                : row.weight,
          })),
        },
      });
      return;
    }
    const args = written.body.routing_strategy_args;
    const next = { ...rule };
    if (nonEmptyRecord(args)) next.routing_strategy_args = args;
    else delete next.routing_strategy_args;
    patch(index, next);
  };

  return (
    <div className="space-y-4">
      <div>
        <p className="text-sm font-medium text-foreground">{t("pages.routeTemplates.modelRouting.title")}</p>
        <p className="text-xs text-muted-foreground">
          {t("pages.routeTemplates.modelRouting.hint", { strategy: t(formatTemplateStrategyLabel(defaultStrategy)) })}
        </p>
      </div>
      <div className="flex max-w-xl gap-2">
        <Select value={selectedModel} onValueChange={(value) => setSelectedModel(value ?? "")}>
          <SelectTrigger
            className="min-w-0 flex-1"
            aria-label={t("pages.routeTemplates.modelRouting.publicModelLabel")}
          >
            <SelectValue
              placeholder={
                availableModels.length
                  ? t("pages.routeTemplates.modelRouting.publicModelPlaceholder")
                  : t("pages.routeTemplates.modelRouting.allConfigured")
              }
            />
          </SelectTrigger>
          <SelectContent>
            {availableModels.map((name) => (
              <SelectItem key={name} value={name}>
                {name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={endpoint || "model-default"} onValueChange={value => setEndpoint(value === "model-default" ? "" : value ?? "")}>
          <SelectTrigger aria-label="用户入口覆盖"><SelectValue /></SelectTrigger>
          <SelectContent><SelectItem value="model-default">模型默认（继承模板）</SelectItem>{entries.map(entry => <SelectItem key={entry.id} value={entry.id}>{entry.label}</SelectItem>)}</SelectContent>
        </Select>
        <Button type="button" onClick={addRule} disabled={!selectedAvailable}>
          <Plus className="size-4" />
          {t("pages.routeTemplates.modelRouting.add")}
        </Button>
      </div>
      {rules.length === 0 && (
        <p className="rounded-md border border-dashed p-3 text-xs text-muted-foreground">
          {t("pages.routeTemplates.modelRouting.empty")}
        </p>
      )}
      {rules.map((rule, index) => {
        const options = strategyOptions(rule.routing_strategy);
        return (
          <section key={rule.model_name + "-" + index} className="space-y-4 rounded-md border p-3">
            <div className="flex flex-wrap items-end gap-3">
              <div className="min-w-0 flex-1">
                <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.modelRouting.exactName")}</p>
                <p className="truncate font-mono text-sm font-medium">{rule.model_name}</p><p className="text-xs text-muted-foreground">{rule.endpoint_id ? entries.find(entry => entry.id === rule.endpoint_id)?.label ?? rule.endpoint_id : "模型默认"} · 删除覆盖后恢复继承</p>
              </div>
              <label className="min-w-56 space-y-1">
                <span className="text-xs text-muted-foreground">
                  {t("pages.routeTemplates.modelRouting.strategyLabel")}
                </span>
                <Select value={rule.routing_strategy} onValueChange={(value) => value && changeStrategy(index, value)}>
                  <SelectTrigger
                    aria-label={t("pages.routeTemplates.modelRouting.strategyFor", { model: rule.model_name })}
                  >
                    <SelectValue>{t(formatTemplateStrategyLabel(rule.routing_strategy))}</SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {options.map((strategy) => (
                      <SelectItem key={strategy} value={strategy}>
                        {t(formatTemplateStrategyLabel(strategy))}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </label>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                aria-label={t("pages.routeTemplates.modelRouting.removeLabel", { model: rule.model_name })}
                onClick={() => remove(index)}
              >
                <Trash2 className="size-4 text-destructive" />
                {t("pages.routeTemplates.modelRouting.remove")}
              </Button>
            </div>
            {rule.routing_strategy === "weighted-split" && (
              <WeightedSplitFields
                saved={weightsFromRule(rule)}
                deployments={deployments.filter((row) => row.model_name === rule.model_name && (!rule.endpoint_id || row.endpoint_types?.includes(rule.endpoint_id)))}
                modelName={rule.model_name}
                onChange={(weights) => changeWeights(index, weights)}
              />
            )}
          </section>
        );
      })}
    </div>
  );
};
export default ModelRoutingFields;

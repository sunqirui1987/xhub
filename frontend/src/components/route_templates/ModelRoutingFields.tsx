"use client";

import React, { useMemo, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { t } from "@/i18n";
import { formatStrategyLabel } from "@/components/routing_groups/strategy";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import WeightedSplitFields from "./WeightedSplitFields";
import {
  bodyFromForm,
  formFromBody,
  strategyOptions,
  type ModelRoutingRule,
  type SplitDeployment,
  type WeightRow,
} from "./templateForm";

const weightsFromRule = (rule: ModelRoutingRule): WeightRow[] =>
  formFromBody({ routing_strategy: rule.routing_strategy, routing_strategy_args: rule.routing_strategy_args }).weights;

const ModelRoutingFields: React.FC<{
  rules: ModelRoutingRule[];
  deployments: SplitDeployment[];
  defaultStrategy: string;
  onChange: (rules: ModelRoutingRule[]) => void;
}> = ({ rules, deployments, defaultStrategy, onChange }) => {
  const [selectedModel, setSelectedModel] = useState("");
  const publicModels = useMemo(
    () => [...new Set(deployments.map((row) => row.model_name).filter(Boolean))],
    [deployments],
  );
  const availableModels = publicModels.filter((name) => !rules.some((rule) => rule.model_name === name));
  const selectedAvailable = availableModels.includes(selectedModel);

  const addRule = () => {
    if (!selectedAvailable) return;
    onChange([...rules, { model_name: selectedModel, routing_strategy: defaultStrategy }]);
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
    if (!written.ok) return;
    const args = written.body.routing_strategy_args;
    const next = { ...rule };
    if (args && typeof args === "object" && !Array.isArray(args) && Object.keys(args).length > 0)
      next.routing_strategy_args = args as Record<string, unknown>;
    else delete next.routing_strategy_args;
    patch(index, next);
  };

  return (
    <div className="space-y-4">
      <div>
        <p className="text-sm font-medium text-foreground">{t("pages.routeTemplates.modelRouting.title")}</p>
        <p className="text-xs text-muted-foreground">
          {t("pages.routeTemplates.modelRouting.hint", { strategy: t(formatStrategyLabel(defaultStrategy)) })}
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
                <p className="truncate font-mono text-sm font-medium">{rule.model_name}</p>
              </div>
              <label className="min-w-56 space-y-1">
                <span className="text-xs text-muted-foreground">
                  {t("pages.routeTemplates.modelRouting.strategyLabel")}
                </span>
                <Select value={rule.routing_strategy} onValueChange={(value) => value && changeStrategy(index, value)}>
                  <SelectTrigger
                    aria-label={t("pages.routeTemplates.modelRouting.strategyFor", { model: rule.model_name })}
                  >
                    <SelectValue>{t(formatStrategyLabel(rule.routing_strategy))}</SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {options.map((strategy) => (
                      <SelectItem key={strategy} value={strategy}>
                        {t(formatStrategyLabel(strategy))}
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
                deployments={deployments.filter((row) => row.model_name === rule.model_name)}
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

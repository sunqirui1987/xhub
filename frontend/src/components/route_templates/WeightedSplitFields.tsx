"use client";

import React, { useMemo, useState } from "react";
import { Plus, RotateCcw, TriangleAlert } from "lucide-react";
import { t } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { deploymentKey, type SplitDeployment, type WeightRow } from "./templateForm";

type Candidate = { deployment: SplitDeployment; savedIndex?: number; effectiveWeight: string };
const positiveWeight = (raw: string) => {
  const value = Number(raw);
  return Number.isFinite(value) && value > 0 ? value : 0;
};
const rowModel = (row: WeightRow) => row.model_name || row.model;
const label = (row: Pick<SplitDeployment, "model" | "api_base">) =>
  row.api_base ? row.model + " · " + row.api_base : row.model;
const metadata = (row: SplitDeployment) =>
  [row.supplier, row.provider].filter((value, index, all) => value && all.indexOf(value) === index).join(" · ");

const WeightedSplitFields: React.FC<{
  saved: WeightRow[];
  deployments: SplitDeployment[];
  onChange: (rows: WeightRow[]) => void;
  modelName?: string;
}> = ({ saved, deployments, onChange, modelName }) => {
  const [selectedModel, setSelectedModel] = useState("");
  const publicModels = useMemo(
    () => [...new Set(deployments.map((row) => row.model_name).filter(Boolean))],
    [deployments],
  );
  const savedModels = useMemo(() => [...new Set(saved.map(rowModel).filter(Boolean))], [saved]);
  const visibleModels = modelName
    ? [modelName]
    : [...new Set([...savedModels, ...(selectedModel ? [selectedModel] : [])])];
  const update = (index: number, weight: string) =>
    onChange(saved.map((row, i) => (i === index ? { ...row, weight } : row)));
  const reset = (index: number) => onChange(saved.filter((_, i) => i !== index));
  const add = (row: SplitDeployment, weight: string) =>
    onChange([...saved, { model_name: row.model_name, model: row.model, api_base: row.api_base, weight }]);

  return (
    <div className="space-y-4">
      <div>
        <p className="text-sm font-medium text-foreground">{t("pages.routeTemplates.weights.title")}</p>
        <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.weights.relativeHint")}</p>
        <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.weights.inheritedHint")}</p>
      </div>
      {!modelName &&
        (publicModels.length ? (
          <Select value={selectedModel} onValueChange={(value) => setSelectedModel(value ?? "")}>
            <SelectTrigger className="w-full max-w-xl" aria-label={t("pages.routeTemplates.weights.publicModelLabel")}>
              <SelectValue placeholder={t("pages.routeTemplates.weights.publicModelPlaceholder")} />
            </SelectTrigger>
            <SelectContent>
              {publicModels.map((name) => (
                <SelectItem key={name} value={name}>
                  {name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.weights.noDeployments")}</p>
        ))}
      {!modelName && visibleModels.length === 0 && (
        <p className="rounded-md border border-dashed p-3 text-xs text-muted-foreground">
          {t("pages.routeTemplates.weights.chooseModelHint")}
        </p>
      )}
      {visibleModels.map((name) => {
        const live = deployments.filter((row) => row.model_name === name);
        const liveKeys = new Set(live.map(deploymentKey));
        const candidates: Candidate[] = live.map((deployment) => {
          const savedIndex = saved.findIndex(
            (row) => rowModel(row) === name && deploymentKey(row) === deploymentKey(deployment),
          );
          return {
            deployment,
            ...(savedIndex >= 0 ? { savedIndex } : {}),
            effectiveWeight: savedIndex >= 0 ? saved[savedIndex].weight : (deployment.weight ?? "1"),
          };
        });
        const missing = saved
          .map((row, savedIndex) => ({ row, savedIndex }))
          .filter(({ row }) => rowModel(row) === name && !liveKeys.has(deploymentKey(row)));
        const total = candidates.reduce((sum, row) => sum + positiveWeight(row.effectiveWeight), 0);
        return (
          <section key={name} className="space-y-3 rounded-md border p-3">
            <div>
              <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                {t("pages.routeTemplates.weights.publicModel")} · {name}
              </p>
              {candidates.length === 1 && (
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("pages.routeTemplates.weights.singleDeployment")}
                </p>
              )}
            </div>
            {candidates.length === 0 && missing.length === 0 && (
              <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.weights.noModelDeployments")}</p>
            )}
            {candidates.map(({ deployment, savedIndex, effectiveWeight }) => {
              const explicit = savedIndex !== undefined;
              const percent = total ? Math.round((positiveWeight(effectiveWeight) / total) * 100) : 0;
              return (
                <div
                  key={deploymentKey(deployment)}
                  className="grid gap-2 rounded-md bg-muted/30 p-2 sm:grid-cols-[minmax(0,1fr)_8rem_6rem_auto] sm:items-center"
                >
                  <div className="min-w-0">
                    <p className="truncate font-mono text-xs">{deployment.model}</p>
                    <p className="truncate text-xs text-muted-foreground">{deployment.api_base || "—"}</p>
                    {metadata(deployment) && (
                      <p className="truncate text-xs text-muted-foreground">{metadata(deployment)}</p>
                    )}
                    <Badge variant="outline" className="mt-1 font-normal">
                      {t(
                        explicit
                          ? "pages.routeTemplates.weights.sourceOverride"
                          : deployment.weight != null
                            ? "pages.routeTemplates.weights.sourceDeployment"
                            : "pages.routeTemplates.weights.sourceDefault",
                      )}
                    </Badge>
                  </div>
                  {explicit ? (
                    <Input
                      type="number"
                      min={0}
                      aria-label={name + " " + deployment.model + " " + t("pages.routeTemplates.weights.weight")}
                      value={effectiveWeight}
                      onChange={(event) => update(savedIndex, event.target.value)}
                      className="font-mono"
                    />
                  ) : (
                    <span className="font-mono text-sm tabular-nums">
                      {t("pages.routeTemplates.weights.effectiveWeight", { weight: effectiveWeight })}
                    </span>
                  )}
                  <span className="text-xs tabular-nums text-muted-foreground">
                    {t("pages.routeTemplates.weights.estimatedShare", { percent })}
                  </span>
                  {explicit ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      aria-label={t("pages.routeTemplates.weights.resetLabel", { deployment: label(deployment) })}
                      onClick={() => reset(savedIndex)}
                    >
                      <RotateCcw className="size-4" />
                      {t("pages.routeTemplates.weights.reset")}
                    </Button>
                  ) : (
                    <Button type="button" variant="outline" size="sm" onClick={() => add(deployment, effectiveWeight)}>
                      <Plus className="size-4" />
                      {t("pages.routeTemplates.weights.override")}
                    </Button>
                  )}
                </div>
              );
            })}
            {missing.map(({ row, savedIndex }) => (
              <div
                key={deploymentKey(row) + "-" + savedIndex}
                className="grid gap-2 rounded-md border border-dashed p-2 sm:grid-cols-[minmax(0,1fr)_8rem_6rem_auto] sm:items-center"
              >
                <div className="min-w-0">
                  <p className="truncate font-mono text-xs">{row.model}</p>
                  <p className="truncate text-xs text-muted-foreground">{row.api_base || "—"}</p>
                  <Badge variant="outline" className="mt-1 gap-1 font-normal text-amber-700">
                    <TriangleAlert className="size-3" />
                    {t("pages.routeTemplates.weights.missingCatalog")}
                  </Badge>
                </div>
                <Input
                  type="number"
                  min={0}
                  aria-label={name + " " + row.model + " " + t("pages.routeTemplates.weights.weight")}
                  value={row.weight}
                  onChange={(event) => update(savedIndex, event.target.value)}
                  className="font-mono"
                />
                <span className="text-xs text-muted-foreground">{t("pages.routeTemplates.weights.excluded")}</span>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  aria-label={t("pages.routeTemplates.weights.resetLabel", { deployment: label(row) })}
                  onClick={() => reset(savedIndex)}
                >
                  <RotateCcw className="size-4" />
                  {t("pages.routeTemplates.weights.reset")}
                </Button>
              </div>
            ))}
            {candidates.length > 0 && total === 0 && (
              <p className="text-xs text-amber-700">{t("pages.routeTemplates.weights.zeroTotal")}</p>
            )}
            {candidates.length > 1 && (
              <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.weights.estimateNote")}</p>
            )}
          </section>
        );
      })}
    </div>
  );
};
export default WeightedSplitFields;

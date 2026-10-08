"use client";

import React from "react";
import { t } from "@/i18n";
import { Input } from "@/components/ui/input";
import { deploymentKey, weightRowsForEditor, type SplitDeployment, type WeightRow } from "./templateForm";

/**
 * Edits the shares stored in routing_strategy_args.weights.
 *
 * The numbers are a ratio, not a percentage that has to total 100. The percent
 * next to a row is only so the operator can see what 3 and 7 will do.
 */
const WeightedSplitFields: React.FC<{
  saved: WeightRow[];
  deployments: SplitDeployment[];
  onChange: (rows: WeightRow[]) => void;
}> = ({ saved, deployments, onChange }) => {
  const rows = weightRowsForEditor(saved, deployments);
  const totals = new Map<string, number>();
  for (const row of rows) {
    const weight = Number(row.weight);
    if (!Number.isFinite(weight) || weight <= 0) continue;
    totals.set(row.model_name, (totals.get(row.model_name) ?? 0) + weight);
  }

  const update = (key: string, weight: string) => {
    onChange(rows.map((row) => (deploymentKey(row) === key ? { ...row, weight } : row)));
  };

  if (rows.length === 0) {
    return <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.noDeployments")}</p>;
  }

  const groups = new Map<string, WeightRow[]>();
  for (const row of rows) {
    const list = groups.get(row.model_name) ?? [];
    list.push(row);
    groups.set(row.model_name, list);
  }

  return (
    <div className="space-y-4">
      <div>
        <p className="text-sm font-medium text-foreground">{t("pages.routeTemplates.weightsTitle")}</p>
        <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.weightsHint")}</p>
      </div>
      {[...groups.entries()].map(([name, group]) => (
        <div key={name} className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {t("pages.routeTemplates.publicModel")} · {name}
          </p>
          {group.map((row) => {
            const weight = Number(row.weight);
            const total = totals.get(name) ?? 0;
            const percent = total > 0 && Number.isFinite(weight) && weight > 0 ? Math.round((weight / total) * 100) : 0;
            return (
              <div key={deploymentKey(row)} className="grid grid-cols-[1fr_8rem_5rem] items-center gap-2">
                <div className="min-w-0">
                  <p className="truncate font-mono text-xs">{row.model}</p>
                  <p className="truncate text-xs text-muted-foreground">{row.api_base || "—"}</p>
                </div>
                <Input
                  type="number"
                  min={0}
                  aria-label={`${name} ${row.model} ${t("pages.routeTemplates.weight")}`}
                  value={row.weight}
                  onChange={(event) => update(deploymentKey(row), event.target.value)}
                  className="font-mono"
                />
                <span className="text-xs text-muted-foreground">
                  {t("pages.routeTemplates.share", { percent })}
                </span>
              </div>
            );
          })}
        </div>
      ))}
    </div>
  );
};

export default WeightedSplitFields;

"use client";

import React, { useMemo, useState } from "react";
import { FallbackSelectionForm } from "@/components/Settings/RouterSettings/Fallbacks/FallbackSelectionForm";
import type { FallbackGroup } from "@/components/Settings/RouterSettings/Fallbacks/FallbackGroupConfig";
import type { ChainRow } from "./templateForm";
import { chainsFromGroups, groupsFromChains } from "./modelFallbackGroups";

/**
 * The same fallback editor the router settings page already uses: up to five
 * groups, each with a primary model and an ordered chain of at most ten.
 *
 * A group that has no primary yet stays in this component. The document only
 * stores groups that have a primary, so the editor remembers the blank group
 * until the operator fills it in or the document is replaced from JSON.
 */
const ModelFallbackEditor: React.FC<{
  title: string;
  hint: string;
  rows: ChainRow[];
  modelNames: string[];
  onChange: (rows: ChainRow[]) => void;
}> = ({ title, hint, rows, modelNames, onChange }) => {
  const serialized = JSON.stringify(rows);
  const [groups, setGroups] = useState<FallbackGroup[]>(() => groupsFromChains(rows));
  const [seen, setSeen] = useState(serialized);
  if (serialized !== seen) {
    setSeen(serialized);
    setGroups(groupsFromChains(rows));
  }

  const availableModels = useMemo(() => {
    const names = new Set(modelNames);
    for (const group of groups) {
      if (group.primaryModel) names.add(group.primaryModel);
      for (const model of group.fallbackModels) names.add(model);
    }
    return [...names].sort((a, b) => a.localeCompare(b));
  }, [modelNames, groups]);

  const change = (next: FallbackGroup[]) => {
    const chains = chainsFromGroups(next);
    setSeen(JSON.stringify(chains));
    setGroups(next);
    onChange(chains);
  };

  return (
    <div className="space-y-2">
      <div>
        <p className="text-sm font-medium text-foreground">{title}</p>
        <p className="text-xs text-muted-foreground">{hint}</p>
      </div>
      <FallbackSelectionForm
        groups={groups}
        onGroupsChange={change}
        availableModels={availableModels}
        maxGroups={5}
        maxFallbacks={10}
      />
    </div>
  );
};

export default ModelFallbackEditor;

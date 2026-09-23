import { Search, X } from "lucide-react";
import React, { useMemo, useState } from "react";

import { ActivityMetrics } from "@/components/activity_metrics";
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group";

import { filterKeyActivity } from "../keyActivityFilter";
import type { ModelActivityData } from "../types";
import { t } from "@/i18n";

interface KeyActivityPanelProps {
  keyMetrics: Record<string, ModelActivityData>;
  hidePromptCachingMetrics?: boolean;
}

const KeyActivityPanel: React.FC<KeyActivityPanelProps> = ({ keyMetrics, hidePromptCachingMetrics = false }) => {
  const [query, setQuery] = useState("");
  const filtered = useMemo(() => filterKeyActivity(keyMetrics, query), [keyMetrics, query]);
  const totalKeys = Object.keys(keyMetrics).length;
  const shownKeys = Object.keys(filtered).length;
  const isFiltering = query.trim() !== "";

  return (
    <div className="space-y-4">
      <div className="mt-2 flex items-center gap-3">
        <InputGroup className="max-w-md">
          <InputGroupAddon>
            <Search className="size-4 text-muted-foreground" />
          </InputGroupAddon>
          <InputGroupInput
            aria-label={t("Search keys")}
            placeholder={t("Search by key alias, key hash, user ID, or email")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          {isFiltering && (
            <InputGroupAddon align="inline-end">
              <InputGroupButton size="icon-xs" aria-label={t("Clear key search")} onClick={() => setQuery("")}>
                <X />
              </InputGroupButton>
            </InputGroupAddon>
          )}
        </InputGroup>
        <span className="text-sm text-muted-foreground">
          {t("Showing {value0} of {value1} keys", { value0: (shownKeys.toLocaleString()), value1: (totalKeys.toLocaleString()) })}</span>
      </div>
      {isFiltering && totalKeys > 0 && shownKeys === 0 ? (
        <p className="rounded-lg border p-6 text-center text-sm text-muted-foreground">
          {t("No keys match \"{value0}\" in this date range", { value0: (query.trim()) })}</p>
      ) : (
        <ActivityMetrics modelMetrics={filtered} hidePromptCachingMetrics={hidePromptCachingMetrics} />
      )}
    </div>
  );
};

export default KeyActivityPanel;

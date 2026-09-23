import React from "react";
import { MCPServerCostInfo } from "@/components/mcp_tools/types";
import { t } from "@/i18n";

interface MCPServerCostDisplayProps {
  costConfig?: MCPServerCostInfo | null;
}

const MCPServerCostDisplay: React.FC<MCPServerCostDisplayProps> = ({ costConfig }) => {
  const hasDefaultCost =
    costConfig?.default_cost_per_query !== undefined && costConfig?.default_cost_per_query !== null;
  const hasToolCosts =
    costConfig?.tool_name_to_cost_per_query && Object.keys(costConfig.tool_name_to_cost_per_query).length > 0;
  const hasCostConfig = hasDefaultCost || hasToolCosts;

  if (!hasCostConfig) {
    return (
      <div className="mt-6 border-t border-border pt-6">
        <div className="space-y-4">
          <div className="rounded-lg border border-border bg-muted p-4">
            <p className="text-sm text-muted-foreground">
              {t("No cost configuration set for this server. Tool calls will be charged at $0.00 per tool call.")}
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="mt-6 border-t border-border pt-6">
      <div className="space-y-4">
        {hasDefaultCost &&
          costConfig?.default_cost_per_query !== undefined &&
          costConfig?.default_cost_per_query !== null && (
            <div>
              <p className="text-sm font-medium">{t("Default Cost per Query")}</p>
              <div className="font-mono text-sm">${costConfig.default_cost_per_query.toFixed(4)}</div>
            </div>
          )}

        {hasToolCosts && costConfig?.tool_name_to_cost_per_query && (
          <div>
            <p className="text-sm font-medium">{t("Tool-Specific Costs")}</p>
            <div className="mt-2 space-y-2">
              {Object.entries(costConfig.tool_name_to_cost_per_query).map(
                ([toolName, cost]) =>
                  cost !== null &&
                  cost !== undefined && (
                    <div key={toolName} className="flex items-center justify-between rounded-lg bg-muted p-3">
                      <p className="text-sm font-medium">{toolName}</p>
                      <p className="font-mono text-sm">{t("${value0} per query", { value0: (cost.toFixed(4)) })}</p>
                    </div>
                  ),
              )}
            </div>
          </div>
        )}

        <div className="mt-4 rounded-lg border border-border bg-muted p-4">
          <p className="text-sm font-medium">{t("Cost Summary:")}</p>
          <div className="mt-2 space-y-1">
            {hasDefaultCost &&
              costConfig?.default_cost_per_query !== undefined &&
              costConfig?.default_cost_per_query !== null && (
                <p className="text-sm text-muted-foreground">
                  {t("• Default cost: ${value0} per query", { value0: (costConfig.default_cost_per_query.toFixed(4)) })}</p>
              )}
            {hasToolCosts && costConfig?.tool_name_to_cost_per_query && (
              <p className="text-sm text-muted-foreground">
                {t("• {value0} tool(s) with custom pricing", { value0: (Object.keys(costConfig.tool_name_to_cost_per_query).length) })}</p>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

export default MCPServerCostDisplay;

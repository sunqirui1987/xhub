import React from "react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { formatStrategyLabel } from "@/components/routing_groups/strategy";
import { t } from "@/i18n";

interface RoutingStrategySelectorProps {
  selectedStrategy: string | null;
  availableStrategies: string[];
  routingStrategyDescriptions: { [key: string]: string };
  routerFieldsMetadata: { [key: string]: any };
  onStrategyChange: (strategy: string) => void;
}

const RoutingStrategySelector: React.FC<RoutingStrategySelectorProps> = ({
  selectedStrategy,
  availableStrategies,
  routingStrategyDescriptions,
  routerFieldsMetadata,
  onStrategyChange,
}) => {
  const strategyFieldLabel = routerFieldsMetadata["routing_strategy"]?.ui_field_name || "Routing Strategy";
  return (
    <div className="space-y-2 max-w-3xl">
      <div>
        <label className="text-xs font-medium text-foreground uppercase tracking-wide">
          {t(strategyFieldLabel)}
        </label>
        <p className="text-xs text-muted-foreground mt-0.5 mb-2">
          {t(routerFieldsMetadata["routing_strategy"]?.field_description || "")}
        </p>
      </div>
      <div className="routing-strategy-select max-w-3xl">
        <Select
          value={selectedStrategy}
          onValueChange={(strategy: string | null) => strategy && onStrategyChange(strategy)}
        >
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {availableStrategies.map((strategy) => (
              <SelectItem key={strategy} value={strategy}>
                <div className="flex flex-col gap-0.5 py-1">
                  <span className="text-sm font-medium">{t(formatStrategyLabel(strategy))}</span>
                  <span className="font-mono text-xs text-muted-foreground">{strategy}</span>
                  {routingStrategyDescriptions[strategy] && (
                    <span className="text-xs font-normal text-muted-foreground">
                      {t(routingStrategyDescriptions[strategy])}
                    </span>
                  )}
                </div>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </div>
  );
};

export default RoutingStrategySelector;

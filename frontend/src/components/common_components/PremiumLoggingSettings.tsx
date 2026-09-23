import React from "react";
import { Badge } from "@/components/ui/badge";
import LoggingSettings from "../team/LoggingSettings";
import { t } from "@/i18n";

interface PremiumLoggingSettingsProps {
  value: any[];
  onChange: (settings: any[]) => void;
  premiumUser?: boolean;
  disabledCallbacks?: string[];
  onDisabledCallbacksChange?: (disabledCallbacks: string[]) => void;
}

export function PremiumLoggingSettings({
  value,
  onChange,
  premiumUser = false,
  disabledCallbacks = [],
  onDisabledCallbacksChange,
}: PremiumLoggingSettingsProps) {
  if (!premiumUser) {
    return (
      <div>
        <div className="flex flex-wrap gap-2 mb-3">
          <Badge variant="secondary" className="opacity-50">
            {t("✨ langfuse-logging")}
          </Badge>
          <Badge variant="secondary" className="opacity-50">
            {t("✨ datadog-logging")}
          </Badge>
        </div>
        <div className="p-3 bg-muted border border-border rounded-lg">
          <p className="text-sm text-muted-foreground">
            {t("Setting Key/Team logging settings is a LiteLLM Enterprise feature. Global Logging Settings are available for all free users. Get a trial key")}{" "}
            
              {t("here")}
            
            .
          </p>
        </div>
      </div>
    );
  }

  return (
    <LoggingSettings
      value={value}
      onChange={onChange}
      disabledCallbacks={disabledCallbacks}
      onDisabledCallbacksChange={onDisabledCallbacksChange}
    />
  );
}

export default PremiumLoggingSettings;

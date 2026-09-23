import { Switch } from "@/components/ui/switch";
import React from "react";
import type { ComplexityRouterConfigValue } from "./ComplexityRouterConfig";
import { t } from "@/i18n";

const ResponseFormatControls: React.FC<{
  value: ComplexityRouterConfigValue;
  onChange: (value: ComplexityRouterConfigValue) => void;
}> = ({ value, onChange }) => (
  <>
    <div className="flex items-center gap-2 mb-2">
      <Switch
        checked={value.return_raw_model_name ?? false}
        onCheckedChange={(returnRawModelName) => onChange({ ...value, return_raw_model_name: returnRawModelName })}
        aria-label={t("Return raw model name")}
      />
      <strong className="font-semibold">{t("Return raw model name")}</strong>
    </div>
    <span className="block text-xs text-muted-foreground">
      {t("Return the resolved underlying model name in responses instead of the autorouter alias.")}
    </span>
  </>
);

export default ResponseFormatControls;

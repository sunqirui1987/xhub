import { Badge } from "@/components/ui/badge";
import { hasRouterSettings } from "./routerSettingsPayload";
import { t } from "@/i18n";

interface RouterSettingsSummaryProps {
  routerSettings: Record<string, unknown> | null | undefined;
  emptyText?: string;
}

const fallbackEntries = (fallbacks: unknown): Array<[string, string[]]> => {
  if (!Array.isArray(fallbacks)) return [];
  return fallbacks.flatMap((entry) =>
    entry && typeof entry === "object" ? (Object.entries(entry) as Array<[string, string[]]>) : [],
  );
};

export default function RouterSettingsSummary({
  routerSettings,
  emptyText = t("No router settings configured"),
}: RouterSettingsSummaryProps) {
  if (!hasRouterSettings(routerSettings)) {
    return <div className="text-muted-foreground">{emptyText}</div>;
  }

  const settings = routerSettings as Record<string, unknown>;
  const fallbacks = fallbackEntries(settings.fallbacks);

  return (
    <div className="space-y-1 text-sm">
      {settings.routing_strategy != null && (
        <div>
          {t("Routing Strategy:")} <Badge variant="secondary">{String(settings.routing_strategy)}</Badge>
        </div>
      )}
      {settings.num_retries != null && <div>{t("Number of Retries: {value0}", { value0: (String(settings.num_retries)) })}</div>}
      {settings.allowed_fails != null && <div>{t("Allowed Failures: {value0}", { value0: (String(settings.allowed_fails)) })}</div>}
      {settings.cooldown_time != null && <div>{t("Cooldown Time: {value0}s", { value0: (String(settings.cooldown_time)) })}</div>}
      {settings.timeout != null && <div>Timeout: {String(settings.timeout)}s</div>}
      {settings.retry_after != null && <div>{t("Retry After: {value0}s", { value0: (String(settings.retry_after)) })}</div>}
      {Boolean(settings.enable_tag_filtering) && <div>{t("Tag Filtering: Enabled")}</div>}
      {fallbacks.length > 0 && (
        <div>
          <div>{t("Fallbacks:")}</div>
          <div className="mt-1 space-y-1">
            {fallbacks.map(([model, targets]) => (
              <div key={model} className="text-xs text-muted-foreground">
                <span className="font-medium">{model}</span>
                <span className="mx-1 text-muted-foreground">-&gt;</span>
                {Array.isArray(targets) ? targets.join(", ") : String(targets)}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

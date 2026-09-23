import React from "react";
import { pricingIssueUrl, totalUnits, type UsageUnits } from "./usageUnits";
import { t } from "@/i18n";

export function UnpricedNote({ unpriced, provider }: { unpriced: UsageUnits; provider?: string }) {
  const total = totalUnits(unpriced);
  if (total === 0) return null;
  const [noun, verb] = total === 1 ? ["unit", "is"] : ["units", "are"];
  return (
    <p className="text-xs text-warning">
      {`${total.toLocaleString()} ${noun} with no known price ${verb} left out of the cost. `}
      <a
        href={pricingIssueUrl(unpriced, provider)}
        target="_blank"
        rel="noreferrer"
        className="underline underline-offset-2"
      >
        {t("Request pricing on GitHub")}
      </a>
    </p>
  );
}

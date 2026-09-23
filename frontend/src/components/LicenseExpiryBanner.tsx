"use client";

import React, { useState } from "react";
import { CircleAlert, TriangleAlert, X } from "lucide-react";
import { Alert, AlertAction, AlertDescription, AlertTitle } from "@/components/shared/Alert";
import { Button } from "@/components/ui/button";
import { LicenseInfo } from "@/components/networking";
import { formatExpiryDate, getDaysUntilExpiration, getLicenseExpiryTier } from "@/utils/licenseUtils";
import { t } from "@/i18n";

const DISMISS_KEY_PREFIX = "litellm:licenseExpiryBannerDismissed:";

interface LicenseExpiryBannerProps {
  accessToken: string | null;
}

interface LicenseExpiryBannerViewProps {
  licenseInfo: LicenseInfo | null;
}

const describeCountdown = (days: number): string => {
  if (days <= 0) return t("license.expiresToday");
  if (days === 1) return t("license.expiresInOneDay");
  return t("license.expiresInDays", { days });
};

const expiryDescription = (tier: "warning" | "critical" | "expired"): React.ReactNode => {
  if (tier === "expired") {
    return (
      <>
        {t("Enterprise features are now disabled.")}
      </>
    );
  }
  if (tier === "critical") {
    return (
      <>
        {t("Renew now to avoid losing enterprise features.")}
      </>
    );
  }
  return (
    <>
      {t("Renew before it lapses to keep enterprise features.")}
    </>
  );
};

export const LicenseExpiryBannerView: React.FC<LicenseExpiryBannerViewProps> = ({ licenseInfo }) => {
  const [locallyDismissed, setLocallyDismissed] = useState(false);

  const expirationDate = licenseInfo?.expiration_date ?? null;
  const tier = getLicenseExpiryTier(expirationDate);
  const days = getDaysUntilExpiration(expirationDate);

  if (expirationDate === null || tier === "none" || days === null) {
    return null;
  }

  const isDismissible = tier === "warning";
  const dismissKey = `${DISMISS_KEY_PREFIX}${expirationDate}`;
  const previouslyDismissed =
    isDismissible && typeof window !== "undefined" ? sessionStorage.getItem(dismissKey) === "true" : false;

  if (isDismissible && (locallyDismissed || previouslyDismissed)) {
    return null;
  }

  const formattedDate = formatExpiryDate(expirationDate);

  const message =
    tier === "expired"
      ? t("license.expiredOn", { date: formattedDate })
      : t("license.active", { when: describeCountdown(days), date: formattedDate });

  const description = expiryDescription(tier);

  const handleClose = () => {
    if (typeof window !== "undefined") {
      sessionStorage.setItem(dismissKey, "true");
    }
    setLocallyDismissed(true);
  };

  return (
    <Alert variant={tier === "warning" ? "warning" : "error"} className="rounded-none border-x-0 border-t-0">
      {tier === "warning" ? (
        <TriangleAlert className="size-4" aria-hidden />
      ) : (
        <CircleAlert className="size-4" aria-hidden />
      )}
      <AlertTitle>{message}</AlertTitle>
      <AlertDescription>{description}</AlertDescription>
      {isDismissible && (
        <AlertAction>
          <Button variant="ghost" size="icon-sm" aria-label={t("login.close")} onClick={handleClose}>
            <X className="size-4" />
          </Button>
        </AlertAction>
      )}
    </Alert>
  );
};

export const LicenseExpiryBanner: React.FC<LicenseExpiryBannerProps> = () => null;

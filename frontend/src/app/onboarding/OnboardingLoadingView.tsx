import React from "react";
import { UiLoadingSpinner } from "@/components/ui/ui-loading-spinner";
import { t } from "@/i18n";

export function OnboardingLoadingView() {
  return (
    <div className="mx-auto w-full max-w-md mt-10 flex justify-center">
      <UiLoadingSpinner role="status" aria-label={t("onboarding.loadingInvitation")} className="size-8 text-muted-foreground" />
    </div>
  );
}

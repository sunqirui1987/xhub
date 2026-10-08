"use client";

import AuditLogsPanel from "@/components/view_logs/AuditLogsPanel";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { CAPABILITIES, useSessionIdentity } from "@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity";
import { UiLoadingSpinner } from "@/components/ui/ui-loading-spinner";
import { useT } from "@/i18n";

export default function AuditLogsPage() {
  const t = useT();
  const { accessToken, userRole, userId, token } = useAuthorized();
  const identity = useSessionIdentity();
  if (!accessToken || !token || !userId || !userRole || identity.isLoading) {
    return (
      <div role="status" aria-label={t("Loading")} className="flex h-64 items-center justify-center">
        <UiLoadingSpinner className="size-8 text-primary" />
      </div>
    );
  }
  const allowed =
    identity.data?.capabilities?.includes(CAPABILITIES.platformAdmin) &&
    identity.data?.capabilities?.includes(CAPABILITIES.audit);
  if (!allowed) {
    return (
      <div role="alert" className="p-6 text-sm text-muted-foreground">
        {t("audit.adminOnly")}
      </div>
    );
  }
  return <AuditLogsPanel accessToken={accessToken} token={token} userRole={userRole} userID={userId} isActive />;
}

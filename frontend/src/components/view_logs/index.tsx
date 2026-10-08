import RequestLogsPanel from "./RequestLogsPanel";
import { UiLoadingSpinner } from "@/components/ui/ui-loading-spinner";
import { t } from "@/i18n";

interface SpendLogsTableProps {
  accessToken: string | null;
  token: string | null;
  userRole: string | null;
  userID: string | null;
  premiumUser: boolean;
}
export default function SpendLogsTable({ accessToken, token, userRole, userID }: SpendLogsTableProps) {
  if (!accessToken || !token || !userRole || !userID) {
    return (
      <div role="status" aria-busy="true" aria-label={t("Loading")} className="flex h-64 items-center justify-center">
        <UiLoadingSpinner className="size-8 text-primary" />
      </div>
    );
  }
  return (
    <div className="flex h-full w-full flex-col">
      <RequestLogsPanel accessToken={accessToken} token={token} userRole={userRole} userID={userID} isActive />
    </div>
  );
}

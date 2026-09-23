import { ExternalLink, Loader2 } from "lucide-react";

import { Alert, AlertDescription } from "@/components/shared/Alert";
import { Button } from "@/components/ui/button";
import { t } from "@/i18n";

interface PaginationStatusAlertsProps {
  isFetchingMore: boolean;
  cancelled: boolean;
  progress: { currentPage: number; totalPages: number };
  cancel: () => void;
  subject?: string;
}

const PaginationStatusAlerts = ({
  isFetchingMore,
  cancelled,
  progress,
  cancel,
  subject = "spend data",
}: PaginationStatusAlertsProps) => (
  <>
    {isFetchingMore && (
      <Alert variant="warning" className="mb-2">
        <AlertDescription className="flex items-center justify-between text-inherit">
          <span>
            <Loader2 className="mr-2 inline size-4 animate-spin align-text-bottom" />
            {t("Currently fetching")} {subject}{t(": fetched")} {progress.currentPage} / {progress.totalPages} {t("pages. Charts will update periodically as data loads. Moving off of this page will stop and reset this. To continue using the UI in the meantime,")}{" "}
            <a href={window.location.href} target="_blank" rel="noopener noreferrer">
              {t("open a new tab")} <ExternalLink className="inline size-3.5 align-text-bottom" />
            </a>
            .
          </span>
          <Button variant="destructive" onClick={cancel}>
            {t("Stop")}
          </Button>
        </AlertDescription>
      </Alert>
    )}
    {cancelled && (
      <Alert variant="info" className="mb-2">
        <AlertDescription className="text-inherit">
          Showing partial {subject} ({progress.currentPage}/{progress.totalPages} pages loaded)
        </AlertDescription>
      </Alert>
    )}
  </>
);

export default PaginationStatusAlerts;

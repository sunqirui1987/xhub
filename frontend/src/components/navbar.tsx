import { useHealthReadinessDetails } from "@/app/(dashboard)/hooks/healthReadiness/useHealthReadinessDetails";
import { useWorker } from "@/hooks/useWorker";
import { uiHref } from "@/utils/uiHref";
import { useTheme } from "@/contexts/ThemeContext";
import { clearTokenCookies } from "@/utils/cookieUtils";
import { clearStoredReturnUrl, getLoginUrl } from "@/utils/returnUrlUtils";
import useProxySettings from "@/app/(dashboard)/hooks/proxySettings/useProxySettings";
import { Badge } from "@/components/ui/badge";
import { PanelLeftClose, PanelLeftOpen } from "lucide-react";
import Link from "next/link";
import React from "react";
import { NotificationsBell } from "./Navbar/NotificationsBell/NotificationsBell";
import UserDropdown from "./Navbar/UserDropdown/UserDropdown";
import ThemeToggle from "./ThemeToggle/ThemeToggle";
import ViewSwitcher from "./Navbar/ViewSwitcher";
import WorkerDropdown from "./Navbar/WorkerDropdown/WorkerDropdown";
import { t } from "@/i18n";

interface NavbarProps {
  accessToken: string | null;
  isPublicPage: boolean;
  sidebarCollapsed?: boolean;
  onToggleSidebar?: () => void;
}

const NAV_LOGO_CLASS_NAME = "h-auto max-h-full w-auto max-w-full object-contain";

const Navbar: React.FC<NavbarProps> = ({
  accessToken,
  isPublicPage = false,
  sidebarCollapsed = false,
  onToggleSidebar,
}) => {
  const proxySettings = useProxySettings(accessToken);
  const { logoUrl } = useTheme();
  const { data: healthData } = useHealthReadinessDetails(accessToken);
  const version = healthData?.litellm_version;
  const { isControlPlane, selectedWorker } = useWorker();
  const showWorkerSwitch = isControlPlane && selectedWorker !== null;

  const handleLogout = () => {
    clearTokenCookies();
    localStorage.removeItem("litellm_selected_worker_id");
    localStorage.removeItem("litellm_worker_url");
    window.location.href = proxySettings.PROXY_LOGOUT_URL || "";
  };

  const handleWorkerSwitch = (workerId: string) => {
    clearTokenCookies();
    clearStoredReturnUrl();
    localStorage.removeItem("litellm_selected_worker_id");
    localStorage.removeItem("litellm_worker_url");
    window.location.href = `${getLoginUrl()}?worker=${encodeURIComponent(workerId)}`;
  };

  return (
    <nav className="sticky top-0 z-chrome border-b border-border bg-card">
      <div className="w-full">
        <div className="flex h-14 items-center px-4">
          <div className="flex shrink-0 items-center">
            {onToggleSidebar && (
              <button
                onClick={onToggleSidebar}
                className="mr-2 flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                title={sidebarCollapsed ? t("Expand sidebar") : t("Collapse sidebar")}
              >
                <span className="text-lg">
                  {sidebarCollapsed ? (
                    <PanelLeftOpen className="size-[18px]" />
                  ) : (
                    <PanelLeftClose className="size-[18px]" />
                  )}
                </span>
              </button>
            )}

            <div className="flex items-center gap-2">
              <Link href={uiHref("")} className="flex items-center" aria-label={t("site.homeAria")}>
                {logoUrl ? (
                  <div className="flex h-10 max-w-48 items-center justify-center overflow-hidden">
                    <img src={logoUrl} alt={t("site.logoAlt")} className={NAV_LOGO_CLASS_NAME} />
                  </div>
                ) : (
                  <span className="text-base font-semibold tracking-tight text-foreground">{t("site.product")}</span>
                )}
              </Link>
              {version && (
                <Badge variant="outline" className="text-xs font-medium">
                  v{version}
                </Badge>
              )}
            </div>
          </div>

          {!isPublicPage && (
            <div className="ml-4 flex shrink-0 items-center border-l border-border pl-4">
              <ViewSwitcher />
            </div>
          )}

          <div className="ml-auto flex min-w-0 flex-1 items-center justify-end gap-4">
            {showWorkerSwitch && (
              <div className="flex shrink-0 items-center">
                <WorkerDropdown onWorkerSwitch={handleWorkerSwitch} />
              </div>
            )}

            {!isPublicPage && (
              <div className="flex shrink-0 items-center border-l border-border pl-4">
                <div className="flex items-center gap-0.5 rounded-lg bg-muted px-1 py-0 transition-colors hover:bg-accent">
                  <ThemeToggle />
                  <span className="mx-0.5 h-6 w-px shrink-0 bg-border" aria-hidden />
                  <NotificationsBell />
                  <span className="mx-0.5 h-6 w-px shrink-0 bg-border" aria-hidden />
                  <UserDropdown onLogout={handleLogout} />
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </nav>
  );
};

export default Navbar;

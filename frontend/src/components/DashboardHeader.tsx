"use client";

import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { ToolbarSeparator } from "@/components/shared/ToolbarSeparator";
import { getBreadcrumb } from "@/components/leftnav";
import { BlogDropdown } from "@/components/Navbar/BlogDropdown/BlogDropdown";
import { DocsLink } from "@/components/Navbar/DocsLink/DocsLink";
import { NotificationsBell } from "@/components/Navbar/NotificationsBell/NotificationsBell";
import ViewSwitcher from "@/components/Navbar/ViewSwitcher";
import ThemeToggle from "@/components/ThemeToggle/ThemeToggle";
import LanguageSwitcher from "@/components/LanguageSwitcher";
import { t } from "@/i18n";
import WorkerDropdown from "@/components/Navbar/WorkerDropdown/WorkerDropdown";
import { useWorker } from "@/hooks/useWorker";
import { clearTokenCookies } from "@/utils/cookieUtils";
import { clearStoredReturnUrl, getLoginUrl } from "@/utils/returnUrlUtils";
import { usePathname } from "next/navigation";

// Top bar over the content column. The product name lives in the sidebar, so this
// bar starts with the view switcher and the current page.
export function DashboardHeader() {
  const { title } = getBreadcrumb(usePathname(), t);
  const { isControlPlane, selectedWorker } = useWorker();
  const showWorkerSwitch = isControlPlane && selectedWorker !== null;

  const handleWorkerSwitch = (workerId: string) => {
    clearTokenCookies();
    clearStoredReturnUrl();
    localStorage.removeItem("litellm_selected_worker_id");
    localStorage.removeItem("litellm_worker_url");
    window.location.href = `${getLoginUrl()}?worker=${encodeURIComponent(workerId)}`;
  };

  return (
    <header
      data-testid="admin-header"
      className="flex h-16 flex-none items-center justify-between gap-6 border-b border-border bg-card px-5 shadow-sm"
    >
      <Breadcrumb className="min-w-0 flex-1">
        <BreadcrumbList className="flex-nowrap items-center gap-3 text-base">
          <BreadcrumbItem className="flex-none">
            <ViewSwitcher />
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem className="min-w-0">
            <BreadcrumbPage className="truncate text-lg font-semibold text-foreground">{title}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>

      <div className="flex flex-none items-center gap-1">
        {showWorkerSwitch && (
          <>
            <WorkerDropdown onWorkerSwitch={handleWorkerSwitch} />
            <ToolbarSeparator />
          </>
        )}
        <DocsLink />
        <BlogDropdown />
        <ToolbarSeparator />
        <LanguageSwitcher />
        <ThemeToggle />
        <NotificationsBell />
      </div>
    </header>
  );
}

export default DashboardHeader;

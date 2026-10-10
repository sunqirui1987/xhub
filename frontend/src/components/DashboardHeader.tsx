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
import ViewSwitcher from "@/components/Navbar/ViewSwitcher";
import ThemeToggle from "@/components/ThemeToggle/ThemeToggle";
import LanguageSwitcher from "@/components/LanguageSwitcher";
import { useT } from "@/i18n";
import WorkerDropdown from "@/components/Navbar/WorkerDropdown/WorkerDropdown";
import { useWorker } from "@/hooks/useWorker";
import { clearTokenCookies } from "@/utils/cookieUtils";
import { clearStoredReturnUrl, getLoginUrl } from "@/utils/returnUrlUtils";
import { usePathname } from "next/navigation";

/** 展示当前页导航和语言入口；无参数，返回订阅语言的顶部栏。
 * Dashboard 布局调用，语言切换保留页面状态；工作节点切换清除旧凭据并导航到登录页。 */
export function DashboardHeader() {
  const t = useT();
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
      className="flex h-16 flex-none items-center justify-between gap-2 border-b border-border bg-card px-2 shadow-sm sm:gap-6 sm:px-5"
    >
      <Breadcrumb className="min-w-0 flex-1">
        <BreadcrumbList className="flex-nowrap items-center gap-3 text-base">
          <BreadcrumbItem className="hidden flex-none sm:flex">
            <ViewSwitcher />
          </BreadcrumbItem>
          <BreadcrumbSeparator className="hidden sm:block" />
          <BreadcrumbItem className="min-w-0">
            <BreadcrumbPage className="truncate text-sm font-semibold text-foreground sm:text-lg">
              {title}
            </BreadcrumbPage>
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
      </div>
    </header>
  );
}

export default DashboardHeader;

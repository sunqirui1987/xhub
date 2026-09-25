import { useTeams } from "@/app/(dashboard)/hooks/teams/useTeams";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import useIsOrgAdmin from "@/app/(dashboard)/hooks/useIsOrgAdmin";
import { useHealthReadinessDetails } from "@/app/(dashboard)/hooks/healthReadiness/useHealthReadinessDetails";
import { useLogout } from "@/app/(dashboard)/hooks/useLogout";
import { useTheme } from "@/contexts/ThemeContext";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Sidebar,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarSeparator,
  sidebarMenuButtonVariants,
} from "@/components/shared/Sidebar";
import {
  Activity,
  BarChart3,
  Bell,
  Building2,
  Boxes,
  ChevronRight,
  ExternalLink,
  Folder,
  HeartPulse,
  KeyRound,
  Lock,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  PiggyBank,
  PlayCircle,
  Route,
  Settings as SettingsIcon,
  Shield,
  User,
  Users,
} from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useMemo, useState } from "react";
import { cn } from "@/lib/cva.config";
import { rolesWithCapability } from "../utils/capabilities";
import {
  all_admin_roles,
  internalUserRoles,
  isAdminRole,
  isUserTeamAdminForAnyTeam,
  rolesAllowedToViewWriteScopedPages,
  rolesWithWriteAccess,
} from "../utils/roles";
import BetaBadge from "./BetaBadge";
import SidebarAccountMenu from "./SidebarAccountMenu/SidebarAccountMenu";
import SidebarUsageCard from "./SidebarUsageCard";
import { routeSegmentForPathname, uiHref } from "@/utils/uiHref";
import { useT, t as tRuntime, type TFunction } from "@/i18n";

const ICON = { strokeWidth: 1.75 } as const;

const LOGO_CLASS_NAME = "h-7 w-auto max-w-[150px] object-contain group-data-[collapsed=true]/sidebar:w-7";

interface SidebarProps {
  collapsed?: boolean;
  onToggleCollapsed?: () => void;
  enabledPagesInternalUsers?: string[] | null;
  enableProjectsUI?: boolean;
  disableAgentsForInternalUsers?: boolean;
  allowAgentsForTeamAdmins?: boolean;
  disableVectorStoresForInternalUsers?: boolean;
  allowVectorStoresForTeamAdmins?: boolean;
}

interface MenuItem {
  key: string;
  page: string;
  route?: string;
  label: string;
  beta?: boolean;
  roles?: string[];
  children?: MenuItem[];
  icon?: React.ReactNode;
  external_url?: string;
}

interface MenuGroup {
  groupLabel: string;
  items: MenuItem[];
  roles?: string[];
}

// Menu groups organized by category - defined outside component for export.
// Shape (key/page/label/roles/children) is consumed by page_utils.ts; only the
// icons changed to lucide as part of the sidebar redesign.
const menuGroups: MenuGroup[] = [
  {
    groupLabel: "nav.groups.gateway",
    items: [
      { key: "api-keys", page: "api-keys", label: "nav.apiKeys", icon: <KeyRound {...ICON} /> },
      {
        key: "llm-playground",
        page: "llm-playground",
        route: "playground",
        label: "nav.playground",
        icon: <PlayCircle {...ICON} />,
        roles: rolesWithWriteAccess,
      },
      {
        key: "models",
        page: "models",
        route: "models-and-endpoints",
        label: "nav.models",
        icon: <Network {...ICON} />,
        roles: rolesAllowedToViewWriteScopedPages,
      },
      { key: "guardrails", page: "guardrails", label: "nav.guardrails", icon: <Shield {...ICON} /> },
    ],
  },
  {
    groupLabel: "nav.groups.observability",
    items: [
      {
        key: "new_usage",
        page: "new_usage",
        route: "usage",
        icon: <BarChart3 {...ICON} />,
        roles: [...all_admin_roles, ...internalUserRoles],
        label: "nav.usage",
      },
      {
        key: "cost-optimization",
        page: "cost-optimization",
        icon: <PiggyBank {...ICON} />,
        roles: [...all_admin_roles, ...internalUserRoles],
        label: "nav.costOptimization",
      },
      { key: "logs", page: "logs", label: "nav.logs", icon: <Activity {...ICON} /> },
      {
        key: "guardrails-monitor",
        page: "guardrails-monitor",
        label: "nav.guardrailsMonitor",
        icon: <HeartPulse {...ICON} />,
        roles: rolesWithCapability("viewGuardrailUsage"),
      },
    ],
  },
  {
    groupLabel: "nav.groups.access",
    items: [
      { key: "teams", page: "teams", label: "nav.teams", icon: <Users {...ICON} /> },
      {
        key: "projects",
        page: "projects",
        label: "nav.projects",
        beta: true,
        icon: <Folder {...ICON} />,
        roles: all_admin_roles,
      },
      { key: "users", page: "users", label: "nav.users", icon: <User {...ICON} />, roles: all_admin_roles },
      {
        key: "organizations",
        page: "organizations",
        label: "nav.organizations",
        icon: <Building2 {...ICON} />,
        roles: all_admin_roles,
      },
      {
        key: "access-groups",
        page: "access-groups",
        label: "nav.accessGroups",
        icon: <Boxes {...ICON} />,
        roles: all_admin_roles,
      },
    ],
  },
  {
    groupLabel: "nav.groups.settings",
    roles: all_admin_roles,
    items: [
      {
        key: "settings",
        page: "settings",
        label: "nav.settings",
        icon: <SettingsIcon {...ICON} />,
        roles: all_admin_roles,
        children: [
          {
            key: "router-settings",
            page: "router-settings",
            label: "nav.routerSettings",
            icon: <Route {...ICON} />,
            roles: all_admin_roles,
          },
          {
            key: "logging-and-alerts",
            page: "logging-and-alerts",
            label: "nav.loggingAndAlerts",
            icon: <Bell {...ICON} />,
            roles: all_admin_roles,
          },
          {
            key: "cost-tracking",
            page: "cost-tracking",
            label: "nav.costTracking",
            icon: <BarChart3 {...ICON} />,
            roles: all_admin_roles,
          },
          {
            key: "admin-panel",
            page: "admin-panel",
            label: "nav.adminPanel",
            icon: <Lock {...ICON} />,
            roles: all_admin_roles,
          },
        ],
      },
    ],
  },
];

const HOME_ROUTE = "api-keys";

const routeOf = (item: MenuItem): string => item.route ?? item.page;

const routeForPathname = (pathname: string): string => routeSegmentForPathname(pathname) || HOME_ROUTE;

const findParentKey = (route: string): string | null => {
  for (const group of menuGroups) {
    for (const item of group.items) {
      if (item.children?.some((c) => routeOf(c) === route)) return item.key;
    }
  }
  return null;
};

const findMenuItemKey = (route: string): string => {
  for (const group of menuGroups) {
    for (const item of group.items) {
      if (routeOf(item) === route) return item.key;
      const child = item.children?.find((c) => routeOf(c) === route);
      if (child) return child.key;
    }
  }
  return HOME_ROUTE;
};

const prettify = (key: string): string =>
  key
    .split(/[-_]/)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");

const labelText = (item: MenuItem, t: TFunction = tRuntime): string => t(item.label);

const itemLabel = (item: MenuItem, t: TFunction) => (
  <span className="flex flex-1 items-center gap-2 truncate group-data-[collapsed=true]/sidebar:hidden">
    {labelText(item, t)}
    {item.beta ? <BetaBadge /> : null}
  </span>
);

// Breadcrumb ("Section" / "Page") for the top bar, derived from the same nav config.
export const getBreadcrumb = (pathname: string, t: TFunction = tRuntime): { section: string | null; title: string } => {
  const route = routeForPathname(pathname);
  for (const group of menuGroups) {
    for (const item of group.items) {
      const section = t(group.groupLabel);
      if (routeOf(item) === route) return { section, title: labelText(item, t) };
      const child = item.children?.find((c) => routeOf(c) === route);
      if (child) return { section, title: labelText(child, t) };
    }
  }
  return { section: null, title: prettify(route) };
};

const Sidebar_: React.FC<SidebarProps> = ({
  collapsed = false,
  onToggleCollapsed,
  enabledPagesInternalUsers,
  enableProjectsUI,
  disableAgentsForInternalUsers,
  allowAgentsForTeamAdmins,
  disableVectorStoresForInternalUsers,
  allowVectorStoresForTeamAdmins,
}) => {
  const t = useT();
  const { userId, accessToken, userRole, isViewOnly } = useAuthorized();
  const isOrgAdmin = useIsOrgAdmin();
  const { data: teams } = useTeams();
  const { logoUrl, logoUrlDark } = useTheme();
  const [erroredDarkLogo, setErroredDarkLogo] = useState<string | null>(null);
  const { data: healthData } = useHealthReadinessDetails(accessToken);
  const logout = useLogout(accessToken);

  const version = healthData?.litellm_version;
  const currentRoute = routeForPathname(usePathname());
  const selectedKey = findMenuItemKey(currentRoute);

  const [openGroups, setOpenGroups] = useState<Set<string>>(() => {
    const parent = findParentKey(currentRoute);
    return new Set(parent ? [parent] : []);
  });

  // Keep the active page's parent group expanded as the user navigates, using the
  // "adjust state during render" pattern rather than an effect (avoids a
  // setState-in-effect render cascade).
  const [prevRoute, setPrevRoute] = useState(currentRoute);
  if (currentRoute !== prevRoute) {
    setPrevRoute(currentRoute);
    const parent = findParentKey(currentRoute);
    if (parent && !openGroups.has(parent)) {
      setOpenGroups((prev) => new Set(prev).add(parent));
    }
  }

  const isTeamAdmin = useMemo(() => isUserTeamAdminForAnyTeam(teams ?? null, userId ?? ""), [teams, userId]);

  const filterItemsByRole = (items: MenuItem[]): MenuItem[] => {
    const isAdmin = isAdminRole(userRole);
    return items
      .map((item) => ({ ...item, children: item.children ? filterItemsByRole(item.children) : undefined }))
      .filter((item) => {
        // A parent whose children were all filtered out renders as a leaf link
        // to its own page id, which is not a real route. Drop it instead.
        if (item.children && item.children.length === 0) return false;
        if (item.key === "llm-playground" && isViewOnly) return false;
        if (item.key === "organizations" || item.key === "users") {
          const hasRoleAccess = !item.roles || item.roles.includes(userRole) || isOrgAdmin;
          if (!hasRoleAccess) return false;
          if (!isAdmin && enabledPagesInternalUsers != null) return enabledPagesInternalUsers.includes(item.page);
          return true;
        }
        if (item.key === "projects" && !enableProjectsUI) return false;
        if (
          !isAdmin &&
          item.key === "agents" &&
          disableAgentsForInternalUsers &&
          !(allowAgentsForTeamAdmins && isTeamAdmin)
        )
          return false;
        if (
          !isAdmin &&
          item.key === "vector-stores" &&
          disableVectorStoresForInternalUsers &&
          !(allowVectorStoresForTeamAdmins && isTeamAdmin)
        )
          return false;
        if (item.roles && !item.roles.includes(userRole)) return false;
        if (!isAdmin && enabledPagesInternalUsers != null) {
          if (item.children && item.children.length > 0) {
            const hasVisibleChildren = item.children.some((child) => enabledPagesInternalUsers.includes(child.page));
            if (hasVisibleChildren) return true;
          }
          return enabledPagesInternalUsers.includes(item.page);
        }
        return true;
      });
  };

  const visibleGroups = menuGroups
    .filter((group) => !group.roles || group.roles.includes(userRole))
    .map((group) => ({ groupLabel: group.groupLabel, items: filterItemsByRole(group.items) }))
    .filter((group) => group.items.length > 0);

  const toggleGroup = (key: string) => {
    if (collapsed) {
      onToggleCollapsed?.();
      setOpenGroups((prev) => new Set(prev).add(key));
      return;
    }
    setOpenGroups((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const renderLeaf = (item: MenuItem, isChild: boolean) => {
    const active = selectedKey === item.key;
    const size = isChild ? "sub" : "default";
    const label = itemLabel(item, t);

    if (item.external_url) {
      return (
        <a
          key={item.key}
          href={item.external_url}
          target="_blank"
          rel="noopener noreferrer"
          title={collapsed ? labelText(item, t) : undefined}
          data-active={active || undefined}
          className={cn(sidebarMenuButtonVariants({ isActive: active, size }))}
        >
          {item.icon}
          {label}
          <ExternalLink className="size-3.5 shrink-0 opacity-70 group-data-[collapsed=true]/sidebar:hidden" />
        </a>
      );
    }

    return (
      <Link
        key={item.key}
        href={uiHref(routeOf(item))}
        title={collapsed ? labelText(item, t) : undefined}
        data-active={active || undefined}
        className={cn(sidebarMenuButtonVariants({ isActive: active, size }))}
      >
        {item.icon}
        {label}
      </Link>
    );
  };

  const renderItem = (item: MenuItem) => {
    const isGroup = !!item.children && item.children.length > 0;
    if (!isGroup) {
      return <SidebarMenuItem key={item.key}>{renderLeaf(item, false)}</SidebarMenuItem>;
    }

    const active = selectedKey === item.key;
    const open = openGroups.has(item.key);
    return (
      <SidebarMenuItem key={item.key}>
        <SidebarMenuButton
          isActive={active}
          aria-expanded={open}
          onClick={() => toggleGroup(item.key)}
          title={collapsed ? labelText(item, t) : undefined}
        >
          {item.icon}
          {itemLabel(item, t)}
          <ChevronRight
            className={cn(
              "size-4 shrink-0 transition-transform group-data-[collapsed=true]/sidebar:hidden",
              open && "rotate-90",
            )}
          />
        </SidebarMenuButton>
        {open && (
          <SidebarMenuSub>
            {item.children!.map((child) => (
              <SidebarMenuItem key={child.key}>{renderLeaf(child, true)}</SidebarMenuItem>
            ))}
          </SidebarMenuSub>
        )}
      </SidebarMenuItem>
    );
  };

  const reachableDarkLogo = logoUrlDark && logoUrlDark !== erroredDarkLogo ? logoUrlDark : null;
  const darkLogoSrc = reachableDarkLogo || logoUrl;

  return (
    <Sidebar
      collapsed={collapsed}
      className="border-r-0 bg-[#343a40] text-[#c2c7d0] shadow-[2px_0_8px_rgba(0,0,0,0.15)]"
    >
      <SidebarHeader className="h-16 justify-center border-b border-sidebar-border bg-sidebar px-4 group-data-[collapsed=true]/sidebar:h-auto">
        <div className="flex items-center justify-between gap-2 group-data-[collapsed=true]/sidebar:flex-col">
          <div className="flex min-w-0 items-center gap-2">
            <Link href={uiHref("")} className="flex min-w-0 items-center" aria-label={t("site.homeAria")}>
              {logoUrl ? (
                <>
                  <img src={logoUrl} alt={t("site.logoAlt")} className={cn(LOGO_CLASS_NAME, "dark:hidden")} />
                  <img
                    src={darkLogoSrc || logoUrl}
                    alt=""
                    aria-hidden
                    onError={() => setErroredDarkLogo(logoUrlDark)}
                    className={cn(LOGO_CLASS_NAME, "hidden dark:block")}
                  />
                </>
              ) : (
                <span className="truncate text-lg font-semibold tracking-tight text-sidebar-foreground group-data-[collapsed=true]/sidebar:hidden">
                  {t("site.product")}
                </span>
              )}
            </Link>
            {version && (
              <Badge
                variant="outline"
                className="border-sidebar-border px-1.5 py-0 font-mono text-[10px] font-medium text-sidebar-foreground group-data-[collapsed=true]/sidebar:hidden"
              >
                v{version}
              </Badge>
            )}
          </div>
          {onToggleCollapsed && (
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onToggleCollapsed}
              aria-label={collapsed ? t("header.expandSidebar") : t("header.collapseSidebar")}
              className="flex-none text-sidebar-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
            >
              {collapsed ? <PanelLeftOpen /> : <PanelLeftClose />}
            </Button>
          )}
        </div>
      </SidebarHeader>

      <nav className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto px-2 py-2 [scrollbar-color:#4b545c_transparent]">
          {visibleGroups.map((group, gi) => (
            <SidebarGroup key={group.groupLabel}>
              {gi > 0 && <SidebarSeparator className="hidden group-data-[collapsed=true]/sidebar:block" />}
              <SidebarGroupLabel>{t(group.groupLabel)}</SidebarGroupLabel>
              <SidebarMenu>{group.items.map((item) => renderItem(item))}</SidebarMenu>
            </SidebarGroup>
          ))}
      </nav>

      <SidebarFooter>
        {isAdminRole(userRole) && (
          <SidebarUsageCard
            accessToken={accessToken}
            collapsed={collapsed}
            onExpandRail={() => onToggleCollapsed?.()}
          />
        )}
        <SidebarAccountMenu onLogout={logout} collapsed={collapsed} />
      </SidebarFooter>
    </Sidebar>
  );
};

export default Sidebar_;

export { menuGroups };

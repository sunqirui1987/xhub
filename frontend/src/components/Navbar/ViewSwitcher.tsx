import React, { useSyncExternalStore } from "react";
import { usePathname } from "next/navigation";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Check, ChevronsUpDown, LayoutGrid } from "lucide-react";
import { usePluginMode } from "@/contexts/PluginModeContext";
import { uiHref, routeSegmentForPathname } from "@/utils/uiHref";
import { useT } from "@/i18n";

const GATEWAY = "ai-gateway";
const CHAT = "chat";

interface ViewSwitcherItem {
  key: string;
  label: React.ReactNode;
  disabled?: boolean;
  onClick?: () => void;
}

/** 展示工作区切换入口；无参数，返回订阅当前语言的菜单；导航调用，切换工作区时更新路由。 */
export default function ViewSwitcher() {
  const t = useT();
  const { mode, setMode, plugins } = usePluginMode();
  const pathname = usePathname();

  // A /ui rewrite may expose a different pathname during server rendering.
  // Keep the first client label identical to the server, then read the route.
  const hydrated = useSyncExternalStore(() => () => {}, () => true, () => false);
  const isChatRoute = hydrated && routeSegmentForPathname(pathname ?? "") === CHAT;

  const activeLabel = isChatRoute ? t("header.viewChat") : plugins.find((p) => p.name === mode)?.display_name ?? t("header.viewGateway");

  const modeEntries = [
    { key: GATEWAY, label: t("header.viewGateway") },
    ...plugins.map((p) => ({ key: p.name, label: p.display_name })),
  ];

  const selectMode = (key: string) => {
    setMode(key);
    // The chat route lives outside the dashboard SPA shell that reacts to `mode`,
    // so switching modes from there needs a real navigation, not just state.
    if (isChatRoute) {
      window.location.assign(uiHref(""));
    }
  };

  const chatItem: ViewSwitcherItem = {
    key: CHAT,
    label: (
      <div className="flex items-center justify-between gap-6 py-0.5">
        <span className="font-medium">{t("header.viewChat")}</span>
        {isChatRoute && <Check className="size-4 text-info" />}
      </div>
    ),
    onClick: () => window.location.assign(uiHref(CHAT)),
  };

  const items: ViewSwitcherItem[] = [
    ...modeEntries.map((e) => ({
      key: e.key,
      label: (
        <div className="flex items-center justify-between gap-6 py-0.5">
          <span className="font-medium">{e.label}</span>
          {!isChatRoute && e.key === mode && <Check className="size-4 text-info" />}
        </div>
      ),
      onClick: () => selectMode(e.key),
    })),
    chatItem,
  ];

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <button
            type="button"
            className="flex h-10 max-w-[240px] items-center gap-2 rounded-sm px-2 text-base font-medium text-foreground transition-colors hover:bg-accent"
          />
        }
      >
        <LayoutGrid className="size-4 flex-none text-primary" />
        <span className="truncate">{activeLabel}</span>
        <ChevronsUpDown className="size-4 flex-none text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-auto">
        {items.map((item) => (
          <DropdownMenuItem key={item.key} disabled={item.disabled} onClick={item.onClick}>
            {item.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

import React from "react";
import { usePathname } from "next/navigation";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Check, ChevronsUpDown, LayoutGrid } from "lucide-react";
import { usePluginMode } from "@/contexts/PluginModeContext";
import { useUISettings } from "@/app/(dashboard)/hooks/uiSettings/useUISettings";
import { uiHref } from "@/utils/uiHref";
import { t } from "@/i18n";

const GATEWAY = "ai-gateway";
const CHAT = "chat";

interface ViewSwitcherItem {
  key: string;
  label: React.ReactNode;
  disabled?: boolean;
  onClick?: () => void;
}

export default function ViewSwitcher() {
  const { mode, setMode, plugins } = usePluginMode();
  const { data: uiSettings } = useUISettings();
  const pathname = usePathname();

  const chatEnabled = Boolean(uiSettings?.values?.enable_chat_ui);

  const chatHref = uiHref(CHAT);
  const normalizedPathname = (pathname ?? "").replace(/\/+$/, "");
  const isChatRoute = chatEnabled && (normalizedPathname === chatHref || normalizedPathname.startsWith(`${chatHref}/`));

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

  const chatItem: ViewSwitcherItem = chatEnabled
    ? {
        key: CHAT,
        label: (
          <div className="flex items-center justify-between gap-6 py-0.5">
            <span className="font-medium">{t("header.viewChat")}</span>
            {isChatRoute && <Check className="size-4 text-info" />}
          </div>
        ),
        onClick: () => window.location.assign(uiHref(CHAT)),
      }
    : {
        key: CHAT,
        disabled: true,
        label: (
          <div className="flex max-w-[220px] flex-col py-0.5">
            <span className="font-medium">{t("header.viewChat")}</span>
            <span className="whitespace-normal text-xs leading-snug text-muted-foreground">
              {t("header.chatEnableHint")}
            </span>
          </div>
        ),
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

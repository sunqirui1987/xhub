import React from "react";
import { hasCapability, type Capability } from "@/utils/capabilities";
import { all_admin_roles } from "@/utils/roles";
import { t } from "@/i18n";
export type UsageOption =
  | "global"
  | "my-usage"
  | "organization"
  | "team"
  | "customer"
  | "tag"
  | "user";
export interface UsageViewSelectProps {
  value: UsageOption;
  onChange: (value: UsageOption) => void;
  userRole: string | null;
  canViewTagUsage?: boolean;
  isOrgAdmin?: boolean;
  title?: string;
  description?: string;
  "data-id"?: string;
}
interface OptionConfig {
  value: UsageOption;
  labelKey: string;
  descriptionKey: string;
  capability?: Capability;
  adminOnly?: boolean;
  showForAdminKey?: string;
  showForNonAdminKey?: string;
  descriptionForAdminKey?: string;
  descriptionForNonAdminKey?: string;
  badgeText?: string;
}
const OPTIONS: OptionConfig[] = [
  {
    value: "global",
    labelKey: "pages.usage.global",
    showForAdminKey: "pages.usage.global",
    showForNonAdminKey: "pages.usage.yours",
    descriptionKey: "pages.usage.acrossAll",
    descriptionForAdminKey: "pages.usage.acrossAll",
    descriptionForNonAdminKey: "pages.usage.viewYours",
  },
  {
    value: "my-usage",
    labelKey: "pages.usage.yours",
    descriptionKey: "pages.usage.yourOwn",
    adminOnly: true,
  },
  {
    value: "organization",
    labelKey: "pages.usage.organization",
    descriptionKey: "pages.usage.organizationDesc",
    capability: "viewOrganizationUsage",
  },
  {
    value: "team",
    labelKey: "pages.usage.team",
    descriptionKey: "pages.usage.teamDesc",
  },
  {
    value: "customer",
    labelKey: "pages.usage.customer",
    descriptionKey: "pages.usage.customerDesc",
    adminOnly: true,
  },
  {
    value: "tag",
    labelKey: "pages.usage.tag",
    descriptionKey: "pages.usage.tagDesc",
    adminOnly: true,
  },
  {
    value: "user",
    labelKey: "pages.usage.user",
    descriptionKey: "pages.usage.userDesc",
    adminOnly: true,
  },
];
export const UsageViewSelect: React.FC<UsageViewSelectProps> = ({
  value,
  onChange,
  userRole,
  canViewTagUsage = false,
  isOrgAdmin = false,
  title,
  description,
  "data-id": dataId,
}) => {
  const isAdmin = all_admin_roles.includes(userRole ?? "");
  const getFilteredOptions = () => {
    return OPTIONS.filter((option) => {
      if (option.capability) {
        return hasCapability(userRole, option.capability, isOrgAdmin);
      }
      if (option.value === "tag" && canViewTagUsage) {
        return true;
      }
      if (option.adminOnly && !isAdmin) {
        return false;
      }
      return true;
    }).map((option) => {
      let label = t(option.labelKey);
      let desc = t(option.descriptionKey);
      if (option.showForAdminKey && option.showForNonAdminKey) {
        label = isAdmin ? t(option.showForAdminKey) : t(option.showForNonAdminKey);
      }
      if (option.descriptionForAdminKey && option.descriptionForNonAdminKey) {
        desc = isAdmin ? t(option.descriptionForAdminKey) : t(option.descriptionForNonAdminKey);
      }
      return {
        value: option.value,
        label,
        description: desc,
      };
    });
  };
  const filteredOptions = getFilteredOptions();
  return (
    <div className="w-full" data-id={dataId}>
      <div className="flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1">
        <h2 className="text-sm font-medium text-muted-foreground">{title ?? t("pages.usage.viewTitle")}</h2>
        <p className="text-sm text-muted-foreground">{description ?? t("pages.usage.viewDescription")}</p>
      </div>
      <div className="mt-3 flex flex-wrap gap-2" role="tablist" aria-label={title ?? t("pages.usage.viewTitle")}>
        {filteredOptions.map((option) => {
          const selected = option.value === value;
          return (
            <button
              key={option.value}
              type="button"
              role="tab"
              aria-selected={selected}
              onClick={() => onChange(option.value)}
              className={
                selected
                  ? "rounded-full bg-foreground px-4 py-2 text-sm font-medium text-background"
                  : "rounded-full bg-muted px-4 py-2 text-sm text-muted-foreground hover:text-foreground"
              }
            >
              {option.label}
            </button>
          );
        })}
      </div>
    </div>
  );
};

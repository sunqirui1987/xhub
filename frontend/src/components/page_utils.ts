/**
 * Utility functions for working with navigation pages
 */

import { menuGroups } from "./leftnav";
import { PageMetadata } from "./page_metadata";
import { internalUserRoles } from "@/utils/roles";
import { t } from "@/i18n";

/**
 * Check if a page is accessible to internal users
 * A page is accessible if:
 * 1. It has no role restrictions, OR
 * 2. Its roles include at least one internal user role
 */
const isPageAccessibleToInternalUsers = (pageRoles?: string[]): boolean => {
  if (!pageRoles || pageRoles.length === 0) {
    return true; // No role restrictions
  }

  // Check if any of the page's roles match internal user roles
  return pageRoles.some((role) => internalUserRoles.includes(role));
};

/**
 * Get all available pages from the navigation menu configuration
 * Used by UI Settings to display available pages for visibility control
 *
 * IMPORTANT: Only returns pages that internal users can access.
 * Pages restricted to admin-only roles are excluded because internal users
 * cannot see them regardless of the UI visibility setting.
 */
export const getAvailablePages = (): PageMetadata[] => {
  const pages: PageMetadata[] = [];

  menuGroups.forEach((group) => {
    group.items.forEach((item) => {
      // Add top-level items (skip parent containers like 'tools', 'experimental', 'settings')
      // Also skip items that internal users cannot access
      if (
        item.page &&
        item.page !== "tools" &&
        item.page !== "experimental" &&
        item.page !== "settings" &&
        isPageAccessibleToInternalUsers(item.roles)
      ) {
        pages.push({
          page: item.page,
          label: t(item.label),
          group: t(group.groupLabel),
          description: t(`desc.${item.page}`) || t("common.noDescription"),
        });
      }

      // Add children items (also skip those internal users cannot access)
      if (item.children) {
        const parentLabel = typeof item.label === "string" ? item.label : item.key;
        item.children.forEach((child) => {
          // Include if internal users can access
          if (isPageAccessibleToInternalUsers(child.roles)) {
            pages.push({
              page: child.page,
              label: t(child.label),
              group: `${t(group.groupLabel)} > ${t(parentLabel)}`,
              description: t(`desc.${child.page}`) || t("common.noDescription"),
            });
          }
        });
      }
    });
  });

  return pages;
};

"use client";

import Sidebar from "@/components/leftnav";
import { getUISettings } from "@/components/networking";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { isAdminRole } from "@/utils/roles";
import { useEffect, useState } from "react";

interface SidebarProviderProps {
  sidebarCollapsed: boolean;
  onToggleCollapsed?: () => void;
}

const SidebarProvider = ({ sidebarCollapsed, onToggleCollapsed }: SidebarProviderProps) => {
  const { accessToken, userRole } = useAuthorized();
  const [enabledPagesInternalUsers, setEnabledPagesInternalUsers] = useState<string[] | null>(null);
  const [disableAgentsForInternalUsers, setDisableAgentsForInternalUsers] = useState<boolean>(false);
  const [allowAgentsForTeamAdmins, setAllowAgentsForTeamAdmins] = useState<boolean>(false);
  const [disableVectorStoresForInternalUsers, setDisableVectorStoresForInternalUsers] = useState<boolean>(false);
  const [allowVectorStoresForTeamAdmins, setAllowVectorStoresForTeamAdmins] = useState<boolean>(false);

  // These flags decide which pages the deployment shows to a team admin and a
  // member, so only a platform administrator may read them. Asking as anybody
  // else answered 403 and logged a console error on every page load; the state
  // then simply keeps its defaults, which are the ones a non-administrator
  // would be told to use anyway.
  const canReadUISettings = userRole ? isAdminRole(userRole) : false;

  useEffect(() => {
    const fetchUISettings = async () => {
      if (!accessToken || !canReadUISettings) {
        return;
      }

      try {
        const settings = await getUISettings(accessToken);

        // API returns 'values' not 'settings'
        if (settings?.values?.enabled_ui_pages_internal_users !== undefined) {
          setEnabledPagesInternalUsers(settings.values.enabled_ui_pages_internal_users);
        }

        if (settings?.values?.disable_agents_for_internal_users !== undefined) {
          setDisableAgentsForInternalUsers(Boolean(settings.values.disable_agents_for_internal_users));
        }

        if (settings?.values?.allow_agents_for_team_admins !== undefined) {
          setAllowAgentsForTeamAdmins(Boolean(settings.values.allow_agents_for_team_admins));
        }

        if (settings?.values?.disable_vector_stores_for_internal_users !== undefined) {
          setDisableVectorStoresForInternalUsers(Boolean(settings.values.disable_vector_stores_for_internal_users));
        }

        if (settings?.values?.allow_vector_stores_for_team_admins !== undefined) {
          setAllowVectorStoresForTeamAdmins(Boolean(settings.values.allow_vector_stores_for_team_admins));
        }
      } catch {
        // A rejected settings read leaves the defaults. It is not a page error.
      }
    };

    fetchUISettings();
  }, [accessToken, canReadUISettings]);

  return (
    <Sidebar
      collapsed={sidebarCollapsed}
      onToggleCollapsed={onToggleCollapsed}
      enabledPagesInternalUsers={enabledPagesInternalUsers}
      enableProjectsUI /* kept destination; the projects UI flag must not drop it */
      disableAgentsForInternalUsers={disableAgentsForInternalUsers}
      allowAgentsForTeamAdmins={allowAgentsForTeamAdmins}
      disableVectorStoresForInternalUsers={disableVectorStoresForInternalUsers}
      allowVectorStoresForTeamAdmins={allowVectorStoresForTeamAdmins}
    />
  );
};

export default SidebarProvider;

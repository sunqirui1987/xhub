/**
 * Platform settings that still change a request. Prompt storage is the one
 * switch here: SSO, SCIM, vaults, plugins, and the IP list used to share this
 * page and never reached the gateway.
 */
import LoggingSettings from "@/components/Settings/AdminSettings/LoggingSettings/LoggingSettings";
import { t } from "@/i18n";
import React from "react";

const AdminPanel: React.FC = () => {
  return (
    <div className="m-2 mt-2 w-full p-8">
      <h2 className="mb-2 text-base font-semibold text-foreground">{t("pages.adminPanel.access")}</h2>
      <p className="mb-4 text-sm text-foreground">{t("pages.adminPanel.goToUsers")}</p>
      <LoggingSettings />
    </div>
  );
};

export default AdminPanel;

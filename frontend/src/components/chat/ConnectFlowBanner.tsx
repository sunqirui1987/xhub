"use client";

import React from "react";
import { CheckCircle } from "lucide-react";
import { getProxyBaseUrl, ConnectFlowStatus } from "@/components/networking";
import { OAuth2ConnectButton } from "@/components/chat/MCPAppsPanel";
import { t } from "@/i18n";

interface Props {
  flowHandle: string;
  flow?: ConnectFlowStatus;
  accessToken: string;
  onConnected: () => void;
  failed: boolean;
}

/** Finish remains an explicit POST because a cross-site navigation must never mint a code. */
export function isLoopbackOrigin(origin: string | null): boolean {
  if (!origin) return false;
  try {
    const hostname = new URL(origin).hostname.replace(/^\[|\]$/g, "");
    return hostname === "localhost" || hostname === "::1" || /^127(\.\d{1,3}){3}$/.test(hostname);
  } catch {
    return false;
  }
}

const copyFor = (flow: ConnectFlowStatus | undefined, failed: boolean): readonly [string, string] => {
  const clientLabel = flow?.client_origin ?? t("connect.application");
  const serverLabel = flow?.server_name ?? t("connect.requestedServer");
  if (failed || flow === undefined || flow.state === "stale") {
    return [t("connect.cannotContinue"), t("connect.cannotContinueBody", { client: clientLabel })];
  }
  if (flow.state === "unscoped") {
    return [t("connect.connectServers", { client: clientLabel }), t("connect.connectServersBody", { client: clientLabel })];
  }
  if (flow.state === "interactive" && !flow.connected) {
    return [t("connect.allowUse", { client: clientLabel, server: serverLabel }), t("connect.authorizeBelow", { client: clientLabel, server: serverLabel })];
  }
  return [t("connect.allowUse", { client: clientLabel, server: serverLabel }), t("connect.finishAsYou", { client: clientLabel, server: serverLabel })];
};

const ConnectFlowBanner: React.FC<Props> = ({ flowHandle, flow, accessToken, onConnected, failed }) => {
  const action = `${getProxyBaseUrl()}/authorize/complete`;
  const state = failed || flow === undefined ? "stale" : flow.state;
  const canFinish = state === "unscoped" || (state !== "stale" && flow?.connected === true);
  const canCancel = state !== "unscoped";
  const loopbackClient = isLoopbackOrigin(flow?.client_origin ?? null);
  const vendorServer =
    state === "interactive" && flow?.connected === false && flow.server_id !== null
      ? { server_id: flow.server_id, server_name: flow.server_name }
      : null;
  const copy = copyFor(flow, failed);

  return (
    <div className="mb-6 rounded-lg border border-primary/30 bg-primary/5 px-5 py-4">
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div className="flex items-start gap-3 min-w-0">
          <CheckCircle className="h-5 w-5 text-primary shrink-0 mt-0.5" />
          <div className="min-w-0">
            <p className="text-sm font-semibold text-foreground">{copy[0]}</p>
            <p className="text-[13px] text-muted-foreground mt-0.5">{copy[1]}</p>
          </div>
        </div>
        <div className="flex shrink-0 gap-2">
          {vendorServer !== null && (
            <OAuth2ConnectButton
              server={vendorServer}
              accessToken={accessToken}
              onConnect={onConnected}
              variant="button"
              autoStartKey={`litellm-mcp-autostart:${flowHandle}`}
            />
          )}
          <form method="POST" action={action}>
            <input type="hidden" name="flow" value={flowHandle} />
            {canFinish && (
              <button
                type="submit"
                className="h-[38px] rounded-md bg-primary px-4 text-sm font-semibold text-primary-foreground hover:bg-primary/90"
              >
                {t("connect.finish")}
              </button>
            )}
            {canCancel && (
              <button
                type="submit"
                name="decision"
                value="deny"
                className="ml-2 h-[38px] rounded-md border px-4 text-sm font-semibold text-foreground hover:bg-accent/40"
              >
                {t("connect.cancel")}
              </button>
            )}
            {loopbackClient && (
              <label className="mt-2 flex items-center gap-2 text-[13px] text-muted-foreground">
                <input type="checkbox" name="delivery" value="manual" />
                {t("My client is on a remote or SSH machine")}
              </label>
            )}
          </form>
        </div>
      </div>
    </div>
  );
};

export default ConnectFlowBanner;

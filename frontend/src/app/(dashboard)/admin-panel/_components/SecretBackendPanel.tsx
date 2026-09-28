"use client";

import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { Card } from "@/components/ui/card";
import { t } from "@/i18n";
import React, { useEffect, useState } from "react";

type BackendKind = "hashicorp_vault" | "cyberark";

const copy: Record<BackendKind, { title: string; description: string; empty: string; error: string; loading: string }> = {
  hashicorp_vault: {
    title: "Hashicorp Vault",
    description: "Configure Hashicorp Vault to securely manage provider API keys and secrets for your LiteLLM deployment.",
    empty: "No Hashicorp Vault Configuration Found",
    error: "Could not load Hashicorp Vault configuration",
    loading: "Loading Hashicorp Vault configuration",
  },
  cyberark: {
    title: "CyberArk Conjur",
    description: "Configure CyberArk Conjur to securely manage provider API keys and secrets for your LiteLLM deployment.",
    empty: "No CyberArk Configuration Found",
    error: "Could not load CyberArk configuration",
    loading: "Loading CyberArk configuration",
  },
};

// SecretBackendPanel shows the stored Vault or CyberArk settings, or that backend's empty state.
const SecretBackendPanel: React.FC<{ kind: BackendKind }> = ({ kind }) => {
  const { accessToken } = useAuthorized();
  const text = copy[kind];
  const [phase, setPhase] = useState<"loading" | "empty" | "ready" | "error">("loading");
  const [values, setValues] = useState<Record<string, unknown>>({});

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const response = await fetch(`/config_overrides/${kind}`, {
          headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : {},
        });
        if (!response.ok) {
          if (!cancelled) setPhase("error");
          return;
        }
        const body = (await response.json()) as { values?: Record<string, unknown> };
        const next = body.values && typeof body.values === "object" ? body.values : {};
        if (!cancelled) {
          setValues(next);
          setPhase(Object.keys(next).length > 0 ? "ready" : "empty");
        }
      } catch {
        if (!cancelled) setPhase("error");
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [accessToken, kind]);

  return (
    <Card className="block p-6" data-testid={`secret-backend-${kind}`}>
      <h3 className="mb-2 text-base font-semibold text-foreground">{t(text.title)}</h3>
      <p className="mb-4 text-sm text-muted-foreground">{t(text.description)}</p>
      {phase === "loading" && <p className="text-sm text-muted-foreground">{t(text.loading)}</p>}
      {phase === "empty" && <p className="text-sm text-foreground">{t(text.empty)}</p>}
      {phase === "error" && <p className="text-sm text-foreground">{t(text.error)}</p>}
      {phase === "ready" && (
        <dl className="text-sm" data-testid={`secret-backend-values-${kind}`}>
          {Object.entries(values).map(([key, value]) => (
            <div key={key} className="flex gap-2 py-1">
              <dt className="font-medium">{key}</dt>
              <dd className="text-muted-foreground">{typeof value === "string" ? value : JSON.stringify(value)}</dd>
            </div>
          ))}
        </dl>
      )}
    </Card>
  );
};

export default SecretBackendPanel;

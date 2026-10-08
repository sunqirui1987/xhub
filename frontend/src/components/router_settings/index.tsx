import React, { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { toast } from "@/lib/toast";
import { getCallbacksCall, getRouterSettingsCall, modelInfoCall, setCallbacksCall } from "../networking";
import RouterSettingsForm, { RouterSettingsFormValue } from "./RouterSettingsForm";
import WeightedSplitFields from "@/components/route_templates/WeightedSplitFields";
import {
  bodyFromForm,
  deploymentsFromInfo,
  formFromBody,
  parseDocument,
  prettyDocument,
  type SplitDeployment,
  type WeightRow,
} from "@/components/route_templates/templateForm";
import { t } from "@/i18n";

interface RouterSettingsProps {
  accessToken: string | null;
  userRole: string | null;
  userID: string | null;
}

interface routingStrategyArgs {
  ttl?: number;
  lowest_latency_buffer?: number;
}

const RouterSettings: React.FC<RouterSettingsProps> = ({ accessToken, userRole, userID }) => {
  const [formValue, setFormValue] = useState<RouterSettingsFormValue>({
    routerSettings: {},
    selectedStrategy: null,
    enableTagFiltering: false,
  });
  const [availableRoutingStrategies, setAvailableRoutingStrategies] = useState<string[]>([]);
  const [routerFieldsMetadata, setRouterFieldsMetadata] = useState<{ [key: string]: any }>({});
  const [routingStrategyDescriptions, setRoutingStrategyDescriptions] = useState<{ [key: string]: string }>({});
  const [deployments, setDeployments] = useState<SplitDeployment[]>([]);
  const [platformJson, setPlatformJson] = useState("");
  const [platformJsonError, setPlatformJsonError] = useState("");
  const [platformJsonDirty, setPlatformJsonDirty] = useState(false);

  useEffect(() => {
    if (!accessToken || !userRole || !userID) {
      return;
    }
    modelInfoCall(accessToken, userID, userRole, 1, 200)
      .then((data) => setDeployments(deploymentsFromInfo(data?.data ?? [])))
      .catch(() => undefined);
    getCallbacksCall(accessToken, userID, userRole).then((data) => {
      let router_settings = data.router_settings;
      if ("model_group_retry_policy" in router_settings) {
        delete router_settings["model_group_retry_policy"];
      }
      // Set initial selected strategy
      const initialStrategy = router_settings.routing_strategy || null;
      setFormValue((prev) => ({
        ...prev,
        routerSettings: router_settings,
        selectedStrategy: initialStrategy,
      }));
      setPlatformJson(prettyDocument(router_settings ?? {}));
    });
    getRouterSettingsCall(accessToken).then((data) => {
      if (data.fields) {
        // Build metadata map for easy lookup
        const fieldsMap: { [key: string]: any } = {};
        data.fields.forEach((field: any) => {
          fieldsMap[field.field_name] = {
            ui_field_name: field.ui_field_name,
            field_description: field.field_description,
            options: field.options,
            link: field.link,
          };
        });
        setRouterFieldsMetadata(fieldsMap);

        // Extract routing strategies from the routing_strategy field's options
        const routingStrategyField = data.fields.find((field: any) => field.field_name === "routing_strategy");
        if (routingStrategyField?.options) {
          const options = routingStrategyField.options.includes("weighted-split")
            ? routingStrategyField.options
            : [...routingStrategyField.options, "weighted-split"];
          setAvailableRoutingStrategies(options);
        }

        // Store routing strategy descriptions
        if (data.routing_strategy_descriptions) {
          setRoutingStrategyDescriptions(data.routing_strategy_descriptions);
        }

        // Set enable_tag_filtering value
        const tagFilteringField = data.fields.find((field: any) => field.field_name === "enable_tag_filtering");
        if (tagFilteringField?.field_value !== null && tagFilteringField?.field_value !== undefined) {
          setFormValue((prev) => ({
            ...prev,
            enableTagFiltering: tagFilteringField.field_value,
          }));
        }
      }
    });
  }, [accessToken, userRole, userID]);

  const handleSaveChanges = async () => {
    if (!accessToken) {
      return;
    }
    if (platformJsonDirty) {
      const parsed = parseDocument(platformJson);
      if (!parsed.ok) {
        setPlatformJsonError(t("pages.routeTemplates.jsonInvalid"));
        return;
      }
      try {
        await setCallbacksCall(accessToken, { router_settings: parsed.body });
        toast.success(t("router settings updated successfully"));
        window.location.reload();
      } catch (error) {
        toast.fromError(error);
      }
      return;
    }

    const router_settings = formValue.routerSettings;

    const numberKeys = new Set(["allowed_fails", "cooldown_time", "num_retries", "timeout", "retry_after"]);
    const jsonKeys = new Set(["model_group_alias"]);
    // retry_policy and model_group_retry_policy are owned by the Model Retry Settings tab;
    // routing_groups is owned by the Routing Groups tab. This page must not read or write them.
    const tabOwnedKeys = new Set(["retry_policy", "model_group_retry_policy", "routing_groups"]);

    const parseInputValue = (key: string, raw: string | undefined, fallback: unknown) => {
      if (raw === undefined) return fallback;

      const v = raw.trim();

      if (v.toLowerCase() === "null") return null;

      if (numberKeys.has(key)) {
        const n = Number(v);
        return Number.isNaN(n) ? fallback : n;
      }

      if (jsonKeys.has(key)) {
        if (v === "") return null;
        try {
          return JSON.parse(v);
        } catch {
          return fallback;
        }
      }

      if (v.toLowerCase() === "true") return true;
      if (v.toLowerCase() === "false") return false;

      return v;
    };

    // Add enable_tag_filtering to router_settings before processing
    const settingsToUpdate = {
      ...router_settings,
      enable_tag_filtering: formValue.enableTagFiltering,
    };

    const updatedVariables = Object.fromEntries(
      Object.entries(settingsToUpdate)
        .map(([key, value]) => {
          if (tabOwnedKeys.has(key)) {
            return null;
          }
          if (key !== "routing_strategy_args" && key !== "routing_strategy" && key !== "enable_tag_filtering") {
            const inputEl = document.querySelector(`input[name="${key}"]`) as HTMLInputElement | null;
            const parsed = parseInputValue(key, inputEl?.value, value);
            return [key, parsed];
          } else if (key === "routing_strategy") {
            return [key, formValue.selectedStrategy];
          } else if (key === "enable_tag_filtering") {
            return [key, formValue.enableTagFiltering];
          } else if (key === "routing_strategy_args" && formValue.selectedStrategy === "weighted-split") {
            // The share table writes into this object. Dropping the key here
            // would save the strategy and throw away the ratios.
            return ["routing_strategy_args", value];
          } else if (key === "routing_strategy_args" && formValue.selectedStrategy === "latency-based-routing") {
            let setRoutingStrategyArgs: routingStrategyArgs = {};

            const lowestLatencyBufferElement = document.querySelector(
              `input[name="lowest_latency_buffer"]`,
            ) as HTMLInputElement;
            const ttlElement = document.querySelector(`input[name="ttl"]`) as HTMLInputElement;

            if (lowestLatencyBufferElement?.value) {
              setRoutingStrategyArgs["lowest_latency_buffer"] = Number(lowestLatencyBufferElement.value);
            }

            if (ttlElement?.value) {
              setRoutingStrategyArgs["ttl"] = Number(ttlElement.value);
            }

            return ["routing_strategy_args", setRoutingStrategyArgs];
          }
          return null;
        })
        .filter((entry) => entry !== null && entry !== undefined) as Iterable<[string, unknown]>,
    );

    const payload = {
      router_settings: updatedVariables,
    };

    try {
      await setCallbacksCall(accessToken, payload);
      toast.success(t("router settings updated successfully"));
    } catch (error) {
      toast.fromError(t("Failed to update router settings: ") + error);
    }
  };

  if (!accessToken) {
    return null;
  }

  const weightRows = (): WeightRow[] => {
    const parsed = formFromBody(formValue.routerSettings);
    return parsed.weights;
  };

  const changeWeights = (rows: WeightRow[]) => {
    const parsed = formFromBody(formValue.routerSettings);
    parsed.weights = rows;
    const written = bodyFromForm(parsed);
    if (!written.ok) return;
    const nextValue = { ...formValue, routerSettings: written.body };
    setFormValue(nextValue);
    setPlatformJson(prettyDocument(written.body));
  };

  const editPlatformJson = (text: string) => {
    setPlatformJson(text);
    setPlatformJsonDirty(true);
    const parsed = parseDocument(text);
    if (!parsed.ok) {
      setPlatformJsonError(t("pages.routeTemplates.jsonInvalid"));
      return;
    }
    setPlatformJsonError("");
    const strategy = typeof parsed.body.routing_strategy === "string" ? parsed.body.routing_strategy : formValue.selectedStrategy;
    const nextValue = {
      ...formValue,
      routerSettings: parsed.body,
      selectedStrategy: strategy,
      enableTagFiltering: parsed.body.enable_tag_filtering === true,
    };
    setFormValue(nextValue);
  };

  const downloadPlatformJson = () => {
    const blob = new Blob([platformJson], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "router-settings.json";
    link.click();
    URL.revokeObjectURL(url);
  };

  const uploadPlatformJson = (file: File | undefined) => {
    if (!file) return;
    void file.text().then((text) => editPlatformJson(text));
  };

  const savePlatformJson = async () => {
    const parsed = parseDocument(platformJson);
    if (!parsed.ok) {
      setPlatformJsonError(t("pages.routeTemplates.jsonInvalid"));
      return;
    }
    try {
      await setCallbacksCall(accessToken, { router_settings: parsed.body });
      toast.success(t("router settings updated successfully"));
      window.location.reload();
    } catch (error) {
      toast.fromError(error);
    }
  };

  return (
    <div className="w-full space-y-8">
      <RouterSettingsForm
        value={formValue}
        onChange={setFormValue}
        routerFieldsMetadata={routerFieldsMetadata}
        availableRoutingStrategies={availableRoutingStrategies}
        routingStrategyDescriptions={routingStrategyDescriptions}
      />

      {/* Actions - Sticky at bottom */}
      {formValue.selectedStrategy === "weighted-split" && (
        <WeightedSplitFields saved={weightRows()} deployments={deployments} onChange={changeWeights} />
      )}

      <div className="space-y-3 border-t border-border pt-6">
        <div>
          <h3 className="text-sm font-medium text-foreground">{t("pages.routerSettings.documentTitle")}</h3>
          <p className="mt-1 text-xs text-muted-foreground">{t("pages.routerSettings.documentHint")}</p>
        </div>
        <textarea
          aria-label={t("pages.routerSettings.documentTitle")}
          className="h-72 w-full rounded-md border border-border bg-transparent p-3 font-mono text-xs"
          value={platformJson}
          spellCheck={false}
          onChange={(event) => editPlatformJson(event.target.value)}
        />
        {platformJsonError && <p className="text-xs text-destructive">{platformJsonError}</p>}
        <div className="flex flex-wrap gap-2">
          <Button type="button" size="sm" onClick={() => void savePlatformJson()} disabled={platformJsonError !== ""}>
            {t("pages.routerSettings.saveDocument")}
          </Button>
          <Button type="button" size="sm" variant="outline" onClick={downloadPlatformJson}>
            {t("pages.routeTemplates.downloadJson")}
          </Button>
          <label className="inline-flex cursor-pointer items-center">
            <input
              type="file"
              accept="application/json,.json"
              className="sr-only"
              onChange={(event) => {
                uploadPlatformJson(event.target.files?.[0]);
                event.target.value = "";
              }}
            />
            <span className="inline-flex h-8 items-center rounded-md border border-border px-3 text-sm">
              {t("pages.routeTemplates.importJson")}
            </span>
          </label>
        </div>
      </div>

      <div className="border-t border-border pt-6 flex justify-end gap-3">
        <Button variant="outline" onClick={() => window.location.reload()}>
          {t("Reset")}
        </Button>
        <Button onClick={handleSaveChanges}>{t("Save Changes")}</Button>
      </div>
    </div>
  );
};

export default RouterSettings;
export { RouterSettingsForm };
export type { RouterSettingsFormValue };

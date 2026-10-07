import React, { useEffect, useMemo, useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { MountedFormField, type MountedFormValues } from "../common_components/MountedFormField";
import { proxyBaseUrl } from "../networking";
import { provider_map } from "../provider_info_helpers";
import { TEST_MODES } from "./add_model_modes";
import { t } from "@/i18n";

interface EndpointAction {
  name: string;
  method: string;
  public_path: string;
  upstream_path: string;
  task_query?: string;
}

interface EndpointType {
  id: string;
  kind: "adapted" | "bypass" | string;
  label: string;
  providers?: string[];
  api_base?: string;
  model_field?: string;
  task_id?: string;
  strip_prefix?: string;
  actions?: EndpointAction[];
}

interface EndpointPayload {
  types?: EndpointType[];
  models?: Record<string, string>;
}

const fallbackTypes = (): EndpointType[] =>
  TEST_MODES.map((mode) => ({ id: mode.value, kind: "adapted", label: mode.label }));

function providerSlug(selected: string | null): string {
  if (!selected) return "";
  return provider_map[selected] ?? selected.toLowerCase();
}

function endpointBody(type: EndpointType, apiBase: string) {
  return {
    kind: "bypass",
    api_base: apiBase,
    model_field: type.model_field ?? "model",
    task_id: type.task_id ?? "",
    strip_prefix: type.strip_prefix ?? "",
    actions: type.actions ?? [],
  };
}

const EndpointTypeField: React.FC<{
  selectedProvider: string | null;
  modelCostMap?: Record<string, { endpoint_type?: string }> | null;
}> = ({ selectedProvider, modelCostMap }) => {
  const form = useFormContext<MountedFormValues>();
  const [payload, setPayload] = useState<EndpointPayload>({});
  const mode = useWatch({ control: form.control, name: "mode" }) as string | undefined;
  const modelValue = useWatch({ control: form.control, name: "model" });
  const slug = providerSlug(selectedProvider);

  useEffect(() => {
    const url = proxyBaseUrl ? `${proxyBaseUrl}/public/endpoints` : "/public/endpoints";
    let cancel = false;
    fetch(url)
      .then((response) => (response.ok ? response.json() : null))
      .then((body: EndpointPayload | null) => {
        if (!cancel && body) setPayload(body);
      })
      .catch(() => {});
    return () => {
      cancel = true;
    };
  }, []);

  const types = useMemo(() => {
    const remote = payload.types ?? [];
    const source = remote.length > 0 ? remote : fallbackTypes();
    return source.filter((type) => !type.providers || type.providers.length === 0 || type.providers.includes(slug));
  }, [payload.types, slug]);

  const selected = types.find((type) => type.id === mode) ?? null;
  const showBypass = mode === "custom" || selected?.kind === "bypass";

  useEffect(() => {
    const picked = Array.isArray(modelValue) ? modelValue[0] : modelValue;
    if (typeof picked !== "string" || picked === "") return;
    const fromMap = modelCostMap?.[picked]?.endpoint_type || payload.models?.[picked];
    if (fromMap && !form.getValues("mode")) {
      form.setValue("mode", fromMap);
    }
  }, [modelValue, modelCostMap, payload.models, form]);

  const writeBypass = (type: EndpointType, apiBase: string) => {
    form.setValue("endpoint", endpointBody(type, apiBase));
    if (apiBase) form.setValue("api_base", apiBase);
  };

  return (
    <>
      <MountedFormField label={t("Endpoint type")} name="mode" className="mb-1">
        {(control) => (
          <Select
            items={[
              { value: "", label: t("Not Set") },
              ...types.map((type) => ({ value: type.id, label: type.label })),
              { value: "custom", label: t("Custom Bypass") },
            ]}
            value={(control.value as string | undefined) ?? ""}
            onValueChange={(value: string | null) => {
              const next = value ?? "";
              control.onChange(next);
              if (next === "" || next === "custom") {
                form.setValue("endpoint", next === "custom" ? endpointBody({ id: "custom", kind: "bypass", label: "" }, "") : undefined);
                return;
              }
              const type = types.find((item) => item.id === next);
              if (type?.kind === "bypass") writeBypass(type, type.api_base ?? "");
              else form.setValue("endpoint", undefined);
            }}
          >
            <SelectTrigger id={control.id} className="w-full" aria-label={t("Endpoint type")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">{t("Not Set")}</SelectItem>
              {types.map((type) => (
                <SelectItem key={type.id} value={type.id}>
                  {type.label}
                </SelectItem>
              ))}
              <SelectItem value="custom">{t("Custom Bypass")}</SelectItem>
            </SelectContent>
          </Select>
        )}
      </MountedFormField>
      <p className="text-sm text-muted-foreground mb-5 mt-1">
        {t("Choose how this model is called. Bypass copies the provider's own API. Custom Bypass is for an API you fill in from its documentation.")}
      </p>
      {showBypass && <BypassFields selected={selected} onChange={writeBypass} />}
    </>
  );
};

const BypassFields: React.FC<{
  selected: EndpointType | null;
  onChange: (type: EndpointType, apiBase: string) => void;
}> = ({ selected, onChange }) => {
  const form = useFormContext<MountedFormValues>();
  const saved = (form.getValues("endpoint") ?? {}) as {
    api_base?: string;
    model_field?: string;
    task_id?: string;
    actions?: EndpointAction[];
  };
  const [apiBase, setApiBase] = useState(saved.api_base || selected?.api_base || "");
  const [modelField, setModelField] = useState(saved.model_field || selected?.model_field || "model");
  const [taskID, setTaskID] = useState(saved.task_id || selected?.task_id || "");
  const [actions, setActions] = useState<EndpointAction[]>(
    saved.actions?.length ? saved.actions : selected?.actions ?? [{ name: "create", method: "POST", public_path: "", upstream_path: "" }],
  );

  const emit = (nextBase: string, nextField: string, nextTask: string, nextActions: EndpointAction[]) => {
    const type: EndpointType = {
      id: selected?.id ?? "custom",
      kind: "bypass",
      label: selected?.label ?? "",
      model_field: nextField,
      task_id: nextTask,
      strip_prefix: selected?.strip_prefix,
      actions: nextActions,
    };
    onChange(type, nextBase);
  };

  return (
    <div className="mb-5 space-y-3 rounded-md border border-border p-3">
      <label className="block text-sm">
        {t("API Base")}
        <Input className="mt-1" value={apiBase} onChange={(event) => {
          setApiBase(event.target.value);
          emit(event.target.value, modelField, taskID, actions);
        }} />
      </label>
      <label className="block text-sm">
        {t("Model field")}
        <Input className="mt-1" value={modelField} onChange={(event) => {
          setModelField(event.target.value);
          emit(apiBase, event.target.value, taskID, actions);
        }} />
      </label>
      <label className="block text-sm">
        {t("Task id field")}
        <Input className="mt-1" value={taskID} placeholder="data.task_id" onChange={(event) => {
          setTaskID(event.target.value);
          emit(apiBase, modelField, event.target.value, actions);
        }} />
      </label>
      {actions.map((action, index) => (
        <div key={`${action.name}-${index}`} className="grid grid-cols-2 gap-2">
          <label className="block text-sm">
            {t("Method")}
            <Input className="mt-1" value={action.method} onChange={(event) => {
              const next = actions.map((item, i) => (i === index ? { ...item, method: event.target.value } : item));
              setActions(next);
              emit(apiBase, modelField, taskID, next);
            }} />
          </label>
          <label className="block text-sm">
            {t("Public path")}
            <Input className="mt-1" value={action.public_path} onChange={(event) => {
              const next = actions.map((item, i) => (i === index ? { ...item, public_path: event.target.value, upstream_path: item.upstream_path || event.target.value } : item));
              setActions(next);
              emit(apiBase, modelField, taskID, next);
            }} />
          </label>
          <label className="col-span-2 block text-sm">
            {t("Task id query")}
            <Input className="mt-1" value={action.task_query ?? ""} placeholder="taskId" onChange={(event) => {
              const next = actions.map((item, i) => (i === index ? { ...item, task_query: event.target.value } : item));
              setActions(next);
              emit(apiBase, modelField, taskID, next);
            }} />
          </label>
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        onClick={() => {
          const next = [...actions, { name: "get", method: "GET", public_path: "", upstream_path: "" }];
          setActions(next);
          emit(apiBase, modelField, taskID, next);
        }}
      >
        {t("Add action")}
      </Button>
    </div>
  );
};

export default EndpointTypeField;

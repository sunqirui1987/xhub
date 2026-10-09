import { useProviderFields } from "@/app/(dashboard)/hooks/providers/useProviderFields";
import { PasswordInput } from "@/components/shared/PasswordInput";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { Upload as UploadIcon } from "lucide-react";
import React from "react";
import { useFormContext } from "react-hook-form";
import { requiredRule } from "../common_components/formRules";
import {
  MountedFormField,
  type MountedFieldControlProps,
  type MountedFormValues,
} from "../common_components/MountedFormField";
import { CredentialItem, ProviderCredentialFieldMetadata } from "../networking";
import { provider_map, Providers } from "../provider_info_helpers";
import { labelWithHint } from "@/components/shared/form/LabelWithHint";
import { t } from "@/i18n";

interface ProviderSpecificFieldsProps {
  selectedProvider: string | null;
}

/** 读取上传文件；file 为服务账号文件，onLoaded 接收文本。返回无；上传控件调用，异步读取不写入磁盘。 */
const readTextFile = (file: File, onLoaded: (contents: string) => void) => {
  const reader = new FileReader();
  reader.onload = (event) => {
    if (event.target) {
      onLoaded(event.target.result as string);
    }
  };
  reader.readAsText(file);
};

interface ProviderCredentialField {
  key: string;
  label: string;
  placeholder?: string;
  tooltip?: string;
  required?: boolean;
  type?: "text" | "password" | "select" | "upload" | "textarea";
  options?: string[];
  defaultValue?: string;
}

export interface CredentialValues {
  key: string;
  value: string;
}

/** 将 value 转为受控文本，fallback 为默认值；返回字符串，供文本与密钥控件调用，无副作用。 */
const controlledTextValue = (value: unknown, fallback?: string): string =>
  typeof value === "string" ? value : (fallback ?? "");

/** 从 apiBase 查询参数解析 Azure 版本，返回版本或 null；地址变化调用，不修改源地址。 */
const getApiVersionFromApiBase = (apiBase: string): string | null => {
  const queryStartIndex = apiBase.indexOf("?");
  if (queryStartIndex === -1) {
    return null;
  }

  const queryString = apiBase.slice(queryStartIndex + 1).split("#")[0];
  const searchParams = new URLSearchParams(queryString);

  return searchParams.get("api_version") || searchParams.get("api-version");
};

/** 将后台 field 转换为 UI 字段；返回类型、默认值和校验定义，未知类型降级文本，不修改源数据。 */
const mapFieldMetadataToUiField = (field: ProviderCredentialFieldMetadata): ProviderCredentialField => {
  const type: ProviderCredentialField["type"] =
    field.field_type === "password"
      ? "password"
      : field.field_type === "select"
        ? "select"
        : field.field_type === "upload"
          ? "upload"
          : field.field_type === "textarea"
            ? "textarea"
            : "text";

  return {
    key: field.key,
    label: field.label,
    placeholder: field.placeholder ?? undefined,
    tooltip: field.tooltip ?? undefined,
    required: field.required ?? false,
    type,
    options: field.options ?? undefined,
    defaultValue: field.default_value ?? undefined,
  };
};

// In-memory cache of provider credential fields keyed by provider display name.
// This lets us reuse the data across multiple mounts and also supports
// non-React helpers like createCredentialFromModel.
const providerFieldsByDisplayName: Record<string, ProviderCredentialField[]> = {};

export const createCredentialFromModel = (provider: string, modelData: any): CredentialItem => {
  const enumKey = Object.keys(provider_map).find((key) => provider_map[key].toLowerCase() === provider.toLowerCase());
  if (!enumKey) {
    throw new Error(t("Provider {provider} not found in provider_map", { provider }));
  }
  const providerDisplayName = Providers[enumKey as keyof typeof Providers];
  const providerFields = providerFieldsByDisplayName[providerDisplayName] || [];
  const credentialValues: object = {};

  // Go through each field defined for this provider
  providerFields.forEach((field) => {
    const value = modelData.litellm_params[field.key];
    if (value !== undefined) {
      (credentialValues as Record<string, string>)[field.key] = value.toString();
    }
  });

  const credential: CredentialItem = {
    credential_name: `${provider}-credential-${Math.floor(Math.random() * 1000000)}`,
    credential_values: credentialValues,
    credential_info: {
      custom_llm_provider: provider,
      description: t("Credential for {provider}. Created from model {value1}", { provider, value1: (modelData.model_name) }),
    },
  };

  return credential;
};

/** 根据 selectedProvider 标识渲染当前后台认证字段；返回控件列表，供模型与凭据表单使用。默认值进入表单状态，上传读取 JSON，Azure 地址可推导版本。 */
const ProviderSpecificFields: React.FC<ProviderSpecificFieldsProps> = ({ selectedProvider }) => {
  const selectedProviderEnum = Providers[selectedProvider as keyof typeof Providers] as Providers;
  const form = useFormContext<MountedFormValues>();
  const credentialsFileRef = React.useRef<HTMLInputElement>(null);
/** 文件选择处理器：onLoaded 写入表单；返回事件回调，非 JSON 忽略，并清空控件以允许重复选择。 */
  const pickCredentialsFile =
    (onLoaded: (contents: string) => void) => (event: React.ChangeEvent<HTMLInputElement>) => {
      const file = event.target.files?.[0];
      event.target.value = "";
      if (file?.type === "application/json") {
        readTextFile(file, onLoaded);
      }
    };

  const { data: providerMetadata, isLoading, error: loadError } = useProviderFields();

  // Memoize the expensive cache computation
  const cacheEntries = React.useMemo(() => {
    if (!providerMetadata) {
      return null;
    }

    // Compute cache entries keyed by provider display name and identifiers
    const entries: Record<string, ProviderCredentialField[]> = {};
    providerMetadata.forEach((providerInfo) => {
      const displayName = providerInfo.provider_display_name;
      const mappedFields = providerInfo.credential_fields.map(mapFieldMetadataToUiField);

      // Primary key: human-readable display name
      entries[displayName] = mappedFields;

      // Also cache by backend identifiers so lookups by provider slug work
      if (providerInfo.provider) {
        entries[providerInfo.provider] = mappedFields;
      }
      if (providerInfo.litellm_provider) {
        entries[providerInfo.litellm_provider] = mappedFields;
      }
    });
    return entries;
  }, [providerMetadata]);

  // Sync memoized cache entries to module-level cache
  React.useEffect(() => {
    if (!cacheEntries) {
      return;
    }

    Object.assign(providerFieldsByDisplayName, cacheEntries);
  }, [cacheEntries]);

  const allFields = React.useMemo(() => {
    if (selectedProvider === null) return [];
    // 仅使用当前查询定义，避免失败重试时显示历史缓存。
    if (!providerMetadata) {
      return [];
    }

    const providerInfo = providerMetadata.find(
      (p) =>
        p.provider_display_name === selectedProviderEnum ||
        p.provider === selectedProvider ||
        p.litellm_provider === selectedProvider,
    );
    if (!providerInfo) {
      return [];
    }

    return providerInfo.credential_fields.map(mapFieldMetadataToUiField);
  }, [selectedProviderEnum, selectedProvider, providerMetadata]);

  // Controller 的默认值可能在观察器订阅前注册；显式同步确保未触碰字段进入提交状态。
  React.useEffect(() => {
    for (const field of allFields) {
      if (form.getValues(field.key) == null && field.defaultValue != null) {
        form.setValue(field.key, field.defaultValue);
      }
    }
  }, [allFields, form]);

  const hasApiVersionField = React.useMemo(() => allFields.some((field) => field.key === "api_version"), [allFields]);
  const lastInferredApiVersionRef = React.useRef<string | null>(null);

/** 处理地址事件并同步推导版本；返回回调，仅清除上次自动推导值，保留手工版本。 */
  const handleApiBaseChange = React.useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      if (!hasApiVersionField) {
        return;
      }

      const apiVersion = getApiVersionFromApiBase(event.target.value);
      if (apiVersion) {
        lastInferredApiVersionRef.current = apiVersion;
        form.setValue("api_version", apiVersion);
        return;
      }

      if (form.getValues("api_version") === lastInferredApiVersionRef.current) {
        form.setValue("api_version", "");
      }
      lastInferredApiVersionRef.current = null;
    },
    [form, hasApiVersionField],
  );

/** 用 field 定义和 control 绑定渲染输入；返回 React 元素，上传内容或选择结果写入所属表单。 */
  const renderFieldControl = (field: ProviderCredentialField, control: MountedFieldControlProps) => {
    if (field.type === "select") {
      return (
        <Select
          items={(field.options ?? []).map((option) => ({ value: option, label: option }))}
          value={(control.value as string | undefined) ?? field.defaultValue ?? null}
          onValueChange={control.onChange}
        >
          <SelectTrigger id={control.id} onBlur={control.onBlur} className="w-full">
            <SelectValue placeholder={field.placeholder} />
          </SelectTrigger>
          <SelectContent>
            {field.options?.map((option) => (
              <SelectItem key={option} value={option}>
                {option}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      );
    }

    if (field.type === "upload") {
      return (
        <>
          <Button type="button" variant="outline" className="w-fit" onClick={() => credentialsFileRef.current?.click()}>
            <UploadIcon />
            {t("Click to Upload")}
          </Button>
          <input
            ref={credentialsFileRef}
            id={control.id}
            type="file"
            accept=".json"
            className="sr-only"
            onBlur={control.onBlur}
            onChange={pickCredentialsFile(control.onChange)}
          />
        </>
      );
    }

    if (field.type === "textarea") {
      return (
        <Textarea
          id={control.id}
          value={controlledTextValue(control.value, field.defaultValue)}
          onChange={control.onChange}
          onBlur={control.onBlur}
          placeholder={field.placeholder}
          rows={6}
          className="font-mono text-xs"
        />
      );
    }

    if (field.type === "password") {
      return (
        <PasswordInput
          id={control.id}
          value={controlledTextValue(control.value, field.defaultValue)}
          onChange={control.onChange}
          onBlur={control.onBlur}
          placeholder={field.placeholder}
        />
      );
    }

    return (
      <Input
        id={control.id}
        value={controlledTextValue(control.value, field.defaultValue)}
        onBlur={control.onBlur}
        placeholder={field.placeholder}
        type="text"
        onChange={(event) => {
          control.onChange(event);
          if (field.key === "api_base") {
            handleApiBaseChange(event);
          }
        }}
      />
    );
  };

  return (
    <>
      {isLoading && allFields.length === 0 && <p className="text-sm mb-2">{t("Loading provider fields...")}</p>}
      {loadError && allFields.length === 0 && (
        <p className="text-sm mb-2 text-destructive">
          {loadError instanceof Error ? loadError.message : t("Failed to load provider credential fields")}
        </p>
      )}
      {allFields.map((field) => (
        <React.Fragment key={selectedProvider + ":" + field.key}>
          <MountedFormField
            label={field.tooltip ? labelWithHint(field.label, field.tooltip) : field.label}
            name={field.key}
            defaultValue={field.defaultValue}
            required={field.required}
            rules={field.required ? { validate: { required: requiredRule("Required") } } : undefined}
            className={field.key === "vertex_credentials" ? "mb-0" : "mb-4"}
          >
            {(control) => renderFieldControl(field, control)}
          </MountedFormField>

          {/* Special case for Vertex Credentials help text */}
          {field.key === "vertex_credentials" && (
            <p className="text-sm mb-3 mt-1">{t("Give a gcp service account(.json file)")}</p>
          )}

          {/* Special case for Azure Base Model help text */}
          {field.key === "base_model" && (
            <div className="grid grid-cols-24">
              <p className="col-start-11 col-span-10 text-sm mb-2">
                {t("The actual model your azure deployment uses. Used for accurate cost tracking. Select name from")}{" "}
                
                  {t("here")}
                
              </p>
            </div>
          )}
        </React.Fragment>
      ))}
    </>
  );
};

export default ProviderSpecificFields;

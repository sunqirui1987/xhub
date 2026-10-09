import React, { useState, useEffect } from "react";
import {
  guardrail_provider_map,
  populateGuardrailProviders,
  populateGuardrailProviderMap,
  shouldRenderContentFilterConfigSettings,
} from "./guardrail_info_helpers";
import { getGuardrailProviderSpecificParams } from "@/components/networking";
import { MultiSelect } from "@/components/shared/MultiSelect";
import { PasswordInput } from "@/components/shared/PasswordInput";
import NumericalInput from "@/components/shared/numerical_input";
import { FieldGroup } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import { Textarea } from "@/components/ui/textarea";
import { toast } from "@/lib/toast";
import { UiLoadingSpinner } from "@/components/ui/ui-loading-spinner";
import {
  asStringArray,
  asText,
  GuardrailField,
  labelWithHint,
  readRecord,
  requiredRule,
  type GuardrailFieldControlProps,
  type GuardrailFieldRules,
  type GuardrailFormControl,
} from "./GuardrailFormField";
import { t } from "@/i18n";

interface GuardrailProviderFieldsProps {
  selectedProvider: string | null;
  control: GuardrailFormControl;
  accessToken?: string | null;
  providerParams?: ProviderParamsResponse | null;
  value?: Record<string, unknown> | null;
}

interface ProviderParam {
  param: string;
  description: string;
  required: boolean;
  default_value?: string | number;
  options?: string[];
  type?: string;
  fields?: { [key: string]: ProviderParam };
  dict_key_options?: string[];
  dict_value_type?: string;
  min?: number;
  max?: number;
  step?: number;
}

/** 提供商字段集合同时包含 ui_friendly_name 字符串元数据和配置字段描述。 */
interface ProviderParamsResponse {
  [provider: string]: { [key: string]: ProviderParam | string };
}

const BOOLEAN_ITEMS = [
  { label: t("True"), value: true },
  { label: t("False"), value: false },
];

/**
 * 用途：识别历史字段中的密钥命名，决定密码输入显示。
 * 参数：fieldKey：字段完整路径。
 * 返回：boolean；只是展示判断，不代替后端密钥处理。
 * 调用：ProviderFieldInput。
 * 测试：guardrail_provider_fields.integration.test.tsx。
 */
const isSecretKey = (fieldKey: string): boolean =>
  fieldKey.includes("password") || fieldKey.includes("secret") || fieldKey.includes("key");

/**
 * 用途：检查 JSON 值是否为普通对象，排除 null 与数组。
 * 参数：value：未知值。
 * 返回：类型谓词，符合对象约束时为 true。
 * 调用：对象校验与失焦提交。
 * 测试：guardrail_provider_fields.integration.test.tsx。
 */
const isPlainObject = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);

// Object fields hold the raw text while the user types, so submission must be
// blocked until the value parses to a plain JSON object (or is cleared).
/**
 * 用途：构建对象字段校验规则，非法输入不能提交。
 * 参数：fieldKey：用于错误信息的字段名。
 * 返回：校验声明对象。
 * 调用：fieldRules。
 * 测试：guardrail_provider_fields.integration.test.tsx。
 */
const jsonObjectRule = (fieldKey: string): GuardrailFieldRules => ({
  validate: (value: unknown) =>
    value === undefined || isPlainObject(value) ? true : `${fieldKey} must be a valid JSON object`,
});

// Commits a parsed object (or undefined for a cleared field) to the form on
// blur; anything else stays as raw text so jsonObjectRule blocks submission.
/**
 * 用途：失焦时解析 JSON 对象，清空转 undefined，非法输入保留并提示。
 * 参数：raw：输入字符串；onChange：表单回调。
 * 返回：void；有效对象写入表单。
 * 调用：对象文本框失焦事件。
 * 测试：guardrail_provider_fields.integration.test.tsx。
 */
const commitObjectField = (raw: string, onChange: (value: unknown) => void): void => {
  const next = raw.trim();
  if (next === "") {
    onChange(undefined);
    return;
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(next);
  } catch {
    parsed = next;
  }
  if (isPlainObject(parsed)) {
    onChange(parsed);
  } else {
    toast.error(t("Enter a valid JSON object for this configuration"));
  }
};

/**
 * 用途：选择对象校验或必填校验声明。
 * 参数：field：字段描述；fieldKey：字段名。
 * 返回：校验声明或 undefined。
 * 调用：renderFields。
 * 测试：guardrail_provider_fields.integration.test.tsx。
 */
const fieldRules = (field: ProviderParam, fieldKey: string): GuardrailFieldRules | undefined => {
  if (field.type === "object") {
    return jsonObjectRule(fieldKey);
  }
  return field.required ? requiredRule(`${fieldKey} is required`) : undefined;
};

interface ProviderFieldInputProps {
  descriptor: ProviderParam;
  fieldKey: string;
  control: GuardrailFieldControlProps;
}

/**
 * 用途：按字段类型展示控件，并转发表单标识与无障碍属性。
 * 参数：descriptor：字段描述；fieldKey：路径；control：表单绑定。
 * 返回：React 输入控件。
 * 调用：递归提供商字段渲染。
 * 测试：guardrail_provider_fields.integration.test.tsx。
 */
const ProviderFieldInput: React.FC<ProviderFieldInputProps> = ({ descriptor, fieldKey, control }) => {
  const { id, value, onChange, onBlur, ref, name, ...aria } = control;

  if (descriptor.type === "list") {
    return (
      <Textarea
        id={id}
        name={name}
        ref={ref}
        placeholder={descriptor.description}
        value={asStringArray(value).join("\n")}
        onChange={(event) => onChange(event.target.value.split("\n"))}
        onBlur={(event) => {
          onChange(
            event.target.value
              .split("\n")
              .map((entry) => entry.trim())
              .filter(Boolean),
          );
          onBlur();
        }}
        {...aria}
      />
    );
  }

  if (descriptor.type === "select" && descriptor.options) {
    return (
      <Select
        items={descriptor.options.map((option) => ({ label: option, value: option }))}
        value={asText(value) || null}
        onValueChange={(next: string | null) => onChange(next)}
      >
        <SelectTrigger id={id} className="w-full" {...aria}>
          <SelectValue placeholder={descriptor.description} />
        </SelectTrigger>
        <SelectContent>
          {descriptor.options.map((option) => (
            <SelectItem key={option} value={option}>
              {option}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }

  if (descriptor.type === "multiselect" && descriptor.options) {
    return (
      <MultiSelect
        id={id}
        options={descriptor.options.map((option) => ({ label: option, value: option }))}
        value={asStringArray(value)}
        onValueChange={onChange}
        placeholder={descriptor.description}
      />
    );
  }

  if (descriptor.type === "bool" || descriptor.type === "boolean") {
    return (
      <Select
        items={BOOLEAN_ITEMS}
        value={typeof value === "boolean" ? value : null}
        onValueChange={(next: boolean | null) => onChange(next)}
      >
        <SelectTrigger id={id} className="w-full" {...aria}>
          <SelectValue placeholder={descriptor.description} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={true}>{t("True")}</SelectItem>
          <SelectItem value={false}>{t("False")}</SelectItem>
        </SelectContent>
      </Select>
    );
  }

  if (descriptor.type === "percentage" && descriptor.min != null && descriptor.max != null) {
    return (
      <div className="w-full">
        <Slider
          id={id}
          min={descriptor.min}
          max={descriptor.max}
          step={descriptor.step ?? 0.1}
          value={typeof value === "number" ? value : descriptor.min}
          onValueChange={(next: number | readonly number[]) => onChange(Array.isArray(next) ? next[0] : next)}
          onBlur={onBlur}
        />
        <div className="mt-1 flex justify-between text-xs text-muted-foreground">
          <span>0%</span>
          <span>50%</span>
          <span>100%</span>
        </div>
      </div>
    );
  }

  if (descriptor.type === "object") {
    const objectValue = typeof value === "object" && value !== null ? JSON.stringify(value, null, 2) : asText(value);
    return (
      <Textarea
        id={id}
        name={name}
        ref={ref}
        placeholder={descriptor.description}
        value={objectValue}
        onChange={(event) => onChange(event.target.value)}
        onBlur={(event) => {
          commitObjectField(event.target.value, onChange);
          onBlur();
        }}
        {...aria}
      />
    );
  }

  if (descriptor.type === "number") {
    return (
      <NumericalInput
        id={id}
        name={name}
        step={1}
        placeholder={descriptor.description}
        value={asText(value)}
        onChange={onChange}
        onBlur={onBlur}
        {...aria}
      />
    );
  }

  if (isSecretKey(fieldKey)) {
    return (
      <PasswordInput
        id={id}
        name={name}
        ref={ref}
        placeholder={descriptor.description}
        value={asText(value)}
        onChange={onChange}
        onBlur={onBlur}
        {...aria}
      />
    );
  }

  return (
    <Input
      id={id}
      name={name}
      ref={ref}
      placeholder={descriptor.description}
      value={asText(value)}
      onChange={onChange}
      onBlur={onBlur}
      {...aria}
    />
  );
};

/**
 * 用途：按字段描述生成表单，区分字符串元数据与真正配置项。
 * 参数：selectedProvider：目录键；control：表单控制器；providerParams/accessToken：静态配置或远端加载；value：已有值。
 * 返回：React 字段组或加载/错误提示；逐行转换列表，保留正则中的逗号。
 * 调用：护栏创建及详情表单。
 * 测试：guardrail_provider_fields.integration.test.tsx、guardrail_provider_fields.test.tsx。
 */
const GuardrailProviderFields: React.FC<GuardrailProviderFieldsProps> = ({
  selectedProvider,
  control,
  accessToken,
  providerParams: providerParamsProp = null,
  value = null,
}) => {
  const [loading, setLoading] = useState(false);
  const [providerParams, setProviderParams] = useState<ProviderParamsResponse | null>(providerParamsProp);
  const [error, setError] = useState<string | null>(null);

  // Fetch provider-specific parameters when component mounts
  useEffect(() => {
    if (providerParamsProp) {
      // Props updated externally
      setProviderParams(providerParamsProp);
      return;
    }

    const fetchProviderParams = async () => {
      if (!accessToken) return;

      setLoading(true);
      setError(null);

      try {
        const data = await getGuardrailProviderSpecificParams(accessToken);
        setProviderParams(data);

        // Populate dynamic providers from API response
        populateGuardrailProviders(data);
        populateGuardrailProviderMap(data);
      } catch (error) {
        console.error("Error fetching provider params:", error);
        setError(t("Failed to load provider parameters"));
      } finally {
        setLoading(false);
      }
    };

    // Only fetch if not provided via props
    if (!providerParamsProp) {
      fetchProviderParams();
    }
  }, [accessToken, providerParamsProp]);

  // If no provider is selected, don't render anything
  if (!selectedProvider) {
    return null;
  }

  // Show loading state
  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <UiLoadingSpinner className="size-4" />
        {t("Loading provider parameters...")}
      </div>
    );
  }

  // Show error state
  if (error) {
    return <div className="text-destructive">{error}</div>;
  }

  // Get the provider key matching the selected provider in the guardrail_provider_map
  const providerKey = guardrail_provider_map[selectedProvider]?.toLowerCase();

  // Get parameters for the selected provider
  const providerFields = providerParams && providerParams[providerKey];

  if (!providerFields || Object.keys(providerFields).length === 0) {
    return <div>{t("No configuration fields available for this provider.")}</div>;
  }

  // Fields to skip for content filter provider (handled in dedicated steps)
  const contentFilterFieldsToSkip = new Set([
    "patterns",
    "blocked_words",
    "blocked_words_file",
    "categories",
    "severity_threshold",
    "pattern_redaction_format",
    "keyword_redaction_tag",
  ]);

  const isContentFilterProvider = shouldRenderContentFilterConfigSettings(selectedProvider);

  // Convert object to array of entries and render fields
  /**
   * 用途：递归渲染字段描述，跳过 UI 名称元数据和专用编辑器已处理字段。
   * 参数：fields：字段声明；parentKey：父路径；parentValue：父对象。
   * 返回：React 节点数组。
   * 调用：GuardrailProviderFields。
   * 测试：guardrail_provider_fields.integration.test.tsx。
   */
  const renderFields = (fields: { [key: string]: ProviderParam | string }, parentKey = "", parentValue?: unknown) => {
    return Object.entries(fields).map(([fieldKey, field]) => {
      // ":" keeps nested children out of the submitted object graph: nothing binds the parent
      const fullFieldKey = parentKey ? `${parentKey}:${fieldKey}` : fieldKey;
      const fieldValue = parentValue ? readRecord(parentValue, fieldKey) : value?.[fieldKey];
      // Skip ui_friendly_name - it's metadata for the UI dropdown, not a user configuration field
      if (fieldKey === "ui_friendly_name" || typeof field === "string") {
        return null;
      }

      // Skip optional_params - they are handled in a separate step
      if (fieldKey === "optional_params" && field.type === "nested" && field.fields) {
        return null;
      }

      // Skip content filter specific fields when it's a content filter provider (handled in dedicated steps)
      if (isContentFilterProvider && contentFilterFieldsToSkip.has(fieldKey)) {
        return null;
      }

      // Handle other nested fields (like azure/text_moderations optional_params)
      if (field.type === "nested" && field.fields) {
        return (
          <div key={fullFieldKey}>
            <div className="mb-2 font-medium">{fieldKey}</div>
            <FieldGroup className="ml-4 border-l-2 border-border pl-4">
              {renderFields(field.fields, fullFieldKey, fieldValue)}
            </FieldGroup>
          </div>
        );
      }

      const resolvedInitialValue =
        fieldValue !== undefined ? fieldValue : field.default_value ?? (field.type === "percentage" ? 0.5 : undefined);

      return (
        <GuardrailField
          key={fullFieldKey}
          control={control}
          name={fullFieldKey}
          label={labelWithHint(fieldKey, field.description)}
          rules={fieldRules(field, fieldKey)}
          defaultValue={resolvedInitialValue}
        >
          {(fieldControl) => <ProviderFieldInput descriptor={field} fieldKey={fieldKey} control={fieldControl} />}
        </GuardrailField>
      );
    });
  };

  return <FieldGroup>{renderFields(providerFields)}</FieldGroup>;
};

export default GuardrailProviderFields;

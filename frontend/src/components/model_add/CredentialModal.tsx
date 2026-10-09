import { Input } from "@/components/ui/input";
import { SearchSelect } from "@/components/shared/SearchSelect";
import { Button } from "@/components/ui/button";
import { useEffect, useMemo, useState } from "react";
import { FormProvider, useForm, useWatch } from "react-hook-form";
import { useProviderFields } from "@/app/(dashboard)/hooks/providers/useProviderFields";
import ProviderSpecificFields from "../add_model/provider_specific_fields";
import { requiredRule } from "../common_components/formRules";
import { MountedFormField, MountedFormProvider, projectMountedValues, useMountRegistry, type MountedFormValues } from "../common_components/MountedFormField";
import { CredentialItem } from "../networking";
import { Logo } from "@/components/molecules/logo/Logo";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { t } from "@/i18n";

/** 提供商弹窗参数：open 控制显示，mode 决定新建或编辑，existingCredential 为旧数据。
 * onSubmit 返回 false 或抛错时保留草稿；成功时由调用方关闭；onCancel 负责关闭弹窗。 */
interface CredentialModalProps {
  open: boolean;
  onCancel: () => void;
  onSubmit: (values: Record<string, unknown>) => unknown | Promise<unknown>;
  mode: "add" | "edit";
  existingCredential?: CredentialItem | null;
}

/** 根据后台供应商定义渲染可搜索的连接表单。
 * 参数见 CredentialModalProps；返回弹窗。新建不预选类型，新建切换清除旧认证且保留名称。
 * 编辑兼容旧 slug/枚举值并固定类型，避免后台合并留下跨协议旧字段；异步保存期间禁止重复提交，错误保留当前输入并可重试。 */
export default function CredentialModal({ open, onCancel, onSubmit, mode, existingCredential = null }: CredentialModalProps) {
  const isEdit = mode === "edit";
  const { data: providers, isLoading, error, refetch } = useProviderFields();
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState(false);
  const form = useForm<MountedFormValues>({ mode: "onChange", defaultValues: {
    ...existingCredential?.credential_values,
    credential_name: existingCredential?.credential_name ?? "",
    custom_llm_provider: existingCredential?.credential_info.provider_id ?? existingCredential?.credential_info.custom_llm_provider ?? null,
  } });
  const registry = useMountRegistry();
  const selection = useWatch({ control: form.control, name: "custom_llm_provider" });
  const selected = providers?.find(p => p.provider === selection)
    ?? providers?.find(p => p.litellm_provider === selection || p.provider_display_name === selection);
  const options = useMemo(() => (providers ?? []).map(p => ({
    label: p.provider_display_name, value: p.provider, sublabel: p.litellm_provider,
    icon: <Logo provider={p.litellm_provider} label={p.provider_display_name} className="size-5" />,
  })), [providers]);

  // 默认值写入真实表单状态，保证未触碰字段也进入提交；旧数据和用户输入优先。
  useEffect(() => {
    for (const field of selected?.credential_fields ?? []) {
      if (form.getValues(field.key) == null && field.default_value != null) {
        form.setValue(field.key, field.default_value);
      }
    }
  }, [selected, form]);

  /** 校验已挂载字段并等待保存；返回 Promise<void>，失败不清空输入或关闭弹窗。 */
  const handleSubmit = async () => {
    if (saving || !selected || !selected.credential_fields.length) return;
    if (!await form.trigger(registry.mountedNames() as string[])) return;
    setSaving(true);
    setSaveError(false);
    try {
      const values = projectMountedValues(registry, form.getValues);
      const filtered = Object.fromEntries(Object.entries(values).filter(([, v]) => v !== "" && v != null));
      // 选择器使用唯一 provider 标识，持久化使用协议 slug；兼容端点与 OpenAI 共用协议。
      const result = await onSubmit({ ...filtered, custom_llm_provider: selected.litellm_provider, provider_id: selected.provider });
      if (result === false) setSaveError(true);
    } catch {
      setSaveError(true);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={value => !value && !saving && onCancel()}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-[680px]" showCloseButton={!saving}>
        <DialogHeader>
          <DialogTitle>{t(isEdit ? "Edit model provider" : "Add model provider")}</DialogTitle>
          <DialogDescription>{t("Choose a provider to configure its connection and authentication.")}</DialogDescription>
        </DialogHeader>
        <FormProvider {...form}>
          <MountedFormProvider value={{ control: form.control, registry }}>
            <form onSubmit={event => { event.preventDefault(); void handleSubmit(); }} noValidate>
              <fieldset disabled={saving} className="min-w-0">
                <MountedFormField label={t("Provider type")} name="custom_llm_provider" required
                  rules={{ validate: { required: requiredRule("Required") } }} className="mb-5">
                  {control => <SearchSelect inputId={control.id} placeholder={t("Select a provider")} options={options}
                    disabled={isEdit || isLoading || !!error || saving} value={selected?.provider ?? (typeof selection === "string" ? selection : null)}
                    onValueChange={value => {
                      // reset({}) 避免 RHF 把编辑时的旧默认密钥、区域和地址重新带入。
                      form.reset({ credential_name: form.getValues("credential_name"), custom_llm_provider: value });
                      setSaveError(false);
                    }} />}
                </MountedFormField>
                {isLoading && <p role="status" className="mb-4 text-muted-foreground">{t("Loading provider fields...")}</p>}
                {error && <div role="alert" className="mb-4 text-destructive">
                  {t("Failed to load provider configuration")}
                  <Button type="button" variant="link" onClick={() => void refetch()}>{t("Retry")}</Button>
                </div>}
                <MountedFormField label={t("Provider name")} name="credential_name" required className="mb-5"
                  rules={{ validate: { required: requiredRule(t("Credential name is required")) } }}>
                  {control => <Input id={control.id} value={typeof control.value === "string" ? control.value : ""}
                    onChange={control.onChange} onBlur={control.onBlur} placeholder={t("Enter a friendly name for these credentials")}
                    disabled={isEdit || saving} />}
                </MountedFormField>
                {selected && <section className="mb-5 rounded-lg border bg-muted/20 p-4" aria-label={t("Connection and authentication")}>
                  <div className="mb-4 flex items-center gap-2">
                    <Logo provider={selected.litellm_provider} label={selected.provider_display_name} className="size-6" />
                    <h3 className="font-medium">{selected.provider_display_name}</h3>
                  </div>
                  <p className="mb-4 text-xs text-muted-foreground">{t("Configuration fields do not guarantee model or account availability.")}</p>
                  {selected.credential_fields.length ? <ProviderSpecificFields key={selected.provider} selectedProvider={selected.provider} />
                    : <p role="alert">{t("This provider requires a separate authorization flow and cannot be saved here yet.")}</p>}
                </section>}
                {!selected && !isLoading && !error && <p className="mb-5 text-sm text-muted-foreground">{t("Select a provider to see its configuration fields.")}</p>}
              </fieldset>
              {saveError && <p role="alert" className="mb-4 text-sm text-destructive">{t("Save failed. Your input has been kept; please retry.")}</p>}
              <div className="flex justify-end gap-2 border-t pt-4">
                <Button type="button" variant="outline" disabled={saving} onClick={onCancel}>{t("Cancel")}</Button>
                <Button type="submit" disabled={saving || isLoading || !!error || !selected?.credential_fields.length}>
                  {t(saving ? "Saving…" : isEdit ? "Save model provider" : "Add model provider")}
                </Button>
              </div>
            </form>
          </MountedFormProvider>
        </FormProvider>
      </DialogContent>
    </Dialog>
  );
}

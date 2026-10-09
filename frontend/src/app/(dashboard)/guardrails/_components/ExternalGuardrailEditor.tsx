import { GuardrailModeSelect } from "./GuardrailModeSelect";
import { useState } from "react";
import { t } from "@/i18n";
import { Guardrail } from "@/components/guardrails/types";
import { applyGuardrail, createGuardrailCall, updateGuardrailCall } from "@/components/networking";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ChevronDown, PlayCircle, Save, ShieldCheck } from "lucide-react";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { EXTERNAL_PROVIDERS, externalGuardrailText, type ExternalField } from "./externalProviders";
/**
 * 用途：根据字段声明选择浏览器输入类型，密钥字段优先隐藏。
 * 参数：field：提供商字段描述。
 * 返回：password、number 或 text。
 * 调用：ExternalForm 的提供商字段渲染。
 * 测试：guardrail_garden.integration.test.tsx 的 Azure 密钥与阈值输入。
 */
function externalInputType(field: ExternalField): "password" | "number" | "text" {
  if (field.secret) return "password";
  if (field.numeric) return "number";
  return "text";
}
interface Props {
  provider: string;
  accessToken: string | null;
  rule?: Guardrail;
  readOnly?: boolean;
  onSuccess: () => void;
  onClose: () => void;
}
/**
 * 用途：展示外部护栏配置入口，通过规则和提供商的 key 在切换时重新初始化表单。
 * 参数：props：提供商 ID、令牌、可选规则、只读状态及成功/关闭回调。
 * 返回：React 配置表单。
 * 调用：护栏花园、护栏详情。
 * 测试：guardrail_garden.integration.test.tsx。
 */
export default function ExternalGuardrailEditor(props: Props) {
  return <ExternalForm key={JSON.stringify([props.provider, props.rule])} {...props} />;
}
/**
 * 用途：管理提供商字段、默认启用、消息范围，以及未保存配置的调试和保存。
 * 参数：Props：提供商协议、已有规则、访问令牌及生命周期回调。
 * 返回：带状态的表单；YAML 来源和非管理员只能查看/测试已保存配置。
 * 调用：ExternalGuardrailEditor。
 * 测试：guardrail_garden.integration.test.tsx。
 */
function ExternalForm({ provider, accessToken, rule, readOnly = false, onSuccess, onClose }: Props) {
  const spec = EXTERNAL_PROVIDERS[provider];
  const [name, setName] = useState(rule?.guardrail_name ?? "");
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      (spec?.fields ?? []).map((field) => [field.key, String(rule?.litellm_params[field.key] ?? field.initial ?? "")]),
    ),
  );
  const [defaultOn, setDefaultOn] = useState(rule?.litellm_params.default_on ?? false);
  const [priority, setPriority] = useState(rule?.litellm_params.priority ?? 100);
  const [action, setAction] = useState(
    rule?.litellm_params.action ?? (provider === "hide-secrets" ? "redact" : "block"),
  );
  const [skipSystem, setSkipSystem] = useState(
    rule?.litellm_params.skip_system_message_in_guardrail == null
      ? "inherit"
      : String(rule.litellm_params.skip_system_message_in_guardrail),
  );
  const [skipTool, setSkipTool] = useState(
    rule?.litellm_params.skip_tool_message_in_guardrail == null
      ? "inherit"
      : String(rule.litellm_params.skip_tool_message_in_guardrail),
  );
  const [text, setText] = useState("");
  const [result, setResult] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  // YAML 只能通过配置文件修改；无管理权限不能通过表单写入。
  const locked = readOnly || rule?.guardrail_definition_location === "config";
  if (!spec) return <p>{externalGuardrailText("尚未移植：可以通过远端 LiteLLM 使用该服务。")}</p>;
  /**
   * 用途：构建调试和保存共用的完整参数，避免表单与实际请求的字段映射不同。
   * 参数：无；读取当前字段、规则和继承选项。
   * 返回：规则对象；数字字段转 number，空字段和 inherit 转 null，后端删除继承覆盖项。
   * 调用：save、test。
   * 测试：Azure 未保存配置调试与保存测试。
   */
  function payload() {
    const params: Record<string, unknown> = {
      ...rule?.litellm_params,
      guardrail: provider,
      mode: "pre_call",
      default_on: defaultOn,
      priority,
      action,
      skip_system_message_in_guardrail: skipSystem === "inherit" ? null : skipSystem === "true",
      skip_tool_message_in_guardrail: skipTool === "inherit" ? null : skipTool === "true",
    };
    for (const field of spec.fields) {
      const value = values[field.key];
      if (!value) params[field.key] = null;
      else if (field.numeric) params[field.key] = Number(value);
      else params[field.key] = value;
    }
    return { guardrail_name: name.trim() || "external-test", litellm_params: params };
  }
  /**
   * 用途：保存当前有效配置并触发列表刷新，错误保持在表单内供修正。
   * 参数：无；读取当前令牌、规则 ID 和 payload。
   * 返回：Promise<void>；成功调用 onSuccess/onClose，失败显示错误，finally 恢复按钮。
   * 调用：保存按钮。
   * 测试：guardrail_garden.integration.test.tsx。
   */
  async function save() {
    if (!accessToken || locked) return;
    setBusy(true);
    setError("");
    try {
      const data = payload();
      if (rule) await updateGuardrailCall(accessToken, rule.guardrail_id, data);
      else await createGuardrailCall(accessToken, data);
      onSuccess();
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("保存失败"));
    } finally {
      setBusy(false);
    }
  }
  /**
   * 用途：使用统一调试接口执行当前表单；密钥被遮蔽时改为执行服务器已保存规则。
   * 参数：无；读取测试文本、当前表单、规则和令牌。
   * 返回：Promise<void>；显示动作/拦截结果或错误，并明确说明是否使用已保存配置。
   * 调用：测试护栏按钮。
   * 测试：guardrail_garden.integration.test.tsx。
   */
  async function test() {
    if (!accessToken) return;
    setBusy(true);
    setError("");
    setResult("");
    try {
      // 掩码不是可发送的密钥。只有服务器能读取已保存凭据；
      // 未输入完整新密钥时使用已保存规则，并提示修改尚未参与这次测试。
      const useSaved = !!rule && spec.fields.some((field) => field.secret && values[field.key] === "********");
      if (useSaved) setResult(t("此测试使用已保存的配置。修改后请保存再测试。") + "\n");
      const response = await applyGuardrail(
        accessToken,
        rule?.guardrail_name ?? name,
        text,
        null,
        null,
        null,
        useSaved || locked ? undefined : payload(),
      );
      setResult(
        (useSaved ? t("此测试使用已保存的配置。修改后请保存再测试。") + "\n" : "") + JSON.stringify(response, null, 2),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : t("测试失败"));
    } finally {
      setBusy(false);
    }
  }
  const invalid = spec.fields.some((field) => field.required && !values[field.key]?.trim());
  return (
    <div className="min-w-0 space-y-6">
      <div className="flex items-start gap-3">
        <span className="rounded-xl bg-primary/10 p-3 text-primary">
          <ShieldCheck className="size-5" />
        </span>
        <div>
          <h3 className="text-lg font-semibold">{spec.name}</h3>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">{externalGuardrailText(spec.description)}</p>
        </div>
      </div>
      {locked && (
        <p className="rounded-lg bg-muted p-3 text-sm">
          {t(
            rule?.guardrail_definition_location === "config"
              ? "该护栏来自 YAML，请在配置文件中编辑。"
              : "当前账号只有查看权限。",
          )}
        </p>
      )}
      <fieldset
        disabled={busy || locked}
        className="grid gap-5 border-b pb-6 sm:grid-cols-2 lg:grid-cols-[1fr_1fr_auto]"
      >
        <label className="space-y-2 text-sm font-medium text-muted-foreground">
          <span>{t("护栏名称")}</span>
          <Input value={name} onChange={(e) => setName(e.target.value)} className="text-foreground" />
        </label>
        <GuardrailModeSelect disabled={busy || locked} />
        <label className="flex items-center gap-3 sm:pt-7">
          <span className="text-sm text-muted-foreground">{t("默认启用（所有请求）")}</span>
          <Switch checked={defaultOn} onCheckedChange={setDefaultOn} disabled={busy || locked} />
        </label>
      </fieldset>
      <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,1fr)_310px]">
        <div className="min-w-0 space-y-4">
          <fieldset disabled={busy || locked} className="rounded-xl border bg-card p-5">
            <h4 className="mb-5 text-sm font-semibold">{t("提供商设置")}</h4>
            <div className="grid gap-5 sm:grid-cols-2">
              {spec.fields.map((field) => (
                <label key={field.key} className="space-y-2 text-sm">
                  <span>
                    {externalGuardrailText(field.label)}
                    {field.required ? " *" : ""}
                  </span>
                  <Input
                    type={externalInputType(field)}
                    value={values[field.key]}
                    autoComplete="off"
                    onChange={(e) => setValues({ ...values, [field.key]: e.target.value })}
                  />
                </label>
              ))}
              {spec.redact && (
                <label className="space-y-2 text-sm">
                  <span>{t("命中后的动作")}</span>
                  <select
                    className="h-10 w-full rounded-md border bg-background px-3"
                    value={action}
                    onChange={(e) => setAction(e.target.value)}
                  >
                    <option value="block">{t("拦截请求")}</option>
                    <option value="redact">{t("脱敏后继续")}</option>
                  </select>
                </label>
              )}
            </div>
          </fieldset>
          <details open className="group rounded-xl border bg-card">
            <summary className="flex cursor-pointer list-none items-center justify-between p-4 text-sm font-semibold">
              <span className="flex items-center gap-2">
                <PlayCircle className="size-4 text-primary" />
                {t("测试你的护栏")}
              </span>
              <ChevronDown className="size-4 -rotate-90 group-open:rotate-0" />
            </summary>
            <div className="space-y-3 px-4 pb-4">
              <label className="block space-y-2 text-sm">
                <span>{t("测试文本")}</span>
                <Textarea value={text} onChange={(e) => setText(e.target.value)} className="min-h-28" />
              </label>
              <Button
                variant="outline"
                disabled={!accessToken || busy || (!rule && invalid)}
                onClick={() => void test()}
              >
                <PlayCircle className="size-4" />
                {t("测试护栏")}
              </Button>
              {result && (
                <pre
                  role="status"
                  className="max-h-80 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted/50 p-4 text-xs leading-6"
                >
                  {result}
                </pre>
              )}
            </div>
          </details>
          <details className="group rounded-xl border bg-card">
            <summary className="flex cursor-pointer list-none items-center justify-between p-4 text-sm font-medium">
              {t("高级设置")}
              <ChevronDown className="size-4 -rotate-90 group-open:rotate-0" />
            </summary>
            <fieldset disabled={busy || locked} className="grid gap-4 px-4 pb-4 sm:grid-cols-3">
              <label className="space-y-2 text-sm">
                <span>{t("优先级")}</span>
                <Input
                  type="number"
                  min={0}
                  max={10000}
                  value={priority}
                  onChange={(e) => setPriority(Number(e.target.value))}
                />
              </label>
              {[
                { label: "跳过系统消息", value: skipSystem, set: setSkipSystem },
                { label: "跳过工具消息", value: skipTool, set: setSkipTool },
              ].map((field) => (
                <label key={field.label} className="space-y-2 text-sm">
                  <span>{externalGuardrailText(field.label)}</span>
                  <select
                    className="h-10 w-full rounded-md border bg-background px-3"
                    value={field.value}
                    onChange={(e) => field.set(e.target.value)}
                  >
                    <option value="inherit">{t("使用全局默认")}</option>
                    <option value="true">{t("是")}</option>
                    <option value="false">{t("否")}</option>
                  </select>
                </label>
              ))}
            </fieldset>
          </details>
        </div>
        <aside className="space-y-4 lg:border-l lg:pl-6">
          <h4 className="font-semibold">{t("接入说明")}</h4>
          <div className="rounded-xl border p-4">
            <h5 className="text-sm font-medium">{t("执行方式")}</h5>
            <p className="mt-2 text-xs leading-6 text-muted-foreground">
              {t("执行阶段：pre_call。服务超时或返回格式错误时拦截；HTTP 总时限为 10 秒。")}
            </p>
          </div>
          <div className="rounded-xl border p-4">
            <h5 className="text-sm font-medium">{t("凭据配置")}</h5>
            <p className="mt-2 break-words text-xs leading-6 text-muted-foreground">
              {t("密钥可以填写 os.environ/XHUB_GUARDRAIL_API_KEY 等环境变量引用；已保存的密钥会隐藏。")}
            </p>
          </div>
        </aside>
      </div>
      {error && (
        <p role="alert" className="whitespace-pre-wrap rounded-lg bg-destructive/5 p-3 text-sm text-destructive">
          {error}
        </p>
      )}
      <div className="flex justify-end gap-2 border-t pt-4">
        <Button variant="outline" disabled={busy} onClick={onClose}>
          {t("返回")}
        </Button>
        <Button disabled={!accessToken || locked || busy || invalid || !name.trim()} onClick={() => void save()}>
          <Save className="size-4" />
          {t("保存护栏")}
        </Button>
      </div>
    </div>
  );
}

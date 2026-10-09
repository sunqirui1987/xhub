import { GuardrailModeSelect } from "./GuardrailModeSelect";
import { t } from "@/i18n";
import React, { useState } from "react";
import { createGuardrailCall, updateGuardrailCall, applyGuardrail } from "@/components/networking";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Switch } from "@/components/ui/switch";
import { ChevronDown, PlayCircle, Save, BookOpen, Copy, Check } from "lucide-react";
import { copyToClipboard } from "@/utils/dataUtils";
import { Guardrail } from "@/components/guardrails/types";

interface Props {
  accessToken: string | null;
  rule?: Guardrail;
  onSuccess: () => void;
  onClose: () => void;
  readOnly?: boolean;
}
/**
 * 用途：展示本地关键词/RE2 规则表单，切换规则时重新初始化输入。
 * 参数：props：访问令牌、可选规则、只读状态、保存成功和关闭回调。
 * 返回：React 表单。
 * 调用：本地护栏弹窗和详情页。
 * 测试：后端 engine_test.go 校验规则语义；LocalGuardrailEditor.integration.test.tsx。
 */
export function LocalGuardrailEditor(props: Props) {
  return <LocalGuardrailForm key={JSON.stringify(props.rule ?? null)} {...props} />;
}

/**
 * 用途：管理关键词、正则、动作与三态消息范围；调试直接执行当前未保存规则；只读规则使用已保存配置。
 * 参数：Props：已有规则、权限及生命周期回调。
 * 返回：React 表单；YAML 规则只读，显示后端实际执行结果。
 * 调用：LocalGuardrailEditor。
 * 测试：后端 engine_test.go 校验规则语义；LocalGuardrailEditor.integration.test.tsx。
 */
function LocalGuardrailForm({ accessToken, rule, onSuccess, onClose, readOnly: explicitlyReadOnly = false }: Props) {
  const p: Partial<Guardrail["litellm_params"]> = rule?.litellm_params ?? {};
  const [name, setName] = useState(rule?.guardrail_name ?? "");
  const [words, setWords] = useState(() => [...(p.blocked_words ?? []), ...(p.keywords ?? [])].join("\n"));
  const [patterns, setPatterns] = useState(() => (p.patterns ?? []).join("\n"));
  const [action, setAction] = useState(
    p.action ?? (p.guardrail === "redact" || p.mode === "redact" ? "redact" : "block"),
  );
  const [replacement, setReplacement] = useState(p.replacement ?? "[REDACTED]");
  const [defaultOn, setDefaultOn] = useState(p.default_on ?? false);
  const [priority, setPriority] = useState(p.priority ?? 100);
  const [skipSystem, setSkipSystem] = useState(
    p.skip_system_message_in_guardrail == null ? "inherit" : String(p.skip_system_message_in_guardrail),
  );
  const [skipTool, setSkipTool] = useState(
    p.skip_tool_message_in_guardrail == null ? "inherit" : String(p.skip_tool_message_in_guardrail),
  );
  const [description, setDescription] = useState(rule?.guardrail_info?.description ?? "");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [text, setText] = useState("");
  const [result, setResult] = useState("");
  const [copied, setCopied] = useState("");
  const [copyError, setCopyError] = useState(false);
  const [template, setTemplate] = useState("custom");
  const readOnly = explicitlyReadOnly || rule?.guardrail_definition_location === "config";
  const saveDisabled = !accessToken || busy || readOnly || !name.trim() || (!words.trim() && !patterns.trim());
  /**
   * 用途：把逐行关键词或正则输入转换成后端规则数组，不改变有效行的顺序。
   * 参数：value：文本框的完整字符串。
   * 返回：移除首尾空白及空行后的字符串数组；不自动合并或重写正则。
   * 调用：save 构建 blocked_words 和 patterns。
   * 测试：LocalGuardrailEditor.integration.test.tsx，后端 engine_test.go 校验数组规则语义。
   */
  const lines = (value: string) =>
    value
      .split("\n")
      .map((v) => v.trim())
      .filter(Boolean);
  /**
   * 用途：生成保存与调试共用的规则快照，避免调试结果来自另一份配置。
   * 参数：无；读取关键词、正则、动作、执行顺序和消息范围。
   * 返回：完整护栏对象；继承选项用 null，未命名草稿使用 local-test。
   * 调用：save、test；后端负责 RE2 编译及参数边界校验。
   * 测试：LocalGuardrailEditor.integration.test.tsx 验证未保存调试与保存一致。
   */
  function payload() {
    return {
      guardrail_name: name.trim() || "local-test",
      guardrail_info: { ...(rule?.guardrail_info ?? {}), description },
      litellm_params: {
        guardrail: "local",
        mode: "pre_call",
        action,
        blocked_words: lines(words),
        keywords: [],
        patterns: lines(patterns),
        replacement,
        default_on: defaultOn,
        priority,
        skip_system_message_in_guardrail: skipSystem === "inherit" ? null : skipSystem === "true",
        skip_tool_message_in_guardrail: skipTool === "inherit" ? null : skipTool === "true",
      },
    };
  }
  /**
   * 用途：加载内置规则示例，保留名称和执行设置，并清除旧结果。
   * 参数：value：block、phone 或 email 模板 ID；custom 保留现有内容。
   * 返回：void；更新关键词、正则、动作和替换文本。
   * 调用：模板下拉框。
   * 测试：LocalGuardrailEditor.integration.test.tsx。
   */
  function changeTemplate(value: string) {
    setTemplate(value);
    if (value === "custom") return;
    setWords(value === "block" ? "内部密钥\n机密资料" : "");
    const templatePatterns: Record<string, string> = {
      phone: "1[3-9][0-9]{9}",
      email: "[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}",
    };
    setPatterns(templatePatterns[value] ?? "");
    setAction(value === "block" ? "block" : "redact");
    setReplacement(value === "email" ? "[邮箱已隐藏]" : "[手机号已隐藏]");
    setResult("");
    setError("");
  }
  /**
   * 用途：修改任一匹配输入时退出模板状态，并清除旧测试结果。
   * 参数：value：新文本；setValue：对应输入框的状态更新函数。
   * 返回：void；仅修改当前草稿，保存仍需显式提交。
   * 调用：关键词和正则文本框。
   * 测试：LocalGuardrailEditor.integration.test.tsx。
   */
  function changeRuleText(value: string, setValue: (value: string) => void) {
    setValue(value);
    setTemplate("custom");
    setResult("");
  }
  /**
   * 用途：复制真实可用的 RE2 示例，不改变当前配置。
   * 参数：pattern：完整正则字符串。
   * 返回：Promise<void>；成功显示已复制，失败提供可访问提示。
   * 调用：右侧参考卡片。
   * 测试：人工检查参考区；复制函数采用公共 copyToClipboard。
   */
  async function copyPattern(pattern: string) {
    const ok = await copyToClipboard(pattern, t("Copied to clipboard"));
    setCopied(ok ? pattern : "");
    setCopyError(!ok);
  }
  /**
   * 用途：把逐行输入转为数组，创建或更新本地护栏配置。
   * 参数：无；读取表单、令牌和当前规则。
   * 返回：Promise<void>；成功通知刷新，失败展示错误。
   * 调用：保存按钮。
   * 测试：后端 guardrail_manage_test.go；LocalGuardrailEditor.integration.test.tsx。
   */
  async function save() {
    if (saveDisabled) return;
    setBusy(true);
    setError("");
    const data = payload();
    try {
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
   * 用途：测试当前表单快照，创建前即可验证；只读状态使用服务器已保存规则。
   * 参数：无；读取测试文本、令牌及 payload。
   * 返回：Promise<void>；显示动作、原因及变换文本，失败清除旧结果。
   * 调用：测试当前规则按钮。
   * 测试：LocalGuardrailEditor.integration.test.tsx。
   */
  async function test() {
    if (!accessToken) return;
    setBusy(true);
    setError("");
    setResult("");
    try {
      const draft = payload();
      const r = await applyGuardrail(
        accessToken,
        rule?.guardrail_name ?? draft.guardrail_name,
        text,
        null,
        null,
        null,
        readOnly ? undefined : draft,
      );
      setResult(JSON.stringify({ action: r.action, reason: r.reason, text: r.response_text }, null, 2));
    } catch (e) {
      setError(e instanceof Error ? e.message : t("测试失败"));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="min-w-0 space-y-6">
      {readOnly && (
        <p className="rounded-lg bg-muted p-3 text-sm">
          {rule?.guardrail_definition_location === "config"
            ? t("该护栏来自 YAML，请在配置文件中编辑。")
            : t("当前账号只有查看权限。")}
        </p>
      )}
      <fieldset
        disabled={busy || readOnly}
        className="grid gap-5 border-b pb-6 sm:grid-cols-2 lg:grid-cols-[1fr_1.25fr_1fr_auto]"
      >
        <label className="space-y-2 text-sm font-medium text-muted-foreground">
          <span>{t("护栏名称")}</span>
          <Input
            value={name}
            placeholder="e.g. sensitive-data"
            onChange={(e) => setName(e.target.value)}
            className="text-foreground"
          />
        </label>
        <GuardrailModeSelect disabled={busy || readOnly} />
        <label className="space-y-2 text-sm font-medium text-muted-foreground">
          <span>{t("模板")}</span>
          <select
            aria-label={t("模板")}
            value={template}
            onChange={(e) => changeTemplate(e.target.value)}
            className="h-10 w-full rounded-md border bg-background px-3 text-foreground"
          >
            <option value="custom">{t("自定义规则")}</option>
            <option value="block">{t("关键词拦截模板")}</option>
            <option value="phone">{t("手机号脱敏模板")}</option>
            <option value="email">{t("邮箱脱敏模板")}</option>
          </select>
        </label>
        <label className="flex items-center gap-3 sm:pt-7">
          <span className="text-sm text-muted-foreground">{t("默认启用")}</span>
          <Switch
            checked={defaultOn}
            onCheckedChange={setDefaultOn}
            aria-label={t("默认启用（客户端不能关闭）")}
            disabled={busy || readOnly}
          />
        </label>
      </fieldset>
      <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,1fr)_310px] xl:grid-cols-[minmax(0,1fr)_340px]">
        <div className="min-w-0 space-y-4">
          <fieldset disabled={busy || readOnly} className="rounded-xl border bg-card p-5">
            <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
              <h3 className="text-sm font-semibold">{t("匹配规则")}</h3>
              <span className="text-xs text-muted-foreground">{t("任一规则命中即执行动作")}</span>
            </div>
            <div className="grid gap-5 sm:grid-cols-2">
              <label className="space-y-2 text-sm">
                <span className="font-medium">{t("关键词（每行一个）")}</span>
                <Textarea
                  aria-label={t("关键词（每行一个）")}
                  value={words}
                  onChange={(e) => changeRuleText(e.target.value, setWords)}
                  placeholder={t("例如：内部密钥")}
                  className="min-h-52 resize-y font-mono text-sm"
                />
                <span className="block text-xs text-muted-foreground">{t("按文本包含匹配，忽略大小写。")}</span>
              </label>
              <label className="space-y-2 text-sm">
                <span className="font-medium">{t("正则表达式（每行一个，可选）")}</span>
                <Textarea
                  aria-label={t("正则表达式（每行一个，可选）")}
                  value={patterns}
                  onChange={(e) => changeRuleText(e.target.value, setPatterns)}
                  placeholder={t("例如：1[3-9][0-9]{9}")}
                  spellCheck={false}
                  className="min-h-52 resize-y font-mono text-sm"
                />
                <span className="block text-xs text-muted-foreground">{t("使用 Go RE2，保存和测试时校验语法。")}</span>
              </label>
            </div>
            <div className="mt-5 grid gap-4 border-t pt-4 sm:grid-cols-2">
              <label className="space-y-2 text-sm">
                <span className="font-medium">{t("命中后的动作")}</span>
                <select
                  value={action}
                  onChange={(e) => {
                    setAction(e.target.value);
                    setResult("");
                  }}
                  className="h-10 w-full rounded-md border bg-background px-3"
                >
                  <option value="block">{t("拦截请求")}</option>
                  <option value="redact">{t("脱敏后继续")}</option>
                </select>
              </label>
              {action === "redact" && (
                <label className="space-y-2 text-sm">
                  <span className="font-medium">{t("替换文本")}</span>
                  <Input
                    value={replacement}
                    onChange={(e) => {
                      setReplacement(e.target.value);
                      setResult("");
                    }}
                  />
                </label>
              )}
            </div>
          </fieldset>
          <details open className="group rounded-xl border bg-card">
            <summary className="flex cursor-pointer list-none items-center gap-2 p-4 font-semibold">
              <ChevronDown className="size-4 -rotate-90 group-open:rotate-0" />
              <PlayCircle className="size-4" />
              {t("测试你的护栏")}
            </summary>
            <div className="space-y-3 px-4 pb-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <label htmlFor="local-guardrail-test" className="text-sm font-medium text-muted-foreground">
                  {t("测试文本")}
                </label>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={busy}
                  onClick={() => setText("联系我：13800138000，邮箱 demo@example.com，内容包含内部密钥。")}
                >
                  {t("加载请求示例")}
                </Button>
              </div>
              <p className="text-xs leading-5 text-muted-foreground">
                {readOnly
                  ? t("测试已保存的规则；输入包含关键词与普通文本，确认拦截或替换结果。")
                  : t("直接测试当前规则，无需先保存。")}
              </p>
              <Textarea
                id="local-guardrail-test"
                value={text}
                onChange={(e) => {
                  setText(e.target.value);
                  setResult("");
                }}
                disabled={busy}
                className="min-h-28"
              />
              <Button
                variant="outline"
                disabled={busy || !accessToken || (!readOnly && !words.trim() && !patterns.trim())}
                onClick={test}
              >
                <PlayCircle className="size-4" />
                {t("测试当前规则")}
              </Button>
              {result && (
                <pre
                  aria-label={t("测试结果")}
                  className="max-h-64 overflow-auto whitespace-pre-wrap rounded-lg border bg-muted/30 p-3 font-mono text-xs"
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
            <fieldset disabled={busy || readOnly} className="grid gap-4 px-4 pb-4 sm:grid-cols-3">
              <label className="space-y-2 text-sm">
                <span>{t("执行优先级（0–10000）")}</span>
                <Input
                  type="number"
                  min={0}
                  max={10000}
                  value={priority}
                  onChange={(e) => setPriority(Number(e.target.value))}
                />
              </label>
              {(
                [
                  [t("系统消息"), skipSystem, setSkipSystem],
                  [t("工具消息"), skipTool, setSkipTool],
                ] as const
              ).map(([label, value, set]) => (
                <label className="space-y-2 text-sm" key={label}>
                  <span>{t("跳过{label}", { label })}</span>
                  <select
                    value={value}
                    onChange={(e) => set(e.target.value)}
                    className="h-10 w-full rounded-md border bg-background px-3"
                  >
                    <option value="inherit">{t("使用全局默认")}</option>
                    <option value="true">{t("跳过")}</option>
                    <option value="false">{t("检查")}</option>
                  </select>
                </label>
              ))}
              <label className="space-y-2 text-sm sm:col-span-3">
                <span>{t("说明")}</span>
                <Textarea value={description} onChange={(e) => setDescription(e.target.value)} />
              </label>
            </fieldset>
          </details>
        </div>
        <aside aria-label={t("规则参考")} className="min-w-0 space-y-4 lg:border-l lg:pl-6">
          <h3 className="flex items-center gap-2 font-semibold">
            <BookOpen className="size-4" />
            {t("规则参考")}
          </h3>
          <p className="text-sm text-muted-foreground">{t("点击示例复制正则表达式。")}</p>
          <details open className="group rounded-xl border">
            <summary className="flex cursor-pointer list-none items-center justify-between p-3 text-sm font-medium">
              {t("常用正则")}
              <ChevronDown className="size-4 -rotate-90 group-open:rotate-0" />
            </summary>
            <div className="space-y-2 px-3 pb-3">
              {[
                [t("手机号"), "1[3-9][0-9]{9}"],
                [t("邮箱"), "[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}"],
                [t("IPv4 格式"), "[0-9]{1,3}(\\.[0-9]{1,3}){3}"],
              ].map(([label, pattern]) => (
                <button
                  key={label}
                  type="button"
                  onClick={() => void copyPattern(pattern)}
                  aria-label={t("复制 {signature} 示例", { signature: label })}
                  className="w-full rounded-lg bg-muted/50 p-3 text-left transition hover:bg-primary/5 focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <span className="flex items-center justify-between text-sm font-medium">
                    {label}
                    {copied === pattern ? (
                      <Check className="size-3.5 text-emerald-600" />
                    ) : (
                      <Copy className="size-3.5 text-muted-foreground" />
                    )}
                  </span>
                  <code className="mt-2 block break-all text-xs text-muted-foreground">{pattern}</code>
                </button>
              ))}
            </div>
          </details>
          <div className="rounded-xl border p-4">
            <h4 className="text-sm font-medium">{t("执行方式")}</h4>
            <p className="mt-2 text-xs leading-6 text-muted-foreground">
              {t(
                "在请求发送给模型前检查文本。关键词忽略大小写；正则使用 Go RE2。按优先级从小到大执行，拦截后停止，脱敏后继续下一条护栏。",
              )}
            </p>
          </div>
          <div className="rounded-xl border p-4">
            <h4 className="text-sm font-medium">{t("匹配提示")}</h4>
            <p className="mt-2 text-xs leading-6 text-muted-foreground">
              {t(
                "关键词和正则采用任一命中；不支持前后查找或反向引用。脱敏使用替换文本原样替换。示例用于格式匹配，不代替完整数据校验。",
              )}
            </p>
          </div>
          <p role="status" className="break-all text-xs text-muted-foreground">
            {copyError && t("复制失败，请手动复制代码。")}
            {copied && t("Copied to clipboard")}
          </p>
        </aside>
      </div>
      {error && (
        <p role="alert" className="whitespace-pre-wrap rounded-lg bg-destructive/5 p-3 text-sm text-destructive">
          {error}
        </p>
      )}
      <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
        <p className="text-xs text-muted-foreground">{t("测试与实际请求使用相同的匹配规则。")}</p>
        <div className="flex gap-2">
          <Button variant="outline" disabled={busy} onClick={onClose}>
            {t("Cancel")}
          </Button>
          <Button disabled={saveDisabled} onClick={save}>
            <Save className="size-4" />
            {t("保存护栏")}
          </Button>
        </div>
      </div>
    </div>
  );
}
/**
 * 用途：将本地表单放入标准弹窗，并仅在可见时展示。
 * 参数：visible：是否打开；其余 Props：表单属性。
 * 返回：React 弹窗或关闭状态。
 * 调用：GuardrailsPanel。
 * 测试：后端 engine_test.go 校验规则语义；LocalGuardrailEditor.integration.test.tsx。
 */
export default function LocalGuardrailModal({ visible, ...props }: Props & { visible: boolean }) {
  return (
    <Dialog
      open={visible}
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent className="max-h-[94vh] w-[calc(100vw-2rem)] max-w-none overflow-y-auto p-6 sm:max-w-[min(96vw,1600px)] sm:p-8">
        <DialogHeader>
          <DialogTitle className="text-xl font-semibold">{t("创建关键词 / 正则护栏")}</DialogTitle>
          <DialogDescription>{t("配置敏感词拦截与文本脱敏，右侧提供规则说明和可复制示例。")}</DialogDescription>
        </DialogHeader>
        {visible && <LocalGuardrailEditor {...props} />}
      </DialogContent>
    </Dialog>
  );
}

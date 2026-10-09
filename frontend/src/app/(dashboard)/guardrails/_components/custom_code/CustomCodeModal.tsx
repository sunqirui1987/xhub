import { GuardrailModeSelect } from "../GuardrailModeSelect";
import { t } from "@/i18n";
import React, { useEffect, useRef, useState } from "react";
import { createGuardrailCall, updateGuardrailCall, testCustomCodeGuardrail } from "@/components/networking";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { ChevronDown, PlayCircle, Save } from "lucide-react";
import PrimitivesPanel from "./PrimitivesPanel";
import { DEFAULT_TEST_INPUT, parseTestInput, PRIMITIVE_GROUPS } from "./primitives";

export interface EditGuardrailData {
  guardrail_id: string;
  guardrail_name: string;
  litellm_params: { mode?: string | string[]; default_on?: boolean; custom_code?: string; [key: string]: any };
}
interface Props {
  visible: boolean;
  onClose: () => void;
  onSuccess: () => void;
  accessToken: string | null;
  editData?: EditGuardrailData | null;
  readOnly?: boolean;
}
// 所有模板都使用同一入口签名；虚拟包只提供白名单能力，不开放任意系统导入。
// 注释随模板复制到编辑器，方便管理员直接理解参数来源和动作回写条件。
const header = [
  'import . "xhub/guardrail"',
  "",
  "// ApplyGuardrail 在模型调用前检查文本，调试与真实请求共用此入口。",
  "// 参数：texts 为按协议顺序提取的文本；requestData 仅提供 model、metadata 副本。",
  "// 参数：inputType 当前固定为 request；metadata 属于客户端数据，不代表可信身份。",
  "// 返回：Allow 放行原文；Block 拦截；Flag 记录告警；Modify 按原数量和顺序回写文本。",
  "// 调用：XHub 的 XGo 执行器；保存会编译校验，测试按钮执行当前未保存源码。",
  "// 测试：分别输入正常文本和命中文本，检查 action、reason 及修改后的 texts。",
  "func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {",
  "",
].join("\n");
// 外部审核和大模型模板取自侧栏的真实示例，防止两处函数签名发生漂移。
const templates = {
  external: header + PRIMITIVE_GROUPS[3].items[1].code + "\n    return Allow()\n}",
  llm: header + PRIMITIVE_GROUPS[5].items[0].code + "\n    return Allow()\n}",
  flag: header + '    return Flag("需要复核", map[string]any{"category":"review"})\n}',
  empty: header + "    return Allow()\n}",
  block:
    header +
    '    for text <- texts {\n        if Contains(Lower(text), "secret") {\n            return Block("文本包含敏感内容")\n        }\n    }\n    return Allow()\n}',
  redact:
    header +
    '    for i, text := range texts {\n        texts[i] = RegexReplace(text, "1[3-9][0-9]{9}", "[手机号已隐藏]")\n    }\n    return Modify(texts)\n}',
};
/**
 * 用途：提供截图布局的 XGo 自定义护栏编辑器、模板、测试区和 primitives 侧栏。
 * 参数：组件属性：可见状态、令牌、可选规则、只读标志及关闭/成功回调。
 * 返回：React 弹窗；语言为 XGo，执行阶段固定 pre_call。
 * 调用：GuardrailsPanel、guardrail_info。
 * 测试：CustomCodeModal.integration.test.tsx。
 */
export default function CustomCodeModal({
  visible,
  onClose,
  onSuccess,
  accessToken,
  editData,
  readOnly = false,
}: Props) {
  const [name, setName] = useState("");
  const [code, setCode] = useState(templates.block);
  const [defaultOn, setDefaultOn] = useState(false);
  const [priority, setPriority] = useState(100);
  const [skipSystem, setSkipSystem] = useState("inherit");
  const [skipTool, setSkipTool] = useState("inherit");
  const [template, setTemplate] = useState("block");
  const [testInput, setTestInput] = useState(DEFAULT_TEST_INPUT);
  const lineNumbersRef = useRef<HTMLDivElement>(null);
  const [result, setResult] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    // 仅在打开弹窗或切换规则时重置；编辑输入不会触发重置，未配置的角色选项保留继承语义。
    if (!visible) return;
    const p = editData?.litellm_params;
    setName(editData?.guardrail_name ?? "");
    setCode(p?.custom_code ?? templates.block);
    setDefaultOn(p?.default_on ?? false);
    setPriority(p?.priority ?? 100);
    setSkipSystem(p?.skip_system_message_in_guardrail == null ? "inherit" : String(p.skip_system_message_in_guardrail));
    setSkipTool(p?.skip_tool_message_in_guardrail == null ? "inherit" : String(p.skip_tool_message_in_guardrail));
    setError("");
    setResult("");
    setTemplate(p?.custom_code ? "custom" : "block");
    setTestInput(DEFAULT_TEST_INPUT);
  }, [visible, editData]);
  /**
   * 用途：测试当前未保存源码，解析输入并把客户端 metadata 明确作为非可信数据传入。
   * 参数：无；读取源码、测试 JSON 和令牌。
   * 返回：Promise<void>；区分编译/运行失败和成功执行后的 block/flag/modify。
   * 调用：测试当前代码按钮。
   * 测试：CustomCodeModal.integration.test.tsx。
   */
  async function test() {
    if (!accessToken) return;
    setBusy(true);
    setError("");
    setResult("");
    try {
      const input = parseTestInput(testInput);
      const response = await testCustomCodeGuardrail(accessToken, {
        custom_code: code,
        test_input: { texts: input.texts, model: input.model },
        request_data: { model: input.model, metadata: input.metadata },
        input_type: "request",
      });
      if (!response.success) setError(response.error ?? t("脚本执行失败"));
      else setResult(JSON.stringify(response.result, null, 2));
    } catch (e) {
      setError(e instanceof Error ? t(e.message) : t("测试失败"));
    } finally {
      setBusy(false);
    }
  }
  /**
   * 用途：创建或更新 XGo 规则，由后端编译校验再持久化。
   * 参数：无；读取名称、源码、规则参数与令牌。
   * 返回：Promise<void>；成功刷新并关闭，失败保留内容。
   * 调用：保存护栏按钮。
   * 测试：CustomCodeModal.integration.test.tsx。
   */
  async function save() {
    if (!accessToken || readOnly) return;
    setBusy(true);
    setError("");
    const data = {
      guardrail_name: name,
      litellm_params: {
        guardrail: "custom_code",
        custom_code_language: "xgo",
        mode: "pre_call",
        custom_code: code,
        default_on: defaultOn,
        priority,
        // null 表示移除本规则覆盖项，由后端继承全局设置；false 则明确要求检查。
        skip_system_message_in_guardrail: skipSystem === "inherit" ? null : skipSystem === "true",
        skip_tool_message_in_guardrail: skipTool === "inherit" ? null : skipTool === "true",
      },
    };
    try {
      if (editData) await updateGuardrailCall(accessToken, editData.guardrail_id, data);
      else await createGuardrailCall(accessToken, data);
      onSuccess();
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("保存失败"));
    } finally {
      setBusy(false);
    }
  }
  const editorDisabled = busy || readOnly;
  const saveDisabled = editorDisabled || !accessToken || !name.trim() || !code.trim();
  const templateLabels = {
    external: t("外部 API 审核模板"),
    llm: t("大模型审核模板"),
    flag: t("告警记录模板"),
    empty: t("空白模板"),
    block: t("关键词拦截模板"),
    redact: t("手机号脱敏模板"),
  };
  /**
   * 用途：切换示例源码并清除之前代码的测试结果。
   * 参数：value：模板 ID，必须来自内置模板表。
   * 返回：无；更新模板和源码状态。
   * 调用：模板下拉框。
   * 测试：CustomCodeModal.integration.test.tsx。
   */
  function changeTemplate(value: string) {
    if (!(value in templates)) return;
    setTemplate(value);
    setCode(templates[value as keyof typeof templates]);
    setResult("");
    setError("");
  }
  /**
   * 用途：更新用户源码，并使旧测试结果失效。
   * 参数：value：代码编辑器的完整文本。
   * 返回：无；保留用户输入并清除旧结果。
   * 调用：XGo 代码编辑器。
   * 测试：CustomCodeModal.integration.test.tsx。
   */
  function changeCode(value: string) {
    setCode(value);
    setTemplate("custom");
    setResult("");
    setError("");
  }
  return (
    <Dialog
      open={visible}
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent className="flex max-h-[94vh] w-[calc(100vw-2rem)] max-w-none flex-col gap-0 overflow-hidden p-0 sm:max-w-[min(96vw,1600px)]">
        <DialogHeader className="shrink-0 px-6 pb-5 pt-6 sm:px-8">
          <DialogTitle className="text-xl font-semibold">
            {editData ? t("编辑 XGo 护栏") : t("创建 XGo 护栏")}
          </DialogTitle>
          <DialogDescription>{t("使用 Go / XGo 语法定义自定义逻辑，右侧函数卡片可复制到代码中。")}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 overflow-y-auto px-6 pb-6 sm:px-8">
          {readOnly && (
            <p className="mb-4 rounded-md bg-muted p-3 text-sm">
              {t("该护栏仅供查看。YAML 规则请在配置文件中编辑；数据库规则需管理员权限。")}
            </p>
          )}
          <fieldset
            disabled={editorDisabled}
            className="grid gap-5 border-b pb-6 sm:grid-cols-2 lg:grid-cols-[1fr_1.25fr_1fr_auto]"
          >
            <label className="block space-y-2 text-sm font-medium text-muted-foreground">
              <span>{t("护栏名称")}</span>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. block-sensitive-data"
                className="text-foreground"
              />
            </label>
            <GuardrailModeSelect disabled={editorDisabled} />
            <label className="block space-y-2 text-sm font-medium text-muted-foreground">
              <span>{t("模板")}</span>
              <select
                value={template}
                onChange={(e) => changeTemplate(e.target.value)}
                className="h-10 w-full rounded-md border bg-background px-3 text-foreground"
              >
                {Object.entries(templateLabels).map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
                <option value="custom" disabled>
                  {t("自定义代码")}
                </option>
              </select>
            </label>
            <label className="flex items-center gap-3 sm:pt-7">
              <span className="text-sm text-muted-foreground">{t("默认启用")}</span>
              <Switch
                checked={defaultOn}
                onCheckedChange={setDefaultOn}
                disabled={editorDisabled}
                aria-label={t("默认启用（客户端不能关闭）")}
              />
            </label>
          </fieldset>
          <div className="mt-6 grid min-w-0 gap-6 lg:grid-cols-[minmax(0,1fr)_310px] xl:grid-cols-[minmax(0,1fr)_340px]">
            <div className="min-w-0 space-y-4">
              <div>
                <div className="mb-2 flex flex-wrap items-center justify-between gap-2 text-sm">
                  <label htmlFor="xgo-guardrail-code" className="font-semibold text-muted-foreground">
                    {t("XGo 代码")}
                  </label>
                  <span className="text-xs text-muted-foreground">{t("受限环境 · 仅可导入 xhub/guardrail")}</span>
                </div>
                <div className="flex h-[360px] overflow-hidden rounded-lg border border-slate-700 bg-[#1e1e1e] focus-within:ring-2 focus-within:ring-ring sm:h-[400px]">
                  <div
                    ref={lineNumbersRef}
                    aria-hidden="true"
                    className="w-12 shrink-0 overflow-hidden border-r border-slate-700 py-4 text-right font-mono text-sm leading-6 text-slate-500"
                  >
                    {code.split("\n").map((_, i) => (
                      <div key={i} className="pr-3">
                        {i + 1}
                      </div>
                    ))}
                  </div>
                  <textarea
                    id="xgo-guardrail-code"
                    value={code}
                    disabled={editorDisabled}
                    spellCheck={false}
                    wrap="off"
                    onChange={(e) => changeCode(e.target.value)}
                    onScroll={(e) => {
                      if (lineNumbersRef.current) lineNumbersRef.current.scrollTop = e.currentTarget.scrollTop;
                    }}
                    className="h-full min-w-0 flex-1 resize-none overflow-auto bg-transparent p-4 font-mono text-sm leading-6 text-slate-200 outline-none disabled:opacity-70"
                  />
                </div>
              </div>
              <details open className="group rounded-lg border">
                <summary className="flex cursor-pointer list-none items-center gap-2 p-4 font-semibold">
                  <ChevronDown className="size-4 -rotate-90 group-open:rotate-0" />
                  <PlayCircle className="size-4" />
                  {t("测试你的护栏")}
                </summary>
                <div className="space-y-3 px-4 pb-4">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <label htmlFor="xgo-test-json" className="text-sm font-medium text-muted-foreground">
                      {t("测试输入（JSON）")}
                    </label>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={busy}
                      onClick={() => setTestInput(DEFAULT_TEST_INPUT)}
                    >
                      {t("加载请求示例")}
                    </Button>
                  </div>
                  <p className="rounded-md bg-muted/40 p-3 text-xs leading-5 text-muted-foreground">
                    {t("texts：待检查文本数组；model：模型名称；metadata：客户端元数据。inputType 固定为 request。")}
                  </p>
                  <Textarea
                    id="xgo-test-json"
                    value={testInput}
                    onChange={(e) => setTestInput(e.target.value)}
                    disabled={busy}
                    spellCheck={false}
                    className="min-h-40 font-mono text-sm"
                  />
                  <Button variant="outline" disabled={busy || !accessToken} onClick={test}>
                    <PlayCircle className="size-4" />
                    {t("测试当前代码")}
                  </Button>
                  {error && (
                    <p
                      role="alert"
                      className="whitespace-pre-wrap rounded-md bg-destructive/5 p-3 text-sm text-destructive"
                    >
                      {error}
                    </p>
                  )}
                  {result && (
                    <pre
                      aria-label={t("测试结果")}
                      className="max-h-64 overflow-auto whitespace-pre-wrap rounded-md border bg-muted/30 p-3 font-mono text-xs"
                    >
                      {result}
                    </pre>
                  )}
                </div>
              </details>
              <details className="group rounded-lg border">
                <summary className="flex cursor-pointer list-none items-center justify-between p-4 text-sm font-medium">
                  {t("高级设置")}
                  <ChevronDown className="size-4 -rotate-90 group-open:rotate-0" />
                </summary>
                <fieldset disabled={editorDisabled} className="grid gap-4 px-4 pb-4 sm:grid-cols-3">
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
                    <label className="block space-y-2 text-sm" key={label}>
                      <span>{t("跳过{label}", { label })}</span>
                      <select
                        value={value}
                        onChange={(e) => set(e.target.value)}
                        className="h-10 w-full rounded-md border bg-background px-2"
                      >
                        <option value="inherit">{t("使用全局默认")}</option>
                        <option value="true">{t("跳过")}</option>
                        <option value="false">{t("检查")}</option>
                      </select>
                    </label>
                  ))}
                </fieldset>
                <p className="px-4 pb-4 text-xs leading-5 text-muted-foreground">
                  {t(
                    "当前支持 pre_call 文本检查。仅可导入 xhub/guardrail，通过提供的 HTTP 与 LLM 函数访问服务；不支持文件、goroutine、channel、init/main。纯本地脚本限时 100ms，含网络函数的脚本总限时 10 秒。错误或超时会拦截请求。仅供可信管理员使用，不提供独立的内存隔离。",
                  )}
                </p>
              </details>
            </div>
            <PrimitivesPanel />
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-t bg-background px-6 py-4 sm:px-8">
          <p className="text-xs text-muted-foreground">{t("保存时编译校验；测试与实际请求共用执行器。")}</p>
          <div className="flex shrink-0 gap-2">
            <Button variant="outline" disabled={busy} onClick={onClose}>
              {t("Cancel")}
            </Button>
            <Button disabled={saveDisabled} onClick={save}>
              <Save className="size-4" />
              {t("保存护栏")}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

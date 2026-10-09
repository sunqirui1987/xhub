import React, { useState } from "react";
import { Copy, Info } from "lucide-react";
import { toast } from "@/lib/toast";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { UiLoadingSpinner } from "@/components/ui/ui-loading-spinner";
import GuardrailTestResults from "./GuardrailTestResults";
import { t } from "@/i18n";

interface GuardrailTestPanelProps {
  guardrailNames: string[];
  onSubmit: (text: string, metadata?: Record<string, unknown> | null) => void;
  isLoading: boolean;
  results: Array<{
    guardrailName: string;
    response_text: string;
    latency: number;
    action?: string;
    reason?: string;
  }> | null;
  errors: Array<{ guardrailName: string; error: Error; latency: number }> | null;
  onClose: () => void;
}

/**
 * 用途：提供普通护栏调试输入、metadata 校验及结果展示。
 * 参数：属性：规则名称、提交/关闭回调、忙碌状态、结果和错误数组。
 * 返回：React 测试表单；实际规则执行由父组件处理。
 * 调用：GuardrailTestPlayground。
 * 测试：GuardrailTestPanel.test.tsx。
 */
export function GuardrailTestPanel({
  guardrailNames,
  onSubmit,
  isLoading,
  results,
  errors,
  onClose,
}: GuardrailTestPanelProps) {
  const [inputText, setInputText] = useState("");
  const [metadataText, setMetadataText] = useState("");
  const [metadataError, setMetadataError] = useState<string | null>(null);

  /**
   * 用途：严格解析可选 metadata 对象，避免把数组或基础值发给执行器。
   * 参数：raw：JSON 文本，空白代表未提供。
   * 返回：metadata/error 对象；解析失败使用错误字段，不抛出。
   * 调用：handleSubmit。
   * 测试：GuardrailTestPanel.test.tsx。
   */
  const parseMetadata = (raw: string): { metadata: Record<string, unknown> | null; error: string | null } => {
    if (!raw.trim()) {
      return { metadata: null, error: null };
    }
    try {
      const parsed = JSON.parse(raw);
      if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
        return { metadata: null, error: "Metadata must be a JSON object" };
      }
      return { metadata: parsed, error: null };
    } catch {
      return { metadata: null, error: "Invalid JSON" };
    }
  };

  /**
   * 用途：校验测试文本和 metadata，再调用父组件的调试入口。
   * 参数：无；读取输入状态。
   * 返回：void；校验失败显示错误，成功传递原文本和 metadata。
   * 调用：测试按钮。
   * 测试：GuardrailTestPanel.test.tsx。
   */
  const handleSubmit = () => {
    if (!inputText.trim()) {
      toast.fromError(t("Please enter text to test"));
      return;
    }

    const { metadata, error } = parseMetadata(metadataText);
    if (error) {
      setMetadataError(error);
      toast.fromError(`Metadata: ${error}`);
      return;
    }
    setMetadataError(null);

    onSubmit(inputText, metadata);
  };

  /**
   * 用途：处理 Enter 提交，同时允许 Shift/Ctrl/Meta 修饰键保留编辑行为。
   * 参数：e：文本区键盘事件。
   * 返回：void；普通 Enter 阻止默认换行并触发提交。
   * 调用：输入文本区。
   * 测试：GuardrailTestPanel.test.tsx。
   */
  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !e.ctrlKey && !e.metaKey) {
      e.preventDefault();
      handleSubmit();
    }
  };

  /**
   * 用途：复制测试输入，兼容安全剪贴板和旧浏览器执行命令。
   * 参数：text：要复制的原输入。
   * 返回：Promise<boolean>；复制失败返回 false。
   * 调用：handleCopyInput。
   * 测试：GuardrailTestPanel.test.tsx。
   */
  const copyToClipboard = async (text: string) => {
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text);
        return true;
      } else {
        const textArea = document.createElement("textarea");
        textArea.value = text;
        textArea.style.position = "fixed";
        textArea.style.opacity = "0";
        document.body.appendChild(textArea);
        textArea.focus();
        textArea.select();

        const successful = document.execCommand("copy");
        document.body.removeChild(textArea);

        if (!successful) {
          throw new Error(t("execCommand failed"));
        }
        return true;
      }
    } catch (error) {
      console.error("Copy failed:", error);
      return false;
    }
  };

  /**
   * 用途：复制当前测试文本，并依据复制结果提示成功或失败。
   * 参数：无；读取 inputText。
   * 返回：Promise<void>；通过 toast 展示结果。
   * 调用：复制输入按钮。
   * 测试：GuardrailTestPanel.test.tsx。
   */
  const handleCopyInput = async () => {
    const success = await copyToClipboard(inputText);
    if (success) {
      toast.success(t("Input copied to clipboard"));
    } else {
      toast.fromError(t("Failed to copy input"));
    }
  };

  return (
    <div className="space-y-4 h-full flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-border pb-3">
        <div className="flex items-center space-x-3">
          <div className="flex-1 min-w-0">
            <div className="mb-1 flex items-center space-x-2">
              <h2 className="text-lg font-semibold">{t("Test Guardrails:")}</h2>
              <div className="flex flex-wrap gap-2">
                {guardrailNames.map((name) => (
                  <div
                    key={name}
                    className="inline-flex items-center space-x-1 rounded-md border border-info/20 bg-info/10 px-3 py-1"
                  >
                    <span className="font-mono text-sm font-medium text-info">{name}</span>
                  </div>
                ))}
              </div>
            </div>
            <p className="text-sm text-muted-foreground">
              {t("Test {value0} and compare results", {
                value0: guardrailNames.length > 1 ? "guardrails" : "guardrail",
              })}
            </p>
          </div>
        </div>
      </div>

      {/* Input Section */}
      <div className="flex-1 space-y-4 overflow-auto px-1">
        <div className="space-y-3">
          <div>
            <div className="mb-2 flex items-center justify-between">
              <div className="flex items-center gap-2">
                <label className="text-sm font-medium">{t("Input Text")}</label>
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <span className="cursor-help text-muted-foreground">
                        <Info className="size-3.5" />
                      </span>
                    }
                  />
                  <TooltipContent>{t("Press Enter to submit. Use Shift+Enter for new line.")}</TooltipContent>
                </Tooltip>
              </div>
              {inputText && (
                <Button size="sm" variant="secondary" onClick={handleCopyInput}>
                  <Copy />
                  {t("Copy Input")}
                </Button>
              )}
            </div>
            <Textarea
              value={inputText}
              onChange={(e) => setInputText(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder={t("Enter text to test with guardrails...")}
              rows={8}
              className="font-mono text-sm field-sizing-fixed"
            />
            <div className="mt-1 flex items-center justify-between">
              <span className="text-xs text-muted-foreground">
                {t("Press")}{" "}
                <kbd className="rounded-sm border border-border bg-muted px-1 py-0.5 text-xs">{t("Enter")}</kbd>{" "}
                {t("to submit •")}{" "}
                <kbd className="rounded-sm border border-border bg-muted px-1 py-0.5 text-xs">Shift+Enter</kbd>{" "}
                {t("for new line")}
              </span>
              <span className="text-xs text-muted-foreground">Characters: {inputText.length}</span>
            </div>
          </div>

          <div>
            <div className="mb-2 flex items-center gap-2">
              <label className="text-sm font-medium">{t("Metadata (optional)")}</label>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <span className="cursor-help text-muted-foreground">
                      <Info className="size-3.5" />
                    </span>
                  }
                />
                <TooltipContent>
                  {t(
                    "JSON object forwarded to the guardrail as request_data['metadata']. Custom guardrails can read per-request configuration from it.",
                  )}
                </TooltipContent>
              </Tooltip>
            </div>
            <Textarea
              value={metadataText}
              onChange={(e) => {
                setMetadataText(e.target.value);
                if (metadataError) {
                  setMetadataError(parseMetadata(e.target.value).error);
                }
              }}
              placeholder='{"forbidden_topics": ["tax", "finance"]}'
              rows={3}
              className="font-mono text-sm field-sizing-fixed"
              aria-invalid={metadataError ? true : undefined}
            />
            {metadataError && <span className="text-xs text-destructive">{metadataError}</span>}
          </div>

          <div className="pt-2">
            <Button
              onClick={handleSubmit}
              disabled={!inputText.trim() || isLoading}
              aria-busy={isLoading}
              className="w-full"
            >
              {isLoading && <UiLoadingSpinner className="size-4" />}
              {isLoading
                ? t("Testing {value0} guardrail{value1}...", {
                    value0: guardrailNames.length,
                    value1: guardrailNames.length > 1 ? "s" : "",
                  })
                : t("Test {value0} guardrail{value1}", {
                    value0: guardrailNames.length,
                    value1: guardrailNames.length > 1 ? "s" : "",
                  })}
            </Button>
          </div>
        </div>

        {/* Results Section */}
        <GuardrailTestResults results={results} errors={errors} />
      </div>
    </div>
  );
}

export default GuardrailTestPanel;

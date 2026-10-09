import { useState } from "react";
import { ChevronDown, Code, Copy, Check } from "lucide-react";
import { t } from "@/i18n";
import { copyToClipboard } from "@/utils/dataUtils";
import { PRIMITIVE_GROUPS, LANGUAGE_PRIMITIVES } from "./primitives";

/**
 * 用途：渲染可折叠的 XGo 函数参考，点击复制可粘贴的片段，并提供可访问的结果提示。
 * 参数：无；读取 PRIMITIVE_GROUPS 与 LANGUAGE_PRIMITIVES。
 * 返回：React 侧栏；注册函数和语言内置能力分别列出。
 * 调用：CustomCodeModal。
 * 测试：CustomCodeModal.integration.test.tsx。
 */
export default function PrimitivesPanel() {
  const [copied, setCopied] = useState("");
  const [copyError, setCopyError] = useState(false);
  const groups = [...PRIMITIVE_GROUPS, { title: "Go / XGo 语言能力", items: LANGUAGE_PRIMITIVES }];
  /**
   * 用途：复制函数片段并记录成功签名或失败状态。
   * 参数：signature：展示签名；code：实际复制的 XGo 示例。
   * 返回：Promise<void>；更新复制图标及 role=status，失败不显示成功标记。
   * 调用：函数参考卡片的点击事件。
   * 测试：CustomCodeModal.integration.test.tsx 的复制成功/失败测试。
   */
  async function copy(signature: string, code: string) {
    const success = await copyToClipboard(code, t("Copied to clipboard"));
    setCopied(success ? signature : "");
    setCopyError(!success);
  }
  let copyStatus = "";
  if (copyError) copyStatus = t("复制失败，请手动复制代码。");
  else if (copied) copyStatus = t("已复制 {signature} 示例", { signature: copied });
  return (
    <aside aria-label={t("可用 Primitives")} className="min-w-0 space-y-4 lg:border-l lg:pl-6">
      <h3 className="flex items-center gap-2 text-base font-semibold">
        <Code className="size-4" />
        {t("可用 Primitives")}
      </h3>
      <p className="text-sm text-muted-foreground">{t("点击函数卡片复制 XGo 示例。")}</p>
      <p className="text-xs text-muted-foreground">
        {t("返回值、HTTP、JSON、正则与大模型函数由护栏运行环境提供，导入 xhub/guardrail 后可用。")}
      </p>
      {groups.map((group, i) => (
        <details key={group.title} open={i === 0} className="group rounded-lg border">
          <summary className="flex cursor-pointer list-none items-center justify-between p-3 font-medium">
            {t(group.title)}
            <ChevronDown className="size-4 -rotate-90 transition-transform group-open:rotate-0" />
          </summary>
          <div className="space-y-2 px-3 pb-3">
            {group.items.map((item) => (
              <button
                key={item.signature}
                type="button"
                aria-label={t("复制 {signature} 示例", { signature: item.signature })}
                onClick={() => void copy(item.signature, item.code)}
                className="w-full rounded-md border border-transparent bg-muted/50 p-3 text-left transition hover:border-primary/40 hover:bg-primary/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="flex items-start justify-between gap-2">
                  <code className="break-words font-mono text-sm">{item.signature}</code>
                  {copied === item.signature ? (
                    <Check className="size-3.5 shrink-0 text-green-600" />
                  ) : (
                    <Copy className="size-3.5 shrink-0 text-muted-foreground" />
                  )}
                </span>
                <span className="mt-1 block text-xs text-muted-foreground">{t(item.description)}</span>
              </button>
            ))}
          </div>
        </details>
      ))}
      <p role="status" className="text-xs text-muted-foreground">
        {copyStatus}
      </p>
    </aside>
  );
}

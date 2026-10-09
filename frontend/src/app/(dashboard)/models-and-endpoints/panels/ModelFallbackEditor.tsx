"use client";
import { useState, useEffect } from "react";
import { apiClient } from "@/components/networking";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

export type FallbackPolicy = {
  fallbacks?: string[];
  context_window_fallbacks?: string[];
  content_policy_fallbacks?: string[];
};
const categories = [
  ["fallbacks", "通用错误", "限流、超时、连接失败或服务端错误时尝试。"],
  ["context_window_fallbacks", "上下文超限", "请求超过模型上下文窗口时尝试。"],
  ["content_policy_fallbacks", "内容策略错误", "上游模型返回内容策略错误时尝试。"],
] as const;
/** 在公开模型标题旁编辑三类有序回退；参数为模型、策略、凭证与刷新回调。
 * 返回带标题、焦点管理的弹窗；打开时读取全部分页目录，保存失败保留草稿，取消不写入。
 * initiallyOpen 用于路由设置已有弹窗中的内嵌表单，避免重复创建弹窗。保存期间禁止关闭。
 * 目标必须是其他公开模型；后端负责循环校验和请求权限检查。
 */
export default function ModelFallbackEditor({
  model,
  policy,
  accessToken,
  onSaved,
  initiallyOpen = false,
  onBusyChange,
  onCancel,
}: {
  model: string;
  policy?: FallbackPolicy;
  accessToken: string;
  onSaved: () => void;
  initiallyOpen?: boolean;
  /** 通知外层弹窗保存状态，避免保存途中更换主模型或关闭弹窗。 */
  onBusyChange?: (busy: boolean) => void;
  /** 外层弹窗取消回调；独立使用时仅关闭本地草稿。 */
  onCancel?: () => void;
}) {
  const [draft, setDraft] = useState<FallbackPolicy | null>(null);
  const [options, setOptions] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  /** 初始化独立草稿并加载所有公开模型；返回 Promise，加载失败显示错误并禁止保存。 */
  const edit = async () => {
    setDraft(Object.fromEntries(categories.map(([key]) => [key, [...(policy?.[key] ?? [])]])));
    setError("");
    setLoading(true);
    try {
      const names: string[] = [];
      let page = 1,
        total = 1;
      do {
        const result = await apiClient.get<{ data: { model_name: string }[]; total_pages: number }>("/model/groups", {
          accessToken,
          query: { page, size: 200 },
        });
        names.push(...result.data.map((row) => row.model_name));
        total = result.total_pages;
        page++;
      } while (page <= total);
      setOptions(
        Array.from(new Set(names))
          .filter((name) => name !== model)
          .sort(),
      );
    } catch {
      setError("回退模型加载失败，请取消后重试。");
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    if (initiallyOpen) void edit();
  }, [initiallyOpen, model]);
  /** 写入完整策略；空列表清除回退，成功刷新目录，失败保留输入和错误。 */
  const save = async () => {
    if (!draft) return;
    setSaving(true);
    onBusyChange?.(true);
    setError("");
    try {
      await apiClient.put("/model/fallback", { accessToken, body: { model_name: model, policy: draft } });
      setDraft(null);
      onSaved();
    } catch (err) {
      setError(err instanceof Error ? err.message : "回退配置保存失败");
    } finally {
      setSaving(false);
      onBusyChange?.(false);
    }
  };
  /** 替换单类有序目标，参数为类别与新数组；仅修改草稿，无服务器副作用。 */
  const update = (key: keyof FallbackPolicy, targets: string[]) =>
    setDraft((current) => ({ ...current, [key]: targets }));
  const form = draft && (
    <section aria-label={model + " 回退配置"} className="min-h-0 w-full space-y-4 overflow-y-auto p-1">
      {loading && <p role="status">正在加载回退模型…</p>}
      {categories.map(([key, label, help]) => (
        <fieldset key={key} disabled={loading || saving} className="space-y-2">
          <legend className="font-medium">{label}</legend>
          <p className="text-sm text-muted-foreground">{help}</p>
          {(draft[key] ?? []).map((name, index, targets) => (
            <div key={index} className="flex flex-wrap gap-2">
              <select
                className="w-full min-w-0 rounded border bg-background p-2 sm:flex-1"
                aria-label={label + "目标 " + (index + 1)}
                value={name}
                onChange={(event) =>
                  update(
                    key,
                    targets.map((item, i) => (i === index ? event.target.value : item)),
                  )
                }
              >
                <option value="">选择回退模型</option>
                {Array.from(new Set([...options, ...targets.filter(Boolean)])).map((option) => (
                  <option key={option} value={option} disabled={targets.includes(option) && option !== name}>
                    {option}
                    {!options.includes(option) ? "（不可用）" : ""}
                  </option>
                ))}
              </select>
              <Button
                variant="outline"
                aria-label={label + "上移 " + (index + 1)}
                disabled={index === 0}
                onClick={() => {
                  const next = [...targets];
                  [next[index - 1], next[index]] = [next[index], next[index - 1]];
                  update(key, next);
                }}
              >
                上移
              </Button>
              <Button
                variant="outline"
                aria-label={label + "下移 " + (index + 1)}
                disabled={index === targets.length - 1}
                onClick={() => {
                  const next = [...targets];
                  [next[index + 1], next[index]] = [next[index], next[index + 1]];
                  update(key, next);
                }}
              >
                下移
              </Button>
              <Button
                variant="ghost"
                aria-label={label + "删除 " + (index + 1)}
                onClick={() =>
                  update(
                    key,
                    targets.filter((_, i) => i !== index),
                  )
                }
              >
                删除
              </Button>
            </div>
          ))}
          <Button
            variant="outline"
            disabled={(draft[key]?.length ?? 0) >= 32 || options.length === 0}
            onClick={() => update(key, [...(draft[key] ?? []), ""])}
          >
            添加{label}回退
          </Button>
        </fieldset>
      ))}
      {error && <p role="alert">{error}</p>}
      <div className="sticky bottom-0 flex flex-wrap gap-2 border-t bg-popover py-3">
        <Button
          disabled={
            loading ||
            saving ||
            error.includes("加载失败") ||
            categories.some(([key]) => (draft[key] ?? []).some((name) => !name))
          }
          onClick={save}
        >
          {saving ? "正在保存…" : "保存回退"}
        </Button>
        <Button
          variant="outline"
          disabled={saving}
          onClick={() => setDraft({ fallbacks: [], context_window_fallbacks: [], content_policy_fallbacks: [] })}
        >
          清空回退
        </Button>
        <Button
          variant="ghost"
          disabled={saving}
          onClick={() => {
            setDraft(null);
            onCancel?.();
          }}
        >
          取消
        </Button>
      </div>
    </section>
  );
  // 路由设置已经提供弹窗和主模型选择；此场景只渲染编辑内容。
  if (initiallyOpen) return form;
  return (
    <Dialog
      open={draft !== null}
      onOpenChange={(open) => {
        // 写入完成前不响应遮罩、Escape 或关闭按钮，防止待保存草稿被丢弃。
        if (saving) return;
        if (open) void edit();
        else setDraft(null);
      }}
    >
      <div className="min-w-0 max-w-sm space-y-2">
        <DialogTrigger render={<Button size="sm" variant="outline" />}>配置回退</DialogTrigger>
        <p className="break-words text-xs text-muted-foreground">
          {categories
            .filter(([key]) => policy?.[key]?.length)
            .map(([key, label]) => label + "：" + policy![key]!.join(" → "))
            .join("；") || "未配置回退"}
        </p>
      </div>
      <DialogContent className="flex max-h-[calc(100dvh-2rem)] flex-col sm:max-w-3xl" showCloseButton={!saving}>
        <DialogHeader className="shrink-0 pr-8">
          <DialogTitle className="break-words">{model} 回退配置</DialogTitle>
          <DialogDescription>
            先尝试当前模型的部署，再按顺序尝试回退模型。目标模型也需具有调用权限；开始输出后不会切换模型。
          </DialogDescription>
        </DialogHeader>
        {form}
      </DialogContent>
    </Dialog>
  );
}

"use client";
import { useState } from "react";
import { Send, X } from "lucide-react";
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogClose,
} from "@/components/ui/dialog";
import { useT } from "@/i18n";
import { ApiRunner } from "./ApiRunner";

/** 接口栏与运行弹窗；参数为当前网关、方法、路径和示例正文，返回模态调试入口。
 * 打开不发送请求，关闭卸载运行器并清除内存密钥、取消等待；上游请求可能已执行。供文档文章使用，切换文章由稳定 key 重置。 */
export function ApiRunnerDialog(props: { base: string; endpoint: string; method: string; initialBody?: string }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <div className="mt-6 flex flex-wrap items-center gap-3 rounded-xl border border-border p-3 text-sm">
        <strong className="rounded bg-emerald-500/10 px-2 py-1 text-emerald-600">{props.method}</strong>
        <span className="min-w-0 break-all text-muted-foreground">{props.base}</span>
        <code className="min-w-0 flex-1 break-all">{props.endpoint}</code>
        <DialogTrigger className="flex items-center gap-2 rounded border border-emerald-600/30 px-3 py-2 text-emerald-600">
          <Send size={16} aria-hidden="true" />
          {t("docs.runner.title")}
        </DialogTrigger>
      </div>
      <DialogContent showCloseButton={false} className="max-h-[90dvh] overflow-y-auto sm:max-w-3xl">
        <DialogTitle className="pr-10 text-xl">{t("docs.runner.title")}</DialogTitle>
        <DialogDescription className="break-all font-mono">
          {props.method} {props.base}
          {props.endpoint}
        </DialogDescription>
        <DialogClose aria-label={t("docs.runner.close")} className="absolute top-4 right-4 rounded p-2">
          <X size={18} />
        </DialogClose>
        {open && <ApiRunner {...props} />}
      </DialogContent>
    </Dialog>
  );
}

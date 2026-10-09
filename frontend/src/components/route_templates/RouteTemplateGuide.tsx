"use client";

import { useState } from "react";
import { BookOpen } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import RouteTemplateJsonGuide from "./RouteTemplateJsonGuide";

/** RouteTemplateGuide 返回完整配置说明弹窗；可选参数兼容已有调用。
 * 模板库和编辑页共用；打开关闭只改变展示状态，不修改草稿或后台数据。 */
export default function RouteTemplateGuide(_props: { compact?: boolean }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-muted/20 px-4 py-3">
        <p className="text-sm text-muted-foreground">
          在一份模板中配置负载均衡、路由组和故障转移，再绑定到组织、团队或个人密钥。
        </p>
        <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
          <BookOpen />
          完整 JSON 配置指南
        </Button>
      </div>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="flex max-h-[90vh] flex-col sm:max-w-6xl">
          <DialogHeader>
            <DialogTitle>完整 JSON 配置指南</DialogTitle>
            <DialogDescription>完整字段说明、可导入示例、执行顺序与 LiteLLM 配置对照。</DialogDescription>
          </DialogHeader>
          <div className="min-h-0 overflow-y-auto pr-1">
            <RouteTemplateJsonGuide />
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

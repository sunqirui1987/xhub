import type { FC, ReactNode } from "react";
import { Separator } from "@/components/ui/separator";

/**
 * 用途：统一旧详情分区标题样式。
 * 参数：children：标题内容。
 * 返回：React 标题与分隔线。
 * 调用：历史详情布局。
 * 测试：guardrail_info.integration.test.tsx；旧工具/PII 分支无新增专门测试。
 */
export const GuardrailSectionHeading: FC<{ children: ReactNode }> = ({ children }) => (
  <div className="my-6 flex items-center gap-3">
    <span className="shrink-0 text-sm font-medium text-foreground">{children}</span>
    <Separator className="flex-1" />
  </div>
);

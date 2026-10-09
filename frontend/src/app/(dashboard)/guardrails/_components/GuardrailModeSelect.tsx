import { useId } from "react";
import { t } from "@/i18n";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

/**
 * 用途：统一三个护栏编辑器的执行阶段下拉框，展示当前支持范围。
 * 参数：disabled：只读或提交期间禁止操作；默认允许展开菜单。
 * 返回：带标签、说明及可访问禁用状态的阶段选择控件。
 * 调用：关键词/正则、XGo 和外部服务护栏编辑器。
 * 测试：LocalGuardrailEditor.integration.test.tsx 验证菜单能展开且未接通阶段不可选。
 * 约束：当前后端只执行 pre_call；不能允许保存尚未接通的阶段，避免配置成功却没有执行检查。
 */
export function GuardrailModeSelect({ disabled = false }: { disabled?: boolean }) {
  const id = useId();
  return (
    <div className="space-y-2 text-sm font-medium text-muted-foreground">
      <label htmlFor={id}>{t("执行模式")}</label>
      <Select value="pre_call" disabled={disabled}>
        <SelectTrigger id={id} aria-describedby={`${id}-hint`} className="h-10 w-full bg-background text-foreground">
          <SelectValue>{t("调用前（请求） · pre_call")}</SelectValue>
        </SelectTrigger>
        <SelectContent align="start">
          <SelectItem value="pre_call">{t("调用前（请求） · pre_call")}</SelectItem>
          <SelectItem value="during_call" disabled>
            {t("调用中 · during_call（暂不支持）")}
          </SelectItem>
          <SelectItem value="post_call" disabled>
            {t("调用后（响应） · post_call（暂不支持）")}
          </SelectItem>
          <SelectItem value="pre_mcp_call" disabled>
            {t("MCP 工具调用前 · pre_mcp_call（暂不支持）")}
          </SelectItem>
        </SelectContent>
      </Select>
      <p id={`${id}-hint`} className="text-xs font-normal leading-5">
        {t("当前支持调用前检查，其他阶段尚未接通。")}
      </p>
    </div>
  );
}

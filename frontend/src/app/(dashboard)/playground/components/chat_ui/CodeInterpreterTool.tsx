import React from "react";
import { toast } from "@/lib/toast";
import { Code, Info, TriangleAlert } from "lucide-react";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { t } from "@/i18n";

interface CodeInterpreterToolProps {
  accessToken: string;
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
  selectedContainerId: string | null;
  onContainerChange: (containerId: string | null) => void;
  selectedModel: string;
  disabled?: boolean;
}

/** 判断模型名是否具有已知 OpenAI 代码工具入口；参数模型别名，返回候选支持状态，实际权限由上游验证。 */
export const supportsCodeInterpreter = (model: string): boolean => {
  if (!model) return false;
  const lowerModel = model.toLowerCase();
  return (
    lowerModel.startsWith("openai/") ||
    lowerModel.startsWith("gpt-") ||
    lowerModel.startsWith("o1") ||
    lowerModel.startsWith("o3") ||
    lowerModel.includes("openai")
  );
};

/** 展示代码解释器开关；参数为候选模型与启用回调，返回工具卡；不支持时禁用，不创建外部容器。 */
const CodeInterpreterTool: React.FC<CodeInterpreterToolProps> = ({
  enabled,
  onEnabledChange,
  selectedModel,
  disabled = false,
}) => {
  const isOpenAI = supportsCodeInterpreter(selectedModel);
  const isDisabled = disabled || !isOpenAI;

  /** 更新启用状态；参数 checked 为开关值，返回无；不支持的开启操作仅提示，不调用上游。 */
  const handleToggle = (checked: boolean) => {
    if (checked && !isOpenAI) {
      toast.warning(t("Code Interpreter is only available for OpenAI models"));
      return;
    }
    onEnabledChange(checked);
  };

  return (
    <div className="border border-border rounded-lg p-3 bg-muted/20">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Code className="size-4 text-info" />
          <span className="font-medium text-foreground">{t("Code Interpreter")}</span>
          <Tooltip>
            <TooltipTrigger aria-label={t("About Code Interpreter")}>
              <Info className="size-3 text-muted-foreground" />
            </TooltipTrigger>
            <TooltipContent>
              {t("Run Python code to generate files, charts, and analyze data. Container is created automatically.")}
            </TooltipContent>
          </Tooltip>
        </div>
        <Switch
          checked={enabled && isOpenAI}
          onCheckedChange={handleToggle}
          disabled={isDisabled}
          size="sm"
          aria-label={t("Enable Code Interpreter")}
        />
      </div>

      {!isOpenAI && (
        <div className="mt-2 pt-2 border-t border-border">
          <div className="flex items-start gap-2">
            <TriangleAlert className="mt-0.5 size-4 shrink-0 text-warning" />
            <div className="text-xs text-muted-foreground">
              <span>{t("Code Interpreter is currently only supported for OpenAI models.")} </span>

              {t("Request support for other providers")}
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default CodeInterpreterTool;

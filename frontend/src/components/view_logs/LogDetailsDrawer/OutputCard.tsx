/**
 * OutputCard - Displays output message with token count and cost
 * Datadog-style: header with icon/metrics, content below
 */

import { useState } from "react";
import { toast } from "@/lib/toast";
import { COLOR_BORDER } from "./constants";
import { ParsedMessage } from "./prettyMessagesTypes";
import { SectionHeader } from "./SectionHeader";
import { SimpleMessageBlock } from "./SimpleMessageBlock";
import { t } from "@/i18n";
import { ROLE_STYLES } from "./prettyMessagesUtils";

interface OutputCardProps {
  message: ParsedMessage | null;
  completionTokens?: number;
  outputCost?: number;
}

/**
 * 展示日志响应消息及输出计量。
 * 参数 message 为可空的解析响应，completionTokens/outputCost 为可选计量；返回可折叠输出卡片。
 * PrettyMessagesView 调用；空响应展示本地化占位，复制跳过空消息，角色从 ROLE_STYLES 读取当前语言标签。
 */
export function OutputCard({ message, completionTokens, outputCost }: OutputCardProps) {
  const [isCollapsed, setIsCollapsed] = useState(false);

  const handleCopy = () => {
    if (!message) return;

    navigator.clipboard.writeText(message.content || "");
    toast.success(t("Output copied"));
  };

  return (
    <div className="overflow-hidden rounded-md" style={{ border: `1px solid ${COLOR_BORDER}` }}>
      <SectionHeader
        type="output"
        tokens={completionTokens}
        cost={outputCost}
        onCopy={handleCopy}
        isCollapsed={isCollapsed}
        onToggleCollapse={() => setIsCollapsed(!isCollapsed)}
      />

      <div
        className="overflow-hidden transition-[max-height,opacity] duration-300 ease-out"
        style={{ maxHeight: isCollapsed ? "0px" : "10000px", opacity: isCollapsed ? 0 : 1 }}
      >
        <div className="px-4 py-3">
          {message ? (
            <SimpleMessageBlock
              label={ROLE_STYLES[message.role]?.label ?? message.role.toUpperCase()}
              content={message.content}
              toolCalls={message.toolCalls}
            />
          ) : (
            <span className="text-[13px] text-muted-foreground italic">{t("No response data available")}</span>
          )}
        </div>
      </div>
    </div>
  );
}

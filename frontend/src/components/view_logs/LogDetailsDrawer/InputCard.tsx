/**
 * InputCard - Displays all input messages with token count and cost
 * Datadog-style: header with icon/metrics, content below
 */

import { useState } from "react";
import { toast } from "@/lib/toast";
import { ParsedMessage } from "./prettyMessagesTypes";
import { SectionHeader } from "./SectionHeader";
import { CollapsibleMessage } from "./CollapsibleMessage";
import { HistoryTree } from "./HistoryTree";
import { SimpleMessageBlock } from "./SimpleMessageBlock";
import { t } from "@/i18n";
import { ROLE_STYLES } from "./prettyMessagesUtils";

interface InputCardProps {
  messages: ParsedMessage[];
  promptTokens?: number;
  inputCost?: number;
}

/**
 * 展示日志请求中的系统提示、历史消息和最后一条输入。
 * 参数 messages 为已解析消息，promptTokens/inputCost 为可选计量；返回可折叠输入卡片，无消息时返回 null。
 * PrettyMessagesView 调用；复制只处理最后一条非系统消息，已知角色从 ROLE_STYLES 读取当前语言标签，未知角色保留大写标识。
 */
export function InputCard({ messages, promptTokens, inputCost }: InputCardProps) {
  const [isCollapsed, setIsCollapsed] = useState(false);

  if (messages.length === 0) {
    return null;
  }

  // Separate system, history, and last message
  const systemMessage = messages.find((m) => m.role === "system");
  const nonSystemMessages = messages.filter((m) => m.role !== "system");
  const lastMessage = nonSystemMessages.length > 0 ? nonSystemMessages[nonSystemMessages.length - 1] : null;
  const historyMessages = nonSystemMessages.slice(0, -1);

  const handleCopy = () => {
    const content = lastMessage?.content || "";
    navigator.clipboard.writeText(content);
    toast.success(t("Input copied"));
  };

  return (
    <div
      style={{
        border: "1px solid var(--color-border)",
        borderRadius: 6,
        marginBottom: 8,
        overflow: "hidden",
      }}
    >
      {/* Datadog-style Header */}
      <SectionHeader
        type="input"
        tokens={promptTokens}
        cost={inputCost}
        onCopy={handleCopy}
        isCollapsed={isCollapsed}
        onToggleCollapse={() => setIsCollapsed(!isCollapsed)}
      />

      {/* Content */}
      <div
        style={{
          maxHeight: isCollapsed ? "0px" : "10000px",
          overflow: "hidden",
          transition: "max-height 0.3s ease-out, opacity 0.3s ease-out",
          opacity: isCollapsed ? 0 : 1,
        }}
      >
        <div style={{ padding: "12px 16px" }}>
          {/* System Message - Collapsible with arrow */}
          {systemMessage && (
            <CollapsibleMessage
              label={ROLE_STYLES.system.label}
              content={systemMessage.content}
              defaultExpanded={!!(systemMessage.content && systemMessage.content.length < 200)}
            />
          )}

          {/* History - Tree style, collapsed by default */}
          {historyMessages.length > 0 && <HistoryTree messages={historyMessages} />}

          {/* Last User Message - Always visible */}
          {lastMessage && (
            <SimpleMessageBlock
              label={ROLE_STYLES[lastMessage.role]?.label ?? lastMessage.role.toUpperCase()}
              content={lastMessage.content}
              toolCalls={lastMessage.toolCalls}
            />
          )}
        </div>
      </div>
    </div>
  );
}

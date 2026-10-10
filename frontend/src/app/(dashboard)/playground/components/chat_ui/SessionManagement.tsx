import React from "react";
import { Copy, Info } from "lucide-react";
import { EndpointType } from "@/components/chat_ui/mode_endpoint_mapping";
import { toast } from "@/lib/toast";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { getActiveLocale, t } from "@/i18n";

interface SessionManagementProps {
  endpointType: string | null;
  responsesSessionId: string | null;
  useApiSessionManagement: boolean;
  onToggleSessionManagement: (useApi: boolean) => void;
}

/** 展示当前上下文续接方式；参数为协议、响应 ID 和切换回调；返回 Responses 会话设置。
 * 未取得响应 ID 时只表示新会话，不能声称上游已就绪；复制失败在界面提示，无后台写入。 */
const SessionManagement: React.FC<SessionManagementProps> = ({
  endpointType,
  responsesSessionId,
  useApiSessionManagement,
  onToggleSessionManagement,
}) => {
  if (endpointType !== EndpointType.RESPONSES) {
    return null;
  }

  /** 复制当前响应 ID；无参数，返回异步结果；浏览器拒绝剪贴板时展示失败提示。 */
  const handleCopySessionId = async () => {
    if (responsesSessionId) {
      try {
        await navigator.clipboard.writeText(responsesSessionId);
        toast.success(t("Response ID copied to clipboard!"));
      } catch {
        toast.error(t("Unable to copy response ID"));
      }
    }
  };

  const zh = getActiveLocale() === "zh-CN";
  /** 返回当前会话状态文本；无参数，无副作用；仅本组件调用。 */
  const getSessionDisplay = () =>
    responsesSessionId
      ? "Response ID: " + responsesSessionId.slice(0, 10) + "…"
      : zh
        ? "新会话 · 首次请求后建立上下文"
        : "New session · context starts after the first response";
  /** 返回上下文发送方式说明；无参数，无副作用；仅本组件调用。 */
  const getSessionDescription = () =>
    useApiSessionManagement
      ? zh
        ? "使用上一条响应 ID 续接对话。"
        : "Continue with the previous response ID."
      : zh
        ? "每次请求发送完整对话历史。"
        : "Send the full conversation with each request.";

  return (
    <div className="mb-4">
      {/* Session Management Toggle */}
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-foreground">{t("Session Management")}</span>
          <Tooltip>
            <TooltipTrigger aria-label={t("About session management")}>
              <Info className="size-3 text-muted-foreground" />
            </TooltipTrigger>
            <TooltipContent>
              {t(
                "Choose between LiteLLM API session management (using previous_response_id) or UI-based session management (using chat history)",
              )}
            </TooltipContent>
          </Tooltip>
        </div>
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span aria-hidden="true">UI</span>
          <Switch
            checked={useApiSessionManagement}
            onCheckedChange={onToggleSessionManagement}
            aria-label={t("Use API session management")}
            size="sm"
          />
          <span aria-hidden="true">API</span>
        </div>
      </div>

      {/* Session Status Indicator */}
      <div
        className={`text-xs p-2 rounded-md ${
          responsesSessionId
            ? "bg-success/10 text-success border border-success/20"
            : "bg-info/10 text-info border border-info/20"
        }`}
      >
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-1">
            <Info className="size-3" />
            {getSessionDisplay()}
          </div>
          {responsesSessionId && (
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    onClick={handleCopySessionId}
                    aria-label={t("Copy response ID")}
                    className="ml-2 hover:bg-success/15"
                  />
                }
              >
                <Copy className="size-3" />
              </TooltipTrigger>
              <TooltipContent className="max-w-lg">
                <div className="text-xs">
                  <div className="mb-1">{t("Copy response ID to continue session:")}</div>
                  <div className="bg-gray-800 text-gray-100 p-2 rounded-sm font-mono text-xs whitespace-pre-wrap">
                    {`curl -X POST "your-proxy-url/v1/responses" \\
  -H "Authorization: Bearer your-api-key" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "your-model",
    "input": [{"role": "user", "content": "your message", "type": "message"}],
    "previous_response_id": "${responsesSessionId}",
    "stream": true
  }'`}
                  </div>
                </div>
              </TooltipContent>
            </Tooltip>
          )}
        </div>
        <div className="text-xs opacity-75 mt-1">{getSessionDescription()}</div>
      </div>
    </div>
  );
};

export default SessionManagement;

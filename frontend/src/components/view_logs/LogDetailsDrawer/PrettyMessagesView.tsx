/**
 * PrettyMessagesView - Datadog-style view with Input/Output cards
 * Two main cards showing request and response with token counts and costs.
 * Detects realtime API responses and renders a specialized view.
 */

import { parseMediaPayload, parseMessages, requestBody, requestHeaders, requestLine } from "./prettyMessagesUtils";
import { InputCard } from "./InputCard";
import { OutputCard } from "./OutputCard";
import { isRealtimeResponse, RealtimePrettyView } from "./RealtimePrettyView";
import { MediaRequestResponseView } from "./MediaRequestResponseView";

interface PrettyMessagesViewProps {
  request: any;
  response: any;
  model?: string;
  metrics?: {
    prompt_tokens?: number;
    completion_tokens?: number;
    input_cost?: number;
    output_cost?: number;
  };
}

/**
 * 用途：把请求日志中的聊天、Responses、实时和媒体协议转换为可读详情。
 * 参数：request 和 response 是持久化原文，model 是日志记录的真实模型 ID，metrics 是计量与费用。
 * 返回值：与协议匹配的 React 详情视图。
 * 调用场景：请求日志抽屉的 Pretty 页签。
 * 边界：媒体请求正文未携带 model 时使用日志模型 ID；未知结构仍回退到普通输入输出卡片。
 */
export function PrettyMessagesView({ request, response, model, metrics }: PrettyMessagesViewProps) {
  if (isRealtimeResponse(response)) {
    return <RealtimePrettyView response={response} metrics={metrics} />;
  }
  const media = parseMediaPayload(request, response, model);
  if (media) return <MediaRequestResponseView request={request} media={media} />;

  const headers = requestHeaders(request);
  const body = requestBody(request);
  const { requestMessages, responseMessage } = parseMessages(body, response);

  return (
    <div className="space-y-4 p-4">
      <section>
        <h4 className="mb-2 text-sm font-semibold text-foreground">Headers</h4>
        {headers || requestLine(request) ? (
          <dl className="grid grid-cols-[minmax(8rem,auto)_1fr] gap-x-4 gap-y-1 rounded-md border bg-muted/30 p-3 text-sm">
            {requestLine(request) && (
              <div className="contents">
                <dt className="font-medium text-muted-foreground">Request</dt>
                <dd className="break-all font-mono text-foreground">{requestLine(request)}</dd>
              </div>
            )}
            {Object.entries(headers ?? {}).map(([key, value]) => (
              <div key={key} className="contents">
                <dt className="font-medium text-muted-foreground">{key}</dt>
                <dd className="break-all font-mono text-foreground">{value}</dd>
              </div>
            ))}
          </dl>
        ) : (
          <p className="text-sm italic text-muted-foreground">No headers recorded</p>
        )}
      </section>
      <section>
        <h4 className="mb-2 text-sm font-semibold text-foreground">Request</h4>
        <InputCard messages={requestMessages} promptTokens={metrics?.prompt_tokens} inputCost={metrics?.input_cost} />
        {requestMessages.length === 0 && <p className="text-sm italic text-muted-foreground">No request body recorded</p>}
      </section>
      <section>
        <h4 className="mb-2 text-sm font-semibold text-foreground">Response</h4>
        <OutputCard
          message={responseMessage}
          completionTokens={metrics?.completion_tokens}
          outputCost={metrics?.output_cost}
        />
      </section>
    </div>
  );
}

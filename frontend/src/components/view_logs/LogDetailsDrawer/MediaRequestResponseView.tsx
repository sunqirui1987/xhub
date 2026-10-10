import type { ParsedMediaPayload } from "./prettyMessagesTypes";
import { requestHeaders, requestLine } from "./prettyMessagesUtils";

interface MediaRequestResponseViewProps {
  request: unknown;
  media: ParsedMediaPayload;
}

/**
 * 用途：在请求日志详情中直接显示图片与视频 bypass 的输入、任务信息和真实媒体结果。
 * 参数：request 是含方法、路径和脱敏请求头的代理请求，media 是解析后的媒体请求与响应。
 * 返回值：可预览图片、可播放视频及完整关键字段的 React 视图。
 * 调用场景：PrettyMessagesView 识别媒体协议后使用。
 * 边界：创建阶段尚无媒体 URL 时仍显示任务 ID、状态和本地查询地址。
 */
export function MediaRequestResponseView({ request, media }: MediaRequestResponseViewProps) {
  const headers = requestHeaders(request);
  const line = requestLine(request);
  const input = media.request;
  const output = media.response;
  const images = [...output.imageUrls, ...output.imageDataUrls];
  return (
    <div className="space-y-4 p-4">
      <section>
        <h4 className="mb-2 text-sm font-semibold text-foreground">Headers</h4>
        {headers || line ? (
          <dl className="grid grid-cols-[minmax(8rem,auto)_1fr] gap-x-4 gap-y-1 rounded-md border bg-muted/30 p-3 text-sm">
            {line && <Field label="Request" value={line} mono />}
            {Object.entries(headers ?? {}).map(([key, value]) => <Field key={key} label={key} value={value} mono />)}
          </dl>
        ) : <p className="text-sm italic text-muted-foreground">No headers recorded</p>}
      </section>
      <section data-testid="media-request">
        <h4 className="mb-2 text-sm font-semibold text-foreground">Request</h4>
        <dl className="grid grid-cols-[minmax(7rem,auto)_1fr] gap-x-4 gap-y-2 rounded-md border bg-card p-4 text-sm">
          <Field label="Media" value={input.kind} />
          {input.model && <Field label="Model" value={input.model} mono />}
          {input.prompt && <Field label="Prompt" value={input.prompt} />}
          {input.duration != null && <Field label="Duration" value={String(input.duration)} />}
          {input.resolution && <Field label="Resolution" value={input.resolution} />}
          {(input.ratio || input.aspectRatio) && <Field label="Aspect ratio" value={input.ratio || input.aspectRatio || ""} />}
        </dl>
      </section>
      <section data-testid="media-response">
        <h4 className="mb-2 text-sm font-semibold text-foreground">Response</h4>
        <div className="space-y-3 rounded-md border bg-card p-4">
          {(output.taskId || output.status || output.duration != null) && (
            <dl className="grid grid-cols-[minmax(7rem,auto)_1fr] gap-x-4 gap-y-2 text-sm">
              {output.taskId && <Field label="Task ID" value={output.taskId} mono />}
              {output.status && <Field label="Status" value={output.status} />}
              {output.duration != null && <Field label="Duration" value={String(output.duration)} />}
              {output.responseUrl && <Field label="Response URL" value={output.responseUrl} mono />}
              {output.statusUrl && <Field label="Status URL" value={output.statusUrl} mono />}
            </dl>
          )}
          {images.map((src, index) => (
            <img key={index} data-testid="media-response-image" src={src} alt={`Generated result ${index + 1}`} className="max-h-[36rem] max-w-full rounded-md border object-contain" />
          ))}
          {output.videoUrl && (
            <div className="space-y-2">
              <video data-testid="media-response-video" controls preload="metadata" src={output.videoUrl} className="max-h-[36rem] w-full rounded-md border bg-black" />
              <a href={output.videoUrl} target="_blank" rel="noreferrer" className="block break-all font-mono text-xs text-primary underline">{output.videoUrl}</a>
            </div>
          )}
          {output.usage != null && <pre data-testid="media-response-usage" className="overflow-auto rounded-md bg-muted p-3 text-xs">{JSON.stringify(output.usage, null, 2)}</pre>}
          {images.length === 0 && !output.videoUrl && !output.taskId && <p className="text-sm italic text-muted-foreground">No media response data available</p>}
        </div>
      </section>
    </div>
  );
}

/**
 * 用途：统一渲染媒体请求和响应的一个键值字段。
 * 参数：label 是字段名，value 是已格式化的值，mono 控制是否使用等宽字体。
 * 返回值：定义列表中的一行 React 视图。
 * 调用场景：图片与视频日志详情的请求、任务和状态区域。
 * 边界：调用方只传入需要展示的非空值；长模型 ID、任务 ID 与 URL 会自动换行。
 */
function Field({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="contents">
      <dt className="font-medium text-muted-foreground">{label}</dt>
      <dd className={`break-all text-foreground ${mono ? "font-mono" : ""}`}>{value}</dd>
    </div>
  );
}

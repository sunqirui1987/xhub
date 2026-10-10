"use client";
import { useT } from "@/i18n";
import { useEffect, useRef, useState } from "react";
import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";
import { endpointURL } from "@/components/llm_calls/model_endpoints";
import { getProxyBaseUrl } from "@/components/networking";
import { ArrowUpRight, Loader2, Terminal } from "lucide-react";
import { diagnoseRequest, RequestDiagnostic, type RequestFailure } from "../RequestDiagnostic";
import { nativeRequestTemplate } from "./nativeRequestTemplate";
import { Button } from "@/components/ui/button";
import { ImageRequestForm } from "./ImageRequestForm";
import { CurlRequestGuide } from "./CurlRequestGuide";
import { imagePreviewSources, isOpenAIImageEndpoint, parseNativeRequest, validateImageRequest } from "./imageRequest";

/**
 * NativeEndpointPlayground 提供所选媒体或 Google 端点的原生参数编辑和任务操作。
 * 参数 endpoint/model/apiKey/base：模型绑定、对外别名、网关测试密钥和可选根地址。
 * 返回图片表单、同步 JSON 和图片预览的双栏界面，窄屏纵向排列；任务 ID 自动回填。
 * 媒体提交注入 model，Google 使用路径模型；其余参数按原生协议发送，图片编辑支持 multipart。
 * 切换模型中止旧请求，任务查询由用户点击执行，不自动重复提交。调用：ChatUI。
 */
export default function NativeEndpointPlayground({
  endpoint,
  model,
  apiKey,
  base,
}: {
  endpoint: ModelEndpoint;
  model: string;
  apiKey: string;
  base?: string;
}) {
  const t = useT();
  const [input, setInput] = useState('{\n  "prompt": ""\n}');
  const [output, setOutput] = useState("");
  const imageEndpoint = isOpenAIImageEndpoint(endpoint);
  const [responseFormat, setResponseFormat] = useState<unknown>("png");
  const [error, setError] = useState<RequestFailure | null>(null);
  const [taskId, setTaskId] = useState("");
  const [busy, setBusy] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const controller = useRef<AbortController | null>(null);
  useEffect(() => {
    controller.current?.abort();
    controller.current = null;
    setBusy(false);
    setOutput("");
    setError(null);
    setTaskId("");
    setFile(null);
    setInput(nativeRequestTemplate(endpoint));
    return () => {
      controller.current?.abort();
      controller.current = null;
    };
  }, [model, endpoint.path, endpoint.protocol, apiKey, base]);
  let imageDoc: Record<string, unknown> | null = null;
  try {
    imageDoc = parseNativeRequest(input);
  } catch {
    /* 非法 JSON 保留原文，表单不可覆盖。 */
  }
  const previews = imageEndpoint ? imagePreviewSources(output, responseFormat) : [];
  /** 更新图片表单字段；参数 key/value 为字段及值，返回 void。表单调用，保留扩展字段。
   * 非法草稿不自动重建；undefined 删除可选字段，错误交由 JSON 编辑器修正。 */
  const updateImageField = (key: string, value: unknown) => {
    try {
      const doc = parseNativeRequest(input);
      if (value === undefined) delete doc[key];
      else doc[key] = value;
      setInput(JSON.stringify(doc, null, 2));
      setError(null);
    } catch (error) {
      setError(diagnoseRequest(error));
    }
  };
  /** run 执行登记的创建或任务操作。
   * 参数 path/method：公开路径和方法；query：可选任务参数名。返回：Promise<void>，结果写入界面。
   * 只允许当前请求更新状态，防止模型切换后旧响应覆盖新界面。
   */
  const run = async (path: string, method: string, query?: string) => {
    if (controller.current) return;
    setBusy(true);
    setOutput("");
    setError(null);
    const abort = new AbortController();
    controller.current = abort;
    try {
      if (!apiKey) throw new Error(t("myModels.selectTestKey"));
      let body: BodyInit | undefined;
      const headers: Record<string, string> = { Authorization: "Bearer " + apiKey };
      if (method === "POST" && path === endpoint.path) {
        const doc = parseNativeRequest(input);
        if (imageEndpoint) {
          validateImageRequest(doc);
          setResponseFormat(doc.output_format);
        }
        // 原生 Google 模型位于绑定路径，正文保持原厂格式。
        if (endpoint.protocol !== "gemini" && endpoint.protocol !== "vertex") doc.model = model;
        if (endpoint.path.endsWith("/images/edits")) {
          if (!file) throw new Error(t("myModels.selectEditImage"));
          const form = new FormData();
          Object.entries(doc).forEach(([key, value]) =>
            form.append(key, typeof value === "object" ? JSON.stringify(value) : String(value)),
          );
          form.append("image", file);
          body = form;
        } else {
          headers["Content-Type"] = "application/json";
          body = JSON.stringify(doc);
        }
      }
      let url = endpointURL(base || getProxyBaseUrl(), path);
      if (query) {
        const u = new URL(url, window.location.origin);
        u.searchParams.set(query, taskId.trim());
        url = u.toString();
      }
      const response = await fetch(url, { method, headers, body, signal: abort.signal });
      if (!response.ok) throw Object.assign(new Error(await response.text()), { status: response.status });
      if (response.headers.get("content-type")?.includes("text/event-stream")) {
        const reader = response.body!.getReader();
        const decoder = new TextDecoder();
        try {
          while (true) {
            const { value, done } = await reader.read();
            const text = decoder.decode(value, { stream: !done });
            if (controller.current !== abort || abort.signal.aborted) return;
            setOutput((previous) => (previous + text).slice(-8 * 1024 * 1024));
            if (done) break;
          }
        } finally {
          await reader.cancel();
        }
      } else {
        const text = await response.text();
        if (controller.current !== abort || abort.signal.aborted) return;
        let doc;
        try {
          doc = JSON.parse(text);
        } catch {
          setOutput(text);
          return;
        }
        setOutput(JSON.stringify(doc, null, 2));
        const id = doc.request_id ?? doc.task_id ?? doc.id ?? doc.data?.task_id;
        if (typeof id === "string") setTaskId(id);
      }
    } catch (error) {
      if (controller.current === abort)
        setError(abort.signal.aborted ? diagnoseRequest(new Error(t("myModels.requestStopped"))) : diagnoseRequest(error));
    } finally {
      if (controller.current === abort) {
        setBusy(false);
        controller.current = null;
      }
    }
  };
  const taskActions =
    endpoint.actions?.filter(
      (action) => action.name !== "create" && (action.public_path.includes("{") || action.task_query),
    ) ?? [];
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-auto">
      <header className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b border-border px-5 py-4">
        <h2 className="text-sm font-semibold">{imageEndpoint ? t("myModels.imagePlayground") : t("myModels.requestPlayground")}</h2>
        <details className="max-w-full text-xs text-muted-foreground">
          <summary className="cursor-pointer hover:text-foreground">{t("myModels.endpointDetails")}</summary>
          <p className="mt-2 break-all font-mono">
            {endpoint.method} {endpoint.path}
          </p>
        </details>
      </header>
      <CurlRequestGuide
        endpoint={endpoint}
        model={model}
        base={base || getProxyBaseUrl()}
        body={input}
        multipart={endpoint.path.endsWith("/images/edits")}
        files={endpoint.path.endsWith("/images/edits") ? { image: file?.name || "input.png" } : undefined}
        taskId={taskId}
      />
      {/* 窄屏按内容高度滚动，避免两栏改为纵向后压缩标题和结果；桌面才填满剩余高度。 */}
      <div className="grid shrink-0 xl:min-h-0 xl:flex-1 xl:grid-cols-2">
        <section
          aria-label={t("myModels.requestEditor")}
          className="flex min-w-0 flex-col gap-4 p-5 xl:border-r xl:border-border"
        >
          {imageEndpoint &&
            (imageDoc ? (
              <ImageRequestForm doc={imageDoc} disabled={busy} onChange={updateImageField} />
            ) : (
              <p role="status" className="text-sm text-destructive">
                {t("myModels.invalidNativeJson")}
              </p>
            ))}
          <details open={!imageEndpoint || !imageDoc} className="min-h-0 space-y-3">
            <summary className="cursor-pointer text-sm font-medium">
              {imageEndpoint ? t("myModels.jsonParameters") : t("myModels.nativeParameters")}
            </summary>
            <div className="flex items-center justify-between">
              <label htmlFor="native-request" className="text-sm font-medium">
                {t("myModels.requestParameters")}
              </label>
              <div className="flex gap-1">
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={busy}
                  onClick={() => {
                    try {
                      const doc = parseNativeRequest(input);
                      setInput(JSON.stringify(doc, null, 2));
                      setError(null);
                    } catch (error) {
                      setError(diagnoseRequest(error));
                    }
                  }}
                >
                  {t("myModels.formatParameters")}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={busy}
                  onClick={() => {
                    setInput(nativeRequestTemplate(endpoint));
                    setError(null);
                  }}
                >
                  {t("myModels.resetParameters")}
                </Button>
              </div>
            </div>
            <textarea
              id="native-request"
              aria-label={t("myModels.nativeRequestParameters")}
              spellCheck={false}
              disabled={busy}
              className="min-h-60 w-full flex-1 resize-y rounded-lg border border-border bg-muted/20 p-4 font-mono text-sm leading-relaxed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              value={input}
              onChange={(event) => setInput(event.target.value)}
            />
          </details>
          {endpoint.path.endsWith("/images/edits") && (
            <label className="space-y-2 text-sm">
              <span className="block font-medium">{t("myModels.editImage")}</span>
              <input
                key={[model, endpoint.path, apiKey, base].join("|")}
                type="file"
                accept="image/*"
                aria-label={t("myModels.editImage")}
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              />
            </label>
          )}
          <div className="flex items-center gap-2">
            <Button disabled={busy || !apiKey} onClick={() => void run(endpoint.path, endpoint.method)}>
              {busy ? <Loader2 className="size-4 animate-spin" /> : <ArrowUpRight className="size-4" />}
              {busy ? t("myModels.requesting") : t("myModels.submitRequest")}
            </Button>
            {busy && (
              <Button
                variant="ghost"
                onClick={() => {
                  controller.current?.abort();
                  controller.current = null;
                  setBusy(false);
                  setOutput("");
                  setError(diagnoseRequest(new Error(t("myModels.requestStopped"))));
                }}
              >
                {t("myModels.stopRequest")}
              </Button>
            )}
            <p className="ml-auto text-xs text-muted-foreground">{t("myModels.autoSelectedModel")}</p>
          </div>
        </section>
        <section
          aria-label={t("myModels.requestResults")}
          className="flex min-w-0 flex-col gap-4 border-t border-border p-5 xl:border-t-0"
        >
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium">{t("myModels.responseResult")}</h3>
            <span role="status" className="text-xs text-muted-foreground">
              {busy
                ? t("myModels.waitingResponse")
                : error
                  ? t("myModels.requestIncomplete")
                  : output
                    ? t("myModels.responseReceived")
                    : t("myModels.readySubmit")}
            </span>
          </div>
          {taskActions.length > 0 && (
            <div className="space-y-3 rounded-lg border border-border p-3">
              <label className="block text-xs font-medium" htmlFor="native-task">
                {t("myModels.taskId")}
              </label>
              <input
                id="native-task"
                aria-label={t("myModels.taskId")}
                placeholder={t("myModels.taskIdPlaceholder")}
                className="h-9 w-full min-w-0 rounded-md border border-border bg-card px-3 text-xs font-mono focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                value={taskId}
                onChange={(event) => setTaskId(event.target.value)}
              />
              <div className="flex flex-wrap gap-2">
                {taskActions.map((action) => (
                  <Button
                    key={action.name + action.public_path}
                    variant="outline"
                    size="sm"
                    disabled={busy || !taskId.trim() || !apiKey}
                    onClick={() =>
                      void run(
                        action.public_path.replace(/\{[^}]+\}/g, encodeURIComponent(taskId.trim())),
                        action.method,
                        action.task_query,
                      )
                    }
                  >
                    {action.name === "status"
                      ? t("myModels.queryTaskStatus")
                      : action.name === "get"
                        ? t("myModels.getTaskResult")
                        : action.name === "cancel"
                          ? t("myModels.cancelTask")
                          : action.name}
                  </Button>
                ))}
              </div>
            </div>
          )}
          {error && <RequestDiagnostic failure={error} />}
          {previews.length > 0 && (
            <div aria-label={t("myModels.imagePreviews")} className="grid gap-3">
              {previews.map((src, index) => (
                <figure key={index} className="space-y-2 rounded-lg border border-border p-3">
                  {/* 原生图片可能是 base64 或签名 URL，直接显示避免代理改变地址或响应。 */}
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={src}
                    alt={t("myModels.generatedImage", { index: index + 1 })}
                    className="max-h-96 w-full rounded-md object-contain"
                  />
                  <a
                    href={src}
                    download={"generated-image-" + (index + 1)}
                    target="_blank"
                    rel="noreferrer"
                    className="text-xs underline underline-offset-4"
                  >
                    {t("myModels.openImage", { index: index + 1 })}
                  </a>
                </figure>
              ))}
            </div>
          )}
          {imageEndpoint && output && previews.length === 0 && (
            <p role="status" className="text-sm text-muted-foreground">
              {t("myModels.noImagePreview")}
            </p>
          )}
          {output ? (
            <details open={!imageEndpoint} className="min-w-0">
              <summary className="cursor-pointer text-sm font-medium">{t("myModels.nativeResponse")}</summary>
              <pre
                className="min-h-48 flex-1 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted/30 p-4 font-mono text-xs leading-relaxed"
                aria-label={t("myModels.nativeResponse")}
              >
                {output}
              </pre>
            </details>
          ) : !error ? (
            <div className="flex min-h-48 flex-1 flex-col items-center justify-center gap-3 rounded-lg bg-muted/20 text-muted-foreground">
              <Terminal className="size-6 opacity-50" aria-hidden="true" />
              <p className="text-sm">{busy ? t("myModels.processingRequest") : t("myModels.resultsPlaceholder")}</p>
            </div>
          ) : null}
        </section>
      </div>
    </div>
  );
}

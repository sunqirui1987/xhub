"use client";
import { useEffect, useRef, useState } from "react";
import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";
import { endpointURL } from "@/components/llm_calls/model_endpoints";
import { getProxyBaseUrl } from "@/components/networking";
import { ArrowUpRight, Loader2, Terminal } from "lucide-react";
import { Button } from "@/components/ui/button";

/**
 * NativeEndpointPlayground 提供所选图片/视频端点的原生参数编辑和任务操作。
 * 参数 endpoint/model/apiKey/base：模型绑定、对外别名、网关测试密钥和可选根地址。
 * 返回简洁请求/结果双栏界面，窄屏纵向排列；路径详情折叠，任务 ID 自动回填。
 * 提交仅注入 model；其余参数按原生协议发送，图片编辑支持 multipart。
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
  const [input, setInput] = useState('{\n  "prompt": ""\n}');
  const [output, setOutput] = useState("");
  const [error, setError] = useState("");
  const [taskId, setTaskId] = useState("");
  const [busy, setBusy] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const controller = useRef<AbortController | null>(null);
  useEffect(() => {
    controller.current?.abort();
    controller.current = null;
    setBusy(false);
    setOutput("");
    setError("");
    setTaskId("");
    setFile(null);
    setInput(
      endpoint.protocol === "openai-responses"
        ? '{\n  "input": "",\n  "stream": false\n}'
        : endpoint.protocol === "anthropic-messages"
          ? '{\n  "messages": [{"role": "user", "content": ""}],\n  "max_tokens": 1024\n}'
          : '{\n  "prompt": ""\n}',
    );
    return () => controller.current?.abort();
  }, [model, endpoint.path, endpoint.protocol]);
  /** run 执行登记的创建或任务操作。
   * 参数 path/method：公开路径和方法；query：可选任务参数名。返回：Promise<void>，结果写入界面。
   * 只允许当前请求更新状态，防止模型切换后旧响应覆盖新界面。
   */
  const run = async (path: string, method: string, query?: string) => {
    setBusy(true);
    setOutput("");
    setError("");
    const abort = new AbortController();
    controller.current = abort;
    try {
      if (!apiKey) throw new Error("请选择测试密钥");
      let body: BodyInit | undefined;
      const headers: Record<string, string> = { Authorization: "Bearer " + apiKey };
      if (method === "POST" && path === endpoint.path) {
        const doc = JSON.parse(input);
        if (!doc || Array.isArray(doc) || typeof doc !== "object") throw new Error("请求参数必须是 JSON 对象");
        doc.model = model;
        if (endpoint.path.endsWith("/images/edits")) {
          if (!file) throw new Error("请选择需要编辑的图片");
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
      if (!response.ok) throw new Error(await response.text());
      if (response.headers.get("content-type")?.includes("text/event-stream")) {
        const reader = response.body!.getReader();
        const decoder = new TextDecoder();
        try {
          while (true) {
            const { value, done } = await reader.read();
            const text = decoder.decode(value, { stream: !done });
            if (controller.current !== abort) return;
            setOutput((previous) => (previous + text).slice(-8 * 1024 * 1024));
            if (done) break;
          }
        } finally {
          await reader.cancel();
        }
      } else {
        const text = await response.text();
        if (controller.current !== abort) return;
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
        setError(abort.signal.aborted ? "请求已停止" : error instanceof Error ? error.message : String(error));
    } finally {
      if (controller.current === abort) setBusy(false);
    }
  };
  const taskActions =
    endpoint.actions?.filter(
      (action) => action.name !== "create" && (action.public_path.includes("{") || action.task_query),
    ) ?? [];
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-auto">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-5 py-4">
        <h2 className="text-sm font-semibold">请求调试</h2>
        <details className="max-w-full text-xs text-muted-foreground">
          <summary className="cursor-pointer hover:text-foreground">接口详情</summary>
          <p className="mt-2 break-all font-mono">
            {endpoint.method} {endpoint.path}
          </p>
        </details>
      </header>
      <div className="grid min-h-0 flex-1 xl:grid-cols-2">
        <section aria-label="请求编辑" className="flex min-w-0 flex-col gap-4 p-5 xl:border-r xl:border-border">
          <div className="flex items-center justify-between">
            <label htmlFor="native-request" className="text-sm font-medium">
              请求参数
            </label>
            <span className="text-xs text-muted-foreground">JSON</span>
          </div>
          <textarea
            id="native-request"
            aria-label="原生请求参数"
            spellCheck={false}
            className="min-h-60 w-full flex-1 resize-y rounded-lg border border-border bg-muted/20 p-4 font-mono text-sm leading-relaxed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            value={input}
            onChange={(event) => setInput(event.target.value)}
          />
          {endpoint.path.endsWith("/images/edits") && (
            <label className="space-y-2 text-sm">
              <span className="block font-medium">编辑图片</span>
              <input
                type="file"
                accept="image/*"
                aria-label="编辑图片"
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              />
            </label>
          )}
          <div className="flex items-center gap-2">
            <Button disabled={busy || !apiKey} onClick={() => void run(endpoint.path, endpoint.method)}>
              {busy ? <Loader2 className="size-4 animate-spin" /> : <ArrowUpRight className="size-4" />}
              {busy ? "请求中…" : "提交请求"}
            </Button>
            {busy && (
              <Button variant="ghost" onClick={() => controller.current?.abort()}>
                停止
              </Button>
            )}
            <p className="ml-auto text-xs text-muted-foreground">自动使用所选模型</p>
          </div>
        </section>
        <section aria-label="请求结果" className="flex min-w-0 flex-col gap-4 border-t border-border p-5 xl:border-t-0">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium">响应结果</h3>
            <span role="status" className="text-xs text-muted-foreground">
              {busy ? "等待响应" : error ? "请求未完成" : output ? "已收到响应" : "待提交"}
            </span>
          </div>
          {taskActions.length > 0 && (
            <div className="space-y-3 rounded-lg border border-border p-3">
              <label className="block text-xs font-medium" htmlFor="native-task">
                任务 ID
              </label>
              <input
                id="native-task"
                aria-label="任务 ID"
                placeholder="提交后自动填入，也可粘贴已有 ID"
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
                      ? "查询状态"
                      : action.name === "get"
                        ? "获取结果"
                        : action.name === "cancel"
                          ? "取消任务"
                          : action.name}
                  </Button>
                ))}
              </div>
            </div>
          )}
          {error && (
            <p
              role="alert"
              className="rounded-lg border border-destructive/20 bg-destructive/5 p-3 text-sm text-destructive break-all"
            >
              {error}
            </p>
          )}
          {output ? (
            <pre
              className="min-h-48 flex-1 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted/30 p-4 font-mono text-xs leading-relaxed"
              aria-label="原生响应"
            >
              {output}
            </pre>
          ) : (
            <div className="flex min-h-48 flex-1 flex-col items-center justify-center gap-3 rounded-lg bg-muted/20 text-muted-foreground">
              <Terminal className="size-6 opacity-50" aria-hidden="true" />
              <p className="text-sm">{busy ? "正在处理请求…" : "提交请求后，结果显示在这里"}</p>
            </div>
          )}
        </section>
      </div>
    </div>
  );
}

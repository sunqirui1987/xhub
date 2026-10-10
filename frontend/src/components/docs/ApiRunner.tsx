"use client";
import { useEffect, useRef, useState } from "react";
import { requestDocumentation } from "@/lib/http/documentation";
import { useT } from "@/i18n";

/** 文档内真实接口调试器；参数为当前网关和公开接口，返回凭据、路径及正文编辑器。
 * 仅用户点击运行时请求网关；密钥只存组件内存，换文章后卸载。JSON、multipart 与二进制响应均可处理，超时取消，不写本地存储。 */
export function ApiRunner({
  base,
  endpoint,
  method,
  initialBody = "{}",
}: {
  base: string;
  endpoint: string;
  method: string;
  initialBody?: string;
}) {
  const t = useT();
  const [requestMethod, setRequestMethod] = useState(method);
  const active = useRef<AbortController | null>(null);
  // 离开文章时取消正在运行的请求，避免卸载后的响应更新；取消不保证上游撤销计费。
  useEffect(() => () => active.current?.abort(), []);
  const [key, setKey] = useState("");
  const [path, setPath] = useState(endpoint);
  const [body, setBody] = useState(initialBody);
  const [multipart, setMultipart] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [fileField, setFileField] = useState("file");
  const [result, setResult] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  /** 执行当前文档请求；无参数，返回 Promise。校验路径只允许网关内地址，错误显示双语信息，最多等待 60 秒，不自动重试付费请求。 */
  async function run() {
    setError("");
    setResult("");
    const invalidPath = !path.startsWith("/") || path.startsWith("//") || /[\\?#]/.test(path);
    const anonymousDiscovery = requestMethod === "GET" && path === "/models";
    const missingKey = !key.trim() && !anonymousDiscovery;
    if (missingKey || !base || invalidPath) {
      setError("docs.runner.invalid");
      return;
    }
    let payload: BodyInit | undefined;
    try {
      if (requestMethod !== "GET") {
        const parsed = JSON.parse(body);
        if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") throw new Error("object");
        if (multipart) {
          const form = new FormData();
          for (const [name, value] of Object.entries(parsed))
            form.append(name, typeof value === "object" ? JSON.stringify(value) : String(value));
          if (file) form.append(fileField, file);
          payload = form;
        } else payload = JSON.stringify(parsed);
      }
    } catch {
      setError("docs.runner.jsonError");
      return;
    }
    setBusy(true);
    const controller = new AbortController();
    active.current = controller;
    const timeout = setTimeout(() => controller.abort(), 60_000);
    try {
      const options: RequestInit = {
        method: requestMethod,
        headers: {
          ...(key.trim() ? { Authorization: "Bearer " + key.trim() } : {}),
          ...(!multipart && requestMethod !== "GET" ? { "Content-Type": "application/json" } : {}),
        },
        body: payload,
        signal: controller.signal,
        credentials: "omit",
      };
      const response = await requestDocumentation(base + path, options);
      const type = response.headers.get("content-type") ?? "";
      if (/json|text|event-stream/.test(type)) {
        const raw = await response.text();
        let display = raw;
        try {
          display = JSON.stringify(JSON.parse(raw), null, 2);
        } catch {
          /* SSE 与文本保留原响应。 */
        }
        setResult(
          "HTTP " +
            response.status +
            "\n" +
            (response.headers.get("x-litellm-call-id")
              ? "x-litellm-call-id: " + response.headers.get("x-litellm-call-id") + "\n"
              : "") +
            display,
        );
      } else {
        const blob = await response.blob();
        setResult(t("docs.runner.binary", { status: response.status, type, size: blob.size }));
      }
    } catch {
      setError("docs.runner.networkError");
    } finally {
      clearTimeout(timeout);
      active.current = null;
      setBusy(false);
    }
  }
  return (
    <section className="mt-10 space-y-4 rounded-xl border border-border p-5" aria-label={t("docs.runner.title")}>
      <h2 className="text-xl font-semibold">{t("docs.runner.title")}</h2>
      <p className="text-sm text-muted-foreground">{t("docs.runner.hint")}</p>
      {/* 独立表单隔离密钥与文章搜索；new-password 告知浏览器此处不是已有账户密码，第三方密码管理器也忽略该字段。 */}
      <form
        aria-label={t("docs.runner.title")}
        autoComplete="off"
        onSubmit={(event) => event.preventDefault()}
        className="space-y-4"
      >
        <label className="block text-sm">
          {t("docs.runner.key")}
          <input
            type="password"
            id="xhub-documentation-request-secret"
            name="documentation-request-secret"
            autoComplete="new-password"
            data-1p-ignore="true"
            data-lpignore="true"
            value={key}
            onChange={(e) => setKey(e.target.value)}
            className="mt-2 w-full rounded border p-2"
          />
        </label>
        <label className="block text-sm">
          {t("docs.runner.method")}
          <select
            value={requestMethod}
            onChange={(e) => setRequestMethod(e.target.value)}
            className="mt-2 w-full rounded border p-2"
          >
            <option>GET</option>
            <option>POST</option>
          </select>
        </label>
        <label className="block text-sm">
          {t("docs.runner.path")}
          <input value={path} onChange={(e) => setPath(e.target.value)} className="mt-2 w-full rounded border p-2" />
        </label>
        {requestMethod !== "GET" && (
          <>
            <label className="block text-sm">
              {t("docs.runner.body")}
              <textarea
                value={body}
                onChange={(e) => setBody(e.target.value)}
                rows={8}
                className="mt-2 w-full rounded border p-2 font-mono text-xs"
              />
            </label>
            <label className="block text-sm">
              <input type="checkbox" checked={multipart} onChange={(e) => setMultipart(e.target.checked)} />{" "}
              {t("docs.runner.multipart")}
            </label>
            {multipart && (
              <>
                <label className="block text-sm">
                  {t("docs.runner.fileField")}
                  <input
                    value={fileField}
                    onChange={(e) => setFileField(e.target.value)}
                    className="w-full rounded border p-2"
                  />
                </label>
                <label className="block text-sm">
                  {t("docs.runner.file")}
                  <input type="file" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
                </label>
              </>
            )}
          </>
        )}
        <button
          type="button"
          disabled={busy}
          onClick={run}
          className="rounded bg-emerald-600 px-4 py-2 text-white disabled:opacity-50"
        >
          {t(busy ? "docs.runner.running" : "docs.runner.run")}
        </button>
      </form>
      {error && <p role="alert">{t(error)}</p>}
      {result && (
        <pre role="status" className="max-h-96 overflow-auto whitespace-pre-wrap rounded bg-muted p-4 text-xs">
          {result}
        </pre>
      )}
    </section>
  );
}

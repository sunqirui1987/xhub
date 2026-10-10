"use client";
import { getActiveLocale, t } from "@/i18n";
import { uiHref } from "@/utils/uiHref";

/** RequestFailure 是调试工作区的独立错误状态；不会作为模型输出进入后续上下文。 */
export interface RequestFailure {
  title: string;
  message: string;
  detail: string;
  status?: number;
  configure: boolean;
}
/** diagnoseRequest 解析网关、原生协议及网络异常。参数 error：未知失败对象；返回可操作的诊断。
 * 调用：对话、原生请求、模型对比；缺少凭据与模型失效分别提示，不推断或暴露凭据。 */
export function diagnoseRequest(error: unknown): RequestFailure {
  const zh = getActiveLocale() === "zh-CN";
  let detail: string;
  try {
    detail =
      error instanceof Error
        ? error.message
        : typeof error === "string"
          ? error
          : JSON.stringify(error) ?? String(error);
  } catch {
    detail = String(error);
  }
  let value = error as {
    message?: string;
    status?: number;
    error?: { message?: string; type?: string; code?: string | number };
  } | null;
  try {
    const parsed = JSON.parse(detail);
    if (parsed && typeof parsed === "object") value = parsed;
  } catch {
    /* 非 JSON 异常保留原始说明。 */
  }
  const message =
    typeof value?.error?.message === "string"
      ? value.error.message
      : typeof value?.message === "string"
        ? value.message
        : detail;
  const credential = /no upstream API key/i.test(message) || value?.error?.type === "upstream_auth";
  const missing = /model not found|model is disabled|no deployments|no available deployment/i.test(message);
  const status = Number((error as { status?: number } | null)?.status ?? value?.status ?? value?.error?.code);
  return {
    title: credential
      ? zh
        ? "模型未配置上游密钥"
        : "Upstream key is missing"
      : missing
        ? zh
          ? "模型没有可用部署"
          : "Model has no available deployment"
        : zh
          ? "请求失败"
          : "Request failed",
    message: credential
      ? zh
        ? "请在模型与端点中配置该模型的供应商密钥，再重试。当前界面会话只负责网关鉴权。"
        : "Configure the provider key in Models & endpoints, then retry. The UI session authenticates to the gateway."
      : missing
        ? zh
          ? "请检查模型是否启用、部署是否存在，以及公开模型名称是否与所选模型一致。"
          : "Check the model is enabled, has a deployment, and uses the selected public model name."
        : /Failed to fetch|NetworkError/i.test(message)
          ? zh
            ? "无法连接服务，请检查代理地址和网络后重试。"
            : "Unable to connect. Check the proxy URL and network, then retry."
          : message,
    detail,
    status: Number.isInteger(status) && status >= 100 && status <= 599 ? status : undefined,
    configure: credential || missing,
  };
}
/** RequestDiagnostic 展示独立错误和可展开的原始详情。参数 failure：诊断；返回提示面板；用于三种调试模式。 */
export function RequestDiagnostic({ failure }: { failure: RequestFailure }) {
  const zh = getActiveLocale() === "zh-CN";
  return (
    <div role="alert" className="space-y-2 rounded-lg border border-destructive/20 bg-destructive/5 p-4 text-sm">
      <p className="font-medium text-destructive">
        {failure.title}
        {failure.status ? " · HTTP " + failure.status : ""}
      </p>
      <p className="text-foreground break-words">{failure.message}</p>
      {failure.configure && (
        <a className="inline-block underline underline-offset-4" href={uiHref("/models-and-endpoints")}>
          {t("Check Models & endpoints")}
        </a>
      )}
      <details className="text-xs text-muted-foreground">
        <summary className="cursor-pointer">{t("Error details")}</summary>
        <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-all">{failure.detail}</pre>
      </details>
    </div>
  );
}

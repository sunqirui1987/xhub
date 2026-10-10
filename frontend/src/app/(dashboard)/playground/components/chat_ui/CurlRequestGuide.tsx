"use client";

import { useState } from "react";
import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";
import { useT } from "@/i18n";
import { Button } from "@/components/ui/button";
import { invocationCurl, invocationSteps, shellQuote } from "../../../mine-models/modelInvocation";
import { protocolParameters } from "../../../mine-models/protocolParameters";

/** 展示当前请求的完整终端教程；参数包含真实绑定、公开别名、网关、正文及文件名，不接收密钥。
 * 返回可展开的顺序命令与协议文档，供普通和原生调试界面调用；非法 JSON 不生成误导命令。
 * 用户点击复制才写剪贴板，复制失败保留命令；任务 ID 回填不自动调用，文件需保存在本地当前目录。 */
export function CurlRequestGuide({
  endpoint,
  model,
  base,
  body,
  files,
  multipart,
  taskId,
  headers,
}: {
  endpoint: ModelEndpoint;
  model: string;
  base: string;
  body: string;
  files?: Record<string, string>;
  multipart?: boolean;
  taskId?: string;
  headers?: Record<string, string>;
}) {
  const t = useT();
  const [status, setStatus] = useState("");
  const [expanded, setExpanded] = useState(false);
  let command: string | null = null;
  try {
    const parsed = JSON.parse(body);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      command = invocationCurl(model, endpoint, new URL(base || "/", window.location.origin).href, {
        body: parsed,
        files,
        multipart,
        headers,
      });
    }
  } catch {
    /* 保留无效输入，只显示可观察的修正提示。 */
  }
  let absoluteBase = "";
  try {
    absoluteBase = new URL(base || "/", window.location.origin).href;
  } catch {
    /* 无效网关不生成命令。 */
  }
  const steps = command
    ? invocationSteps(model, endpoint, absoluteBase).map((step) => {
        if (step.copy === "copyExample") return { ...step, command: command!, hint: "curlCurrentHint" };
        if (step.copy === "copyTaskId" && taskId?.trim())
          return { ...step, command: "export TASK_ID=" + shellQuote(taskId.trim()) };
        return step;
      })
    : [];
  /** 复制当前显示的步骤；参数为命令，返回完成 Promise；失败显示提示，不读取调试密钥。 */
  async function copy(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      setStatus("copied");
    } catch {
      setStatus("copyFailed");
    }
  }
  return (
    <details className="min-w-0 shrink-0 border-b border-border p-4" aria-label={t("myModels.curlGuide")}>
      <summary className="cursor-pointer text-sm font-medium" onClick={() => setExpanded(!expanded)}>
        {t("myModels.curlGuide")}
      </summary>
      {expanded && (
        <div className="mt-3 max-h-[50vh] min-w-0 space-y-3 overflow-auto">
          <p className="text-xs leading-5 text-muted-foreground">
            {t("myModels.curlCurrentHint")} {t("myModels.exampleHint")}
          </p>
          {multipart && <p className="text-xs">{t("myModels.localFilesHint")}</p>}
          {!command && <p role="alert">{t("myModels.curlInvalid")}</p>}
          {status && <p role={status === "copied" ? "status" : "alert"}>{t("myModels." + status)}</p>}
          <ol className="min-w-0 space-y-3">
            {steps.map((step, index) => (
              <li key={step.copy} className="min-w-0 rounded-md border p-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <h3 className="text-sm font-medium">
                    {index + 1}. {t("myModels." + step.title)}
                  </h3>
                  <Button size="sm" variant="outline" onClick={() => void copy(step.command)}>
                    {t("myModels." + step.copy)}
                  </Button>
                </div>
                <p className="my-2 text-xs leading-5 text-muted-foreground">{t("myModels." + step.hint)}</p>
                <pre
                  aria-label={t("myModels." + step.copy)}
                  className="max-h-80 overflow-auto rounded bg-slate-950 p-3 text-xs leading-5 text-slate-100"
                >
                  <code>{step.command}</code>
                </pre>
              </li>
            ))}
          </ol>
          <details className="rounded-md border p-3">
            <summary className="cursor-pointer text-sm font-medium">{t("myModels.parameterGuide")}</summary>
            <p className="my-2 text-xs leading-5 text-muted-foreground">{t("myModels.parameterGuideHint")}</p>
            <dl className="space-y-2 text-xs">
              {protocolParameters(endpoint).map((row) => (
                <div key={row.field}>
                  <dt className="font-mono font-semibold">{row.field}</dt>
                  <dd className="mt-1 leading-5">{t("myModels." + row.description)}</dd>
                </div>
              ))}
            </dl>
          </details>
          <p className="text-xs leading-5 text-muted-foreground">
            {t("myModels.errorHint")} {t("myModels.checkUsageHint")}
          </p>
        </div>
      )}
    </details>
  );
}

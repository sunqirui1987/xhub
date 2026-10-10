import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";

export type InvocationStep = { title: string; hint: string; command: string; copy: string };

/** 将模型名、URL 或正文转为 shell 字面量；返回安全单引号参数，供 curl 生成调用，无副作用。 */
export function shellQuote(value: string): string {
  return "'" + value.replaceAll("'", "'\\''") + "'";
}

/** 返回 Fal 创建所需素材；参数为真实绑定路径，返回已知字段或 null。
 * 供示例生成使用；复杂编辑和动作控制读取 request.json，不猜测通用素材字段，无副作用。 */
function falInput(path: string): Record<string, unknown> | null {
  if (path.endsWith("/text-to-video")) return { prompt: "A kitten walking in the sunlight, cinematic style" };
  if (path.endsWith("/image-to-video"))
    return { prompt: "Animate the scene with natural motion", image_url: "<IMAGE_URL>" };
  if (path.endsWith("/reference-to-video"))
    return { prompt: "Generate a video based on the reference images", image_urls: ["<IMAGE_URL>"] };
  return null;
}

/** 拼接可调用的网关地址；参数为当前 API 根地址和后台绑定路径，返回绝对 URL。
 * 供接入详情和示例生成调用；保留反向代理前缀，移除末尾 /v1，非法协议或非本站路径返回 null，无副作用。 */
export function invocationURL(base: string, path: string): string | null {
  try {
    const root = new URL(base);
    if (!["http:", "https:"].includes(root.protocol) || !path.startsWith("/") || path.startsWith("//")) return null;
    root.search = "";
    root.hash = "";
    return root.href.replace(/\/$/, "").replace(/\/v1$/, "") + path;
  } catch {
    return null;
  }
}

/** 为后台开放的普通或 Bypass 端点生成 curl；参数为对外模型名、真实绑定和网关。
 * 返回示例或 null，供模型详情调用；已知协议使用 JSON/multipart，未知协议读取用户准备的 request.json。
 * 模型名和 URL 安全转义，密钥只引用环境变量；无效地址、方法和未展开路径不生成命令，无网络副作用。 */
export function invocationCurl(
  model: string,
  endpoint: ModelEndpoint,
  base: string,
  current?: {
    body: Record<string, unknown>;
    files?: Record<string, string>;
    headers?: Record<string, string>;
    multipart?: boolean;
  },
): string | null {
  const url = invocationURL(base, endpoint.path);
  if (!model.trim() || !url || endpoint.method !== "POST" || /\{[^}]+\}/.test(endpoint.path)) return null;
  const fal = falInput(endpoint.path);
  const bodies: Record<string, unknown> = {
    "openai-chat": { model, messages: [{ role: "user", content: "Hello" }] },
    "openai-responses": { model, input: "Hello" },
    "anthropic-messages": { model, max_tokens: 256, messages: [{ role: "user", content: "Hello" }] },
    gemini: { contents: [{ role: "user", parts: [{ text: "Hello" }] }] },
    vertex: { contents: [{ role: "user", parts: [{ text: "Hello" }] }] },
    "openai-embeddings": { model, input: "Hello" },
    "openai-completions": { model, prompt: "Please introduce yourself", max_tokens: 128 },
    "openai-moderations": { model, input: "Hello" },
    rerank: {
      model,
      query: "What is artificial intelligence?",
      documents: [
        "Artificial intelligence studies how machines perform intelligent tasks.",
        "The weather is sunny today.",
      ],
      top_n: 1,
    },
    ark: { model, content: [{ type: "text", text: "A kitten walking in the sunlight, cinematic style" }] },
    "openai-images": { model, prompt: "A kitten sitting by a window, watercolor style", n: 1, size: "1024x1024" },
    "openai-audio-speech": { model, input: "Hello，Welcome to XHub.", voice: "alloy", response_format: "mp3" },
    fal: fal ? { model, ...fal } : undefined,
  };
  const parts = [
    "curl --request POST " + shellQuote(url),
    "  --silent --show-error --fail-with-body",
    '  --header "Authorization: Bearer $XHUB_API_KEY"',
  ];
  if (current) {
    const body = { ...current.body };
    if (!["gemini", "vertex"].includes(endpoint.protocol)) body.model = model;
    for (const [name, value] of Object.entries(current.headers ?? {})) {
      if (!/^(authorization|cookie|x-api-key)$/i.test(name) && !/[\r\n]/.test(name + value))
        parts.push("  --header " + shellQuote(name + ": " + value));
    }
    if (current.multipart) {
      for (const [name, value] of Object.entries(body)) {
        parts.push(
          "  --form-string " +
            shellQuote(name + "=" + (typeof value === "object" ? JSON.stringify(value) : String(value))),
        );
      }
      for (const [name, filename] of Object.entries(current.files ?? {})) {
        // curl 的 multipart 解析也识别分号等字符，因此文件路径使用双引号并转义，shell 再独立引用。
        const path = ("./" + filename).replaceAll("\\", "\\\\").replaceAll('"', '\\"');
        parts.push("  --form " + shellQuote(name + '=@"' + path + '"'));
      }
    } else {
      parts.push("  --header 'Content-Type: application/json'");
      if (endpoint.protocol === "anthropic-messages") parts.push("  --header 'anthropic-version: 2023-06-01'");
      if (body.stream === true) parts.push("  --no-buffer");
      parts.push("  --data " + shellQuote(JSON.stringify(body, null, 2)));
    }
    if (endpoint.protocol === "openai-audio-speech")
      parts.push(
        "  --output ./speech." +
          (/^(mp3|opus|aac|flac|wav|pcm)$/.test(String(body.response_format)) ? body.response_format : "mp3"),
      );
    return parts.join(" \\\n");
  }
  // multipart 边界交给 curl，不能设置 JSON Content-Type；form-string 避免模型别名被解释为上传指令。
  if (
    ["openai-audio-transcription", "openai-audio-translation", "openai-videos"].includes(endpoint.protocol) ||
    (endpoint.protocol === "openai-images" && endpoint.path.endsWith("/edits"))
  ) {
    parts.push("  --form-string " + shellQuote("model=" + model));
    if (endpoint.protocol === "openai-images") {
      parts.push(
        "  --form " + shellQuote("image=@./input.png"),
        "  --form-string " + shellQuote("prompt=Transform the image into a watercolor painting"),
      );
    } else if (endpoint.protocol === "openai-videos") {
      parts.push("  --form-string " + shellQuote("prompt=A kitten walking in the sunlight, cinematic style"));
    } else {
      parts.push("  --form " + shellQuote("file=@./audio.mp3"));
    }
  } else {
    parts.push("  --header 'Content-Type: application/json'");
    if (endpoint.protocol === "anthropic-messages") parts.push("  --header 'anthropic-version: 2023-06-01'");
    parts.push(
      bodies[endpoint.protocol]
        ? "  --data " + shellQuote(JSON.stringify(bodies[endpoint.protocol], null, 2))
        : "  --data @./request.json",
    );
  }
  if (endpoint.protocol === "openai-audio-speech") parts.push("  --output ./speech.mp3");
  return parts.join(" \\\n");
}

/** 为任务动作生成 curl；参数为后台动作和网关，返回命令或 null，供顺序教程调用。
 * 只替换已知任务占位符或查询参数，拒绝未知方法/占位符；下载保存 video.mp4，URL 安全引用，无副作用。 */
export function invocationActionCurl(
  action: NonNullable<ModelEndpoint["actions"]>[number],
  base: string,
): string | null {
  const url = invocationURL(base, action.public_path);
  if (!url || !["GET", "DELETE"].includes(action.method)) return null;
  const placeholders = url.match(/\{[^}]+\}/g) ?? [];
  if (
    placeholders.some((token) => !["{id}", "{request_id}", "{task_id}"].includes(token)) ||
    (!placeholders.length && !action.task_query)
  )
    return null;
  const target = url
    .split(/\{(?:id|request_id|task_id)\}/)
    .map(shellQuote)
    .join('"$TASK_ID"');
  const parts = [
    "curl --request " + action.method + " " + target,
    "  --silent --show-error --fail-with-body",
    '  --header "Authorization: Bearer $XHUB_API_KEY"',
  ];
  if (action.task_query) {
    if (action.method !== "GET") return null;
    parts.push("  --get", "  --data-urlencode " + shellQuote(action.task_query + "=") + '"$TASK_ID"');
  }
  if (action.name === "content") parts.push("  --output ./video.mp4");
  return parts.join(" \\\n");
}

/** 生成接入教程；参数为对外名称、绑定和网关，返回按执行顺序排列的步骤。
 * 供 API 接入展示；无效配置返回空数组，状态查询先于结果，删除最后显示为可选。
 * 不读取密钥、不自动调用供应商，任务 ID 和素材由用户替换，无副作用。 */
export function invocationSteps(model: string, endpoint: ModelEndpoint, base: string): InvocationStep[] {
  const request = invocationCurl(model, endpoint, base);
  if (!request) return [];
  const actions = (endpoint.actions ?? [])
    .filter((action) => action.name !== "create")
    .map((action) => ({ action, command: invocationActionCurl(action, base) }))
    .filter(
      (entry): entry is { action: NonNullable<ModelEndpoint["actions"]>[number]; command: string } => !!entry.command,
    )
    .sort((a, b) => {
      const order: Record<string, number> = { status: 0, get: 1, content: 2, cancel: 3, delete: 3 };
      return (order[a.action.name] ?? 2) - (order[b.action.name] ?? 2);
    });
  const asyncTask = actions.some(({ action }) => action.method === "GET");
  const known =
    [
      "openai-chat",
      "openai-responses",
      "anthropic-messages",
      "gemini",
      "vertex",
      "openai-embeddings",
      "openai-completions",
      "openai-moderations",
      "rerank",
      "ark",
      "openai-images",
      "openai-videos",
      "openai-audio-speech",
      "openai-audio-transcription",
      "openai-audio-translation",
    ].includes(endpoint.protocol) ||
    (endpoint.protocol === "fal" && !!falInput(endpoint.path));
  let hint = asyncTask ? "createTaskHint" : "requestHint";
  if (!known) hint = "customBodyHint";
  else if (endpoint.protocol === "fal" && !endpoint.path.endsWith("/text-to-video")) hint = "imageUrlHint";
  else if (endpoint.protocol === "openai-images")
    hint = endpoint.path.endsWith("/edits") ? "imageEditHint" : "imageHint";
  else if (["openai-audio-transcription", "openai-audio-translation"].includes(endpoint.protocol))
    hint = "audioUploadHint";
  else if (endpoint.protocol === "openai-audio-speech") hint = "speechHint";
  const steps: InvocationStep[] = [
    {
      title: "prepareKey",
      hint: "prepareKeyHint",
      command: "export XHUB_API_KEY='<YOUR_XHUB_API_KEY>'",
      copy: "copySetup",
    },
    { title: asyncTask ? "createTask" : "sendRequest", hint, command: request, copy: "copyExample" },
  ];
  if (asyncTask)
    steps.push({
      title: "saveTaskId",
      hint: endpoint.protocol === "fal" ? "falTaskIdHint" : "taskIdHint",
      command: "export TASK_ID='<TASK_ID>'",
      copy: "copyTaskId",
    });
  for (const { action, command } of actions) {
    const deleting = action.method === "DELETE";
    steps.push({
      title: deleting
        ? "deleteTask"
        : action.name === "content"
          ? "downloadVideo"
          : action.name === "status"
            ? "queryStatus"
            : "queryResult",
      hint: deleting
        ? "deleteTaskHint"
        : action.name === "content"
          ? "downloadContentHint"
          : endpoint.protocol === "ark"
            ? "arkResultHint"
            : endpoint.protocol === "fal"
              ? "falResultHint"
              : "videoResultHint",
      command,
      copy: deleting
        ? "copyDelete"
        : action.name === "content"
          ? "copyDownload"
          : action.name === "status"
            ? "copyStatus"
            : "copyResult",
    });
  }
  if (
    asyncTask &&
    !actions.some(({ action }) => action.name === "content") &&
    ["ark", "fal"].includes(endpoint.protocol)
  ) {
    steps.push({
      title: "downloadVideo",
      hint: endpoint.protocol === "ark" ? "arkDownloadHint" : "falDownloadHint",
      command:
        "export VIDEO_URL='<VIDEO_URL>'\ncurl --location --silent --show-error --fail --output ./video.mp4 \"$VIDEO_URL\"",
      copy: "copyDownload",
    });
  }
  return steps;
}

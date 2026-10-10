import { additionalPages } from "./expanded";
import type { TFunction } from "@/i18n/translate";

export const docGroups = ["product", "tools", "billing", "api"] as const;
export type DocGroup = (typeof docGroups)[number];
export type DocPage = { id: string; group: DocGroup; path: string; code?: string };
/** 当前分类内的文章分组；参数为文章 ID，返回稳定分组键，供双语目录展示，不改变路由，无副作用。 */
export function docSection(id: string): string {
  if (id.startsWith("native-")) return "bypass";
  if (id.startsWith("fal-")) return "video";
  if (id.startsWith("help-")) return "help";
  const sections: Record<string, string> = {
    overview: "start",
    quickstart: "start",
    keys: "management",
    routing: "management",
    deployments: "management",
    playground: "management",
    teams: "management",
    toolsOverview: "start",
    sdk: "sdks",
    "javascript-sdk": "sdks",
    codex: "coding",
    "claude-code": "coding",
    opencode: "coding",
    cursor: "coding",
    vscode: "coding",
    clients: "clients",
    "cherry-studio": "clients",
    "claude-desktop": "clients",
    agents: "agents",
    usage: "costs",
    budgets: "limits",
    models: "models",
    chat: "text",
    responses: "text",
    messages: "text",
    completions: "text",
    gemini: "text",
    embeddings: "vectors",
    rerank: "vectors",
    moderations: "safety",
    images: "images",
    "image-edits": "images",
    speech: "audio",
    transcriptions: "audio",
    translations: "audio",
    videos: "video",
    ark: "video",
    fal: "video",
    bypass: "bypass",
    errors: "protocols",
  };
  return sections[id] ?? "start";
}

export const chatExample = `curl "$XHUB_BASE_URL/v1/chat/completions" \\
  -H "Authorization: Bearer $XHUB_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"YOUR_MODEL_NAME","messages":[{"role":"user","content":"Hello, introduce yourself."}],"stream":false}'`;

export const docPages: readonly DocPage[] = [
  { id: "overview", group: "product", path: "" },
  { id: "quickstart", group: "product", path: "product/quickstart", code: chatExample },
  { id: "keys", group: "product", path: "product/api-keys" },
  { id: "routing", group: "product", path: "product/routing" },
  { id: "toolsOverview", group: "tools", path: "tools-agents-clients/overview" },
  {
    id: "sdk",
    group: "tools",
    path: "tools-agents-clients/openai-sdk",
    code: `import os
from openai import OpenAI

client = OpenAI(
    api_key=os.environ["XHUB_API_KEY"],
    base_url=os.environ["XHUB_BASE_URL"].rstrip("/"),
)
response = client.chat.completions.create(
    model="YOUR_MODEL_NAME",
    messages=[{"role": "user", "content": "Hello, introduce yourself."}],
)
print(response.choices[0].message.content)`,
  },
  {
    id: "clients",
    group: "tools",
    path: "tools-agents-clients/clients",
    code: `OPENAI_API_KEY=YOUR_XHUB_API_KEY
OPENAI_BASE_URL=https://YOUR_GATEWAY_HOST
OPENAI_MODEL=YOUR_MODEL_NAME`,
  },
  { id: "usage", group: "billing", path: "billing/usage-based-billing" },
  { id: "budgets", group: "billing", path: "billing/budgets" },
  {
    id: "models",
    group: "api",
    path: "api/models",
    code: `curl "$XHUB_BASE_URL/models"`,
  },
  { id: "chat", group: "api", path: "api/chat-completions", code: chatExample },
  { id: "errors", group: "api", path: "api/errors" },
  ...additionalPages,
].sort((a, b) => docGroups.indexOf(a.group as DocGroup) - docGroups.indexOf(b.group as DocGroup)) as DocPage[];

/** 根据可选捕获路径查找文章；参数为 URL 片段数组，返回文章或 undefined。供服务端路由使用，未知路径不得回退首页，无副作用。 */
export function findDocPage(slug: string[] = []): DocPage | undefined {
  return docPages.find((page) => page.path === slug.join("/"));
}

/** 搜索当前语言的标题、简介和正文；参数为用户原始查询与翻译函数，返回匹配文章。供目录搜索调用，忽略首尾空白和大小写，空查询返回全部，不修改输入。 */
export function searchDocs(query: string, t: TFunction): readonly DocPage[] {
  const term = query.trim().toLocaleLowerCase();
  if (!term) return docPages;
  return docPages.filter((page) =>
    ["title", "description", "body1", "body2", "body3", "detail"].some((field) =>
      t(`docs.articles.${page.id}.${field}`).toLocaleLowerCase().includes(term),
    ),
  );
}

/** 返回文章的协议键；参数为文章 ID，目录两端共同调用，未知文章使用通用说明，无副作用。 */
export function docProtocol(id: string): string {
  if (id === "bypass" || id.startsWith("native-")) return "bypass";
  if (id === "fal" || id.startsWith("fal-")) return "fal";
  if (id === "ark") return "contents";
  if (["messages", "gemini"].includes(id)) return "native";
  if (["models", "errors"].includes(id)) return "general";
  return "openai";
}

/** 将文章按能力和协议构成三级导航；参数为过滤后的文章，返回有序模块和协议节点。搜索不制造空组，不修改输入。 */
export function docTree(pages: readonly DocPage[]) {
  return [...new Set(pages.map((page) => docSection(page.id)))].map((section) => {
    const items = pages.filter((page) => docSection(page.id) === section);
    return {
      section,
      protocols: [...new Set(items.map((page) => docProtocol(page.id)))].map((protocol) => ({
        protocol,
        pages: items.filter((page) => docProtocol(page.id) === protocol),
      })),
    };
  });
}

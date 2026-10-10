"use client";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { DocumentationMarkdown, documentationHeadings } from "./DocumentationMarkdown";
import { apiSpecs } from "./expanded";
import { ApiRunnerDialog } from "./ApiRunnerDialog";
import Link from "next/link";
import { useState, useSyncExternalStore } from "react";
import { getProxyBaseUrl } from "@/components/networking";
import { ArrowRight, BookOpen, Copy, Search, Terminal, CreditCard, Layers } from "lucide-react";
import LanguageSwitcher from "@/components/LanguageSwitcher";
import { useI18n } from "@/i18n";
import { docGroups, docPages, docSection, docTree, searchDocs, type DocPage } from "./catalog";

const icons = { product: BookOpen, tools: Terminal, billing: CreditCard, api: Layers };
/** 文档页面导航会重新读取网关；无参数，返回取消订阅函数，无副作用。 */
function subscribeGateway() {
  return () => {};
}
/** 返回调试台当前网关根地址；客户端阅读器调用，保留代理前缀，去掉末尾版本段，无网络调用。 */
function gatewaySnapshot() {
  return getProxyBaseUrl().replace(/\/+$/, "").replace(/\/v1$/, "");
}
/** 服务端尚无浏览器网关选择；返回空值，供 hydration 使用，无副作用。 */
function serverGatewaySnapshot() {
  return "";
}
/** 生成同站文档地址；参数为文章，返回 /docs 下的路径。供导航使用，不读取外部状态，无副作用。 */
export function docHref(page: DocPage): string {
  return "/docs" + (page.path ? "/" + page.path : "");
}

/** 文档阅读器；参数 page 为服务端确认的文章，返回双语目录、正文和索引。搜索保留用户输入，复制只写剪贴板且呈现失败状态，不调用供应商。 */
export function DocsCenter({ page, apiMarkdown }: { page: DocPage; apiMarkdown?: Record<"en" | "zh-CN", string> }) {
  const { t, locale } = useI18n();
  const markdown = apiMarkdown?.[locale];
  const [query, setQuery] = useState("");
  const [copyState, setCopyState] = useState<"copy" | "copied" | "copyFailed">("copy");
  const [responseStatus, setResponseStatus] = useState("200");
  const gatewayBase = useSyncExternalStore(subscribeGateway, gatewaySnapshot, serverGatewaySnapshot);
  const resolvedCode = (page.code ?? "").replaceAll(
    "https://YOUR_GATEWAY_HOST",
    gatewayBase || "https://YOUR_GATEWAY_HOST",
  );
  // shell 使用单引号编码实际地址，避免部署配置中的特殊字符被当作命令执行。
  const exampleCode =
    gatewayBase && resolvedCode.includes("$XHUB_BASE_URL")
      ? "export XHUB_BASE_URL='" + gatewayBase.replaceAll("'", "'\\''") + "'\n" + resolvedCode
      : resolvedCode;
  const responseErrors: Record<string, { message: string; type: string }> = {
    "400": { message: "Invalid request", type: "invalid_request" },
    "401": { message: "Invalid API key", type: "invalid_api_key" },
    "403": { message: "Access denied", type: "forbidden" },
    "429": { message: "Rate limit exceeded", type: "rate_limit_error" },
  };
  const apiMethods = Object.fromEntries(apiSpecs.map((entry) => [entry.id, entry.method]));
  const spec = apiSpecs.find((item) => item.id === page.id);
  const showRunner = Boolean(spec) && Boolean(gatewayBase);
  const runnerBody = page.code?.match(/-d '([^']+)'/)?.[1] ?? '{"model":"YOUR_MODEL_NAME"}';
  const detail = t("docs.articles." + page.id + ".detail");
  const categoryPages = docPages.filter((item) => item.group === page.group);
  const sectionOrder = [...new Set(categoryPages.map((item) => docSection(item.id)))];
  categoryPages.sort((a, b) => sectionOrder.indexOf(docSection(a.id)) - sectionOrder.indexOf(docSection(b.id)));
  const matchingIds = new Set(searchDocs(query, t).map((item) => item.id));
  const matches = categoryPages.filter((item) => matchingIds.has(item.id));
  // 服务端传入的文章经序列化后不保留对象引用，使用稳定 ID 定位前后文章。
  const index = categoryPages.findIndex((item) => item.id === page.id);
  const key = "docs.articles." + page.id;
  /** 复制当前英文示例；无参数，异步完成。按钮调用，剪贴板拒绝或不可用时显示错误，不抛出未处理异常。 */
  async function copyExample() {
    try {
      await navigator.clipboard.writeText(exampleCode);
      setCopyState("copied");
    } catch {
      setCopyState("copyFailed");
    }
  }
  return (
    <div className="min-h-screen bg-background text-foreground">
      <style>{`.docs-prose { line-height: 1.9; overflow-wrap: anywhere; } .docs-prose h2 { font-size: 1.25rem; font-weight: 600; margin: 2rem 0 1rem; } .docs-prose p { margin: 1rem 0; color: var(--muted-foreground); } .docs-prose table { width: 100%; display: block; overflow-x: auto; border-collapse: collapse; font-size: .875rem; } .docs-prose th, .docs-prose td { text-align: left; border-bottom: 1px solid var(--border); padding: .75rem; }`}</style>
      <header className="sticky top-0 z-chrome border-b border-border bg-background/95 backdrop-blur">
        <div className="mx-auto flex max-w-[1440px] flex-wrap items-center justify-between gap-4 px-5 py-4 lg:px-8">
          <Link href="/docs" className="flex items-center gap-3 font-semibold" aria-label={"XHub " + t("docs.center")}>
            <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-600 text-white">
              <BookOpen size={18} />
            </span>
            XHub{" "}
            <span className="border-l border-border pl-3 text-sm font-normal text-muted-foreground">
              {t("docs.center")}
            </span>
          </Link>
          <div className="flex items-center gap-4">
            <Link href="/ui" className="text-sm text-muted-foreground hover:text-foreground">
              {t("docs.console")}
            </Link>
            <LanguageSwitcher />
          </div>
        </div>
        <nav
          aria-label={t("docs.navigation")}
          className="mx-auto flex max-w-[1440px] gap-6 overflow-x-auto px-5 lg:px-8"
        >
          {docGroups.map((group) => (
            <Link
              key={group}
              href={docHref(docPages.find((item) => item.group === group)!)}
              aria-current={page.group === group ? "page" : undefined}
              className={
                "whitespace-nowrap border-b-2 py-3 text-sm font-medium " +
                (page.group === group
                  ? "border-emerald-600 text-emerald-600"
                  : "border-transparent text-muted-foreground hover:text-foreground")
              }
            >
              {t("docs.groups." + group)}
            </Link>
          ))}
        </nav>
      </header>
      <div className="mx-auto grid max-w-[1440px] gap-8 px-5 py-8 md:grid-cols-[230px_minmax(0,1fr)] lg:grid-cols-[240px_minmax(0,1fr)_180px] lg:px-8">
        <aside className="md:sticky md:top-36 md:max-h-[calc(100vh-160px)] md:overflow-auto md:self-start">
          <form autoComplete="off" role="search" onSubmit={(event) => event.preventDefault()}>
            <label className="mb-6 flex items-center gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2.5">
              <Search size={16} className="shrink-0 text-muted-foreground" />
              <input
                type="search"
                id="xhub-documentation-filter-text"
                name="documentation-filter-text"
                autoComplete="off"
                autoCapitalize="none"
                spellCheck={false}
                aria-label={t("docs.search")}
                placeholder={t("docs.search")}
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                className="min-w-0 w-full bg-transparent text-sm outline-none"
              />
            </label>
          </form>
          {query.trim() && (
            <p role="status" className="mb-3 text-xs text-muted-foreground">
              {t("docs.results", { count: matches.length })}
            </p>
          )}
          {!matches.length && <p className="text-sm text-muted-foreground">{t("docs.empty")}</p>}
          <nav aria-label={t("docs.directory")} className="space-y-6">
            {[page.group].map((group) => {
              const items = matches.filter((item) => item.group === group);
              if (!items.length) return null;
              const Icon = icons[group];
              return (
                <div key={group}>
                  <p className="mb-2 flex items-center gap-2 text-xs font-semibold text-muted-foreground">
                    <Icon size={14} />
                    {t("docs.groups." + group)}
                  </p>
                  <div className="space-y-1">
                    {group === "api"
                      ? docTree(items).map((module) => (
                          <section key={module.section}>
                            <h2 className="px-3 pt-4 pb-2 text-sm font-semibold">
                              {t("docs.sections." + module.section)}
                            </h2>
                            {module.protocols.map((branch) => (
                              <details key={branch.protocol} open className="ml-2 border-l border-border pl-2">
                                <summary className="cursor-pointer px-2 py-2 text-xs font-medium text-muted-foreground">
                                  {t("docs.protocols." + branch.protocol)}
                                </summary>
                                {branch.pages.map((item) => (
                                  <Link
                                    key={item.id}
                                    href={docHref(item)}
                                    aria-current={item.id === page.id ? "page" : undefined}
                                    className={
                                      "block rounded-md px-3 py-2 text-sm " +
                                      (item.id === page.id
                                        ? "bg-emerald-500/10 font-medium text-emerald-600"
                                        : "text-muted-foreground hover:bg-muted hover:text-foreground")
                                    }
                                  >
                                    {group === "api" && (
                                      <span aria-hidden="true" className="mr-2 font-mono text-xs text-emerald-600">
                                        {apiMethods[item.id]}
                                      </span>
                                    )}
                                    {t("docs.articles." + item.id + ".title")}
                                  </Link>
                                ))}
                              </details>
                            ))}
                          </section>
                        ))
                      : items.map((item, position) => (
                          <div key={item.id}>
                            {(position === 0 || docSection(items[position - 1].id) !== docSection(item.id)) && (
                              <p className="px-3 pt-4 pb-1 text-xs font-semibold text-muted-foreground">
                                {t("docs.sections." + docSection(item.id))}
                              </p>
                            )}
                            <Link
                              key={item.id}
                              href={docHref(item)}
                              aria-current={item.id === page.id ? "page" : undefined}
                              className={
                                "block rounded-md px-3 py-2 text-sm " +
                                (item.id === page.id
                                  ? "bg-emerald-500/10 font-medium text-emerald-600"
                                  : "text-muted-foreground hover:bg-muted hover:text-foreground")
                              }
                            >
                              {/* 此分支只展示产品、工具和计费文章，HTTP 方法仅在上方 API 目录显示。 */}
                              {t("docs.articles." + item.id + ".title")}
                            </Link>
                          </div>
                        ))}
                  </div>
                </div>
              );
            })}
          </nav>
        </aside>
        <main className="min-w-0 max-w-[800px] pb-16">
          <p className="mb-3 text-sm font-medium text-emerald-600">{t("docs.groups." + page.group)}</p>
          <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">{t(key + ".title")}</h1>
          {!markdown && <p className="mt-4 text-lg leading-8 text-muted-foreground">{t(key + ".description")}</p>}
          {!markdown && gatewayBase && (
            <p className="mt-4 break-all text-sm text-muted-foreground">
              {t("docs.gatewayBase", { base: gatewayBase })}
            </p>
          )}
          {page.id === "overview" && (
            <div className="mt-8 grid gap-4 sm:grid-cols-2">
              {[page.group].map((group) => {
                const first = docPages.find((item) => item.group === group && item.id !== "overview")!;
                const Icon = icons[group];
                return (
                  <Link
                    key={group}
                    href={docHref(first)}
                    className="rounded-xl border border-border p-5 transition-colors hover:border-emerald-500 hover:bg-emerald-500/5"
                  >
                    <Icon className="mb-4 text-emerald-600" size={22} />
                    <h2 className="font-semibold">{t("docs.groups." + group)}</h2>
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                      {t("docs.articles." + first.id + ".description")}
                    </p>
                    <span className="mt-4 flex items-center gap-2 text-xs font-medium text-emerald-600">
                      {t("docs.read")}
                      <ArrowRight size={14} />
                    </span>
                  </Link>
                );
              })}
            </div>
          )}
          {showRunner && (
            <ApiRunnerDialog
              key={page.id}
              base={gatewayBase}
              endpoint={spec?.endpoint ?? "/bypass/openai/v1/chat/completions"}
              method={spec?.method ?? "POST"}
              initialBody={runnerBody}
            />
          )}
          {markdown && (
            <div className="docs-prose mt-8" id={spec?.anchor}>
              <DocumentationMarkdown
                markdown={markdown
                  .replace(/^# .+\n/, "")
                  .replaceAll("https://YOUR_GATEWAY_HOST", gatewayBase)
                  .replaceAll('curl "$XHUB_BASE_URL', 'curl "' + gatewayBase)}
              />
            </div>
          )}
          {!markdown &&
            [1, 2, 3]
              .filter((section) => t(key + ".heading" + section))
              .map((section) => (
                <section key={section} id={"section-" + section} className="mt-10 scroll-mt-40">
                  <h2 className="mb-4 text-xl font-semibold">{t(key + ".heading" + section)}</h2>
                  <div className="docs-prose">
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{t(key + ".body" + section)}</ReactMarkdown>
                  </div>
                </section>
              ))}
          {!markdown && detail !== key + ".detail" && (
            <div className="docs-prose mt-10">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{detail}</ReactMarkdown>
            </div>
          )}
          {!markdown && spec && (
            <section id={spec.anchor} className="mt-10 scroll-mt-40">
              {!markdown && (
                <>
                  {" "}
                  <p className="mt-4 text-sm leading-7 text-muted-foreground">{t("docs.auth")}</p>
                  <h2 className="mt-8 mb-4 text-xl font-semibold">{t("docs.parameters")}</h2>
                  <div className="overflow-x-auto">
                    <table className="w-full text-left text-sm">
                      <thead>
                        <tr className="border-b border-border">
                          {["field", "type", "description"].map((label) => (
                            <th className="p-3" key={label}>
                              {t("docs." + label)}
                            </th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {spec.fields.map((field, position) => (
                          <tr key={field.name} className="border-b border-border">
                            <td className="p-3 font-mono">{field.name}</td>
                            <td className="p-3 text-muted-foreground">
                              {field.type.replace("required", t("docs.required"))}
                            </td>
                            <td className="p-3 leading-6">{t(key + ".fields." + position)}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                  <h2 className="mt-8 mb-4 text-xl font-semibold">{t("docs.response")}</h2>
                  <div className="mb-6 overflow-x-auto">
                    <table className="w-full text-left text-sm">
                      <thead>
                        <tr className="border-b border-border">
                          <th className="p-3">{t("docs.field")}</th>
                          <th className="p-3">{t("docs.type")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {spec.responseFields.map((field) => (
                          <tr className="border-b border-border" key={field.name}>
                            <td className="p-3 font-mono">{field.name}</td>
                            <td className="p-3 text-muted-foreground">{field.type}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </>
              )}
              <div role="tablist" aria-label={t("docs.response")} className="flex gap-6 border-b border-border">
                {["200", "400", "401", "403", "429"].map((status) => (
                  <button
                    key={status}
                    role="tab"
                    aria-selected={responseStatus === status}
                    aria-controls="api-response"
                    id={"response-" + status}
                    onClick={() => setResponseStatus(status)}
                    className={
                      "border-b-2 px-2 py-3 " +
                      (responseStatus === status ? "border-emerald-600 text-emerald-600" : "border-transparent")
                    }
                  >
                    {status}
                  </button>
                ))}
              </div>
              <div role="tabpanel" id="api-response" aria-labelledby={"response-" + responseStatus}>
                <p className="my-4 text-sm text-muted-foreground">{t("docs.status" + responseStatus)}</p>
                <pre className="overflow-x-auto rounded-xl bg-zinc-950 p-5 text-xs leading-7 text-emerald-200">
                  <code>
                    {responseStatus === "200"
                      ? spec.response
                      : JSON.stringify({ error: responseErrors[responseStatus] }, null, 2)}
                  </code>
                </pre>
              </div>
            </section>
          )}
          {!markdown && page.code && (
            <section id="example" className="mt-10 scroll-mt-40">
              <div className="mb-3 flex items-center justify-between gap-3">
                <h2 className="text-xl font-semibold">{t("docs.example")}</h2>
                <button
                  onClick={copyExample}
                  aria-label={t("docs.copy")}
                  className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-xs hover:bg-muted"
                >
                  <Copy size={14} />
                  {t("docs." + (copyState === "copyFailed" ? "copy" : copyState))}
                </button>
              </div>
              <pre
                aria-label={t("docs.example")}
                className="overflow-x-auto rounded-xl border border-border bg-muted/40 p-5 text-xs leading-7 sm:text-sm"
              >
                <code>{exampleCode}</code>
              </pre>
              <p role="status" className="mt-3 text-sm text-muted-foreground">
                {t("docs." + (copyState === "copy" ? "note" : copyState))}
              </p>
            </section>
          )}
          <footer className="mt-12 grid grid-cols-2 gap-4 border-t border-border pt-6">
            {index > 0 ? (
              <Link
                href={docHref(categoryPages[index - 1])}
                className="rounded-lg border border-border p-4 hover:bg-muted"
              >
                <span className="text-xs text-muted-foreground">{t("docs.previous")}</span>
                <p className="mt-2 text-sm font-medium">
                  {t("docs.articles." + categoryPages[index - 1].id + ".title")}
                </p>
              </Link>
            ) : (
              <span />
            )}
            {index < categoryPages.length - 1 && (
              <Link
                href={docHref(categoryPages[index + 1])}
                className="rounded-lg border border-border p-4 text-right hover:bg-muted"
              >
                <span className="text-xs text-muted-foreground">{t("docs.next")}</span>
                <p className="mt-2 text-sm font-medium">
                  {t("docs.articles." + categoryPages[index + 1].id + ".title")}
                </p>
              </Link>
            )}
          </footer>
        </main>
        <aside className="sticky top-36 hidden self-start border-l border-border pl-5 lg:block">
          <nav aria-label={t("docs.onPage")}>
            <p className="mb-4 text-xs font-semibold">{t("docs.onPage")}</p>
            <div className="space-y-4">
              {markdown &&
                documentationHeadings(markdown).map(({ title, id }) => (
                  <a
                    key={id}
                    href={"#" + id}
                    className="block text-xs leading-5 text-muted-foreground hover:text-emerald-600"
                  >
                    {title}
                  </a>
                ))}
              {!markdown &&
                [1, 2, 3]
                  .filter((section) => t(key + ".heading" + section))
                  .map((section) => (
                    <a
                      key={section}
                      href={"#section-" + section}
                      className="block text-xs leading-5 text-muted-foreground hover:text-emerald-600"
                    >
                      {t(key + ".heading" + section)}
                    </a>
                  ))}
              {!markdown && spec && (
                <a href={"#" + spec.anchor} className="block text-xs text-muted-foreground">
                  {t("docs.parameters")} / {t("docs.response")}
                </a>
              )}
              {!markdown && page.code && (
                <a href="#example" className="block text-xs text-muted-foreground">
                  {t("docs.example")}
                </a>
              )}
            </div>
          </nav>
        </aside>
      </div>
    </div>
  );
}

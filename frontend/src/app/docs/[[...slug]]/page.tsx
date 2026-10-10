import { readApiMarkdown } from "@/components/docs/apiMarkdown";
import { notFound } from "next/navigation";
import { DocsCenter } from "@/components/docs/DocsCenter";
import { findDocPage } from "@/components/docs/catalog";

/** 文档路由入口；参数为 Next 异步路径，返回对应文章阅读器。支持首页和子目录，未知路径返回真实 404，不写数据或调用上游。 */
export default async function DocsPage({ params }: { params: Promise<{ slug?: string[] }> }) {
  const { slug } = await params;
  const page = findDocPage(slug);
  if (!page) notFound();
  return <DocsCenter page={page} apiMarkdown={page.group === "api" ? readApiMarkdown(page.id) : undefined} />;
}

import { readFileSync } from "node:fs";
import { resolve } from "node:path";

/** 从仓库独立 Markdown 读取 API 双语正文；参数为目录中确认的文章 ID，返回两种语言正文。
 * 仅服务端文档路由调用，非法 ID 抛错，缺失文件阻止构建或阅读，不回退过期内容；不修改源文件。 */
export function readApiMarkdown(id: string): Record<"en" | "zh-CN", string> {
  if (!/^[a-z0-9-]+$/.test(id)) throw new Error("Invalid API document ID");
  const directory = resolve(process.cwd(), "../docs/api");
  return {
    en: readFileSync(resolve(directory, "en", id + ".md"), "utf8"),
    "zh-CN": readFileSync(resolve(directory, "zh-CN", id + ".md"), "utf8"),
  };
}

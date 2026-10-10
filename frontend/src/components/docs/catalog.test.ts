import { describe, expect, it } from "vitest";
import { collectKeys, translate } from "@/i18n/translate";
import { docsEn } from "@/i18n/messages/docs.en";
import { docsZhCN } from "@/i18n/messages/docs.zh-CN";
import { chatExample, docGroups, docPages, docSection, findDocPage, searchDocs } from "./catalog";

describe("documentation catalog", () => {
  /** 目的：核对首页、所有文章与错误边界；前置静态目录，未知/前缀路径不能返回文章，目录唯一且四类齐全，无清理副作用。 */
  it("resolves known paths and rejects unknown paths", () => {
    expect(findDocPage()).toBe(docPages[0]);
    for (const page of docPages) expect(findDocPage(page.path ? page.path.split("/") : [])).toBe(page);
    expect(findDocPage(["api"])).toBeUndefined();
    expect(findDocPage(["missing"])).toBeUndefined();
    expect(new Set(docPages.map((p) => p.path)).size).toBe(docPages.length);
    expect(new Set(docPages.map((p) => p.group))).toEqual(new Set(docGroups));
    expect(docSection("unknown")).toBe("start");
    for (const page of docPages) {
      const key = "docs.sections." + docSection(page.id);
      expect(translate("zh-CN", key)).not.toBe(key);
      expect(translate("en", key)).not.toBe(key);
    }
  });
  /** 目的：核对语言完整性与动态参数；前置双语目录，所有文章字段可翻译且键一致，计数插值正确，无持久化数据。 */
  it("has complete matching translations", () => {
    expect(collectKeys(docsEn).sort()).toEqual(collectKeys(docsZhCN).sort());
    for (const locale of ["en", "zh-CN"] as const)
      for (const page of docPages)
        for (const field of ["title", "description", "heading1", "heading2", "heading3", "body1", "body2", "body3"]) {
          const key = "docs.articles." + page.id + "." + field;
          expect(translate(locale, key)).not.toBe(key);
        }
    expect(translate("en", "docs.results", { count: 2 })).toBe("2 documents found");
  });
  /** 目的：验证标题和正文搜索、空白和失败输入；前置双语翻译函数，返回匹配结果且不修改用户原文，无清理操作。 */
  it("searches translated prose without changing user input", () => {
    const en = (key: string) => translate("en", key);
    const zh = (key: string) => translate("zh-CN", key);
    expect(searchDocs("  ", en)).toBe(docPages);
    expect(searchDocs("  OPENAI SDK  ", en).map((p) => p.id)).toContain("sdk");
    expect(searchDocs("计费", zh).map((p) => p.id)).toContain("overview");
    expect(searchDocs("no-such-document-xyz", en)).toEqual([]);
    const query = "用户的原始问题";
    searchDocs(query, zh);
    expect(query).toBe("用户的原始问题");
  });
  /** 目的：确保请求示例英文且具备必要协议字段；前置真实文档示例，验证模型、消息和环境变量，没有调用外部服务。 */
  it("provides executable English examples", () => {
    expect(chatExample).toContain("Hello, introduce yourself.");
    expect(chatExample).toContain("$XHUB_API_KEY");
    for (const page of docPages) if (page.code) expect(page.code).not.toMatch(/[\u4e00-\u9fff]/);
  });
});

/** 目的：校验工具配置、计费算例及接口字段解释不遗漏翻译；前置静态真实目录，两种语言均可搜索展开正文，示例保留英文，无数据清理。 */
it("covers detailed tools, billing and API contracts", async () => {
  const { apiSpecs } = await import("./expanded");
  for (const locale of ["en", "zh-CN"] as const) {
    const t = (key: string) => translate(locale, key);
    expect(searchDocs("$0.0051", t).map((page) => page.id)).toContain("usage");
    for (const spec of apiSpecs) {
      expect(docPages.some((page) => page.id === spec.id)).toBe(true);
      expect(spec.responseFields.length).toBeGreaterThan(0);
      for (const [i] of spec.fields.entries()) {
        const key = "docs.articles." + spec.id + ".fields." + i;
        expect(t(key)).not.toBe(key);
      }
    }
  }
  for (const id of [
    "codex",
    "claude-code",
    "opencode",
    "cursor",
    "vscode",
    "cherry-studio",
    "claude-desktop",
    "agents",
  ])
    expect(docPages.some((page) => page.id === id)).toBe(true);
  expect(docPages.some((page) => page.path === "api/batches")).toBe(false);
});

/** 目的：验证能力、协议、文章三级目录；前置真实文章表，生图与视频隔离，Fal 模型齐全，空输入无空组，无需清理。 */
it("groups implemented APIs by capability and protocol", async () => {
  const { docTree } = await import("./catalog");
  const tree = docTree(docPages.filter((page) => page.group === "api"));
  expect(tree.find((module) => module.section === "images")?.protocols.map((branch) => branch.protocol)).toEqual([
    "openai",
  ]);
  const videos = tree.find((module) => module.section === "video")!;
  expect(videos.protocols.find((branch) => branch.protocol === "fal")?.pages.map((page) => page.id)).toEqual(
    expect.arrayContaining(["fal-seedance", "fal-kling", "fal-vidu", "fal-veo", "fal-minimax"]),
  );
  expect(docTree([])).toEqual([]);
  expect(docTree([docPages.find((page) => page.id === "fal-seedance")!])[0].protocols).toHaveLength(1);
});

/** 目的：验证原厂目录每个叶子都有独立协议与路径；前置真实目录，未知 ID 不冒充原厂，空筛选保持无节点，无数据清理。 */
it("provides independent registered native protocol articles", async () => {
  const { docTree, docProtocol } = await import("./catalog");
  const { apiSpecs } = await import("./apiSpecs");
  const native = docPages.filter((page) => page.id.startsWith("native-"));
  expect(native).toHaveLength(7);
  for (const page of native) {
    expect(docProtocol(page.id)).toBe("bypass");
    expect(apiSpecs.find((spec) => spec.id === page.id)?.endpoint.startsWith("/bypass/")).toBe(true);
  }
  expect(docProtocol("unknown")).toBe("openai");
  expect(docTree(native)[0].protocols[0].pages).toEqual(native);
  const tree = docTree(docPages.filter((page) => page.group === "api"));
  const bypass = tree.find((module) => module.section === "bypass")!;
  expect(bypass.protocols[0].pages.map((page) => page.id)).toEqual(
    expect.arrayContaining(["bypass", ...native.map((page) => page.id)]),
  );
  expect(
    tree
      .find((module) => module.section === "text")!
      .protocols.flatMap((branch) => branch.pages)
      .every((page) => page.id !== "bypass" && !page.id.startsWith("native-")),
  ).toBe(true);
});

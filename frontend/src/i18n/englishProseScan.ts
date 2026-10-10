import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import ts from "typescript";

const SRC_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

const SKIP_FILE = /\.(test|spec|cases)\.[cm]?tsx?$|\.d\.ts$|\.md$/;

const TRANSLATION_CALLEES = new Set(["t", "translate", "tDefault", "tRuntime", "externalGuardrailText"]);

const TECHNICAL_ATTRS = new Set([
  "className",
  "class",
  "style",
  "href",
  "src",
  "id",
  "key",
  "htmlFor",
  "for",
  "xmlns",
  "viewBox",
  "d",
  "fill",
  "stroke",
  "strokeWidth",
  "clipPath",
  "mask",
  "points",
  "transform",
  "offset",
  "stopColor",
  "type",
  "role",
  "variant",
  "size",
  "as",
  "asChild",
  "name",
  "value",
  "defaultValue",
  "rel",
  "target",
  "method",
  "action",
  "encType",
  "autoComplete",
  "inputMode",
  "pattern",
  "accept",
  "width",
  "height",
  "tabIndex",
  "colSpan",
  "rowSpan",
  "align",
  "valign",
  "scope",
  "headers",
  "dateTime",
  "datetime",
  "open",
  "slot",
  "color",
  "dir",
  "lang",
  "media",
  "sizes",
  "srcSet",
  "form",
  "formAction",
  "min",
  "max",
  "step",
  "minLength",
  "maxLength",
  "rows",
  "cols",
  "wrap",
  "multiple",
  "checked",
  "readOnly",
  "disabled",
  "required",
  "hidden",
  "draggable",
  "contentEditable",
  "spellCheck",
  "autoFocus",
  "autoPlay",
  "controls",
  "loop",
  "muted",
  "poster",
  "preload",
  "kind",
  "srclang",
  "download",
  "cite",
  "span",
  "start",
  "nonce",
  "integrity",
  "crossOrigin",
  "referrerPolicy",
  "fetchPriority",
  "loading",
  "decoding",
  "frameBorder",
  "allow",
  "sandbox",
  "scrolling",
  "cellPadding",
  "cellSpacing",
  "border",
]);

const USER_FACING_ATTRS = new Set([
  "placeholder",
  "title",
  "alt",
  "label",
  "description",
  "subtitle",
  "tooltip",
  "helperText",
  "emptyText",
  "emptyMessage",
  "message",
  "heading",
  "header",
  "caption",
  "hint",
  "help",
  "confirmText",
  "cancelText",
  "okText",
  "buttonText",
  "text",
  "content",
  "searchPlaceholder",
  "noResultsText",
  "noOptionsMessage",
  "noOptionsText",
  "clearAllLabel",
  "labelText",
  "loadingText",
  "loadingMessage",
  "emptyMessage",
  "ariaLabel",
  "aria-label",
  "aria-description",
  "aria-placeholder",
  "aria-roledescription",
  "aria-valuetext",
  "loadingText",
  "errorMessage",
  "successMessage",
  "deleteTitle",
  "deleteMessage",
  "infoTitle",
  "emptyHint",
]);

const USER_FACING_PROPS = new Set(USER_FACING_ATTRS);

const CODE_TAGS = new Set(["code", "pre", "CodeBlock", "SyntaxHighlighter", "script", "style"]);

const CLASSNAME_FNS = new Set(["cn", "clsx", "cva", "twMerge", "twJoin"]);

export type ProseHit = {
  file: string;
  line: number;
  start: number;
  end: number;
  kind: string;
  template: string;
  vars: { name: string; expr: string }[];
  wrapBraces: boolean;
};

function walkDir(dir: string, out: string[] = []): string[] {
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    if (ent.name.startsWith(".")) continue;
    const p = path.join(dir, ent.name);
    if (ent.isDirectory()) {
      if (ent.name === "node_modules" || ent.name === "__tests__") continue;
      if (ent.name === "data" && path.basename(dir) === "src") continue;
      if (ent.name === "messages" && path.basename(dir) === "i18n") continue;
      walkDir(p, out);
    } else if (/\.tsx?$/.test(ent.name) && !SKIP_FILE.test(ent.name)) {
      out.push(p);
    }
  }
  return out;
}

function decodeEntities(s: string): string {
  return s
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    .replace(/&#39;/g, "'")
    .replace(/&nbsp;/g, " ")
    .replace(/&#x([0-9a-f]+);/gi, (_, n) => String.fromCharCode(parseInt(n, 16)))
    .replace(/&#(\d+);/g, (_, n) => String.fromCharCode(Number(n)));
}

function collapse(s: string): string {
  return s.replace(/\s+/g, " ").trim();
}

function isCssLike(s: string): boolean {
  if (/color-mix\(|var\(--/.test(s)) return true;
  if (/[{};]/.test(s) && /:\s*\S/.test(s) && !/[A-Z][a-z]{2,}\s/.test(s)) return true;
  const parts = s.split(/\s+/).filter(Boolean);
  if (parts.length < 2) return false;
  return parts.every(
    (p) =>
      /^[a-z0-9_:[\]/.#()-]+$/i.test(p) &&
      (p.includes("-") ||
        /^(flex|grid|hidden|block|inline|relative|absolute|fixed|sticky|truncate|container|contents|group|peer|dark|sr-only)$/.test(
          p,
        ) ||
        /^(hover|focus|active|disabled|sm|md|lg|xl|2xl):/.test(p) ||
        /^[a-z]+-\d/.test(p)),
  );
}

function isCodeLike(s: string): boolean {
  if (/=>|<\/?[A-Za-z]/.test(s)) return true;
  if (/^\s*(export|import|return|function|const|let|var)\b/.test(s)) return true;
  if (/\b(function|const|let|var)\s+[A-Za-z_$]/.test(s)) return true;
  if (/\bimport\s+[\w{*]|from\s+["']/.test(s)) return true;
  if (/^\s*(\/\/|\/\*|#|\$ )/.test(s)) return true;
  if (/\bcurl\s+-/.test(s)) return true;
  if (/SELECT\s+\*\s+FROM\b/i.test(s) || /SELECT\b[\s\S]+\bFROM\b[\s\S]+\bWHERE\b/i.test(s)) return true;
  if (
    /^\s*(SELECT|WITH|INSERT|UPDATE|DELETE)\b/i.test(s) &&
    /\b(WHERE|JOIN|GROUP\s+BY|ORDER\s+BY|LIMIT|INTO)\b/i.test(s)
  ) {
    return true;
  }
  const punct = (s.match(/[{}[\]();=]/g) || []).length;
  if (punct >= 3 && (s.includes("\n") || punct >= 6)) return true;
  return false;
}

const UI_SINGLE_WORDS = new Set([
  "clear",
  "save",
  "delete",
  "cancel",
  "next",
  "back",
  "done",
  "close",
  "edit",
  "add",
  "remove",
  "create",
  "update",
  "submit",
  "reset",
  "copy",
  "search",
  "select",
  "loading",
  "yes",
  "no",
  "ok",
  "okay",
  "apply",
  "confirm",
  "continue",
  "finish",
  "retry",
  "download",
  "upload",
  "preview",
  "test",
  "run",
  "stop",
  "hide",
  "show",
  "more",
  "less",
  "open",
  "refresh",
  "export",
  "import",
  "enable",
  "disable",
  "archive",
  "restore",
  "undo",
  "skip",
  "pause",
  "resume",
  "team",
  "teams",
  "name",
  "settings",
  "overview",
  "details",
  "actions",
  "status",
  "filter",
  "filters",
  "help",
  "docs",
  "home",
  "logout",
  "login",
  "system",
  "user",
  "assistant",
]);

const TECHNICAL_LATIN_TOKENS = new Set(
  [
    "api",
    "json",
    "http",
    "https",
    "sse",
    "jwt",
    "aws",
    "openai",
    "github",
    "litellm",
    "xhub",
    "xgo",
    "go",
    "re2",
    "fal",
    "post",
    "get",
    "put",
    "patch",
    "delete",
    "head",
    "options",
    "key",
    "model",
    "metadata",
    "texts",
    "replacement",
    "success",
    "error",
    "body",
    "headers",
    "timeout",
    "tokens",
    "token",
    "flagged",
    "lakera",
    "hate",
    "selfharm",
    "sexual",
    "violence",
    "applyguardrail",
    "guardrail_intervened",
    "analyzer",
    "anonymizer",
    "detect-secrets",
    "modelink",
    "fal",
    "id",
    "null",
    "nan",
    "infinity",
    "number",
    "any",
    "mib",
    "rpm",
    "tpm",
    "qiniu",
    "volcengine",
    "simple-shuffle",
    "traffic-split",
    "bypass",
  ],
);

/**
 * 判断中英文混排文本中的拉丁字符是否全部属于可证明的技术标识符。
 * 参数：text 为已折叠空白并解码实体的候选界面文案。
 * 返回：仅当文本包含汉字、包含拉丁片段，且每个片段都是白名单技术词、代码形态或数值单位时返回 true。
 * 调用场景：isEnglishProse 用它排除“调用 API，返回 status_code”一类中文技术说明。自然英文词只要有一个
 * 无法证明为技术标识符，就继续按未翻译文案报告；因此“Please retry 中文”不会被跳过。
 */
function hasOnlyTechnicalLatinSegments(text: string): boolean {
  if (!/[\u3400-\u9fff]/u.test(text)) return false;
  // JSX/模板表达式使用花括号占位；占位名不是用户可见英文，不能让中文技术说明产生误报。
  const visibleText = text.replace(/\{[A-Za-z_][A-Za-z0-9_]*\}/g, "");
  const segments = visibleText.match(/(?:\/[A-Za-z0-9._/-]+|\d+[A-Za-z]+|[A-Za-z][A-Za-z0-9_.:/-]*)/g) ?? [];
  if (segments.length === 0) return false;
  return segments.every((segment) => {
    const normalized = segment.replace(/[.:]+$/g, "").toLowerCase();
    if (TECHNICAL_LATIN_TOKENS.has(normalized)) return true;
    if (/^[A-Za-z][A-Za-z0-9]*_[A-Za-z0-9_]+$/.test(segment)) return true;
    if (/^\/[A-Za-z0-9._-]+$/.test(segment)) return true;
    if (/^\/?[A-Za-z0-9._-]+(?:\/[A-Za-z0-9._-]+)+$/.test(segment)) return true;
    if (/^[A-Za-z]+-\d+(?:-\d+)*$/.test(segment)) return true;
    if (/^\d+(?:xx|[kmgt])$/i.test(segment)) return true;
    return false;
  });
}

/**
 * 判断字符串是否像应接入翻译目录的英文界面文案。
 * 参数：raw 为源码字符串；singleWordUi 表示调用位置可展示单词标签，anySingleWord 表示任意英文单词都应扫描。
 * 返回：发现自然英文界面文本时返回 true；URL、代码、样式、单位及可证明的技术标识符返回 false。
 * 调用场景：源码遍历器按 JSX、提示、错误和可见属性的上下文启用不同单词检测强度。函数不负责判断目录是否已有键。
 */
export function isEnglishProse(raw: string, opts?: { singleWordUi?: boolean; anySingleWord?: boolean }): boolean {
  const s = collapse(decodeEntities(raw));
  if (!s || s.length > 2000) return false;
  if (!/[A-Za-z]/.test(s.replace(/\{[A-Za-z0-9_]+\}/g, ""))) return false;
  if (hasOnlyTechnicalLatinSegments(s)) return false;
  // 品牌、协议、语言和计价单位可原样展示；这里只接受完整的已知技术标签，避免把含这些词的英文句子放过。
  if (/^(?:XHub API|Vertex \/ Gemini|Claude \/ Anthropic|Python|JavaScript|cURL|OpenAI|Fal)$/i.test(s)) return false;
  if (/^\{[A-Za-z0-9_]+\}\s*[·•-]?\s*USD$/i.test(s)) return false;
  if (/^e\.g\.\s+[a-z0-9_.:/-]+$/i.test(s)) return false;
  if (/^HTTP\s+\{[A-Za-z0-9_]+\}$/i.test(s)) return false;
  if (/^https?:\/\//i.test(s) || /^www\./i.test(s)) return false;
  // 只放过环境变量形态；普通大写词和短句仍可能是按钮或状态文案，必须进入扫描。
  if (/^[A-Z][A-Z0-9]*_[A-Z0-9_]+$/.test(s)) return false;
  if (isCssLike(s)) return false;
  if (isCodeLike(s)) return false;
  const words = s.split(/[^A-Za-z]+/).filter((w) => w.length >= 2);
  if (words.length >= 2) {
    const hyphenLabel = /^[A-Z][A-Za-z]+(-[A-Za-z]+)+$/.test(s.replace(/[.!?…]+$/, ""));
    if (!/\s/.test(s) && !hyphenLabel) return false;
    return true;
  }
  const token = s.replace(/[.!?…]+$/g, "").trim();
  const bare = token.replace(/^[^A-Za-z]+|[^A-Za-z]+$/g, "");
  if (opts?.anySingleWord && /^[A-Za-z][A-Za-z'-]{2,}$/.test(bare) && /[a-z]/.test(bare)) return true;
  if (!opts?.singleWordUi) return false;
  if (UI_SINGLE_WORDS.has(bare.toLowerCase())) return true;
  if (/^[A-Z][a-z]{2,}(?:-[A-Za-z]+)*$/.test(bare)) return true;
  if (/^[a-z]{2,}$/.test(bare) && !UNIT_WORDS.has(bare)) return true;
  return false;
}

const UNIT_WORDS = new Set(["ms", "px", "em", "rem", "kb", "mb", "gb", "tb", "ns"]);

function tagName(node: ts.Node): string {
  const el = ts.isJsxElement(node)
    ? node.openingElement.tagName
    : ts.isJsxSelfClosingElement(node)
      ? node.tagName
      : null;
  if (!el) return "";
  if (ts.isIdentifier(el)) return el.text;
  if (ts.isPropertyAccessExpression(el)) return el.name.text;
  return "";
}

function jsxTagOfAttribute(node: ts.Node): ts.Node | undefined {
  let p: ts.Node | undefined = node.parent;
  if (p && ts.isJsxAttribute(p)) p = p.parent;
  if (p && ts.isJsxAttributes(p)) p = p.parent;
  return p;
}

function inCodeElement(node: ts.Node): boolean {
  const attrTag = jsxTagOfAttribute(node);
  let p: ts.Node | undefined = node.parent;
  while (p) {
    if (ts.isJsxElement(p) || ts.isJsxSelfClosingElement(p)) {
      if (CODE_TAGS.has(tagName(p))) {
        const owner = ts.isJsxElement(p) ? p.openingElement : p;
        if (attrTag === owner) return false;
        return true;
      }
    }
    p = p.parent;
  }
  return false;
}

function inConsole(node: ts.Node): boolean {
  let p: ts.Node | undefined = node.parent;
  while (p) {
    if (ts.isCallExpression(p) && ts.isPropertyAccessExpression(p.expression)) {
      const obj = p.expression.expression;
      const method = p.expression.name.text;
      if (
        ts.isIdentifier(obj) &&
        obj.text === "console" &&
        (method === "log" || method === "debug" || method === "info" || method === "warn" || method === "error" || method === "trace")
      ) {
        return true;
      }
    }
    p = p.parent;
  }
  return false;
}

function inClassNameCall(node: ts.Node): boolean {
  let p: ts.Node | undefined = node.parent;
  while (p) {
    if (ts.isCallExpression(p)) {
      const expr = p.expression;
      const name = ts.isIdentifier(expr) ? expr.text : ts.isPropertyAccessExpression(expr) ? expr.name.text : "";
      if (CLASSNAME_FNS.has(name)) return true;
    }
    if (ts.isJsxAttribute(p)) break;
    p = p.parent;
  }
  return false;
}

function inImport(node: ts.Node): boolean {
  let p: ts.Node | undefined = node;
  while (p) {
    if (ts.isImportDeclaration(p) || ts.isExportDeclaration(p)) return true;
    p = p.parent;
  }
  return false;
}

/**
 * 判断字符串节点是否作为翻译 helper 的直接首参。
 * 参数 node 为候选字符串或其表达式子节点；返回是否由 t/translate/tRuntime 等明确消费，供源码扫描跳过目录键。
 * 遍历在函数和 JSX 边界停止，非首参立即返回 false，避免把同一调用中的默认英文或相邻文案误判为已翻译。
 */
function isDirectTranslationKey(node: ts.Node): boolean {
  let child: ts.Node = node;
  let parent: ts.Node | undefined = node.parent;
  while (parent) {
    if (ts.isCallExpression(parent)) {
      if (parent.arguments[0] !== child) return false;
      const expr = parent.expression;
      const name = ts.isIdentifier(expr) ? expr.text : ts.isPropertyAccessExpression(expr) ? expr.name.text : "";
      return TRANSLATION_CALLEES.has(name);
    }
    if (isFunctionBoundary(parent) || ts.isJsxElement(parent) || ts.isJsxFragment(parent)) return false;
    child = parent;
    parent = parent.parent;
  }
  return false;
}

/**
 * 判断字符串是否属于外部护栏的本地化配置目录。
 * 参数：node 为字符串节点，file 为当前源码路径。
 * 返回：仅对 externalProviders.ts 中 EXTERNAL_PROVIDERS 的 label/description 值返回 true。
 * 调用场景：这些值由 ExternalGuardrailEditor 和 guardrail_garden 统一交给 externalGuardrailText；该 helper 内部调用 t，
 * 同时保留 LiteLLM 品牌占位符。规则限定文件、常量和属性名，避免把普通对象里的英文展示字段误当成已翻译。
 */
function isExternalGuardrailCatalogValue(node: ts.Node, file: string): boolean {
  if (path.basename(file) !== "externalProviders.ts") return false;
  const property = node.parent;
  if (!property || !ts.isPropertyAssignment(property) || property.initializer !== node) return false;
  if (!new Set(["label", "description"]).has(propNameOf(property.name))) return false;
  let current: ts.Node | undefined = property;
  while (current) {
    if (ts.isVariableDeclaration(current) && ts.isIdentifier(current.name)) {
      return current.name.text === "EXTERNAL_PROVIDERS";
    }
    current = current.parent;
  }
  return false;
}

/**
 * 判断字符串是否是已由调用方逐项翻译的固定目录键。
 * 参数：node 为目录字符串节点，file 为声明文件；返回值仅覆盖三个明确的目录/状态函数及其 label 字段。
 * 调用场景：日志筛选器会对筛选目录执行 t/translate，状态函数的返回键由表格和抽屉执行 t。
 * 边界：文件、声明名和字段名必须同时匹配，普通 label 对象仍会报告，避免按属性名宽泛跳过。
 */
function isTranslatedCatalogValue(node: ts.Node, file: string): boolean {
  const property = node.parent;
  if (!property || !ts.isPropertyAssignment(property) || property.initializer !== node || propNameOf(property.name) !== "label") {
    return false;
  }
  const base = path.basename(file);
  let current: ts.Node | undefined = property;
  while (current) {
    if (ts.isVariableDeclaration(current) && ts.isIdentifier(current.name)) {
      return (
        base === "RequestLogsFilters.tsx" &&
        (current.name.text === "STATUS_FILTER_KEYS" || current.name.text === "CACHE_FILTER_KEYS")
      ) || (base === "constants.ts" && current.name.text === "ERROR_CODE_OPTIONS");
    }
    if (ts.isFunctionDeclaration(current) && current.name) {
      return base === "taskStatus.ts" && current.name.text === "requestLogStatus";
    }
    current = current.parent;
  }
  return false;
}

/**
 * 判断 JSX 内联 map 的字符串如何被回调消费。
 * 参数 node 为字符串节点；返回 translated 表示参数进入明确翻译 helper 首参，rendered 表示参数直接展示，其余返回 null。
 * 调用场景：定价模态和动态翻译页签用小写值拼接翻译键；扫描器只跳过 translated。
 * 边界：普通比较、属性、toast 或其他函数调用都不足以证明已翻译；["pricing"].map(x => <span>{x}</span>) 必须继续命中。
 */
function mappedTokenUsage(node: ts.Node): "translated" | "rendered" | null {
  const array = node.parent;
  if (!array || !ts.isArrayLiteralExpression(array)) return null;
  let owner: ts.Expression = array;
  if (ts.isAsExpression(array.parent) && array.parent.expression === array) owner = array.parent;
  if (ts.isParenthesizedExpression(owner.parent) && owner.parent.expression === owner) owner = owner.parent;
  const access = owner.parent;
  if (!access || !ts.isPropertyAccessExpression(access) || access.expression !== owner || access.name.text !== "map") return null;
  const call = access.parent;
  if (!call || !ts.isCallExpression(call) || call.expression !== access) return null;
  const callback = call.arguments[0];
  if (!callback || (!ts.isArrowFunction(callback) && !ts.isFunctionExpression(callback)) || callback.parameters.length === 0) return null;
  const names = new Set<string>();
  /** 收集 map 首参的标识符；参数可为标识符或解构绑定，无返回值，仅服务当前回调分析。 */
  const collectNames = (name: ts.BindingName) => {
    if (ts.isIdentifier(name)) names.add(name.text);
    else name.elements.forEach((element) => { if (ts.isBindingElement(element)) collectNames(element.name); });
  };
  collectNames(callback.parameters[0].name);
  let directlyRendered = false;
  let translated = false;
  /** 遍历回调正文；参数为当前 AST 节点，无返回值，记录直接展示或翻译首参消费，其他调用不构成排除证据。 */
  const inspect = (candidate: ts.Node) => {
    if (
      ts.isJsxExpression(candidate) &&
      !ts.isJsxAttribute(candidate.parent) &&
      candidate.expression &&
      ts.isIdentifier(candidate.expression) &&
      names.has(candidate.expression.text)
    ) {
      directlyRendered = true;
      return;
    }
    if (ts.isCallExpression(candidate)) {
      const callee = ts.isIdentifier(candidate.expression)
        ? candidate.expression.text
        : ts.isPropertyAccessExpression(candidate.expression)
          ? candidate.expression.name.text
          : "";
      if (TRANSLATION_CALLEES.has(callee) && candidate.arguments[0]) {
        const first = candidate.arguments[0];
        /** 判断翻译首参是否引用 map 参数；参数为表达式节点，返回布尔值，只在当前翻译调用内部递归。 */
        const usesMappedName = (part: ts.Node): boolean =>
          (ts.isIdentifier(part) && names.has(part.text)) || part.getChildren().some(usesMappedName);
        if (usesMappedName(first)) translated = true;
      }
    }
    ts.forEachChild(candidate, inspect);
  };
  inspect(callback.body);
  if (directlyRendered) return "rendered";
  return translated ? "translated" : null;
}

function calleeName(expr: ts.Expression): { obj: string; method: string } {
  if (ts.isIdentifier(expr)) return { obj: expr.text, method: "" };
  if (ts.isPropertyAccessExpression(expr)) {
    const obj = ts.isIdentifier(expr.expression) ? expr.expression.text : "";
    return { obj, method: expr.name.text };
  }
  return { obj: "", method: "" };
}

function attrNameOf(attr: ts.JsxAttribute): string {
  const n = attr.name;
  if (ts.isIdentifier(n)) return n.text;
  return `${n.namespace.text}:${n.name.text}`;
}

function propNameOf(name: ts.PropertyName): string {
  if (ts.isIdentifier(name) || ts.isStringLiteral(name) || ts.isNumericLiteral(name)) return name.text;
  return "";
}

function isChatContent(node: ts.Node): boolean {
  const parent = node.parent;
  if (!parent || !ts.isPropertyAssignment(parent) || parent.initializer !== node) return false;
  if (propNameOf(parent.name) !== "content") return false;
  const obj = parent.parent;
  if (!obj || !ts.isObjectLiteralExpression(obj)) return false;
  return obj.properties.some((p) => {
    if (!ts.isPropertyAssignment(p)) return false;
    if (propNameOf(p.name) !== "role") return false;
    return ts.isStringLiteral(p.initializer) && /^(user|system|assistant|tool|developer)$/.test(p.initializer.text);
  });
}

function containsJsx(node: ts.Node): boolean {
  let found = false;
  const visit = (n: ts.Node) => {
    if (found) return;
    if (ts.isJsxElement(n) || ts.isJsxFragment(n) || ts.isJsxSelfClosingElement(n)) {
      found = true;
      return;
    }
    ts.forEachChild(n, visit);
  };
  visit(node);
  return found;
}

function placeholderName(expr: ts.Expression, index: number, used: Set<string>): string {
  if (ts.isIdentifier(expr) && /^[A-Za-z_][A-Za-z0-9_]*$/.test(expr.text) && !used.has(expr.text)) {
    used.add(expr.text);
    return expr.text;
  }
  let name = `value${index}`;
  let n = index;
  while (used.has(name)) {
    n += 1;
    name = `value${n}`;
  }
  used.add(name);
  return name;
}

function fromTemplate(node: ts.TemplateExpression | ts.NoSubstitutionTemplateLiteral | ts.StringLiteral, sf: ts.SourceFile): {
  template: string;
  vars: { name: string; expr: string }[];
  probe: string;
} {
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
    const template = collapse(decodeEntities(node.text));
    return { template, vars: [], probe: template };
  }
  const used = new Set<string>();
  let template = node.head.text;
  let probe = node.head.text;
  const vars: { name: string; expr: string }[] = [];
  node.templateSpans.forEach((span, i) => {
    const name = placeholderName(span.expression, i, used);
    vars.push({ name, expr: span.expression.getText(sf) });
    template += `{${name}}` + span.literal.text;
    probe += " VALUE " + span.literal.text;
  });
  return { template: collapse(decodeEntities(template)), vars, probe: collapse(decodeEntities(probe)) };
}

function isStableTokenOperand(node: ts.Node): boolean {
  const parent = node.parent;
  if (!parent) return false;
  if (ts.isCaseClause(parent) && parent.expression === node) return true;
  if (ts.isBinaryExpression(parent) && (parent.left === node || parent.right === node)) {
    const op = parent.operatorToken.kind;
    return (
      op === ts.SyntaxKind.EqualsEqualsEqualsToken ||
      op === ts.SyntaxKind.ExclamationEqualsEqualsToken ||
      op === ts.SyntaxKind.EqualsEqualsToken ||
      op === ts.SyntaxKind.ExclamationEqualsToken
    );
  }
  return false;
}

function skipNode(node: ts.Node): boolean {
  return (
    inCodeElement(node) ||
    inConsole(node) ||
    inClassNameCall(node) ||
    inImport(node) ||
    isDirectTranslationKey(node) ||
    isChatContent(node) ||
    isStableTokenOperand(node)
  );
}

function technicalAttrContext(node: ts.Node): boolean {
  let p: ts.Node | undefined = node.parent;
  while (p) {
    if (ts.isJsxAttribute(p)) {
      const name = attrNameOf(p);
      if (name.startsWith("data-") || TECHNICAL_ATTRS.has(name)) return true;
      if (name.startsWith("on")) return false;
      return false;
    }
    if (ts.isJsxElement(p) || ts.isJsxFragment(p)) return false;
    p = p.parent;
  }
  return false;
}

export function findHitsInSource(file: string, text: string): ProseHit[] {
  const sf = ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true, file.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
  const hits: ProseHit[] = [];
  const covered = new Set<ts.Node>();

  const push = (
    node: ts.Node,
    start: number,
    end: number,
    kind: string,
    template: string,
    vars: ProseHit["vars"],
    wrapBraces: boolean,
    singleWordUi = false,
  ) => {
    if (!template || !isEnglishProse(template, { singleWordUi, anySingleWord: kind === "title-list" })) return;
    if (start >= end) return;
    const { line } = sf.getLineAndCharacterOfPosition(start);
    hits.push({
      file: path.relative(SRC_ROOT, file),
      line: line + 1,
      start,
      end,
      kind,
      template,
      vars,
      wrapBraces,
    });
  };

  const visitJsxChildren = (children: ts.NodeArray<ts.JsxChild> | readonly ts.JsxChild[]) => {
    const significant = children.filter((c) => !(ts.isJsxText(c) && !c.getText(sf).trim()));
    if (significant.length === 0) return;
    const onlyInline = significant.every((c) => {
      if (ts.isJsxText(c)) return true;
      if (ts.isJsxExpression(c) && c.expression && !containsJsx(c.expression)) return true;
      return false;
    });
    if (!onlyInline || significant.length < 1) return;
    if (significant.some((c) => skipNode(c))) return;

    const used = new Set<string>();
    let template = "";
    let probe = "";
    const vars: ProseHit["vars"] = [];
    let vi = 0;
    for (const c of significant) {
      if (ts.isJsxText(c)) {
        const piece = c.getText(sf);
        template += piece;
        probe += piece;
      } else if (ts.isJsxExpression(c) && c.expression) {
        const expr = c.expression;
        if (ts.isStringLiteral(expr) || ts.isNoSubstitutionTemplateLiteral(expr)) {
          template += expr.text;
          probe += expr.text;
        } else {
          const name = placeholderName(expr, vi, used);
          vi += 1;
          vars.push({ name, expr: expr.getText(sf) });
          template += `{${name}}`;
          probe += " VALUE ";
        }
      }
    }
    const collapsed = collapse(decodeEntities(template));
    const collapsedProbe = collapse(decodeEntities(probe));
    if (!isEnglishProse(collapsedProbe, { singleWordUi: true }) && !isEnglishProse(collapsed, { singleWordUi: true })) return;

    const first = significant[0];
    const last = significant[significant.length - 1];
    let start = first.getStart(sf);
    let end = last.getEnd();
    if (significant.length === 1 && ts.isJsxText(first)) {
      const raw = first.getText(sf);
      const lead = raw.length - raw.trimStart().length;
      const trail = raw.length - raw.trimEnd().length;
      start = first.getStart(sf) + lead;
      end = first.getEnd() - trail;
    }
    significant.forEach((c) => covered.add(c));
    push(first, start, end, "jsx-children", collapsed, vars, false, true);
  };

  const visit = (node: ts.Node) => {
    if (ts.isJsxElement(node)) visitJsxChildren(node.children);
    else if (ts.isJsxFragment(node)) visitJsxChildren(node.children);

    if (!covered.has(node) && ts.isJsxText(node) && !skipNode(node)) {
      const raw = node.getText(sf);
      const lead = raw.length - raw.trimStart().length;
      const trail = raw.length - raw.trimEnd().length;
      const inner = raw.slice(lead, raw.length - trail);
      push(node, node.getStart(sf) + lead, node.getEnd() - trail, "jsx-text", collapse(decodeEntities(inner)), [], false, true);
    }

    const mappedUsage = mappedTokenUsage(node);
    if ((ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node) || ts.isTemplateExpression(node)) && !skipNode(node) && !technicalAttrContext(node) && !isExternalGuardrailCatalogValue(node, file) && !isTranslatedCatalogValue(node, file) && mappedUsage !== "translated") {
      const built = ts.isTemplateExpression(node) ? fromTemplate(node, sf) : fromTemplate(node as ts.StringLiteral, sf);
      const parent = node.parent;
      const asAttr = !!parent && ts.isJsxAttribute(parent) && parent.initializer === node;
      const asPropName = !!parent && ts.isPropertyAssignment(parent) && parent.name === node;
      const asDefault = isUiDefault(node);
      const asTitle = isVisibleTitleList(node) || mappedUsage === "rendered";
      const inHandler = isInsideEventHandler(node);
      const inJsx = !inHandler && (asAttr || isInsideJsx(node));
      const asProp = !!parent && ts.isPropertyAssignment(parent) && parent.initializer === node && USER_FACING_PROPS.has(propNameOf(parent.name));
      const asToast = isToastArg(node);
      const asZod = isZodMessage(node);
      const asError = isErrorMessage(node) || inSetErrorArgument(node) || isUiErrorBinding(node);
      const renderedChild = isRenderedJsxChild(node);
      const facingAttr = userFacingAttributeAncestor(node);
      const asVisibleProp = asProp && propNameOf((parent as ts.PropertyAssignment).name) !== "text";
      const singleWordUi = asAttr || asDefault || asTitle || renderedChild || facingAttr || asToast || asError || asVisibleProp;
      const probe = /[\u3400-\u9fff]/u.test(built.template) ? built.template : built.probe;
      const prose = isEnglishProse(probe, { singleWordUi, anySingleWord: asTitle });
      if (prose) {
        if (inJsx || asProp || asToast || asZod || asError || asDefault || asTitle) {
          if (!(asAttr && !userFacingAttr(parent as ts.JsxAttribute) && !isInsideJsxExpression(node))) {
            const allowAttr = !asAttr || userFacingAttr(parent as ts.JsxAttribute);
            if (allowAttr || !asAttr) {
              const kind = asPropName
                ? "prop-name"
                : asDefault
                  ? "default"
                  : asTitle
                    ? "title-list"
                  : asAttr
                    ? `attr:${attrNameOf(parent as ts.JsxAttribute)}`
                    : asToast
                      ? "toast"
                      : asZod
                        ? "zod"
                        : asError
                          ? "error"
                          : asProp
                            ? `prop:${propNameOf((parent as ts.PropertyAssignment).name)}`
                            : "jsx-string";
              push(node, node.getStart(sf), node.getEnd(), kind, built.template, built.vars, asAttr, singleWordUi);
            }
          }
        }
      }
    }

    ts.forEachChild(node, visit);
  };

  visit(sf);

  const filtered = hits.filter((h) => !hits.some((o) => o !== h && o.start <= h.start && o.end >= h.end && o.end - o.start > h.end - h.start));
  filtered.sort((a, b) => a.start - b.start);
  return filtered;
}

function userFacingAttr(attr: ts.JsxAttribute): boolean {
  const name = attrNameOf(attr);
  if (name.startsWith("aria-")) {
    return name === "aria-label" || name === "aria-description" || name === "aria-placeholder" || name === "aria-roledescription" || name === "aria-valuetext";
  }
  return USER_FACING_ATTRS.has(name);
}

function isInsideJsx(node: ts.Node): boolean {
  let p: ts.Node | undefined = node.parent;
  while (p) {
    if (ts.isJsxElement(p) || ts.isJsxFragment(p) || ts.isJsxExpression(p) || ts.isJsxAttribute(p) || ts.isJsxSelfClosingElement(p)) return true;
    p = p.parent;
  }
  return false;
}

function isFunctionBoundary(node: ts.Node): boolean {
  return (
    ts.isFunctionDeclaration(node) ||
    ts.isFunctionExpression(node) ||
    ts.isArrowFunction(node) ||
    ts.isMethodDeclaration(node) ||
    ts.isConstructorDeclaration(node)
  );
}

function isInsideCallArgument(node: ts.Node): boolean {
  let current: ts.Node | undefined = node;
  while (current) {
    const parent: ts.Node | undefined = current.parent;
    if (!parent) return false;
    if (ts.isCallExpression(parent) && parent.arguments.some((arg) => arg === current)) return true;
    if (ts.isJsxExpression(parent) || ts.isJsxElement(parent) || ts.isJsxFragment(parent) || isFunctionBoundary(parent)) return false;
    current = parent;
  }
  return false;
}

function isRenderedJsxChild(node: ts.Node): boolean {
  if (isInsideCallArgument(node)) return false;
  let p: ts.Node | undefined = node.parent;
  while (p) {
    if (ts.isJsxAttribute(p) || isFunctionBoundary(p)) return false;
    if (ts.isJsxElement(p) || ts.isJsxFragment(p)) return true;
    p = p.parent;
  }
  return false;
}

function userFacingAttributeAncestor(node: ts.Node): boolean {
  let p: ts.Node | undefined = node.parent;
  while (p) {
    if (ts.isJsxAttribute(p)) return userFacingAttr(p);
    if (ts.isJsxElement(p) || ts.isJsxFragment(p)) return false;
    p = p.parent;
  }
  return false;
}

function isInsideJsxExpression(node: ts.Node): boolean {
  let p: ts.Node | undefined = node.parent;
  while (p) {
    if (ts.isJsxExpression(p)) return true;
    if (ts.isJsxAttribute(p)) return false;
    p = p.parent;
  }
  return false;
}

function isInsideEventHandler(node: ts.Node): boolean {
  let p: ts.Node | undefined = node.parent;
  while (p) {
    if (ts.isJsxAttribute(p)) return attrNameOf(p).startsWith("on");
    if (ts.isJsxElement(p) || ts.isJsxFragment(p)) return false;
    p = p.parent;
  }
  return false;
}

function isToastArg(node: ts.Node): boolean {
  let current: ts.Node | undefined = node;
  while (current) {
    const parent: ts.Node | undefined = current.parent;
    if (!parent) break;
    if (ts.isCallExpression(parent) && parent.arguments.length > 0) {
      const arg = parent.arguments[0];
      const { obj, method } = calleeName(parent.expression);
      const toastCall = obj === "toast" || (method === "fromError" && obj === "toast");
      if (toastCall && arg && node.getStart() >= arg.getStart() && node.getEnd() <= arg.getEnd()) return true;
    }
    if (
      ts.isFunctionDeclaration(parent) ||
      ts.isFunctionExpression(parent) ||
      ts.isArrowFunction(parent) ||
      ts.isMethodDeclaration(parent)
    ) {
      break;
    }
    current = parent;
  }
  const parent = node.parent;
  if (parent && ts.isPropertyAssignment(parent) && parent.initializer === node && ts.isObjectLiteralExpression(parent.parent)) {
    const pname = propNameOf(parent.name);
    if (pname !== "description" && pname !== "title") return false;
    const obj = parent.parent;
    const call = obj.parent;
    if (!call || !ts.isCallExpression(call)) return false;
    const { obj: cobj } = calleeName(call.expression);
    return cobj === "toast";
  }
  return false;
}

const ZOD_METHODS = new Set(["min", "max", "length", "regex", "email", "url", "uuid", "includes", "startsWith", "endsWith", "nonempty", "refine"]);

function isZodMessage(node: ts.Node): boolean {
  const parent = node.parent;
  if (!parent) return false;
  if (ts.isCallExpression(parent) && ts.isPropertyAccessExpression(parent.expression)) {
    const method = parent.expression.name.text;
    if (!ZOD_METHODS.has(method)) return false;
    if (method === "refine") {
      return parent.arguments[1] === node;
    }
    return parent.arguments[1] === node;
  }
  if (ts.isPropertyAssignment(parent) && parent.initializer === node && propNameOf(parent.name) === "message") {
    let p: ts.Node | undefined = parent.parent;
    while (p) {
      if (ts.isCallExpression(p) && ts.isPropertyAccessExpression(p.expression) && ZOD_METHODS.has(p.expression.name.text)) return true;
      p = p.parent;
    }
  }
  return false;
}

const UI_DEFAULT_NAMES = new Set([
  ...USER_FACING_ATTRS,
  "labelText",
  "clearAllLabel",
  "loadingText",
  "noOptionsText",
  "emptyText",
  "placeholder",
]);

function isVisibleTitleList(node: ts.Node): boolean {
  const arr = node.parent;
  if (!arr || !ts.isArrayLiteralExpression(arr) || !arr.elements.includes(node as ts.Expression)) return false;
  let init: ts.Node = arr;
  if (ts.isAsExpression(arr.parent) || ts.isSatisfiesExpression(arr.parent)) init = arr.parent;
  const decl = init.parent;
  if (!decl || !ts.isVariableDeclaration(decl) || decl.initializer !== init || !ts.isIdentifier(decl.name)) return false;
  return /title|label|heading|step/i.test(decl.name.text);
}

function isUiDefault(node: ts.Node): boolean {
  const parent = node.parent;
  if (!parent || !ts.isBindingElement(parent) || parent.initializer !== node) return false;
  return ts.isIdentifier(parent.name) && UI_DEFAULT_NAMES.has(parent.name.text);
}

function enclosingFunction(node: ts.Node): ts.Node | undefined {
  let p: ts.Node | undefined = node.parent;
  while (p) {
    if (
      ts.isFunctionDeclaration(p) ||
      ts.isFunctionExpression(p) ||
      ts.isArrowFunction(p) ||
      ts.isMethodDeclaration(p) ||
      ts.isConstructorDeclaration(p)
    ) {
      return p;
    }
    p = p.parent;
  }
  return undefined;
}

function inSetErrorArgument(node: ts.Node): boolean {
  let p: ts.Node | undefined = node.parent;
  while (p) {
    if (ts.isCallExpression(p) && p.arguments.length > 0) {
      const arg = p.arguments[0];
      const expr = p.expression;
      const name = ts.isIdentifier(expr) ? expr.text : ts.isPropertyAccessExpression(expr) ? expr.name.text : "";
      if (name === "setError" && arg && node.getStart() >= arg.getStart() && node.getEnd() <= arg.getEnd()) return true;
    }
    if (enclosingFunction(p) && p === enclosingFunction(node)) break;
    p = p.parent;
  }
  return false;
}

function isUiErrorBinding(node: ts.Node): boolean {
  let name: string | null = null;
  let owner: ts.Node | undefined;
  let current: ts.Node | undefined = node;
  while (current) {
    const parent: ts.Node | undefined = current.parent;
    if (!parent) break;
    if (ts.isVariableDeclaration(parent) && ts.isIdentifier(parent.name) && /^(message|errorMessage|errMsg|errorText)$/.test(parent.name.text)) {
      const init = parent.initializer;
      if (init && node.getStart() >= init.getStart() && node.getEnd() <= init.getEnd()) {
        name = parent.name.text;
        owner = parent;
        break;
      }
    }
    if (
      ts.isBinaryExpression(parent) &&
      parent.operatorToken.kind === ts.SyntaxKind.EqualsToken &&
      ts.isIdentifier(parent.left) &&
      /^(message|errorMessage|errMsg|errorText)$/.test(parent.left.text) &&
      node.getStart() >= parent.right.getStart() &&
      node.getEnd() <= parent.right.getEnd()
    ) {
      name = parent.left.text;
      owner = parent;
      break;
    }
    if (
      ts.isFunctionDeclaration(parent) ||
      ts.isFunctionExpression(parent) ||
      ts.isArrowFunction(parent) ||
      ts.isMethodDeclaration(parent)
    ) {
      break;
    }
    current = parent;
  }
  if (!name || !owner) return false;
  const fn = enclosingFunction(owner);
  if (!fn) return false;
  let used = false;
  const visit = (n: ts.Node) => {
    if (used) return;
    const arg0 = ts.isCallExpression(n) || ts.isNewExpression(n) ? n.arguments?.[0] : undefined;
    if (arg0 && ts.isIdentifier(arg0) && arg0.text === name) {
      if (ts.isNewExpression(n) && ts.isIdentifier(n.expression) && (n.expression.text === "Error" || n.expression.text.endsWith("Error"))) {
        used = true;
      }
      if (ts.isCallExpression(n)) {
        const expr = n.expression;
        const callee = ts.isIdentifier(expr) ? expr.text : ts.isPropertyAccessExpression(expr) ? expr.expression.getText() : "";
        const method = ts.isPropertyAccessExpression(expr) ? expr.name.text : "";
        if (callee === "toast" || method === "setError" || callee === "setError") used = true;
      }
    }
    ts.forEachChild(n, visit);
  };
  visit(fn);
  return used;
}

function isErrorMessage(node: ts.Node): boolean {
  let current: ts.Node | undefined = node;
  while (current) {
    const parent: ts.Node | undefined = current.parent;
    if (!parent) break;
    if (ts.isNewExpression(parent) && parent.arguments && parent.arguments.length > 0) {
      const arg = parent.arguments[0];
      const expr = parent.expression;
      if (
        ts.isIdentifier(expr) &&
        (expr.text === "Error" || expr.text.endsWith("Error")) &&
        node.getStart() >= arg.getStart() &&
        node.getEnd() <= arg.getEnd()
      ) {
        return true;
      }
    }
    if (
      ts.isFunctionDeclaration(parent) ||
      ts.isFunctionExpression(parent) ||
      ts.isArrowFunction(parent) ||
      ts.isMethodDeclaration(parent)
    ) {
      break;
    }
    current = parent;
  }
  return false;
}

export function findUserVisibleEnglish(root = SRC_ROOT): ProseHit[] {
  const files = walkDir(root);
  const all: ProseHit[] = [];
  for (const file of files) {
    const text = fs.readFileSync(file, "utf8");
    all.push(...findHitsInSource(file, text));
  }
  return all;
}

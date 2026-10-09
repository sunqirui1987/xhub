// 本表保存运行时真实注册函数的签名和可执行 XGo 片段。
// 新增 primitive 时同步后端注册、使用说明和编译测试；不要仅增加界面文案。
// xhub/guardrail 是运行时注入的虚拟包；LANGUAGE_PRIMITIVES 单独列出语言内置能力。
export const PRIMITIVE_GROUPS = [
  {
    title: "返回值",
    items: [
      { signature: "Allow()", description: "放行原文，继续下一条护栏。", code: "return Allow()" },
      {
        signature: "Block(reason)",
        description: "拦截请求并返回原因，不调用模型。",
        code: 'return Block("文本包含敏感内容")',
      },
      {
        signature: "Flag(reason, metadata)",
        description: "记录非阻断告警及 metadata，继续请求。",
        code: 'return Flag("需要复核", map[string]any{"category": "review"})',
      },
      {
        signature: "Modify(texts)",
        description: "修改文本后继续，保持数量和顺序。",
        code: 'for i := range texts {\n    texts[i] = "[已隐藏]"\n}\nreturn Modify(texts)',
      },
    ],
  },
  {
    title: "正则函数",
    items: [
      {
        signature: "RegexMatch(text, pattern)",
        description: "使用 Go RE2 判断文本中是否存在匹配。",
        code: 'for text <- texts {\n    if RegexMatch(text, "1[3-9][0-9]{9}") {\n        return Block("检测到手机号")\n    }\n}',
      },
      {
        signature: "RegexReplace(text, pattern, replacement)",
        description: "替换全部匹配，replacement 为字面量。",
        code: 'for i, text := range texts {\n    texts[i] = RegexReplace(text, "1[3-9][0-9]{9}", "[手机号已隐藏]")\n}\nreturn Modify(texts)',
      },
    ],
  },
  {
    title: "文本函数",
    items: [
      {
        signature: "Contains(text, substring)",
        description: "判断文本是否包含指定字符串，区分大小写。",
        code: 'for text <- texts {\n    if Contains(text, "secret") {\n        return Block("检测到敏感词")\n    }\n}',
      },
      {
        signature: "Lower(text)",
        description: "将文本转换为小写。",
        code: 'for text <- texts {\n    if Contains(Lower(text), "secret") {\n        return Block("检测到敏感词")\n    }\n}',
      },
      {
        signature: "Trim(text)",
        description: "移除文本两端的空白字符。",
        code: "for i, text := range texts {\n    texts[i] = Trim(text)\n}\nreturn Modify(texts)",
      },
    ],
  },
  {
    title: "HTTP 请求",
    items: [
      {
        signature: "HTTPGet(url, headers, timeout)",
        description: "发送 GET，返回 success、status_code、body、headers、error。",
        code: 'response := HTTPGet("https://example.com/check", nil, 5)\nif response["success"] != true {\n    return Block("审核服务不可用")\n}',
      },
      {
        signature: "HTTPPost(url, body, headers, timeout)",
        description: "发送 JSON POST，timeout 单位为秒，上限 10 秒。",
        code: 'response := HTTPPost("https://example.com/check", map[string]any{"texts": texts}, map[string]string{"Authorization": "Bearer os.environ/XHUB_GUARDRAIL_API_KEY"}, 5)\nif response["success"] != true {\n    return Block("审核服务不可用")\n}\nbody, ok := response["body"].(map[string]any)\nif !ok { return Block("审核结果格式错误") }\nif body["flagged"] == true { return Block("审核未通过") }',
      },
      {
        signature: "HTTPRequest(url, method, headers, body, timeout)",
        description: "支持 GET、POST、PUT、PATCH、DELETE、HEAD、OPTIONS。",
        code: 'response := HTTPRequest("https://example.com/check", "POST", nil, map[string]any{"texts": texts}, 5)\nif response["success"] != true { return Block("审核服务不可用") }',
      },
    ],
  },
  {
    title: "JSON 函数",
    items: [
      {
        signature: "JSONParse(text)",
        description: "解析 JSON，返回 any；无效 JSON 会产生执行错误。",
        code: 'value := JSONParse(`{"flagged": true}`).(map[string]any)\nif value["flagged"] == true { return Block("命中规则") }',
      },
      {
        signature: "JSONStringify(value)",
        description: "将对象转换为 JSON 字符串，最大 1 MiB。",
        code: 'encoded := JSONStringify(map[string]any{"texts": texts})\nparsed := JSONParse(encoded).(map[string]any)\nif parsed["texts"] == nil { return Block("缺少文本") }',
      },
    ],
  },
  {
    title: "大模型调用",
    items: [
      {
        signature: "LLMChat(base, key, model, messages, timeout)",
        description: "调用 OpenAI 兼容的 chat/completions，返回 HTTP 结果对象。",
        code: 'response := LLMChat("https://api.openai.com/v1", "os.environ/XHUB_GUARDRAIL_LLM_KEY", "your-judge-model", []map[string]any{{"role": "system", "content": "判断内容是否安全，只回答 SAFE 或 UNSAFE。"}, {"role": "user", "content": JSONStringify(texts)}}, 8)\nif response["success"] != true { return Block("审核模型不可用") }\nbody, ok := response["body"].(map[string]any)\nif !ok { return Block("模型结果格式错误") }\nchoices, ok := body["choices"].([]any)\nif !ok || len(choices) == 0 { return Block("模型未返回判定") }\nchoice := choices[0].(map[string]any)\nmessage := choice["message"].(map[string]any)\nif message["content"] != "SAFE" { return Block("模型审核未通过") }',
      },
    ],
  },
] as const;

export const LANGUAGE_PRIMITIVES = [
  {
    signature: "len(texts)",
    description: "Go / XGo 内置函数：获取文本段数。",
    code: "if len(texts) == 0 {\n    return Allow()\n}",
  },
  {
    signature: "for text <- texts",
    description: "XGo 循环语法：依次检查每段文本。",
    code: 'for text <- texts {\n    if Contains(Lower(text), "secret") {\n        return Block("检测到敏感词")\n    }\n}',
  },
] as const;

export const DEFAULT_TEST_INPUT = JSON.stringify(
  { texts: ["hello secret"], model: "your-model", metadata: {} },
  null,
  2,
);
/**
 * 用途：严格解析调试 JSON，拒绝截图中的旧 Python 输入合约和未支持字段。
 * 参数：value：编辑器内的 JSON 字符串，仅支持 texts、model、metadata。
 * 返回：已校验对象，保留空文本位置；类型或字段错误抛出可展示的 Error。
 * 调用：CustomCodeModal 的 test 测试按钮处理器。
 * 测试：primitives.test.ts。
 */
export function parseTestInput(value: string): { texts: string[]; model?: string; metadata?: Record<string, unknown> } {
  const input: unknown = JSON.parse(value);
  if (!input || typeof input !== "object" || Array.isArray(input)) throw new Error("测试输入必须是 JSON 对象");
  const data = input as Record<string, unknown>;
  if (!Array.isArray(data.texts) || !data.texts.every((text) => typeof text === "string")) {
    throw new Error("texts 必须是字符串数组");
  }
  if (data.model !== undefined && typeof data.model !== "string") throw new Error("model 必须是字符串");
  if (
    data.metadata !== undefined &&
    (!data.metadata || typeof data.metadata !== "object" || Array.isArray(data.metadata))
  ) {
    throw new Error("metadata 必须是 JSON 对象");
  }
  const unsupported = Object.keys(data).filter((key) => !["texts", "model", "metadata"].includes(key));
  if (unsupported.length) throw new Error("测试输入仅支持 texts、model 和 metadata");
  return data as { texts: string[]; model?: string; metadata?: Record<string, unknown> };
}

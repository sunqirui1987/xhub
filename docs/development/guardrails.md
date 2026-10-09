# XHub 护栏：配置与 XGo 自定义脚本

护栏在正常数据面的 chat / responses 请求调用上游前检查文本。前端保存的是规则配置，后端才执行检查。当前实现支持本地关键词/RE2、XGo 脚本及已移植的外部服务适配器。

## 从页面开始

页面统一使用轻量卡片和清晰的留白。「护栏花园」先选择本地规则或 XGo，再浏览外部服务；目录提供搜索与支持状态筛选。「护栏」显示总数、默认启用、按需调用三项统计，并支持按名称或执行器搜索。

关键词/正则、XGo 和外部配置共用基础设置、配置与测试、右侧参考说明的布局。关键词编辑器提供敏感词、手机号脱敏、邮箱脱敏模板，以及可复制的 RE2 示例；XGo 的右栏提供真实函数签名和示例。高级设置默认折叠，窄屏改为单栏。执行模式下拉框可展开查看各阶段，当前只有 pre_call 可选；during_call、post_call、pre_mcp_call 标明暂不支持并禁止选择。

1. 打开「护栏」，选择「关键词 / 正则护栏」或「XGo 自定义脚本护栏」。
2. 设置名称、规则、优先级，以及是否默认启用。关键词每行一个，忽略大小写；正则遵循 Go RE2，不支持回溯、前后向断言。
3. 关键词/正则和 XGo 都可以测试未保存的草稿，保存时再次校验。外部服务也支持调试未保存配置；已有密钥被遮蔽时，会提示本次使用已保存配置。YAML 或只读规则使用服务器已保存配置调试。
4. 用普通文本和命中文本分别测试，核对 action、reason 与修改后的文本。
5. 默认启用的护栏自动执行；非默认规则通过请求 guardrails 数组指定名称或 ID。

「已提交护栏」表示团队提交审核，是独立功能，当前尚未接入。页面显示真实能力状态，接口返回 supported:false 与稳定的空 submissions 数组。「护栏花园」保留原有 27 个伙伴目录，并增加 Azure、密钥检测和远端接入卡片。页面区分已接入、远端接入和尚未移植；未移植的伙伴可转入远端 LiteLLM 桥接配置。只有已实现的协议可以作为本地规则保存。

## 执行规则

- mode 指执行时机。当前支持 pre_call，表示发给模型前检查；兼容旧规则的 mode:redact，新规则使用 action:redact。
- action 指检查结果。本地规则为 allow / block / redact；XGo 为 allow / block / modify / flag；flag 记录告警后继续请求。
- 默认启用规则与客户端指定规则取并集，同一条规则只执行一次。客户端空数组不能关闭默认规则。未知名称、非法选择格式会拦截。
- 按 priority 从小到大串行执行，同优先级按 ID 排序。未设置 priority 的旧配置按 0 处理，界面新建默认 100。
- block 立即停止，不调用模型。redact / modify 回写对应文本，下一条护栏读取修改后的文本。
- 关键词/正则逐段检查，不跨消息拼接后匹配。系统、工具消息的跳过设置只影响检查和替换；完整消息仍交给模型。
- 单条规则显式 true / false 覆盖全局设置；未配置或在页面选择「使用全局默认」时继承 litellm_settings。
- XGo 错误、超时或无效返回结果会阻止请求，并在护栏记录中保留错误信息。存储读取失败也会阻止请求。

当前不支持 post_call、during_call、图片内容审核、工具权限控制与团队审批。保存不支持的配置会返回错误；非法 YAML 配置会生成默认拦截规则，避免看似启用却无效。旧数据库中的 post_call 规则保持旧的跳过行为，应迁移为受支持规则。

## YAML 示例

```yaml
litellm_settings:
  skip_system_message_in_guardrail: true
  skip_tool_message_in_guardrail: false

guardrails:
  - guardrail_name: phone-mask
    litellm_params:
      guardrail: local
      mode: pre_call
      default_on: true
      priority: 10
      patterns:
        - '1[3-9][0-9]{9}'
      action: redact
      replacement: '[手机号已隐藏]'
  - guardrail_name: no-secret
    litellm_params:
      guardrail: local
      mode: pre_call
      default_on: false
      priority: 20
      blocked_words:
        - secret
      action: block
      skip_system_message_in_guardrail: false
```

YAML 规则可以在页面查看和测试，修改、删除需编辑配置并按现有配置加载流程重启/重载。页面规则存储在 KV 数据库，两类规则一起参与执行。名称应全局唯一，也不能与另一规则的 ID 冲突；运行时检测到歧义配置会拦截请求。

请求按需启用 no-secret：

```json
{
  "model": "your-model",
  "messages": [{"role": "user", "content": "hello"}],
  "guardrails": ["no-secret"]
}
```

也兼容 metadata.guardrails 字符串数组。不接受每请求规则参数覆盖。metadata 是客户端数据，不能作为已认证的用户/团队身份依据。

## 用 Go 语法写 XGo

在「XGo 自定义脚本护栏」粘贴以下代码：

```go
import . "xhub/guardrail"

func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {
    for i, text := range texts {
        if Contains(Lower(text), "secret") {
            return Block("文本包含敏感内容")
        }
        texts[i] = RegexReplace(text, "1[3-9][0-9]{9}", "[手机号已隐藏]")
    }
    return Modify(texts)
}
```

- texts 是过滤掉被跳过角色后的协议文本叶子，顺序固定；图片 URL、role、其他元数据不属于检查文本。
- requestData 提供 model 与 metadata 的副本。inputType 当前固定为 request。脚本不获得客户端声称的用户/团队字段作为可信身份。
- Allow() 放行原文。修改 texts 后返回 Allow() 不会回写；必须返回 Modify(texts)。
- Block(reason) 拦截并提供原因。
- Flag(reason, metadata) 记录非阻断告警，复制附加 JSON 元数据供日志查看，继续后续规则。
- Modify(texts) 修改后继续，文本数量与顺序必须与输入一致，包括空文本位置。
- 可调用 Contains、Lower、Trim、RegexMatch(text, pattern)、RegexReplace(text, pattern, replacement)。RegexReplace 使用字面量替换，不能使用 $1 捕获组展开。
- 可写 Go 函数、循环、条件、局部/全局变量；可省略 package main。只允许导入 xhub/guardrail。
- 支持 XGo 语法，例如关键词模板中的 `for text <- texts { ... }`，等价于 Go 的 `for _, text := range texts { ... }`。原有 Python 模板需按上述函数合约改写，不会自动转换 Python 代码。

### 编辑器中的可用 Primitives

编辑器左侧是带行号的 XGo 代码区及 JSON 测试区，右侧是可折叠的函数参考。点击函数卡片复制的是可粘贴到 ApplyGuardrail 函数内的代码片段；检查类片段后保留函数末尾的 return Allow()。示例遍历全部文本，也适用于空数组。

Allow、Block、Modify、Flag、Contains、Lower、Trim、RegexMatch、RegexReplace、HTTP、JSON 和 LLM 函数 是 XHub 护栏执行环境通过 iXGo 注册的函数，导入 xhub/guardrail 后可调用；它们不是 XGo 标准库的一部分。len 与 for text <- texts 属于 Go / XGo 语言能力，侧栏单独列出。

测试输入由 texts（文本数组）、model（模型名称）、metadata（客户端元数据对象）组成。编辑器将 model 和 metadata 传给脚本的 requestData，拒绝不支持的输入字段。HTTP、JSON、Flag 和大模型调用已经注册；侧栏显示真实签名及可复制的 XGo 示例。Python 调用需要改写成 Go/XGo 合约，不能原样执行。

YAML 中脚本配置：

```yaml
guardrails:
  - guardrail_name: script-policy
    litellm_params:
      guardrail: custom_code
      custom_code_language: xgo
      mode: pre_call
      default_on: true
      priority: 100
      custom_code: |
        import . "xhub/guardrail"
        func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {
            for _, text := range texts {
                if Contains(Lower(text), "secret") { return Block("敏感内容") }
            }
            return Allow()
        }
```

调试接口 POST /guardrails/test_custom_code 接收 custom_code、test_input.texts、input_type:"request" 与可选 request_data。编译/执行错误返回 success:false 与 error_type。护栏执行完毕即使 action:block 也可以是 HTTP 200，含义是测试成功完成，页面应显示「已拦截」。真实模型请求被护栏拒绝时返回 HTTP 400 guardrail_failed。

## 外部服务与护栏花园

目录是服务列表，实际检查由后端适配器或已有远端 LiteLLM 执行。配置、调试和正式请求共用执行器；内置外部适配器遇到超时、认证失败或畸形响应默认拦截。

| guardrail 协议 | 当前行为 | 配置要点 |
| --- | --- | --- |
| openai_moderation | /moderations，任一 flagged 拦截 | api_key，可选 api_base/model |
| lakera_v2 | /v2/guard，flagged 拦截 | api_key，可选 project_id |
| azure/text_moderations | Hate/SelfHarm/Sexual/Violence 分类 | api_base、api_key，severity_threshold 整数 0–7，默认 4 |
| azure/prompt_shield | userPrompt 攻击检测 | api_base、api_key |
| bedrock | AWS ApplyGuardrail；GUARDRAIL_INTERVENED 拦截 | guardrailIdentifier、guardrailVersion、aws_region_name；默认凭据链或成对密钥 |
| presidio | Analyzer 检测，Anonymizer 脱敏 | api_base；action:redact 需 anonymizer_api_base |
| hide-secrets | 本地密钥特征拦截/脱敏 | action、replacement；非 detect-secrets 全插件实现 |
| litellm_proxy | 远端 /guardrails/apply_guardrail | api_base、api_key、remote_guardrail_name；先在远端配置服务 |

Azure 每段最多 10000 个 Unicode 字符，优先在空白处分割；长单词强制分段，保留全部字符。各段依次审核，共享规则的 10 秒预算；任一段命中立即拦截。Presidio 校验 Unicode 位置并保持文本段对应关系。Bedrock 使用 AWS SDK v2 签名与凭据链。

~~~yaml
guardrails:
  - guardrail_name: azure-safety
    litellm_params:
      guardrail: azure/text_moderations
      mode: pre_call
      default_on: true
      api_base: https://your-resource.cognitiveservices.azure.com
      api_key: os.environ/XHUB_GUARDRAIL_AZURE_KEY
      severity_threshold: 4
  - guardrail_name: remote-partner
    litellm_params:
      guardrail: litellm_proxy
      mode: pre_call
      api_base: https://your-litellm.example
      api_key: os.environ/XHUB_GUARDRAIL_REMOTE_KEY
      remote_guardrail_name: your-existing-guardrail
~~~

管理响应中的密钥显示 ********。更新传入掩码或空字符串保留已有值；显式 null 删除。表单清空可选字段可能提交 null。环境变量引用仅允许 os.environ/XHUB_GUARDRAIL_*，缺失或其他前缀会失败。直接填写的密钥仍存储在规则配置中，推荐环境变量引用。

## HTTP、JSON 与大模型 primitives

这些函数由 XHub 注入虚拟包 xhub/guardrail，不属于 Go/XGo 标准库。支持点导入、guardrail 包名和自定义别名；导入此包不会自动开放其他系统包，新增能力需要显式注册并验证。

| 函数 | 合约 |
| --- | --- |
| JSONParse(text string) any | 解析 JSON，错误触发执行失败 |
| JSONStringify(value any) string | 序列化 JSON，最多 1 MiB |
| HTTPGet(url string, headers map[string]string, timeout int) map[string]any | GET，timeout 单位秒 |
| HTTPPost(url string, body any, headers map[string]string, timeout int) map[string]any | JSON POST |
| HTTPRequest(url, method string, headers map[string]string, body any, timeout int) map[string]any | GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS |
| LLMChat(base, key, model string, messages []map[string]any, timeout int) map[string]any | OpenAI 兼容 /chat/completions |

HTTP 返回 success、status_code、body、headers、error。JSON body 自动解析为 any，非 JSON body 为字符串；脚本必须检查成功状态和字段类型。只允许 HTTP(S)，不跟随重定向；请求和响应各限 1 MiB。单次超时最多 10 秒，多次调用共享脚本总预算。响应头经过筛选，网络错误不包含认证密钥。

~~~go
import . "xhub/guardrail"

// ApplyGuardrail 调用外部审核；服务失败拒绝，疑似内容记录告警。
// 参数：texts 为有序文本，requestData 为模型/metadata 副本，inputType 为 request。
// 返回：Allow、Block 或 Flag；调试和正式请求执行相同逻辑。
func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {
    response := HTTPPost("https://your-review.example/check",
        map[string]any{"texts": texts},
        map[string]string{"Authorization": "Bearer os.environ/XHUB_GUARDRAIL_API_KEY"}, 5)
    if response["success"] != true { return Block("审核服务不可用") }
    body, ok := response["body"].(map[string]any)
    if !ok { return Block("审核结果格式错误") }
    flagged, ok := body["flagged"].(bool)
    if !ok { return Block("审核结果缺少判定") }
    if flagged { return Flag("需要复核", map[string]any{"source": "review-service"}) }
    return Allow()
}
~~~

LLMChat 示例位于编辑器侧栏，可把 texts 序列化后提交审查模型，再读取 SAFE/UNSAFE。它直接访问指定 OpenAI 兼容服务，不自动经过 XHub 模型路由、费用或权限逻辑。不要指向会运行同一护栏的入口，否则产生递归。脚本可明确选择网络失败时 Block 或 Allow；内置外部适配器统一失败拦截。

## 实现位置与边界

- internal/gateway/guard/manage.go：经管理员鉴权后的 CRUD、配置校验和能力信息。
- internal/gateway/guard/rules.go：关键词/正则校验、消息范围与替换。
- internal/gateway/guard/runner.go：统一执行器与 YAML/全局设置。
- internal/gateway/guard/external.go：服务协议、Azure 分段、AWS 签名、Presidio 脱敏。
- internal/gateway/guard/primitives.go：JSON、受控 HTTP、环境凭据和独立调用绑定。
- internal/gateway/guard/secrets.go：本地密钥特征。
- internal/gateway/guard/custom.go：XGo 编译、缓存、iXGo 执行及返回值校验。
- internal/gateway/guard/guard.go：规则选择、执行顺序与日志结果。
- internal/dataplane/serve.go：上游调用前接入护栏，修改后的正文用于请求编码和缓存。

真实执行路径是 XGo 源码 → XGo 编译为 Go AST → Go SSA → iXGo 解释执行。编译结果最多缓存 64 条；每个请求创建新的解释器实例，不共享脚本全局变量。代码最多 32KB，检查文本总量最多 1MiB / 10000 段，metadata 最多 1MiB，纯本地脚本执行上限 100ms；引用 HTTP/LLM 的脚本共享 10 秒总预算（均不包含编译时间）。网络只能通过受控函数访问；禁止任意包导入、文件访问、goroutine、defer、channel、init 与非空 main。

脚本在网关进程内运行，限制导入与执行时长不等同于内存沙箱。极端内存分配仍可能影响网关，因此当前仅供可信管理员配置；面向不可信租户的自助脚本应使用独立进程/容器及内存配额。

本次接入覆盖正常 chat / responses 数据面路径。official bypass 独立路径及其他任务端点不能据此视为已保护。没有实现响应阶段检查，也没有对已发送的流式输出进行撤回。

## 参考

- https://docs.litellm.com.cn/docs/proxy/guardrails/quick_start
- https://docs.litellm.com.cn/docs/adding_provider/simple_guardrail_tutorial
- https://docs.litellm.com.cn/docs/proxy/guardrails/custom_code_guardrail
- https://github.com/goplus/xgo
- https://github.com/goplus/ixgo

保留 LiteLLM 的名称、执行阶段、默认启用与请求选择概念，自定义代码接口采用 XHub 的 XGo/Go 合约，不能直接运行文档里的 Python 脚本。

参考实现：用户提供的 LiteLLM checkout 中 `ui/litellm-dashboard/src/app/(dashboard)/guardrails/_components/` 提供编辑器，`litellm/proxy/guardrails/guardrail_hooks/` 提供服务适配器。XHub 使用独立的 Go 适配器和 XGo 执行器。

# 15 端点类型

这一版回答一个问题：添加模型时，怎么表达「这条模型会被怎么调用」。

现状的「端点类型」下拉把三件不同的事压成了一个选项。它们分开之后就自然了。

## 1. 现在把三件事混在一起

| 混在一起的 | 实际是什么 | 现在的表现 |
| --- | --- | --- |
| 入口面 | 调用方 POST 到哪个路径 | 和上游协议挤在同一个 `mode` 里 |
| 上游协议 | 网关翻译成什么协议、发到哪个路径 | `Type.Operation` 是死字段，没人读 |
| 转发方式 | 网关是翻译还是原样转发 | `Kind` 只有 `adapted` 和 `bypass` |

后果：

- 一条模型只能填一个 `mode`。但 `/v1/chat/completions`、`/v1/messages`、`/v1/responses` 说的是同一件事，应该都能调。
- `provider.SelectedTypes` 已经在读复数的 `model_info.endpoint_types`，读不到才退回单个 `mode`。数据结构早就支持多个，是表单只给单选。
- `Type.Operation` 登记了却没人读（`internal/provider/openai/chat.go:14` 起七处）。真正决定上游路径的是 `llm.Endpoint(op, provider, base, model)`：**入站 op 加供应商**，不是模型上配的那一格。
- 计费不看协议。`catalog.TokenRates(model)` 只按模型名取每 token 单价，`spend.go` 用 op 只是为了知道去正文哪里数 token。

所以「端点类型」真正要表达的是两件独立的事，不是一个。

## 2. 拆成两个字段

### 2.1 `capabilities`：这条模型能应答什么

一种能力对应一组入口路径。同一组里的路径是同一件事的不同拼法，选了就都能调。

| 能力 | 入口路径 | 说明 |
| --- | --- | --- |
| `chat` | `/v1/chat/completions`、`/v1/messages`、`/v1/responses`、`/v1/completions` | 文本进、文本出。三种拼法是一件事 |
| `embedding` | `/v1/embeddings` | |
| `image` | `/v1/images/generations`、`/v1/images/edits` | 生图和改图共用一个模型很常见 |
| `video` | `/v1/videos` | 异步任务 |
| `audio_speech` | `/v1/audio/speech` | |
| `audio_transcription` | `/v1/audio/transcriptions`、`/v1/audio/translations` | |
| `rerank` | `/v1/rerank`、`/v2/rerank` | |
| `moderation` | `/v1/moderations` | |
| `realtime` | `/v1/realtime` | |

为什么 `chat` 要把 `/v1/messages` 和 `/v1/responses` 收进来：网关已经会跨协议翻译，不用操作员告诉它。

- `llm.Endpoint` 的 `OpMessages` 分支：Gemini 没有 Messages 路径，改发 `generateContent`，回来再解成 Messages 形状（`internal/llm/call.go:89`）。
- `buildAnthropic` 把 chat 正文改写成 Messages 正文，换上 `x-api-key` 和 `anthropic-version`（`internal/llm/build.go:196`）。

所以调用方用哪种拼法，和上游说哪种协议，是两件由网关自己配好的事。让操作员在添加模型时二选一，只会把本来能跑的组合挡掉。

### 2.2 `transport`：网关怎么把请求送到上游

| transport | 含义 | 什么时候用 |
| --- | --- | --- |
| `adapted` | 网关按供应商的标准协议翻译并发出 | 供应商说的是 OpenAI / Anthropic / Gemini 标准形状 |
| `bypass:<类型 id>` | 内置 Bypass，按目录里登记的路径和流程原样转发 | 接口形状固定且常用，写成 Go 包（方舟 `ark_contents_generation`、七牛 `qiniu_contents_generation`） |
| `custom` | 自定义 Bypass，操作员照文档填路径 | 接口形状只在某家文档里，不值得写 Go（Suno、Tripo） |

`bypass` 和 `custom` 不该并列成两个同级选项。它们是同一件事的两档：有内置包就用内置包，没有就自己填。合并成一个 `transport` 字段后，UI 是一组二选一加一个「换成自定义」，而不是下拉里混着 vendored 类型和 `custom`。

## 3. 结果：表单长什么样

```
这条模型怎么被调用

  能力（可多选）
    [x] Chat         /v1/chat/completions · /v1/messages · /v1/responses
    [ ] Embedding    /v1/embeddings
    [x] Image        /v1/images/generations · /v1/images/edits
    [ ] Video        /v1/videos

  转发方式（单选）
    (o) 协议适配      网关翻译成供应商的标准协议
    ( ) 内置适配      [方舟内容生成 v]
    ( ) 自定义        API Base / 模型字段 / 任务 id / 路径表

  单价
    每对模型只填一次，和能力无关
```

## 4. 目录（`internal/provider`）怎么跟着改

`Type` 拆成「能力」和「转发」两半。`Providers` 这一格已经能把类型绑到供应商上，保留。

```go
// Capability 是一种入口能力。它决定哪些路径能调用这条模型。
type Capability struct {
    ID       string   // chat / embedding / image / ...
    Label    string
    Paths    []string // 入站路径，同组等价
    Providers []string // 空表示任何供应商
}

// Transport 是网关把请求送到上游的方式。
type Transport struct {
    ID       string    // 内置适配的标识；自定义时是 "custom"
    Kind     Kind      // adapted / bypass
    Label    string
    Providers []string
    APIBase  string
    ModelField string
    TaskID   string
    StripPrefix string
    Actions  []Action
}
```

`Operation` 从 `Type` 上删掉。它是死字段，且真正的上游路径由 `(入站 op, 供应商)` 决定，写在模型上只会和实现打架。

登记一侧：

```go
// 能力：路径表是目录的事实，不是每家供应商的事。
provider.RegisterCapability(provider.Capability{
    ID: "chat",
    Paths: []string{"/v1/chat/completions", "/v1/messages", "/v1/responses", "/v1/completions"},
})

// 转发：只有 Bypass 需要登记，协议适配是网关自带的。
provider.RegisterTransport(provider.Transport{...})
```

## 5. 每条路径实际怎么走

一次请求进来，网关按顺序做四件事。第二步是这一版改动的重点：能力取代了单个 `mode`。

1. **认入口。** `catalog.IsLLMPrefix` 认出这是数据面路径，`family.inferenceOp` 从路径得出 op。这一步不改。
2. **选部署。** 在候选部署里留下「能力包含这个 op」的。现在是 `provider.Includes(m, typeID)`，按单个 id 比；改成按能力组比，`/v1/messages` 和 `/v1/chat/completions` 命中同一个 `chat`。
3. **定上游。** `adapted` 走 `llm.Endpoint(op, provider, apiBase, model)`；`bypass` 走 `provider.Match` 命中的动作，路径照抄。
4. **计费。** `catalog.Cost` 按模型名取单价。和能力、协议都无关，不改。

现在的代码在第二步和第三步之间有个不吻合：`family.inferenceOp` 从**入站路径**得出 op，`provider.SelectedTypes` 从**模型配置**得出允许的 op，两边各自维护一份字符串。合并成能力表之后就只剩一份：能力表定义路径，路径推出 op，op 推出上游。

## 6. Playground 跟着改

Playground 现在需要人先选端点再选模型，等于把第 2 步的判定抄了一遍。改成：

- 选模型，展示这条模型的**能力**标签（Chat / Image …）。
- 输入区按**当前能力**渲染。选了 Chat 就给对话输入和「用哪家协议调」的切换（OpenAI / Anthropic / Responses），因为这是同一能力的三种拼法，来回切不该换模型。
- 一条模型有多个能力时，在输入区上方给能力页签，不要回到模型列表重选。

## 7. 兼容

存量部署上有 `model_info.mode` 和 `model_info.endpoint_types`。

- `endpoint_types` 已经在上线数据里用过，继续读。里面的值按上表映射成能力（`messages` → `chat`，`images_edits` → `image`）。
- 只有 `mode` 的按同样规则映射成一个能力。
- 两个都没有的，默认 `["chat"]`，和现在一样。

映射放在 `provider.SelectedTypes` 里，这样读的地方只认能力。

## 8. 为什么这样更好扩展

新增一件事只在一边动：

| 要加的 | 改哪里 |
| --- | --- |
| 一种新的入口形状（例如 `/v1/video/edits`） | 在已有能力上多一个路径；同能力的模型自动就能应答 |
| 一种新的入口能力 | 登记一条能力，加一个数据面 op |
| 一家供应商的标准协议 | `llm.Endpoint` 里多一个 `(op, provider)` 分支 |
| 一家供应商的自有接口 | 登记一个内置 transport，或让操作员用自定义 |

现在的设计里，「新增一种入口」和「新增一家供应商」都要动同一个下拉，且都只能单选。

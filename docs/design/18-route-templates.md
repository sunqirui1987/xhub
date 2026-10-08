# 18 路由模板

路由策略现在是**一个全局设置**：整个进程一份 `RouterSettings`，一条策略，
一份回退配置。团队、组织、密钥除了"能调哪些模型"和"花多少钱"之外，对流量怎么分、
失败了怎么办、等多久算超时**一句话都说不上**。

这一版把路由设置做成**可以命名的模板**：平台管理员在一处编模板，组织、团队、密钥
各自选用一份。不选就继承上一级，一路继承到平台默认。

参考：LiteLLM 的 [reliability](https://docs.litellm.com.cn/docs/proxy/reliability)、
[routing](https://docs.litellm.com.cn/docs/routing)、
[load_balancing](https://docs.litellm.com.cn/docs/proxy/load_balancing)、
[keys_teams_router_settings](https://docs.litellm.com.cn/docs/proxy/keys_teams_router_settings)、
[timeout](https://docs.litellm.com.cn/docs/proxy/timeout)。

## 决定

左边已有的「路由设置」是**唯一编辑入口**，它变成模板库。组织、团队、密钥不再填写
负载均衡和回退，只选择一份模板。

- 没选就是继承，一路继承到平台默认。
- 选了就整份使用那份模板，不按字段和上一级拼。
- 个人就是密钥。用户账号不选模板。
- 组织今天没有路由设置，补的也只是一个选择框。

## 现在是什么样

先把事实摆清楚，因为下面每条都对应一处要改的地方。

### 回退存了，但请求路径不读

`fallbacks`、`context_window_fallbacks`、`content_policy_fallbacks`、
`max_fallbacks` 在 `internal/gateway/prefs/` 里能存能读，**在 `internal/dataplane/`
和 `internal/router/` 里一次都没出现过**（grep 确认）。

LiteLLM 文档里那个例子：

```yaml
litellm_settings:
  fallbacks: [{"zephyr-beta": ["gpt-3.5-turbo"]}]
```

配了等于没配。已经在跑的"回退"其实是**同名模型下的另一条部署**——负载均衡的副产品，
不是回退。这两件事被混在一起了：

| | 是什么 | 现状 |
| --- | --- | --- |
| **负载均衡** | 同一个公开名下多条部署，按策略分流量 | 做了 |
| **回退** | A 模型失败，换到**另一个模型名** B | **没做** |

`max_fallbacks` 同理：没有代码在数跳了几次。

### 密钥上的路由设置今天就是死的

`RouterSettingsAccordion` 在密钥的创建和编辑里渲染，提交时把 `router_settings`
放进 `/key/new`、`/key/update` 的正文。但：

- `internal/iam` 里没有任何一列存它（grep 无命中）。
- `internal/gateway/keys/` 下没有任何一个文件读它（grep 无命中）。

所以这个手风琴**填了不生效，也不落库**。从密钥界面移除它不会损失功能。

### 超时构造时冻结

`internal/gateway/server.go:118`：

```go
Client: &http.Client{Timeout: time.Duration(cfg.RouterSettings.Timeout) * time.Second},
```

`ApplyTyped` 确实改 `s.Config().RouterSettings.Timeout`，但 client 已经建好了——
在设置页改超时，**页面显示新值，数据面还用旧值**。

而且它是**每个 HTTP 调用**的超时，不是整条回退链的；流式响应也被同一个值卡着。

### 冷却对所有人共享，且不会提前恢复

`xhub:fails:<id>` 和 `xhub:cooldown:<id>`，`<id>` 是 `api_base|model`。
一个租户打出的 5xx 让这条部署对**全部租户**冷却。成功一次也不会清掉计数——
`serve.go:283` 成功时只记延迟。

### 团队/组织/密钥今天能说什么

只有模型名单、预算、速率限制。`teams` 表的列是 `models` / `max_budget` / `spend` /
`status`，没有一列能挂路由配置。`router_settings` 是 `proxy_config` 里一个**全局**
命名空间。

---

## 数据

### 模板

```sql
CREATE TABLE IF NOT EXISTS route_templates (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL CHECK (name <> ''),
    -- The template body. It is exactly one router_settings document, the same
    -- shape /config/update already accepts, so an existing settings blob can be
    -- copied into a template without translation.
    body       TEXT NOT NULL DEFAULT '{}',
    -- A fixed row named 平台默认 holds the live platform values. It is the bottom
    -- of the inheritance chain and cannot be deleted or renamed.
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (name)
);
```

`body` 就是现在的 `router_settings`，字段一个不少：`routing_strategy`、
`routing_strategy_args`、`num_retries`、`timeout`、`allowed_fails`、`cooldown_time`、
`retry_after`、`fallbacks`、`context_window_fallbacks`、`content_policy_fallbacks`、
`max_fallbacks`、`retry_policy`、`model_group_alias`、`enable_tag_filtering`。

用一个 JSON `body` 而不是给模板开一堆列，理由是：字段集合还在长（`retry_policy`
今天就没界面），每加一个字段就要改表。`body` 的形状由 `prefs/page.go` 的字段清单
定义，那里已经是唯一的一份来源。

### 三个范围的选择

```sql
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS route_template_id TEXT
    REFERENCES route_templates (id) ON DELETE SET NULL;
ALTER TABLE teams ADD COLUMN IF NOT EXISTS route_template_id TEXT
    REFERENCES route_templates (id) ON DELETE SET NULL;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS route_template_id TEXT
    REFERENCES route_templates (id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS organizations_route_template ON organizations (route_template_id);
CREATE INDEX IF NOT EXISTS teams_route_template ON teams (route_template_id);
CREATE INDEX IF NOT EXISTS api_keys_route_template ON api_keys (route_template_id);
```

**可空，空表示不应用。** 一个字段就是"选没选"，没有第三种状态。

索引是为了"这份模板正被谁用"那三个查询——删除前要查。

`ON DELETE SET NULL` 是兜底：正常路径上删除会被接口拒绝（下面写了），但数据库
层面也不该让一次误操作把引用悬空。

### 为什么不是一张绑定表

我先前的版本用 `(kind, scope_type, scope_id, module_id)` 的绑定表。按现在这份规格，
**三个可空外键更好**，因为：

- 作用域是固定的三个，而且它们都是**已经存在的实体表**。加一列是加一列，
  加一张表还要处理"作用域被删了、绑定行还在"。
- "这份模板被谁用"是三个按索引的查询，语义直白。
- 应用模板就是改自己那一行的字段，和改预算、改模型名单走同一条更新路径，
  不新增一个写接口，也不新增一类权限检查。

绑定表的优势（作用域可扩展、两个模块共用一个机制）在这份规格里用不上：
范围已经定死是三个，模块只有一个。

---

## 一次请求用哪一份

**只看有没有选中模板，不按字段拼：**

```
1. 密钥选了  → 用密钥的模板
2. 否则团队选了 → 用团队的模板
3. 否则组织选了 → 用组织的模板
4. 否则      → 平台默认
```

下一级没选时**继承上一级，不是跳回平台默认**。例如组织选了 A、团队没选，团队用 A；
团队再选 B，只有这个团队用 B。

### 解析函数接受一个有序范围列表

范围链写成数据，不写成三段 if：

```go
// scopes 由窄到宽。第一个有模板的就是生效的那一份。
var scopes = []scope{
    {kind: scopeKey,   id: keyID},
    {kind: scopeTeam,  id: teamID},
    {kind: scopeOrg,   id: orgID},
}
```

**项目（project）故意不在这个列表里。** `api_keys` 上有 `project_id`，项目也有
`models` 和 `max_budget`，和团队同构——所以它**看起来**该在链子里，但它不在：

- 项目是这三个范围里最少用的。
- 加它就要加一列、加一个下拉、加一组测试，而没有人提过这个需求。
- 以后要加，改动就是在列表里插一项，不是重写解析。

这条不对称是有意的，不是漏掉的。写在代码注释里，免得以后被当成 bug"顺手补上"。

### 为什么整份替换而不是按字段合并

按字段合并要回答"两个模板都写了 `num_retries` 时听谁的"（窄的那层）、"权重按谁
归一"、"策略 A 加策略 B 是什么"。这些问题没有好答案，而且"组织改了设置、团队行为
跟着变了"是运维最难查的一类问题。

整份替换的规则一句话能说完：**生效的那一份，整个生效。**

### 冷却跟着哪一份走，以及为什么不给它开关

模板里的 `allowed_fails` / `cooldown_time` 是**阈值**，按范围生效。但**冷却键
本身是共享的**，这一条不跟着模板走。原因是一句事实而不是一个偏好：

> 冷却说的是"这份凭据打不通这个上游了"。而凭据是共享的。

甲租户打出的 429，就是乙租户会遇到的那个 429。让租户能选"我的冷却只影响我自己"，
等于让每个租户各自把同一个故障重新发现一遍，代价是 N 倍的失败请求——而他们并没有
得到更强的隔离，只是更晚才学到同一件事。

**但今天的冷却键有一个真 bug，这次一起修**：键是 `api_base|model`（`router.go:433`），
**不含凭据**。所以：

| 情况 | 今天 | 改成 |
| --- | --- | --- |
| 同一个具名凭据下的两个团队 | 共享冷却键 | 共享（正确） |
| 两个团队各用**自己的** API key 打同一个上游 | **共享冷却键（错）** | 各自独立 |

第二种情况下，甲的账号被限流会让乙的账号也被判定为冷却，而乙的账号是好的。
所以冷却键加上凭据标识：

```
xhub:cooldown:<credential>|<api_base>|<model>
```

`<credential>` 取 `litellm_credential_name`；部署上没有具名凭据时用原来那个
`api_base|model`（即现状），所以不用具名凭据的部署行为不变。

这样做的结果：**把租户调严格的爆炸半径限制在"跟他共用凭据的人"**——而那本来就是
共享同一份上游约束的同一批人。于是模板字段是安全的，不需要再加一个作用域开关。

### 平台默认不是一个模板名，是一条固定行

列表里固定一行「平台默认」，用来看和改这个底。它：

- 不能删除，不能改名。
- 内容就是今天全局 `router_settings` 里已经生效的值：`simple-shuffle`、重试 2 次、
  超时 60 秒、无回退。
- 三个范围都不选时用的就是它。

把它做成一行而不是"没有模板"的空状态，是因为运维要能在同一个界面上看到底是什么、
并且改它。今天这个底散在 `prefs/settings.go` 的 `Base()` 里，看不到。

**兼容线**：三个范围都不选时，行为与今天**逐项相同**。

---

## 请求期

### 取一份设置

`dataplane/serve.go` 在选路之前按上面的顺序取出一份设置，再选路。

三个范围都已知：`p.Key`（密钥行）、`p.TeamID`、团队行里的 `organization_id`。
团队行和组织的预算在 `enforceIdentityLimits`（`limits.go:188`、`:195`）已经读过一次，
模板 id 可以顺带取出来，不用为它多查两次。

**解析结果按请求缓存一次**。回退会重新走一遍解析，而每一步都查库会让一次调用
变成十几次查询。缓存键是 `(keyID, teamID, orgID)`，进程内、短 TTL，
写模板或改绑定时失效。

### 改哪几处

`serve.go` 现在直接从 `cfg` 读三样东西。要换成读**解析出来的那一份**：

| 位置 | 现在 | 改成 |
| --- | --- | --- |
| `serve.go:147` `ValidateStrategy` | 读全局策略 | 读解析出来的策略 |
| `serve.go:152` `router.Order` | 读全局策略 | 同上 |
| `serve.go:170` `attempts` | 读全局 `NumRetries` | 读解析出来的 |
| `live.go:48` `RecordFailure` | 读全局 `RouterDocument()` | 接一个已解析的配置参数 |
| `router.go:433` `DeploymentID` 的 Redis 键 | `api_base\|model` | 见下面"冷却键"一节 |
| `server.go:118` client 超时 | 构造时冻结 | **删掉**，改在 `serve.go:254` 用 `context.WithTimeout` |

超时那一处是修 bug：client 级超时无法表达"整条链一共 90 秒"，也无法在不重建 client
的前提下改。`context.WithTimeout` 两个都能做。

**注意超时变活是这一步风险最高的地方。** 今天 client 超时冻结着，线上跑的是配置文件
里的值；改成 context 之后，设置页里那个被改过但从未生效的值会**突然生效**。而且
client 级超时覆盖整个往返（含读 body），流式响应也被它卡着——换的时候必须让长流
不被掐断，否则是"一个正常但很长的流被整体超时切断"。

`RouterState()` 组装 Redis 里的冷却、延迟、用量那部分不变。

### 冷却键

`router.go:433` 现在是：

```go
func DeploymentID(e config.ModelEntry) string {
    return e.ParamString("api_base", "") + "|" + e.ParamString("model", e.ModelName)
}
```

这个 id 同时用在四处：冷却、延迟、TPM 用量、分流游标。**只有冷却需要凭据维度**，
所以不改 `DeploymentID`（改了会同时改掉延迟统计和分流游标的分组，那不是这次要动的），
而是加一个只给冷却用的键：

```go
// cooldownKey 是冷却的 Redis 键。它比 DeploymentID 多一段凭据：同一个上游用两个
// 不同账号打，一个被限流不该让另一个也被判定为不能用了。
func cooldownKey(e config.ModelEntry) string {
    cred := e.ParamString("litellm_credential_name", "")
    if cred == "" {
        return DeploymentID(e)   // 没有具名凭据，就是现状
    }
    return cred + "|" + DeploymentID(e)
}
```

`live.go` 的 `RecordFailure` 和 `redis.go` 的 `Cooled` / `CooledList` 换成这个键。
没有具名凭据的部署**行为完全不变**，这是这次改动要守住的一条。

`router.State.Cooldown` 的键也跟着换，因为它就是被同一批 id 查的。改动落在六处，
都是同一件事，但一处漏掉就是"冷却写了没人读"：

| 位置 | 现在 | 改成 |
| --- | --- | --- |
| `live/redis.go:98` `Cooled` | 拼 `DeploymentID` 前缀 | 拼 `cooldownKey` |
| `live/redis.go:77` `RecordFailure` | 收 `DeploymentID` | 收 `cooldownKey` |
| `dataplane/live.go:37` `State` | 传 `DeploymentID` | 只给 `Cooled` 传新的键 |
| `dataplane/live.go:48` `RecordFailure` | 同上 | 同上 |
| `dataplane/serve.go:275` `NoteFailure` | 同上 | 同上 |
| `router/router.go:161` | `st.Cooldown[DeploymentID(e)]` | `st.Cooldown[CooldownKey(e)]` |
| `router/router.go:300`（`pickSplit`） | `st.Cooldown[ids[i]]` | 另建一个冷却键数组 |

**两处容易写错，特别标出来：**

`dataplane/live.go:33-36` 现在**建一个 `ids` 列表，同时喂给 `Cooled`、`Latencies`
和 `Usages`**。冷却键一变，这个共用列表会把延迟和 TPM 也带上凭据前缀——而它们按部署
聚合才是对的。所以这里要拆成两个列表：

```go
ids := make([]string, 0, len(h.Models()))
cooldownIDs := make([]string, 0, len(h.Models()))
for _, m := range h.Models() {
    ids = append(ids, router.DeploymentID(m))
    cooldownIDs = append(cooldownIDs, router.CooldownKey(m))
}
st.Cooldown = redis.Cooled(cooldownIDs)   // 只有这一个换
st.Latency  = redis.Latencies(ids)        // 不变
st.Usage    = redis.Usages(ids)           // 不变
```

`router/router.go:294-300`（`pickSplit`）的 `ids` 数组身兼两职：分流游标的键**和**冷却查表的键。
冷却换成新键之后这两个不再是同一个字符串，所以要留两个数组，`weights` 和
`Splits.PickWeighted` 仍用 `DeploymentID`（游标按部署分，不该按凭据分）。

**延迟（`xhub:latency:`）和 TPM（`xhub:routetpm:`）不动**——一个上游的响应快慢和
凭据无关，按部署聚合是对的。

`CooldownKey` 要导出（现在是包内函数），因为 `dataplane` 和 `live` 两侧都要拼它。

### 回退怎么执行

部署循环跑完之后，如果全都失败：

1. 按**失败类**（下面那张表）挑一条链。
2. 从对应链里取主模型的下一个模型名。
3. 对它**重新走一遍完整解析**：凭据、额度、路由，当作一次新的尝试。
4. 跳数 +1，超过 `max_fallbacks` 就停，返回 502。

这是今天**完全缺失**的那一环。

### 失败分三类，只有第三类有重复计费风险

"无法确认上游没执行的失败不自动换模型"这条原则要落地，关键是分清**上游到底执行了没有**：

| 类 | 是什么 | 上游执行了吗 | 换模型有风险吗 |
| --- | --- | --- | --- |
| `no_response` | 拨号失败、连接被拒、DNS 解析失败 | 确定没有 | 没有 |
| `status` | 收到了 4xx / 5xx / 429 响应 | 执行了，并且**告诉了我们结果** | 极低 |
| `ambiguous` | 超时；body 发完之后连接断掉 | **不知道** | **有** |

`status` 那一类：上游执行了但失败了。重试一次失败的执行是通行做法，因为失败的执行
通常不计费（429 和 5xx 都是上游自己拒绝的）。所以它按你的验收走——500 重试耗尽后
换模型组。

`ambiguous` 那一类才是真窗口：上游收下了连接、收到了完整 body，然后不出声。这时
它**可能已经在跑、已经在计费**，而我们不知道。默认按你的验收保留（超时也换），
但它是 `fallback_causes` 里可以摘掉的一项：

```json
"fallback_causes": ["no_response", "status", "ambiguous"]
```

字段说明里会写明这一项意味着什么。**不是不告诉你，是让你知道自己在关什么**——
想彻底避免重复计费的就摘掉它，代价是超时不再有兜底。

### 顺带记下一个已经存在的风险

"无法确认上游没执行的失败不重发"这条原则，**今天的重试本身就在违反**：
`serve.go:274` 收到 5xx 就重试同一条部署，而 5xx 响应意味着上游**已经收到并处理了**
那个请求。严格按这条原则，重试 500 就是重复计费的风险。

按你的规格，第 2 步保持现有重试语义，所以这次不动它。但这是一个**已经存在的**
风险，不该被这次改动悄悄盖过去：`ambiguous` 和 `status` 在回退这一层分开了，
在重试那一层没有。要处理的话是另一件事（比如上游返回的 `x-request-id` 去重），
不在这次范围里。

### 三种链是独立的

| 原因 | 走哪条链 | 默认是否触发 |
| --- | --- | --- |
| `no_response` | `fallbacks` | 是 |
| `status` 里的 429 | `fallbacks` | 是 |
| `status` 里的 5xx | `fallbacks` | 是 |
| `ambiguous`（超时） | `fallbacks` | 是，**可在 `fallback_causes` 里摘掉** |
| `context_window` | `context_window_fallbacks` | 是，**只走这条** |
| `content_policy` | `content_policy_fallbacks` | 是，**只走这条** |
| 其它 4xx | **哪条都不走** | 否 |

**其它 4xx 默认不换模型**很重要：今天 4xx 直接透传给调用方，加上回退会让一个写错的
请求变成两次计费。

**流已经发出首字节之后不再换模型。** 换模型意味着重新开始一个响应，而对调用方来说
那是一个已经开始的流。今天的代码在拿到成功状态码之前不写任何字节，所以这条几乎
自动满足；真正的边界是"上游返 200、开始流、中途断掉"——那时字节已经写出去了，
今天也不重试，和这里一致。

三条链不互相兜底：`fallbacks` 非空**不会**让 `context_window_fallbacks` 跟着生效。

---

## 界面

### 路由设置页（`/router-settings`）

仍是原来的左边菜单，仍只有平台管理员能进。结构从"一堆表单"变成"模板列表 + 编辑"：

**列表**

| 列 | 内容 |
| --- | --- |
| 名称 | 模板名。第一行固定是「平台默认」 |
| 摘要 | 策略 + 重试次数 + 超时 + 有没有回退 |
| 使用中 | 正在用它的组织 / 团队 / 密钥数量，点开看清单 |
| 更新时间 | |

操作：新建、改名、复制、删除。删除时如果还有人选用，**接口拒绝**并在响应里列出
绑在哪。

**编辑一份模板**

点开一份模板，用现在的「负载均衡」和「回退」两个页签编辑它。字段不变：
策略、重试、超时、冷静期、`fallbacks`、上下文回退、内容策略回退。

`weighted-split` 加进现有的策略下拉。**流量份额仍用部署上的 `weight`**，
不进模板——份额是部署的属性，模板只决定用哪种选法。

同页上的**路由组、提示缓存、常规设置留在平台级**，不进模板。它们是平台运维的事，
不是某个租户能调的。

### 另外三处只选择

各一个下拉，选项是「不应用」和模板名称。不展开负载均衡表单，也不把模板内容抄进
本地字段。

| 位置 | 改什么 |
| --- | --- |
| 组织的创建和编辑 | 新增这个选择 |
| 团队的创建和详情编辑 | 拿掉现在往 `router_settings` 里填表单的做法。详情里那个没挂上的 `routerSettingsRef` 不渲染成表单 |
| 密钥的创建和编辑 | 同样改成选择。`RouterSettingsAccordion` 从这三处移除 |

**谁能选**：平台管理员任意范围；组织管理员自己的组织；团队管理员自己的团队；
密钥主人自己的密钥。**模板正文只有平台管理员能改。**

下拉里除了「不应用」和模板名，还要显示**当前生效的是哪一份、来自哪一层**：

```
路由模板  [ 不应用 ▾ ]
          当前生效：便宜优先（来自组织）
```

这是排查"为什么这个团队的流量分配是这个样子"的入口。不显示来源的话，一个没选模板
的团队和选了模板的组织看起来没区别，运维只能靠猜。

---

## 接口

```
GET    /route_template/list                 列表（含每个模板的使用计数）
POST   /route_template/new                  新建
GET    /route_template/{id}                 读一份
POST   /route_template/{id}/update          改
POST   /route_template/{id}/delete          删（仍被选用时拒绝，正文列出使用者）
GET    /route_template/{id}/usage           正在用它的组织、团队、密钥清单

GET    /route_template/binding?scope=&scope_id=          读一个范围的选择
POST   /route_template/binding                           写一个范围的选择
```

用 `list` / `new` / `update` / `delete` 这套后缀而不是 REST 的裸动词，是因为这个仓库
现有的管理接口全是这个形状（`/model/new`、`/team/update`、`/organization/update`），
前端那套 `networking.tsx` 也是照这个写的。

**三个范围的选择有一条独立路径，同时三个现有更新接口也接这个字段。** 两者都通，
不是重复：

- 模块页的"应用"用 `POST /route_template/binding`，那里本来就是平台管理员在挑范围。
- 组织/团队/密钥自己的编辑界面用各自现有的接口，因为那一步已经发生在这个范围的
  编辑流程里，分两次写会让一次保存变成两个请求、也可能只成功一半。

两条路最终落到同一个存储调用（`SetScopeRouteTemplate`），所以"清空是什么意思"
只有一份定义。

**权限**：模板正文（`new` / `update` / `delete` / `list`）是平台管理员的。
选择是范围自己的管理员：

| 范围 | 要过的判定 |
| --- | --- |
| 组织 | `org.admin`（组织管理员管名字和名册，这一项和它们同级） |
| 团队 | `team.write`（团队管理员管自己团队的设置） |
| 密钥 | `key.write` |

**密钥那一条还没接完。** 存储和 `KeyInput` 已经支持，`patchFrom`（`keys/generate.go`）
也已经读这个字段，但密钥的创建路径 `generate.go:382` 还没把它从正文里取出来写进
`KeyInput`，所以**今天建密钥时给 `route_template_id` 不会生效**。补齐它是一处小改动，
但它不该被算成"已完成"。

## 平台默认

三个范围都不选时用平台默认，而**平台默认不是这个表里的一行**：它就是全局那份
`router_settings`（`prefs.MergedRouter`）。今天在 `/router-settings` 编的那份配置
就是它，所以运维已经有一个地方能改。

这样做而不是把默认复制成一份模板，是因为复制会让同一份设置有两个家。计费读其中一个，
控制台显示另一个；不管哪个是真的，另一个都是谎，而"绑定到默认"和"不绑定"也会变成
两个语义相同、状态不同的东西。现在两者是同一个状态。

---
## 实施顺序

分四步，每步都能单独发布。

### 1. 表和读写（已完成）

- `route_templates` 表 + 三个表的 `route_template_id`（都带外键，已验证会拒绝悬空 id）。
- 模板的列表、读、建、改、删、使用清单、使用计数。
- 删除仍被选用的模板时拒绝，正文列出是谁在用。
- 选择的两条路径：`/route_template/binding`，以及组织/团队各自的更新接口。
- 权限：模板正文只有平台管理员；选择是范围自己的管理员，越界被拒。

**这一步不碰请求路径**，所以行为不变。控制台还没接，所以模板目前是"配了不生效"
——界面接上时必须显式标出来，不然又是一个"配了没用的设置"。

还没接完：密钥的创建路径没把字段写进 `KeyInput`（见上面接口一节）。

### 2. 请求期取一份设置

- 解析函数：一个**有序范围列表**（密钥 → 团队 → 组织），逐级**整份继承**，
  到底用平台默认。列表写成数据，项目以后要加就是插一项。
- `serve.go` 的策略、重试次数、冷却值改成读解析结果。
- **删掉 `server.go:118` 的 client 级超时**，改在 `serve.go` 用 `context.WithTimeout`。
- **冷却键加凭据段**（`cooldownKey`），无具名凭据时保持原样。

**这一步有两个行为变化，都要盯回归**：

1. 超时从"构造时冻结"变成"真的生效"——设置页里那个改过但从未生效的值会突然生效。
2. 冷却键换形状之后，**老的冷却键还在 Redis 里**。不迁移也不删是对的（它们的 TTL
   最多几分钟），但要知道切换后有一小段窗口里旧键不再被读。

这一步之后"按组织/团队/密钥选用模板"就真的可用了。

### 3. 执行回退

- 部署循环耗尽后，按**失败类**（`no_response` / `status` / `ambiguous`）选链，
  换模型组，跳数计入 `max_fallbacks`。
- 流已发首字节之后不换。
- `ambiguous` 类默认参与回退，但可以通过 `fallback_causes` 摘掉。

这是最大的一块，也是唯一一处新增"一次调用访问多个模型"的代码。

### 4. 界面

- 路由设置页改成模板列表 + 现有两个页签。
- 新增模板的编辑抽屉（含策略下拉里的 `weighted-split`）。
- 组织、团队、密钥改成选择框，去掉这三处的路由设置表单。
- 下拉里显示"当前生效：X（来自 Y）"。

## 测试

**解析**

- 三处都不选 → 平台默认，且一次调用的策略、重试次数、超时、冷却值和今天**逐项相同**。
- 只给组织选了 `lowest-cost` → 该组织下没另选的团队和密钥走最低成本。
- 组织选了 A、团队没选 → 团队用 A（**继承，不是跳回默认**）。
- 组织选 A、团队选 B → 只有这个团队用 B。
- 密钥选 C、团队选 B → 密钥用 C。
- 生效的那一份**整体生效**：断言策略来自 B、重试次数也来自 B，不是两者混合。

**模板读写**

- 删除仍被选用的模板 → 接口拒绝，响应里列出绑在哪。
- 改一份模板 → 所有选它的范围立刻按新内容走；没选它的范围不变。
- 平台默认那一行：能读能改，不能删、不能改名。
- 组织管理员改不了模板正文；团队管理员能给自己团队选，改不了别人的。

**冷却键**

- 两个团队各用**自己的**具名凭据打同一个上游：甲被限流冷却之后，乙**仍然能调**
  （这条今天**是红的**——两个团队共用 `api_base|model` 一个键）。
- 两个团队共用**同一个**具名凭据：甲被限流，乙也不选这条部署（共享上游约束，正确）。
- 没有任何具名凭据的部署：冷却行为和今天逐项相同（键没变）。

**回退**

- A 失败（500）且重试耗尽 → 换到 `fallbacks` 里的 B 并成功；响应里的模型名是 B。
- A 返回 400 → **不换**，400 原样透传，**只计费一次**。
- `fallback_causes` 摘掉 `ambiguous` → 超时**不换**，直接 502；保留则换。
- 上下文超限触发的是 `context_window_fallbacks`，不是 `fallbacks`
  （配两条不同的链，断言走的是哪一条）。
- `max_fallbacks: 1` 时两跳就停，第 3 个模型**一次都没被访问**。
- `fallbacks` 为空时不回退——和今天一致。
- 429 在重试耗尽后按模板的 `fallbacks` 换模型组。

**超时**

- `timeout` 在**运行中**改掉之后新请求生效。这条测试今天**是红的**，
  因为 client 超时冻结。

**兼容**

- 不选任何模板时，行为与今天逐项相同。这是整次改动最重要的一条。

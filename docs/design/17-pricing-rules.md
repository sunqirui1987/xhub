# 17 计价规则

这一版重做计价的数据结构和计费路径。它解决的第一个问题不是显示，是**正在漏钱**。

## 现在的问题

市场价目表里，DeepSeek 有四个模型按**高峰/空闲**两档报价：

```
ncache_offpeak   0.00065217  非缓存输入（空闲）
ncache_peak      0.00130435  非缓存输入（高峰）   ← 正好两倍
output_offpeak   0.00195652  输出（空闲）
output_peak      0.00391304  输出（高峰）        ← 正好两倍
cache_offpeak    0.00002174  缓存输入（空闲）
cache_peak       0.00004348  缓存输入（高峰）     ← 正好两倍
```

当前的生成器只认最便宜的变体（兜底逻辑），结果是：

- **高峰时段按空闲价收费**，少收一半。北京时间周一到周五 9:00-12:00 和 14:00-18:00 正是用量高峰。
- 控制台的日志详情里，`original_cost` 是按**当前**价目表重算的，所以它显示的数字和实际扣的不一样，而且随时段变化——同一行日志，上午点开和晚上点开显示不同的"原价"。

第一件是收入问题，第二件是可信度问题。

## 三件事要分开

| | 是什么 | 现状 |
| --- | --- | --- |
| **计价规则** | 这条模型怎么算钱：按 token、按张、按秒，带不带时段 | `rates[]` 数组，四个维度 ✓ |
| **计费时刻** | 这次调用发生在哪个时段 | `IsPeakHour(t)`，计费路径已经接上 ✓ |
| **账目存证** | 这次调用当时按什么价扣的 | `price_snapshot` 列，日志详情读它 ✓ |

## 新的数据结构（已实现）

### `rates[]` 数组

每条模型的价格行里，新增一个 `rates` 字段，是一组费率条目：

```json
{
  "id": "deepseek/deepseek-v4-pro-0813",
  "rates": [
    {
      "measure":    "token",
      "unit_size":  1,
      "side":       "input",
      "variant":    "uncached",
      "window":     "offpeak",
      "source_key": "ncache_offpeak",
      "label":      "非缓存输入（空闲）",
      "usd":        0.00065217
    },
    {
      "measure":    "token",
      "unit_size":  1,
      "side":       "input",
      "variant":    "uncached",
      "window":     "peak",
      "source_key": "ncache_peak",
      "label":      "非缓存输入（高峰）",
      "usd":        0.00130435
    }
  ]
}
```

四个维度，覆盖市场接口能给出的全部信息：

- **measure**：`token`、`second`、`picture`、`query`。来自 `unit_name`。
- **unit_size**：一个计价单位包多少个基础单位。token 类 ÷1000；picture 和 second 是 1。`usd` 已经除完，是**每一个**基础单位的价格。
- **side**：`input`、`output`、`cache_read`、`cache_write`、`batch_input`、`batch_output`。
- **variant**：同一侧内部的限定词。现在已收录的有：
  - `uncached` / `cached`（非缓存/缓存输入，DeepSeek）
  - `non_thinking` / `thinking`（思考/非思考，qwen）
  - `text` / `image`（文本输入、图片输入，gpt-image）
  - `wiv` / `woiv`（含/不含视频输入，vidu）
  - `480p` / `720p` / `1080p` / `4k`（分辨率，视频模型）
  - `t2v` / `i2v` / `r2v`（文生/图生/参考主体生）
  - 空串 = 无限定词
- **window**：`peak` / `offpeak` / `all`。**只有带 peak/offpeak 的模型才需要在计费时做时段判定。**

旧的扁平字段（`input_cost_per_token`、`output_cost_per_token` 等）**同时保留**，取的是最便宜的一档。旧读者不断；新的计费路径改用 `rates[]`。

### 为什么不是继续加扁平字段

扁平方案要加 `input_cost_per_token_peak`、`input_cost_per_token_offpeak`、`cache_read_input_token_cost_peak`…… 每加一个维度是乘法增长。而且分辨率、思考与否、输入模态这十几个变体根本没有对应的字段名可加。

## 计费时刻（已实现）

时段定义（来自 DeepSeek，同样适用于任何带 window 维度的供应商）：

> 空闲时段价格为高峰时段价格的一半。
> 北京时间周一至周五（不含中国法定节假日）9:00-12:00、14:00-18:00 为高峰时段；
> 其余时段，包括周末及中国法定节假日全天均为空闲时段。

实现在 `internal/catalog/holiday.go`：

```go
// IsPeakHour 报告时刻 t 是否落在高峰计费窗口内。
func IsPeakHour(t time.Time) bool
```

整点处理（半开区间）：09:00 是高峰；12:00 不是；14:00 是；18:00 不是。用调用**开始**时刻，不是结束——跨边界的调用有唯一答案，且与用量行的 `start` 字段对得上。

两个关键：

1. **时区**：显式 `Asia/Shanghai`，不用进程时区。Asia/Shanghai 不可用时退回 UTC+8 固定偏移（中国自 1991 年起不再有夏令时）。
2. **中国法定节假日表**：嵌在 `internal/catalog/publicdata/cn_holidays.json`，格式来自 [holiday-cn](https://github.com/NateScarlet/holiday-cn)。调休上班的周末算高峰；节假日的工作日算空闲。2026 年共 33 天强制休息日 + 6 天调休上班日。节假日表每年更新一次（手动），不引入外部 API 依赖。

已测试边界：
- 普通周三 10:00 → 高峰
- 普通周六 10:00 → 空闲
- 元旦（2026-01-01）10:00 → 空闲
- 春节（2026-02-17，周二）10:00 → 空闲
- 调休补班（2026-01-04，周日）10:00 → 高峰
- 春节调休（2026-02-14，周六）10:00 → 高峰
- 12:00 整点 → 空闲（不在 [09:00,12:00) 内）
- 午休 13:00 → 空闲

## 计费路径（已实现）

`catalog.CostAt(model, usage, startedAt)` 按用量和时刻算钱：

```go
type Usage struct {
    PromptTokens     int
    CompletionTokens int
    CachedTokens     int   // 提示侧命中的缓存，是 PromptTokens 的子集
    CacheWriteTokens int
    Images           int
    Seconds          float64
    Searches         int
}

// CostAt 在给定时刻按 rates[] 计价。时段只由 startedAt 决定。
func CostAt(model string, usage Usage, startedAt time.Time) (Charge, bool)
```

`Charge` 除了总额和两侧明细，还带 `Window` 和 `Applied`——这次实际用到的那几条费率。`Applied` 就是快照的内容。

**解析规则是按特异性降级，绝不按价格挑。** 按价格挑正是改动前的毛病：高峰和空闲两档都在表里，便宜的那档赢了，于是高峰调用按空闲价收。现在时段先决定，只有**变体**可以降级：

```
提示侧：先找 (token, input, cached, window)，再找 (token, cache_read, "", window)，
        都没有才把整段提示按 (token, input, uncached, window) 收
输出侧：(token, output, "", window)
时段：  先用调用所在的 window，再用 all（不分时的模型只有 all）
```

每条数量只收一次。缓存命中是提示 token 的**子集**而不是另一笔，所以命中那部分按缓存价、剩下的按输入价，加起来正好是提示总数。

实测（`internal/regression/pricing_test.go`）：同一批 token，高峰那次正好是空闲那次的 2 倍；缓存占大头的调用不会被按输入价收；按秒的模型不再记零费用。

## 账目存单价（已实现）

用量行新增 `price_snapshot` 列，存这次实际用到的那几条费率：

```json
{
  "window": "peak",
  "applied": [
    {"measure": "token", "side": "input", "variant": "uncached",
     "unit_size": 1000, "usd": 0.00130435, "quantity": 1500, "source_key": "ncache_peak"}
  ]
}
```

日志详情 `costBreakdown` 读它，不重算。历史行没有这个列时退回重算，并在响应里标明 `source: "recomputed"`——不假装它是原始记录。

实测：同一次调用读两遍，中间把价改成十倍，数字不动；而改价之后**新发生**的调用按新价收（否则"没动"可能是因为计费根本没读价）。

## 控制台怎么显示（已实现）

`/price-data` 的卡片按 `measure` 分组，一组一个维度标题，组内按时段并排成列：

```
按 token 计费
  费率        不分时段
  缓存读      $0.0927 /1M
  输入 · 非缓存  $0.4638 /1M
  输出        $0.4638 /1M

按 token 计费
  费率              不分时段
  输出 · 含视频输入 · 1080P   $11.7 /1M
  输出 · 不含视频输入 · 1080P  $7 /1M
```

三个细节：

- **分时价并排显示**。它们是一件事的两面，折起来看不出差一倍。卡片上方的"输入/输出"两格对分时模型会标出"(空闲)"，因为它们取的是最便宜那一档。
- **变体翻成词**，不直接把市场键名摆上去。`1080p_wiv_v` 是键名不是文案；认得的一个个翻，分辨率和认不出的原样留着（宁可露出一个生词，也不要猜错意思）。
- **旧行仍能看**。没有 `rates[]` 的条目（扁平字段时代的）走"更多计费项"兜底，两者不重复显示同一条价。

## 同名多部署的分流

一个对外名挂两条不同供应商的部署，按 `litellm_params.weight` 分配流量。这是第一次接新供应商时灰度切换的用法。

原来 `simple-shuffle` 不是随机的，是取权重最大的那条，而权重默认 1，于是永远命中第一条——配了比例也不生效。现在新增 `weighted-split` 策略做平滑加权轮询（nginx 那套）：7:3 跑十次正好是 7 和 3，不是"长期平均"。

它是**自己的策略**，`simple-shuffle` 的语义不变——后者是另外六个别名的实现，回归里钉着它选权重最大者。

两条要说明的边界：

- 会话钉住（`affinity.go`）的请求**绕过**比例，直接走钉住的部署。这是对的，会话必须留在原部署；但要说明，会话密集的负载观测到的比例会偏。
- 掉线的部署拿不到流量，它的游标会被清掉，恢复时从零开始，而不是立刻把掉线期间攒的份额全领走。

## 落地顺序

1. ✅ **数据结构**：`rates[]` 数组进 pricedata.json；旧扁平字段保留（取最便宜档）供旧读者。
2. ✅ **时段判定**：`IsPeakHour(t)` + holiday-cn 节假日表，13 个边界测试。
3. ✅ **计费**：`CostAt` + `Usage`，`spend.go` 改调用点，`price_snapshot` 落库。
4. ✅ **日志详情**：读快照，不再重算；没有快照的旧行标明是重算的。
5. ✅ **控制台**：卡片按 measure 分组，分时价并排。
6. ✅ **分流**：`weighted-split` 策略，同名多部署按权重分流量。

还留着两件事，都不在这一版里：

- **按张、按次计费的落库**。`usage_events` 目前只有 token 三列，没有图片数、秒数、搜索次数的位置。按秒的模型已经能算钱了（`Usage.Seconds` 从上游用量里读），但那些数量没有单独的列可以查，只能从 `price_snapshot` 里看。
- **控制台编辑 `rates[]`**。现在控制台只能编辑扁平的 8 个字段，分时价要从配置文件或接口写。

## 已实现的文件

**计价的数据结构**

| 文件 | 内容 |
| --- | --- |
| `internal/catalog/rates.go` | `Rate` struct、`buildRates()`、`knownKeys` 表、`inferRateSpec()` |
| `internal/catalog/holiday.go` | `IsPeakHour()`、`parseHolidays()`、`shanghaiLoc` |
| `internal/catalog/publicdata/cn_holidays.json` | 2026 年节假日表（holiday-cn 格式） |

**计费路径**

| 文件 | 内容 |
| --- | --- |
| `internal/catalog/cost_at.go` | `Usage`、`Charge`、`CostAt()`、`rateLookup` 偏好链、`Snapshot()` |
| `internal/gateway/spend.go` | `callCost` 接收时刻和用量；部署级费率表；`usageOf()` 读缓存和按秒按张的数量 |
| `internal/gateway/usage/reports.go` | `costBreakdown` 读快照；`Calculate` 按当前时刻估价 |
| `internal/iam/schema.sql` | `price_snapshot` 列 |
| `internal/live/redis.go`、`internal/iam/usage.go`、`internal/dataplane/live.go` | 快照沿两条落库路径带到 `usage_events` |

**控制台**

| 文件 | 内容 |
| --- | --- |
| `frontend/src/lib/rateDisplay.ts` | 侧、变体、维度、时段、单价的显示词汇。**价目表和计费日志共用这一份**，否则同一条费率在两处叫两个名字 |
| `frontend/src/app/(dashboard)/models-and-endpoints/components/priceCatalogRows.ts` | `CatalogRate`、`RateGroup`、`rateGroupsOf()` |
| `frontend/src/app/(dashboard)/models-and-endpoints/components/PriceCatalog.tsx` | 按维度分组、按时段并排的费率表 |
| `frontend/src/components/view_logs/CostBreakdownViewer.tsx` | 时刻标记、实际费率表、重算标记；输入成本不再可能为负 |
| `frontend/src/components/add_model/billing_categories.ts` | 添加模型时按计费维度分组的费率输入 |

**分流**

| 文件 | 内容 |
| --- | --- |
| `internal/router/split.go` | `SplitState`、`PickWeighted`（平滑加权轮询） |
| `internal/router/router.go` | `split` 策略；`cost` 策略不再让未定价的部署靠声明顺序胜出 |

**测试**

| 文件 | 内容 |
| --- | --- |
| `internal/catalog/cost_at_test.go` | 时段、缓存、按秒、按张、快照各自的边界 |
| `internal/catalog/holiday_test.go` | 13 个时段边界（含节假日和调休） |
| `internal/router/split_test.go` | 比例、冷却恢复、单条部署、不回退成最高权重 |
| `internal/gateway/usage/cost_breakdown_test.go` | 快照优先、改价不改历史、按张不编造 token 价 |
| `internal/regression/pricing_test.go` | 端到端：时段两侧一致、按秒不记零、快照不随改价移动、缓存不按输入价、未定价不假装免费 |
| `internal/regression/split_test.go` | 端到端：7:3 正好 7 和 3、simple-shuffle 不变、每次调用都记账 |

# 响应缓存

- Route: `/caching`
- Nav: DEVELOPER TOOLS
- Status: `specified`

## 目的

配置网关响应缓存（Redis / in-memory 等），看命中分析与健康探测，以及 Coordination Redis。这里是 LiteLLM response cache，不是供应商 prompt caching（那在 Usage / Logs）。

## 布局

登录后控制台壳。页内 `p-8`，顶一条线型 Tab + 右侧 Last Refreshed 与刷新图标。

### 顶栏

无独立 PageHeader 标题。Tab 行右侧：`Last Refreshed: {locale time}` + 刷新按钮（重拉 analytics）。

### 筛选

仅 Cache Analytics Tab：Virtual Keys 多选、Models 多选（选项来自 activity `filter_options`）、日期范围（默认近 7 天）。筛选不写 URL。

### 表

Analytics：无资源表。三张卡（Cache Hit Ratio、Cache Hits、Cached Completion Tokens）+ 两张柱状图（Hits vs API Requests；Cached vs Generated Completion Tokens）。点红色 Failed requests 段展开 `ErrorDrilldownCard`（按 error code）。Health：Ping 结果 Summary / Raw 表（host、port、namespace、ping_response）。Settings / Coordination Redis：动态字段表单，不是 CRUD 表。

### Tab

默认 `analytics`：

| Tab | 内容 |
|---|---|
| Cache Analytics | 筛选 + 指标卡 + 图 + 错误下钻 |
| Cache Health | Run health check → Summary / Raw Response |
| Cache Settings | Redis 类型（node/cluster/sentinel/semantic）+ 连接字段 + Test Connection / Save；高级折叠 SSL / cacheManagement / GCP |
| Coordination Redis | 独立 Redis 表单：读 `/coordination_redis/settings`，Test / Save |

Tab `keepMounted`。

### 抽屉

无。错误下钻是页内卡片，点关闭收回。

### 模态

无创建模态。保存/测试失败走 toast。本页不提供 Flush all 按钮（契约里的 `/cache/flushall` 未挂在此 UI）。

### URL

`/caching`。Tab 与日期不写入 URL。

## 交互

进入默认 Analytics，拉 `GET /global/activity/cache_hits`（`start_date`/`end_date`/`key_aliases`/`models`）。改筛选即重查。刷新更新 Last Refreshed。Health：点 Run 调 `GET /cache/ping`，结果填 Summary（healthy/unhealthy、Redis host/port/version）。Settings：`GET /cache/settings` 填表；密码字段后端返回 `***REDACTED***` 不回填明文；Test 走 `POST /cache/settings/test`；Save 走 `POST /cache/settings`。Coordination Redis 同模式。语义缓存可选 embedding 模型（`GET /model_group/info` 里 `mode=embedding`）。校验失败贴在字段旁，不提交。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 命中分析 | `GET /global/activity/cache_hits` | 卡 + 图 |
| Ping | `GET /cache/ping` | Health Summary / Raw |
| 读缓存设置 | `GET /cache/settings` | 填表 |
| 测连接 | `POST /cache/settings/test` | toast 成功/失败 |
| 保存缓存 | `POST /cache/settings` | toast |
| 读 Coordination Redis | `GET /coordination_redis/settings` | 填表 |
| 测 Coordination Redis | `POST /coordination_redis/settings/test` | toast |
| 保存 Coordination Redis | `POST /coordination_redis/settings` | toast |

## 字段

| 字段 | 含义 |
|---|---|
| `type` / redis_type | node / cluster / sentinel / semantic |
| `host` / `port` / `db` | Redis 连接 |
| `url` | 完整 redis URL，优先于 host/port |
| `password` / `username` | 凭证；已配置时表单不回填明文 |
| `ttl` | 缓存秒 |
| `namespace` | 键前缀 |
| `cache_hit_ratio` | Analytics 百分比 |
| `cache_hits` / `api_requests` / `failed_requests` | 图系列 |

## 状态

loading：图与卡等 query。empty：区间内无请求，Hit Ratio 显示 `0%`。forbidden：无登录。校验失败：字段红字可改后重试。超时：toast，表单锁定原 payload。Health 失败：Summary 展示 error.message + traceback，不假装 healthy。

## 验收

四个 Tab 都能走通读/测/保存；Analytics 筛选会改图；Ping 失败展示真实错误；桌面与窄屏；不得 mock 表格。

# iam

## 这个模块做什么

`iam` 是身份存储：账号、组织、团队、成员关系、项目、访问组、虚拟密钥、用量、请求日志和审计。`schema.sql` 负责约束（复合外键、CHECK、邮箱大小写不敏感），xorm 负责全部读写。这里**没有**从旧的 JSON 成员结构迁移的路径，也没有兼容层：数据库整体重建，这份 schema 就是全部。

成员关系只从 `team_members` 读。别处不存在成员的副本，这也是「移除成员在下一个请求就生效」而不是「下次登录才生效」的原因。

## 权限模型

只有两个账号角色和两个团队角色，没有别的：

| 作用域 | 取值 |
| --- | --- |
| 账号（`users.role`） | `admin`、`user` |
| 团队（`team_members.role`） | `team_admin`、`member` |

资源层级是 **组织 → 团队 → 项目**。组织只由平台管理员维护。用户只通过团队成员关系获得权限：没有组织成员、没有项目管理员、没有访问组角色。

一次推理调用的归属在写入时**快照**到 `usage_events`。因此 `usage_events` 完全不带外键：成员离开团队不会带走他已经产生的用量，删除团队也不会抹掉历史。

## 文件

| 文件 | 内容 |
| --- | --- |
| `schema.sql` | 全部表、索引与约束，由 `Migrate` 执行。 |
| `db.go` | `Open`、`Migrate`、事务封装、错误映射、审计写入。 |
| `models.go` | 表 bean 与角色/状态常量。 |
| `users.go` | 账号：创建、读取、资料、密码、管理员修改、删除、`EnsureAdmin`。 |
| `teams.go` | 组织、团队、成员关系、项目、访问组分配。 |
| `keys.go` | 虚拟密钥及其收窄、访问组、模型解析函数。 |
| `usage.go` | 用量写入路径：事件、请求日志、日汇总、实时花费。 |
| `usage_read.go` | 用量与请求日志的分权限读取 API。 |

## 模型集合

`allowedModels` 是模型目录与推理路径唯一使用的函数。每个作用域只归结到一个团队，能力**从不**跨团队合并：

```
team    = 该团队有效访问组的并集
project = team ∩ 项目收窄列表      （作用域指明项目时）
key     = (project 或 team) ∩ 密钥收窄列表   （作用域指明密钥时）
```

两个包装函数：

- `AllowedModelsForTeam(ctx, teamID)` —— 团队集合，密钥还不存在时用
- `AllowedModelsForKey(ctx, k)` —— 密钥的有效集合

项目或密钥上的收窄列表为空表示**继承**，不是**拒绝**。空集合同样不等于拒绝：那只是还没分配过。

## 读用量与日志

`usage_events` 和 `usage_daily` 通过 `UsageQuery` 读取，它接收一个由 `authz.UsageScope` 或 `authz.LogsScope` 生成的 `builder.Cond`。权限条件**先**生效，其余过滤只在其内部进一步收窄，因此从 query string 传入的 `user_id` 或 `api_key` 永远无法放宽读取范围。本包不导入 `authz`，条件由调用方传入，这样策略只存在一处，查询层保持干净。

| 函数 | 返回 |
| --- | --- |
| `ListUsage` | 权限内的事件，最新在前 |
| `CountUsage` | 总数，用于分页 |
| `GetUsageEvent` | 权限内的单个事件；权限外返回 `ErrNotFound` |
| `GetRequestLog` | 单个事件保存的请求/响应体 |
| `DailyUsage` | 按天的汇总 |
| `DailyUsageByModel` | 按天与模型的汇总 |
| `RollupByModel` | 按模型的合计 |
| `RollupByKey` | 按密钥的合计，带展示名 |
| `RollupByTeam` | 按团队的合计 |
| `AuditLogRead` | 写「读取他人日志」的审计行 |

## 其它包怎么用

```go
db, err := iam.Open(ctx, cfg.GeneralSettings.DatabaseURL)
if err != nil {
    log.Fatal(err)
}
defer db.Close()

p, err := auth.Resolve(ctx, cfg, db, r)          // 谁在调用
g, err := authz.New(db).Guard(ctx, p.Actor())    // 他能做什么
sc, err := g.UsageScope(ctx, teamID)             // 他能读哪些行
rows, err := db.ListUsage(ctx, iam.UsageQuery{Cond: sc.Cond, Limit: 50})
```

新增查询时有两个坑必须记住：

- **读投影必须带 xorm tag**。xorm 按它自己的 `xorm:` tag 映射结果列，**绝不**按 `json:`。只有 json tag 的结构体会把每一列都扫成零值，而且不报错。
- `models` 列是 `TEXT` 类型，存的是 JSON 数组。要投影到带 `xorm:"json 'models'"` 的结构体；直接投影到 `[]string` 会把整个 JSON 文档当成一个元素返回。

## 初始管理员

`EnsureAdmin` 在该邮箱还没有账号时，按配置创建一位平台管理员，并返回是否创建了。它**从不**更新已有行：配置里的密码是初始密码，改配置文件不会重置线上账号。`Bootstrapped` 是前端读取的标记，创建流程与 `POST /bootstrap` 都会写它。

## 这个包不做什么

它不做鉴权判定。所有判定——谁能读谁的用量、谁能管理哪个团队——都在 `authz`，本包只负责应用最终得到的过滤条件。它也不签发或校验会话令牌，那是 `auth`。

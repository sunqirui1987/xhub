# 框架配置与模型存储

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

store.go/engine.go/pg.go 创建 PostgreSQL xorm 存储，beans.go 定义模型、配置和 KV 行，crud.go 提供通用操作，models.go/keys.go 提供对应读写辅助。数据库只支持 PostgreSQL，缓存是框架读缓存，不是身份权限缓存。
这里保存模型部署、general/router 配置和框架资源；账号、成员关系、虚拟密钥与真实用量归 IAM。gateway 调用 Store 前已经执行授权，但读取失败仍必须明确处理。
通用 KV 成功不表示某兼容 API 有真实业务实现。避免把未实现功能挂到通用 KV 然后宣称完成。没有直接单测文件，模型和设置 HTTP 回归覆盖部分集成行为；连接、CRUD 错误及并发缓存的全部分支仍需专门验证。

## 源码职责与入口

### beans.go

- [`func (kvRow) TableName() string`](beans.go) — TableName returns the key-value table name.
- [`func (proxyModelRow) TableName() string`](beans.go) — TableName returns the proxy-model table name.
- [`func (configRow) TableName() string`](beans.go) — TableName returns the proxy-config table name.

### crud.go

- [`func (s *Store) PutKV(kind, id, body string) error`](crud.go) — PutKV writes one key-value row. An existing row is updated and a missing row is inserted.
- [`func (s *Store) GetKV(kind, id string) (map[string]any, error)`](crud.go) — GetKV reads one key-value row by kind and id.  The read bypasses the query cache. A session or credential revoked by another gateway process must be observed here, and local cache invalidation cannot provide that guarantee.
- [`func (s *Store) DeleteKV(kind, id string) error`](crud.go) — DeleteKV deletes one key-value row and clears the cache. A missing row returns a no-rows error.
- [`func (s *Store) ListKV(kind string) ([]map[string]any, error)`](crud.go) — ListKV lists every key-value row of one kind.

### engine.go

内部实现和协议边界见 [engine.go](engine.go)。

### keys.go

- [`func (s *Store) UpsertProxyModel(m ProxyModel) error`](keys.go) — UpsertProxyModel updates a proxy model by id, or inserts it when the row is missing.
- [`func (s *Store) ListProxyModels() ([]ProxyModel, error)`](keys.go) — ListProxyModels lists every proxy model. A nil store has no models, which is the same answer as an empty table.
- [`func (s *Store) DeleteProxyModel(id string) error`](keys.go) — DeleteProxyModel deletes a proxy model by id and clears the cache.
- [`func (s *Store) PutConfig(namespace, key string, value any) error`](keys.go) — PutConfig writes one namespaced configuration row. An existing row is updated and a missing row is inserted.
- [`func (s *Store) DeleteConfig(namespace, key string) error`](keys.go) — DeleteConfig deletes one namespaced configuration row and clears the cache.
- [`func (s *Store) ListConfig(namespace string) (map[string]any, error)`](keys.go) — ListConfig reads every configuration row in one namespace. A nil store has no configuration, so a caller that overlays it keeps the YAML baseline instead of crashing on a deployment that runs without framework records.

### models.go

公开类型：`ProxyModel`.

内部实现和协议边界见 [models.go](models.go)。

### pg.go

内部实现和协议边界见 [pg.go](pg.go)。

### store.go

公开类型：`Store`.

- [`func Open(databaseURL string) (*Store, error)`](store.go) — Open opens storage. sqlite, file:, and an empty URL are rejected. Only postgres:// and postgresql:// are accepted.

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../logx/readme_cn.md).

## 验证与维护入口

当前目录没有直接测试文件；上层集成测试仅证明被执行的链路，不代表所有内部失败分支均已覆盖。

```bash
go test ./internal/store -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。

# Framework configuration and deployment storage

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

store, engine, and pg initialize PostgreSQL xorm storage; beans define framework rows; crud supplies generic operations; model/key helpers provide storage access. Framework read caching is separate from authentication state.
This store owns deployments, general/router configuration, and framework resources. IAM owns accounts, memberships, virtual credentials, and authoritative usage. Gateway callers authorize before access and still propagate storage errors.
A successful generic KV operation does not implement a compatibility API's business semantics. There are currently no direct test files; model/settings HTTP regression verifies selected integrations without proving all connection, CRUD failure, or cache concurrency branches.

## Source responsibilities and entry points

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

Internal implementation and protocol boundaries:  [engine.go](engine.go)。

### keys.go

- [`func (s *Store) UpsertProxyModel(m ProxyModel) error`](keys.go) — UpsertProxyModel updates a proxy model by id, or inserts it when the row is missing.
- [`func (s *Store) ListProxyModels() ([]ProxyModel, error)`](keys.go) — ListProxyModels lists every proxy model. A nil store has no models, which is the same answer as an empty table.
- [`func (s *Store) DeleteProxyModel(id string) error`](keys.go) — DeleteProxyModel deletes a proxy model by id and clears the cache.
- [`func (s *Store) PutConfig(namespace, key string, value any) error`](keys.go) — PutConfig writes one namespaced configuration row. An existing row is updated and a missing row is inserted.
- [`func (s *Store) DeleteConfig(namespace, key string) error`](keys.go) — DeleteConfig deletes one namespaced configuration row and clears the cache.
- [`func (s *Store) ListConfig(namespace string) (map[string]any, error)`](keys.go) — ListConfig reads every configuration row in one namespace. A nil store has no configuration, so a caller that overlays it keeps the YAML baseline instead of crashing on a deployment that runs without framework records.

### models.go

Exported types: `ProxyModel`.

Internal implementation and protocol boundaries:  [models.go](models.go)。

### pg.go

Internal implementation and protocol boundaries:  [pg.go](pg.go)。

### store.go

Exported types: `Store`.

- [`func Open(databaseURL string) (*Store, error)`](store.go) — Open opens storage. sqlite, file:, and an empty URL are rejected. Only postgres:// and postgresql:// are accepted.

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../logx/readme.md).

## Verification and maintenance

There are no direct test files here. Integration tests prove executed paths rather than every internal failure branch.

```bash
go test ./internal/store -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.

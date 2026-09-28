# store

## 这个模块做什么

`store` 是 PostgreSQL 持久层。用户、团队、组织、项目、预算、虚拟密钥、花费日志、控制台保存的模型，以及键值配置，都是这里的行。`Open` 拒绝 SQLite 和空的数据库地址。

## 功能

- `Open` 连接数据库，同步表结构，返回 `Store`。
- 用户、团队、组织、项目和预算有插入、列表、读取、更新和删除。花费累加在并发增量下不会丢更新。
- `NewPlainKey` 生成虚拟密钥明文。`InsertKey` 只存哈希。明文不保留。
- `HashPassword` 和 `CheckPassword` 用 bcrypt 保存控制台密码。
- `PutKV` 和 `GetKV` 存放 JSON 文档，例如路由设置覆盖和凭证。
- `ApplySpendBatch` 写入一批来自 Redis 的花费。重放同一批不会再次累加。
- `ProxyModel` 是控制台保存的模型。同名行覆盖 YAML。

## 其它包怎么用

导入 `github.com/sunqirui1987/xhub/internal/store`。

```go
st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
if err != nil {
    log.Fatal(err)
}
plain := store.NewPlainKey()
hash := store.HashKey(plain)
err = st.InsertKey(store.Key{TokenHash: hash, KeyAlias: "ci", UserID: "admin"})
// 明文只返回给调用方一次，不要写进日志
user, err := st.GetUser("admin")
team, err := st.GetTeam("team_19b51fd9ce95")
```

传入 `postgres://` 地址。测试用 `search_path` 隔离到自己的 schema。生产用 `configs/config.yaml` 里的库。

## 这个包不做什么

它不提供 HTTP。identity、keys 和 models 的处理函数调用这些方法。

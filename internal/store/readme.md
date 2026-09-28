# store

## Purpose

`store` is the PostgreSQL persistence layer. Users, teams, organizations, projects, budgets, virtual keys, spend logs, dashboard models, and key-value configuration are rows here. SQLite and an empty database URL are rejected by `Open`.

## Features

- `Open` connects, syncs the tables, and returns a `Store`.
- Users, teams, orgs, projects, and budgets have insert, list, get, update, and delete methods. Spend helpers add a delta without losing concurrent updates.
- `NewPlainKey` creates a virtual-key secret. `InsertKey` stores the hash. The plaintext is not kept.
- `HashPassword` and `CheckPassword` store dashboard passwords as bcrypt hashes.
- `PutKV` and `GetKV` store JSON documents such as router-settings overlays and credentials.
- `ApplySpendBatch` writes a Redis spend batch. Replaying the same batch does not add the amount twice.
- `ProxyModel` is a model saved from the dashboard. A row with the same name overrides YAML.

## How another package uses it

Import `github.com/sunqirui1987/xhub/internal/store`.

```go
st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
if err != nil {
    log.Fatal(err)
}
plain := store.NewPlainKey()
hash := store.HashKey(plain)
err = st.InsertKey(store.Key{TokenHash: hash, KeyAlias: "ci", UserID: "admin"})
// return plain to the caller once; do not log it
user, err := st.GetUser("admin")
team, err := st.GetTeam("team_19b51fd9ce95")
```

Pass a `postgres://` URL. Tests set `search_path` to a private schema. Production uses the database named in `configs/config.yaml`.

## What this package does not do

It does not expose HTTP. The identity, keys, and models packages call these methods from their handlers.

中文使用说明见同目录的 readme_cn.md。

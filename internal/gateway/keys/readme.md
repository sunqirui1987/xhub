# gateway/keys

Virtual-key routes. Mounted as `keys.Module` before the catalog.

POST `/key/generate` creates a personal key for the caller unless the body sets another owner. The plaintext token is returned once, prefixed `sk-`. What is stored is `iam.HashKey` of that plaintext. POST `/key/service-account/generate` creates a service key owned by a team or a project. A team administrator can narrow `models` down to a subset of the team allow-list and cannot widen it.

`hashKeyToken` hashes values that start with `sk-` and passes a hash or an id through. `lookupKey` tries the hash first (`KeyByHash`) and then the key id, because the console sends the id and LiteLLM-compatible clients send the plaintext.

GET `/key/list` is narrowed in SQL by `KeysScope`, not filtered after the rows are read. A platform administrator sees every key. A team administrator sees their own personal keys plus the service keys of teams they administer. A member sees their own personal keys. The master credential gets a deny-all scope so a forgotten authorize check cannot turn it into a cross-tenant list.

POST `/key/block`, `/key/unblock`, `/key/delete`, `/key/update`, `/key/regenerate` authorize `ActionKeyWrite` on that key id. Regenerate rotates the stored hash. POST `/key/{key}/reset_spend` zeros the key's spend counter. It does not rewrite `usage_events`.

`key_alias` is the display name. RPM and TPM limits on the key are enforced later by `gateway/limits.go` against Redis keys `xhub:rpm:<Principal.Hash>` and `xhub:tpm:<Principal.Hash>`, not against `api_base|model`.

## What this package does not do

It does not accept the key on `/v1/chat/completions`. That is `auth` plus `dataplane.Serve`. It does not count in-flight calls.

中文说明见同目录 `readme_cn.md`。

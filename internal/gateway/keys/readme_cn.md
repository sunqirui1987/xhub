# gateway/keys

虚拟密钥的路由。以 `keys.Module` 挂在目录之前。

POST `/key/generate` 默认给调用方建一把个人密钥，除非正文指定了别的属主。明文令牌只返回一次，前缀是 `sk-`。库存的是这段明文的 `iam.HashKey`。POST `/key/service-account/generate` 建一把属于团队或项目的服务密钥。团队管理员只能把 `models` 收窄到团队允许名单的子集，不能放宽。

`hashKeyToken` 对 `sk-` 开头的值做哈希，已经是哈希或 id 的原样通过。`lookupKey` 先按哈希查（`KeyByHash`），再按密钥 id 查，因为控制台传 id，LiteLLM 兼容客户端传明文。

GET `/key/list` 由 `KeysScope` 在 SQL 里收窄，不是查出之后再滤。平台管理员看见全部密钥。团队管理员看见自己的个人密钥，加上所管理团队的服务密钥。成员只看见自己的个人密钥。主密钥得到恒假范围，这样漏掉一次授权也不会变成跨租户列表。

POST `/key/block`、`/key/unblock`、`/key/delete`、`/key/update`、`/key/regenerate` 对这个密钥 id 做 `ActionKeyWrite`。重新生成会轮换库存哈希。POST `/key/{key}/reset_spend` 把这把密钥的花费计数清零。它不改写 `usage_events`。

`key_alias` 是显示名。密钥上的 RPM 和 TPM 限额稍后由 `gateway/limits.go` 对 Redis 键 `xhub:rpm:<Principal.Hash>` 和 `xhub:tpm:<Principal.Hash>` 执行，不是对 `api_base|model`。

## 这个包不做什么

它不在 `/v1/chat/completions` 上验收密钥。那是 `auth` 加上 `dataplane.Serve`。它也不计在途调用。

English notes are in `readme.md` in this directory.

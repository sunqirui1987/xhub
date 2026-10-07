# hooks

`hooks` 只数这一台网关进程里，一把虚拟密钥已经有多少个调用还没返回。它不决定这次调用能不能发出去。

## 入口

`gateway/server.go` 组装进程时调用 `hooks.New()`，把引擎放在 `Server.Hooks`。数据面不自己造一份。`Serve` 通过 `Host.HookEngine` 拿到这一份。

`New` 一定返回非 nil 的 `*Engine`。状态只有一把互斥锁后面的 `inflight map[string]int`。键是 `Principal.KeyID`，也就是虚拟密钥的账号 id。不是 `Principal.Hash`，也不是部署 id `api_base|model`。

## Begin 做什么

`dataplane.Serve` 只在 `EnforceIdentityLimits` 已经放行之后调用：

```text
done := h.HookEngine().Begin(p.KeyID)
defer done()
```

- `keyID` 为空时返回一个空函数，不改表。没有密钥的会话也能拿到这个释放函数。
- 其他 id 把 `inflight[keyID]` 加一，并返回一个减一的函数。减到 0 或以下时，这个 id 从表里删掉。

`Begin` 只返回一个函数。它不返回原因字符串、error 或布尔值。它不看 `max_budget`、RPM、TPM，也不访问 Redis。没有 `"budget"` 结果，也没有 `"parallel"` 结果。计数再高也不会在这里拒绝；这个包甚至没有读计数的方法。

`Serve` 返回时，包括后来上游失败的路径，延迟的释放函数都会跑。`ServeBypass` 不调用 `Begin`。官方任务的创建和查询不在这里占槽。

## 这个包不做什么

密钥、团队、项目和组织的预算在更早的 `gateway/limits.go` 的 `EnforceIdentityLimits` 里查，数据来自 `internal/iam`。Redis 上的 RPM 和 TPM 也在那里：`enforceRedisRateLimits` 用 `Principal.Hash`（令牌哈希）调用 `live.Client.HitRPM` 和 `HitTPM`，键前缀是 `xhub:rpm:` 和 `xhub:tpm:`。

在途计数只属于这一台进程。另一台网关有自己的表。花费、冷却和花费队列在 `internal/live`。

English notes are in `readme.md` in this directory.

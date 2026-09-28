# hooks

## 这个模块做什么

`hooks` 在花钱之前挡住请求。它检查这把密钥的预算，以及这把密钥已经有多少请求在飞。任一上限已经用完时，数据面不能再联系上游。

## 功能

- `New` 创建一个空闸门。计数只放在本进程的内存里。
- `Begin` 占用一个名额。成功时返回释放函数和空的原因字符串。
- 预算已经花完时，`Begin` 返回原因 `"budget"`，释放函数为 nil。
- 并发已满时，`Begin` 返回 `"parallel"`，释放函数为 nil。

## 其它包怎么用

导入 `github.com/sunqirui1987/xhub/internal/hooks`。

```go
gate := hooks.New()
release, reason := gate.Begin(key)
if reason != "" {
    // "budget" 或 "parallel"：不要调用供应商
    return
}
defer release()
// 联系上游
```

网关把一份引擎放在 `Server.Hooks` 上，用 `HookEngine` 交出去。请求处理要用这一份，这样所有请求共用同一张并发表。传入的密钥为 nil 时直接放行，释放函数什么也不做。

## 这个包不做什么

它不读 Redis，也不写花费。热花费和冷却在 `live`。

# dataplane

## 这个模块做什么

`dataplane` 发送一次推理。它按顺序挑选部署，编码正文，调用上游，当前部署失败就换下一个。它也把冷却、延迟和花费增量写进 Redis。它不引用 `gateway`，所以 HTTP 进程和发送循环不会互相导入。

## 功能

- `Serve` 是聊天、嵌入、图像和其它推理操作的入口，网关处理函数调用它。
- `EstimateTokens` 在调用前给预算和 TPM 一个上界。
- `State` 从宿主收集路由快照：冷却、延迟、用量、正在进行的请求数。
- `RecordFailure`、`RecordLatency`、`RecordUsage` 在调用之后写下热路径事实。
- `Flush` 和 `FlushLoop` 把 Redis 里排队的花费写入 PostgreSQL。

## 网关怎么用

网关实现 `dataplane.Host`。处理函数自己不调用供应商：

```go
func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
    dataplane.Serve(s, w, r, "chat")
}
```

宿主要提供配置、存储、HTTP 客户端、路由状态、凭证、护栏、插件注册表和花费记录。字段见 `host.go` 里的 `Host`。注册在宿主上的扩展会在缓存和上游 HTTP 之前运行。扩展拒绝后不会再联系供应商。

只有配置了 Redis 时，`Server.Run` 才会启动 `FlushLoop`。你也可以自己调用 `Flush`，把当前排队的增量写一次。

## 这个包不做什么

它不解析主密钥，也不注册 URL。鉴权发生在调用 `Serve` 之前。

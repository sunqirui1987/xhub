# gateway

## 这个模块做什么

`gateway` 是进程本身。它持有配置、PostgreSQL、Redis、Gin 引擎、会话和模块列表。外部程序启动它。其它内部包不引用它。进程在 `wire.go` 里实现那些包的 Host 和 Gate。

## 功能

- `New` 用 `Config` 和 `Store` 造出 `Server`。`Run` 开始监听，配置了 Redis 时还会启动花费刷写。
- `Handler` 是可以挂到测试或其它服务器上的 `http.Handler`。`GinRoutes` 列出已经注册的路由。没有 `/` 兜底。
- `Use` 在启动之后再装一个 `httpx.Module`。模块重名会失败。同一个方法和路径以先注册的为准。
- `RequireManage` 和 `RequireLLMPrincipal` 是管理路由和推理路由的鉴权门。
- `IsRemovedColumn` 报告保持 404 的路径，包括已经拿掉的智能体、MCP 和技能表面。
- `SetUIProxy` 装上控制台反向代理。传 nil 表示控制台没有启动。
- 健康检查：`GET /health/liveliness` 不访问数据库，返回 `{"status":"ok"}`。就绪检查会 ping PostgreSQL。

## 怎么把它跑起来

在仓库根目录：

```bash
go build -o xhub ./cmd/gateway
./xhub -config configs/config.yaml -addr 127.0.0.1:4000
curl -s http://127.0.0.1:4000/health/liveliness
```

在 Go 里：

```go
cfg, err := config.Load("configs/config.yaml")
st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
srv := gateway.New(cfg, st)
if err := srv.Use(someModule); err != nil {
    log.Fatal(err)
}
log.Fatal(srv.Run("127.0.0.1:4000"))
```

用主密钥或虚拟密钥调用推理：

```bash
curl -s http://127.0.0.1:4000/v1/chat/completions \
  -H "Authorization: Bearer sk-local-master" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}'
```

控制台登录是 `POST /login` 或 `POST /v2/login`，提交用户名和密码。默认用户名是 `admin`。没有设置 `UI_USERNAME` 和 `UI_PASSWORD` 时，默认密码就是主密钥。

## 子包里有什么

密钥、模型、护栏、身份、用量、设置和目录资源族都作为模块装上。它们的 README 写各自的 HTTP 路径。这个目录是调用它们的进程。

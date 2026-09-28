# catalog

## 这个模块做什么

`catalog` 描述网关的静态表面：有哪些 URL、哪些是公开的、哪些进入推理数据面，以及内置模型价格。这些 JSON 打进二进制，进程启动时不从磁盘读路由文件。

## 功能

- `Load` 返回嵌入的 `routes.json` 里的全部路由。
- `AuthOf`、`IsPublicPath`、`IsDataPlanePath`、`IsMixedPath`、`IsLLMPrefix` 给 URL 分类。
- `PathMatch` 把目录模板（例如 `/key/{key}`）和具体路径比较。
- `PublicBody` 返回公开 GET 的固定 JSON。
- `CostMap`、`Count`、`ProviderModels`、`KnownProvider` 读取嵌入的价格表。
- `Cost` 用这一行的每 token 单价乘提示 token 和补全 token。`Format` 把美元金额印成没有多余尾零的字符串。表里没有这一行时返回 `ok == false`。
- `MarkReloaded` 记下内存里的价格表已经刷新。

## 其它包怎么用

导入 `github.com/sunqirui1987/xhub/internal/catalog`。

```go
for _, rt := range catalog.Load() {
    if catalog.IsDataPlanePath(rt.P) {
        // 挂到推理一侧
    }
}
if catalog.IsPublicPath(r.Method, r.URL.Path) {
    httpx.WriteJSON(w, 200, catalog.PublicBody(r.URL.Path))
    return
}
prices := catalog.CostMap()
info := prices["gpt-4o-mini"]
```

`KnownProvider` 查不到的名字不是已实现的供应商。不要把它当成 OpenAI 兼容。数据面会跳过这些部署。

## 这个包不做什么

它不注册 Gin 路由，也不发送 HTTP。挂路由是 gateway 的事。`publicdata` 目录里的文件就是这个包读的嵌入文档。

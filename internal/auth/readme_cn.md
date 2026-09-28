# auth

## 这个模块做什么

`auth` 只回答一件事：这次 HTTP 请求是谁发起的，以及他能做什么。各个处理函数不再自己拆 `Authorization`。主密钥、虚拟密钥和登录会话都会在这里变成一个 `Principal`。

## 功能

- 从请求里取出 bearer token。
- 认出配置里的主密钥。
- 通过存储层解析虚拟密钥，并带上库存里的密钥记录。
- 把管理权限和推理权限分开。
- 用 `IsAuthErr` 标出鉴权失败，处理函数可以直接回 401，不用猜测错误类型。

## 其它包怎么用

导入 `github.com/sunqirui1987/xhub/internal/auth`。

```go
principal, err := auth.Resolve(cfg, st, r)
if err != nil {
    if auth.IsAuthErr(err) {
        httpx.WriteError(w, 401, "auth_error", err.Error())
        return
    }
    httpx.WriteError(w, 500, "internal", err.Error())
    return
}
if !principal.CanManage() {
    httpx.WriteError(w, 403, "forbidden", "management key required")
    return
}
if principal.CanLLM(cfg) {
    // 可以进入推理数据面
}
```

只想拿原始 token 时用 `APIKeyFrom`。控制台和管理接口用 `CanManage`。聊天、嵌入和其它推理接口用 `CanLLM`。配置不允许，或者这把虚拟密钥本身不能调模型时，`CanLLM` 为假。

## 这个包不做什么

它不创建密钥、不保存密码、也不挑选模型部署。那些分别在 `gateway/keys`、`store` 和 `router`。

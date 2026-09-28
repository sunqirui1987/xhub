# gateway/family

## 这个模块做什么

`family` 回答那些没有专用处理函数的目录路由。它为这些资源生成列表、详情和写入的 JSON，并把真正的推理操作转给数据面。进程按 `routes.json` 的每一条挂路由，再调用 `ServeDataPlane`、`ServeMixed` 或 `ServeMgmt`。

## 请求怎么分发

- `/v1/chat/completions` 这类数据面路径进入 `ServeDataPlane`，再调用 `dataplane.Serve`。
- 没有自己模块的管理资源进入 `ServeMgmt`。
- 混合路径两者都可能，`ServeMixed` 按方法和正文选择。

通常不要直接调用这些函数。调用 HTTP 路径即可。主密钥或虚拟密钥是否必需，跟 `catalog.AuthOf` 对这条路径的分类一致。

## 对外版本号

`ProxyVersion` 是健康详情里 `litellm_version` 的字符串。进程把 `gateway.Version` 设成同一个值。检查这个头的客户端会看到 `xhub-dev`，除非你改了这个常量。

## 这个包不做什么

它不拥有用户、密钥或路由设置页。那些有自己的模块，并且因为先注册而胜出。

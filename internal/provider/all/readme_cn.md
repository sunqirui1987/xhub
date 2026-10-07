# provider/all

这个目录没有端点，也没有 `init` 里的业务登记。它只空白导入已经写好的供应商包，让那些包的 `init` 在进程启动时跑起来：

- `github.com/sunqirui1987/xhub/internal/provider/openai`
- `github.com/sunqirui1987/xhub/internal/provider/qiniu`
- `github.com/sunqirui1987/xhub/internal/provider/volcengine`

网关的 `main` 或组装进程的文件导入 `all` 一次即可。再加一个供应商时，新建 `internal/provider/<名字>/`，在那里 `RegisterSupplier` / `RegisterType` / `RegisterModel`，然后在本目录加一行空白导入。不要在 `all` 里写路由。

测试如果自己导入某一个供应商包，就只会看到那一个包的类型。`provider.Types()` 为空通常是测试二进制没导入 `all`。

不在子目录里的官方接口（例如按 Suno、Tripo 文档填的自定义 bypass）不经过这里。它们写在部署的 `litellm_params.endpoint` 上，由 `provider.overrideType` 在匹配时盖上去。

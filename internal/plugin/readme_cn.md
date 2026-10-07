# plugin

联系上游之前的扩展点。这个包不导入网关，自己也不带一个内置扩展。调用方按稳定的 `Name` 登记实现。

`Call` 是扩展能读到的内容：`Op`、`Model`（对外名）和 `Path`。正文不在这个结构体上。

`Decision.Refuse == true` 让 `dataplane.Serve` 跳过响应缓存，并且不联系上游。`Status`、`Code`、`Message` 是数据面写出的错误。`Header` 即使放行也会抄到响应上，这样客户端能看出扩展跑过。

`New` 返回空注册表。`Registry` 的零值不能用。`Register` 按顺序追加。空名字或重复名字返回错误，已有顺序不动。`Names` 返回一份拷贝。

`Run` 按登记顺序调用 `BeforeUpstream`。第一次拒绝就停掉后面的，已经写上的头保留。空注册表返回零值 `Decision`，`Serve` 继续。`Invoke` 只跑一个名字；没有这个名字时返回错误，不会去调别的扩展。

`Serve` 在护栏和 `hooks.Begin` 之后、查缓存之前调用 `Run`。`ServeBypass` 不调用它。这里的拒绝不是护栏拦截，也不是预算失败。

## 这个包不做什么

它不把决定落盘，不读 Redis，也不认识团队。扩展若需要这些，得自己闭包带上。上游返回之后没有钩子。

English notes are in `readme.md` in this directory.

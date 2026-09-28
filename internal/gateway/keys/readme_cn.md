# gateway/keys

## 怎么使用

按下面的 HTTP 路径或 Go 入口调用。除了公开路由，请求都要带主密钥或管理员会话。

## 这个模块做什么

`keys` 是虚拟密钥的 HTTP 接口。操作员可以创建密钥、列出密钥、轮换明文、重置花费、封禁密钥，而不必把主密钥交出去。明文只在创建和轮换的响应里出现一次。

## HTTP 路径

请求头带 `Authorization: Bearer <主密钥或管理员会话>`。

- `POST /key/generate` 创建密钥。JSON 响应里的 `key` 只出现这一次。
- `GET /key/list` 和 `POST /key/list` 列出密钥。可以按用户、团队和别名过滤。
- `GET /key/info` 读取一把密钥。传入处理函数接受的哈希或别名。以后的调用不能用库存哈希代替明文。
- `POST /key/update` 修改预算、模型和元数据。
- `POST /key/delete` 删除密钥。
- `POST /key/regenerate` 轮换明文，新明文只返回一次。
- `POST /key/block` 和 `POST /key/unblock` 停止或恢复使用。
- `POST /key/{key}/reset_spend` 清掉这把密钥的花费。
- `GET /key/aliases` 列出别名，给控制台的选择器使用。

## 例子

```bash
curl -s http://127.0.0.1:4000/key/generate \
  -H "Authorization: Bearer sk-local-master" \
  -H "Content-Type: application/json" \
  -d '{"key_alias":"ci","models":["gpt-4o-mini"],"max_budget":10}'
```

把返回的 `key` 当作 `/v1/chat/completions` 的 bearer token。

## Go 调用方

进程装上 `keys.Module`。其它包不要直接调用 `Generate`，除非它实现了 `keys.Host`。`*gateway.Server` 实现了这个接口。

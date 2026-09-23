# 页面模板

所有管理页落在「左五组导航 + 顶栏 + 主列」壳里，见 [壳层](../frontend/shell.md)。下列模板规定**主列**怎么排。登录 / 开通 / 连接 / OAuth 回调不套这些模板，见 [auth](../frontend/pages/auth/)。

筛选、排序、分页、打开的详情 id **必须写入 URL query**，刷新不丢。

## 列表页

用于 Keys、Users、Teams、Models、Logs 等。默认落地 `/api-keys` 即此模板。

```text
[ 顶栏：面包屑 = 当前页名 | 右工具 ]
[ 可选壳层横幅 ]
主列：
  [PageHeader：标题 + 说明 | 主按钮 Create]
  [Toolbar：搜索 | 筛选 | 刷新]
  [表]
  [分页]
```

| 区域 | 行为 |
|---|---|
| 顶栏 | 只显示页名，不放 Create。 |
| 筛选 | 点 Filters 开抽屉；Apply 写入 URL；Reset 清空 query 并回到第 1 页。 |
| 表 | 骨架 loading，禁止闪空表。行点击进详情。 |
| 抽屉 | 筛选草稿；Apply 才提交。 |
| 模态 | Create；删除确认。Key 创建成功再叠 SecretOnce，见下。 |
| URL | `search` / `filter_*` / `sort_*` / `page` / `page_size`；详情 id。 |
| view-only | 无 Create；行菜单无 Edit / Delete / Regenerate / Block。 |

空列表：EmptyState。view-only 不放「去创建」主按钮。无权限：forbidden 文案，不假装 0 行。

## 详情

从列表点行进入；同一路由，靠 query 或子路径区分。

```text
[ 顶栏仍是该资源的导航名 ]
主列：
  [返回列表]
  [页头：名称 + 状态徽章 + 主操作 Save / Block / Regenerate / Delete]
  [元数据行：User / Team / Org / 时间]
  [Tab：Overview / Members / Budget / …]
  [Tab 内容]
```

| 区域 | 行为 |
|---|---|
| 顶栏 | 不变；返回靠页内 Back。 |
| Tab | 切换不丢未保存草稿，或离开前提示。 |
| 抽屉 | 少用；成员/权限也可用侧抽屉。 |
| 模态 | 删除确认；Key 的 Regenerate 成功走 SecretOnce。 |
| URL | 保留列表筛选，另加资源 id（如 `?key=`）。关掉详情只去掉 id。 |
| view-only | 页头无 Regenerate / Block / Delete / Save；Tab 可看不可改。 |

## 创建

Modal 或独立页。字段分组：基础信息、限额、权限。提交打对应 POST。400 校验贴在字段旁。提交中锁定按钮，超时不换 payload 重放。

view-only 不渲染创建入口。

## 一次性密钥（SecretOnce）

**只用于** `POST /key/generate` 与 Regenerate 成功。

1. 成功后模态：完整 `sk-` + Copy。
2. 写明关闭后无法再查看。
3. 列表 / 详情此后只有 `key_alias` 与前缀。
4. 不得出现在登录、开通、连接、OAuth 回调、Playground、设置页。

## Playground / 调试

```text
[ 顶栏：Playground ]
主列（可全高）：
  [Tab：Chat / Compare / …]
  [左：模型、协议、temperature、max_tokens、stream、tools]
  [中：消息与 SSE]
  [右：usage、x-litellm-response-cost、事件]
```

发送走数据面 POST，不是管理封装接口。view-only / `proxy_admin_viewer`：**侧栏不出现 Playground**；直链显示 Access Denied，不发请求。无可用模型：空态，不是假回复。

## 设置页

```text
[ 顶栏：该 Settings 子页名 ]
主列：
  [分组表单（SSO / IP / 主题 / …）]
  [Save]
```

整组 SETTINGS 仅 admin。切换子页或 Tab 不丢未保存草稿，或离开前提示。Save 失败字段旁或页顶 Banner 展示 `error.message`。此处**没有** SecretOnce。

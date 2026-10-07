# 提示词组件

控制台 `/prompts` 用的 React 组件。数据是网关键值种类 `prompts`，不是推理请求里临时带上的 `messages`。

| 文件 | 作用 |
| --- | --- |
| `index.tsx` | 面板。拉取列表，处理加载和错误 |
| `PromptTable.tsx`、`PromptTableColumns.tsx` | 表格。列包括提示词 id、创建时间、更新时间、类型。点 id 打开详情 |
| `prompt_info.tsx` | 单条详情。概览、详情、原始 JSON。原始 JSON 是接口返回的对象，带复制 |
| `add_prompt_form.tsx` | 新建 |
| `prompt_editor_view.tsx` | 编辑正文和变量 |
| `variable_textarea.tsx` | 带变量占位的输入框 |
| `tool_modal.tsx` | 给这条提示词选工具 |
| `prompt_utils.tsx` | 列表和详情共用的字段读取 |

保存走网关的提示词写接口，存在 `RecordStore` 的 `prompts` 种类下。id 字段是 `prompt_id`。这不是 `Serve` 里的响应缓存，也不是 `deployment_affinity` 会话钉。

聊天请求要检查的文本在护栏里，不在这里。这里的页面不调用上游模型。

测试是同目录的 Vitest 文件（`PromptTable.test.tsx`、`prompt_info.test.tsx`、`add_prompt_form.integration.test.tsx`）。它们不要求 `:4000` 已启动。

# gateway/models

模型列表、添加模型、封禁，以及内存里的价格覆盖。以 `models.Module` 挂在目录之前。

GET `/v1/models` 和 GET `/models` 列出调用方能看见的部署。主密钥在没打开 `allow_master_key_llm` 时 `AllowLLM` 为假；只能看、不能调的视图仍可以列出被授权的元数据。GET `/model/available` 是已登录调用方的卡片列表。真正调用模型走推理路由。

POST `/model/new` 把一条部署写进 `proxy_models` 和进程内的 `ModelTable`。正文多选 `endpoint_types`。空则退回 `mode`，再空就是 `chat`。自定义 bypass 写在 `litellm_params.endpoint`，不必新建 Go 文件。POST `/model/update` 和 PATCH `/model/{model_id}/update` 改这一行。POST `/model/block` 设置 `model_info.blocked`。被封禁的部署留在表里，请求时被拒绝。POST `/model/delete` 删掉这一行。

`LockModels` / `UnlockModels` 包住这些写。`LoadStored` 在启动时把数据库行盖到 YAML 列表上，并丢掉被当成模型存下来的供应商壳。

## 内置供应商

`XHUB_BUILTIN_PROVIDERS` 不是 `off` 时跑 `SeedBuiltins`。它不插入模型。缺凭据时创建两条：

| id | 目录地址 | 调用根 | 环境变量 |
| --- | --- | --- | --- |
| `fennoai` | `https://api.fenno.ai/v1/models` | `https://api.fenno.ai` | `FENNOAI_API_KEY` |
| `qiniu` | `https://api.qnaigc.com/v1/models` | `https://api.qnaigc.com/bypass/openai/v1` | `QINIU_API_KEY` |

这个七牛根是从七牛目录添加聊天模型时用的 OpenAI 兼容 bypass。它不是内容生成接口。内容生成在 `provider/qiniu`，根是 `https://api.qnaigc.com`，路径 `/v3/contents/generations/tasks`。

`providerKey` 只为拉取目录而读密钥。掩码占位符被忽略。存下来的模型记凭据名，不复制密钥。

## 价格路由

POST `/price/model` 和 DELETE `/price/model` 改内存里的覆盖。POST `/price/model/reset` 从 `catalog` 恢复基线行。POST `/reload/model_cost_map` 抓市场源。POST `/schedule/model_cost_map_reload` 打开 `scheduledReloadLoop`，每分钟看一次，`next_run` 到了就调用 `catalog.ReloadFromMarket`。抓取失败时正在使用的价格不动，也不把 `next_run` 往后推。

## 这个包不做什么

它不编码聊天正文。部署选定之后由 `dataplane.Serve` 或 `ServeBypass` 做这件事。

English notes are in `readme.md` in this directory.

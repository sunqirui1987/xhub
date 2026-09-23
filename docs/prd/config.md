# 配置：YAML snapshot 与库实体

## YAML

可导入导出。顶层：

- `model_list`：每项 `model_name` + `litellm_params`（`model`、`api_key`、`api_base`、`rpm`、`tpm`、`timeout`）+ 可选 `model_info`
- `router_settings`：策略、`num_retries`、`timeout`、cooldown、fallback
- `litellm_settings`：drop_params、cache、callbacks
- `general_settings`：`master_key`、`database_url`、`store_model_in_db`、alerting、UI 开关

`api_key: os.environ/OPENAI_API_KEY` 表示读环境变量，明文不进普通列。

## Snapshot

YAML 导入、`POST /model/new`、以及 DB 中 `store_model_in_db` 的行，合并成一个**不可变 snapshot**。一次数据面请求只用一个 snapshot，避免路由到一半模型列表被改。

变更后：失效 DualCache 里的 Auth 与 Router 池，再发布新 snapshot。

组织 / 团队 / Key / 预算等**不是** YAML 主路径，走管理 API 落库（VerificationToken、Team、Organization、Budget 等）。

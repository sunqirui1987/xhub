# 配置

YAML 顶层：`model_list`、`router_settings`、`litellm_settings`、`general_settings`。

部署项：`model_name` + `litellm_params`（`model`、`api_key`、`api_base`、`rpm`、`tpm`、`timeout`）+ `model_info`。

`api_key: os.environ/OPENAI_API_KEY` 表示读环境变量。一次推理只用一个不可变 snapshot。

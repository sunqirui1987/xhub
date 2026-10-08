# gateway/guard

通过 guard.Module 挂载。管理试跑 /apply_guardrail、/guardrails/apply_guardrail 要求管理权限；推理时聊天与 Responses 在缓存和上游前调用 Evaluate。

规则来自 guardrails、guardrail 记录。已配置存储读取失败产生阻断规则；没有配置存储的宿主仍跳过检查。默认启用的 pre_call、redact 规则顺序执行，阻断后不伪造后续执行结果。

扫描 text/input/prompt/messages/contents/system/instructions/systemInstruction 及支持的嵌套 text/content/parts 数组，不再只取第一个字段。保留图片 URL 和消息角色。关键词忽略大小写；打码替换所有命中词，实际改写发送给上游的文本。

阻断返回 400 guardrail_failed，保留失败日志并记零本地费用。本包只是本地关键词匹配，不是 Azure Content Safety 或通用多模态审核。官方 Bypass、工具参数、图片内容不在扫描范围。

测试见 guard_test.go、guard_regression_test.go 及 dataplane 缓存/交换测试。

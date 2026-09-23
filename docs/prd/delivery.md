# 实现顺序

分期只排工期，**不缩小** [product.md](product.md) 的分母。

1. 骨架：health、YAML、控制台壳
2. 垂直切片：`/key/generate` + `/v1/chat/completions` + `/api-keys` + `/playground`
3. 身份：User / Team / Org / Budget
4. 其余数据面
5. 模型管理与日志台
6. Router 策略与缓存
7. 更多 Provider
8. 治理与其余页面
9. Postgres / SSO / 需独立实现的企业能力

第一刀细节可写在迭代任务里，不得把「P1 不做」写进产品范围。

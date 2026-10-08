# XHub 文档

[English](README.md) | **简体中文** · [项目首页](../README.zh-CN.md)

第一次使用，先完成安装，再按客户指南接入供应商、建立团队并发起请求。开发者和 AI 编程助手从开发指南进入，按模块定位实现和验证规则。

## 使用与管理

| 文档 | 解决的问题 | English |
| --- | --- | --- |
| [安装指南](getting-started.zh-CN.md) | Docker Compose、源码启动、网关和控制台配置 | [Installation](getting-started.md) |
| [客户使用与管理指南](user-guide.zh-CN.md) | 接入模型、用户与权限、团队与项目、密钥、路由、费用及故障处理 | [User guide](user-guide.md) |

## 开发与 AI 编程

| 文档 | 解决的问题 |
| --- | --- |
| [开发与 AI 编程指南](development/README.md) | 找代码、理解约束、选择修改范围和验证方法 |
| [架构与代码导航](development/architecture.md) | 服务职责、请求链路、模块边界 |
| [权限规则](development/permissions.md) | 角色、可见范围、秘密串读取、模型范围 |
| [路由规则](development/routing.md) | 模板继承、权重、重试、超时、会话粘性 |
| [计价与用量](development/pricing.md) | 费率、时段、价格快照、持久化链路 |
| [运行行为与限制](development/runtime.md) | 缓存、幂等、协议、媒体任务及运行边界 |
| [测试指南](development/testing.md) | 测试层次、数据库隔离、定向运行、可选演示数据 |

各模块的 `readme.md` / `readme_cn.md` 提供英文和中文代码说明。

## 文档维护

文档描述已经实现的行为，并提供代码依据。改变行为时同步更新对应指南。客户操作留在使用指南，实现细节留在开发参考中；临时计划、AI 对话、审查流水和测试输出不放进正式文档。

[`testdata/catalog.json`](testdata/catalog.json) 是自动化测试使用的路由基线，不能据此承诺全部供应商能力。[`assets/`](assets/) 保存首页使用的控制台截图。

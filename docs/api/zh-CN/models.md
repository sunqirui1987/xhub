# 模型列表

## 接口概述

GET /models：无需密钥发现已启用的公开模型。

### 认证与路径

GET /models 无需密钥，列出已启用公开模型名。显式提供密钥时按权限过滤；GET /v1/models 保留鉴权契约，推理调用始终需要有效密钥。

### 响应与选择

响应的 data 数组列出模型，用 id 作为请求的 model。列表受调用方权限约束，能力需核对部署协议。

### 权限与失败

失效密钥返回认证错误，越权返回权限错误或隐藏资源存在的响应。缺少模型时检查允许范围、启用状态和公开名称。

## 协议行为
GET /models 可匿名发现已启用公开模型名。显式凭据按权限过滤，无效密钥返回 401；GET /v1/models 保留鉴权。id 是可调用名，created 是固定兼容时间戳，owned_by 是兼容标签而非真实供应商。空 data 可能表示没有授权模型。

## 接口与认证

~~~http
GET /models
~~~

GET /models 无凭据可公开查询已启用的模型名称。携带显式密钥时按权限过滤，错误密钥返回 401。GET /v1/models 保留鉴权兼容路径。

Base URL 使用调试台显示的网关根地址，不额外追加 /v1；实际请求使用上面的完整接口路径。

## 请求参数

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| scope | string | 空或 expand；其他值返回 400，expand 扩展管理员可见范围。 |
| team_id | string | 会话团队筛选；密钥保持绑定范围。 |
| return_wildcard_routes | boolean | 返回允许的通配名，默认 false。 |
| only_model_access_groups | boolean | 旧参数，true 返回空列表。 |

## 请求示例

~~~shell
curl "$XHUB_BASE_URL/models"
~~~

## 成功响应

~~~json
{
  "object": "list",
  "data": [
    {
      "id": "YOUR_MODEL_NAME",
      "object": "model",
      "created": 1677610602,
      "owned_by": "openai"
    }
  ]
}
~~~

| 字段 | 类型 |
| --- | --- |
| object | string |
| data | object[] |
| data[].id | string |
| data[].object | string |
| data[].created | number |
| data[].owned_by | string |

## 错误与排查

| HTTP | 说明 |
| --- | --- |
| 400 | 请求无效，请检查 JSON、参数和查询。 |
| 401 | XHub 凭据缺失或无效。 |
| 403 | 权限拒绝，请检查模型范围或策略。 |
| 429 | 超过限制，请检查响应详情与 Retry-After。 |

## 能力与计费边界

参数支持取决于部署模型及转换或 Bypass 路径；XHub 不保证所有上游扩展字段在所有模型中可用。以已实现的协议适配和上游响应为准。usage 的明细属于对应输入/输出总量，不能重复相加；费用以网关日志保存的计费快照为准。示例响应仅用于理解字段。

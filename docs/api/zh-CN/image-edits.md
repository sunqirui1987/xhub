# 图片编辑

## 接口概述

图片编辑

### 图片编辑

## 接口行为
让 curl 设置 multipart boundary。部分原生部署接受 JSON 引用，请匹配协议。

## 接口与认证

~~~http
POST /v1/images/edits
~~~

使用 XHub API 密钥：Authorization: Bearer <XHUB_API_KEY>。不要把供应商密钥当作网关密钥。

Base URL 使用调试台显示的网关根地址，不额外追加 /v1；实际请求使用上面的完整接口路径。

## 请求参数

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| model | string · required | 公开模型名，部署必须支持此协议。 |
| image | file|array · required | multipart 源图片，格式及大小依供应商。 |
| prompt / mask | string / file | 编辑指令与可选遮罩。 |

## 请求示例

~~~shell
curl "$XHUB_BASE_URL/v1/images/edits" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -F "model=YOUR_MODEL_NAME" -F "image=@source.png" \
  -F "prompt=Add a small lighthouse."
~~~

## 成功响应

~~~json
{
  "data": [
    {
      "b64_json": "BASE64_IMAGE_DATA"
    }
  ]
}
~~~

| 字段 | 类型 |
| --- | --- |
| data | object[] |
| data[].b64_json | string |

## 错误与排查

| HTTP | 说明 |
| --- | --- |
| 400 | 请求无效，请检查 JSON、参数和查询。 |
| 401 | XHub 凭据缺失或无效。 |
| 403 | 权限拒绝，请检查模型范围或策略。 |
| 429 | 超过限制，请检查响应详情与 Retry-After。 |

## 能力与计费边界

参数支持取决于部署模型及转换或 Bypass 路径；XHub 不保证所有上游扩展字段在所有模型中可用。以已实现的协议适配和上游响应为准。usage 的明细属于对应输入/输出总量，不能重复相加；费用以网关日志保存的计费快照为准。示例响应仅用于理解字段。

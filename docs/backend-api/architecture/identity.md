# 身份

```text
Organization → Team → Project → Virtual Key
Internal User *—* Team
End User（请求里的 user / end_user）
```

预算与模型白名单向下收紧。Key 明文 `sk-...`，库中哈希。`object_permission` 控制 mcp_servers、vector_stores、agents、search_tools、skills、blocked_tools。

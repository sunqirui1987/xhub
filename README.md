# XHub

自托管 AI 网关。文档分三块：

- [产品需求](docs/prd/README.md)
- [UI](docs/ui/README.md)
- [前端页面](docs/frontend/README.md)
- [后端 API](docs/backend-api/README.md)

实现：Go 网关 + Next.js 控制台（`frontend/` 为 LiteLLM dashboard 同源 UI，经 `/gw` 打到本网关）。

```bash
# 网关 :4000
export OPENAI_API_KEY=sk-...   # 上游；测试可不设，用 go test 假上游
make test
cp configs/config.example.yaml configs/config.yaml
# 把 master_key 配好后：
make run

# 控制台 :3000（/gw 反代到网关）
make ui
# 打开 http://localhost:3000/login  用户名 admin，密码 = master_key

# 浏览器自动化（登录 / 密钥 / Playground / view-only / 全页面）
# 首次：cd frontend && npx playwright install chromium
make e2e
```

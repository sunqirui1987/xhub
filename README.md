# XHub

自托管 AI 网关。控制台和网关是两个进程：

| | 地址 | 做什么 |
| --- | --- | --- |
| 控制台 | http://localhost:3000 | 登录、模型、密钥、Playground。页面在 Next.js 上，`/ui/...` 只属于这个端口 |
| 网关 API | http://localhost:4000 | OpenAI 兼容接口和管理接口。客户端 `base_url` 用这个地址 |

Playground 复制出来的调用示例，以及控制台自己的请求，都指向网关，不指向 `:3000`。网关不提供页面。

文档：[产品需求](docs/prd/README.md) · [UI](docs/ui/README.md) · [前端页面](docs/frontend/README.md) · [后端 API](docs/backend-api/README.md)

登录控制台：http://localhost:3000/login ，用户名 `admin`，密码是配置里的 `master_key`。

调用网关：

```python
import openai

client = openai.OpenAI(
    api_key="sess-... 或虚拟密钥",
    base_url="http://localhost:4000",
)
```

公网部署时，网关用环境变量 `XHUB_PUBLIC_ORIGIN`（例如 `https://api.example.com`）作为对外 API 地址。控制台要连别的网关时，构建前设置 `NEXT_PUBLIC_BASE_URL`。

## 安装

控制台密码是配置里的 `master_key`，不用再导出环境变量。`redis_url` 可以留空；留空时网关不连 Redis，花费日志只走 PostgreSQL。Docker 模式使用镜像里的 `configs/config.docker.yaml`。源代码模式复制 `configs/config.example.yaml` 为 `configs/config.yaml` 后再改。

### Docker 模式

先在本机编译，再把产物打进镜像。`Dockerfile` 只复制 `bin/xhub-linux-amd64`、`bin/node-linux-amd64` 和已经构建好的控制台目录 `bin/console`。镜像里不编译，也不从 Docker Hub 拉 `alpine` 或 `node`。网关用 `scratch`，控制台跑在本机已有的 `postgres:16` 上（只借用它的 glibc）。PostgreSQL 和 Redis 也用本机已有的 `postgres:16`、`redis:7-alpine`。

本机需要 Go 1.25、Node.js `>=24.14.1` 和 Docker。产物跟本机 CPU 一致（Apple Silicon 是 `linux/arm64`）。Node 的 Linux 二进制由脚本从 npmmirror 下载。

```bash
sh deploy/build.sh
docker compose up -d
```

`docker-compose.yml` 拉起 PostgreSQL、Redis、网关和控制台。镜像内配置是 `configs/config.docker.yaml`，`master_key` 为 `sk-local-master`。控制台 http://localhost:3000/login ，网关 http://localhost:4000 。PostgreSQL 映射在本机 `5433`，容器名 `xhub-postgres`。

### 源代码模式

依赖环境：

| 依赖 | 版本 | 用途 |
| --- | --- | --- |
| Go | 1.25 | 网关 `cmd/gateway` |
| Node.js | `>=24.14.1` | 控制台 `frontend/` |
| PostgreSQL | 16 即可 | 必填。`general_settings.database_url` 必须是 `postgres://`，不支持 SQLite |
| Redis | 7 即可 | 可选。配置了 `redis_url` 才连接 |

本机装好 PostgreSQL 后建库，并把 `configs/config.yaml` 指到它。示例配置默认是本机 `5432`：

```yaml
general_settings:
  master_key: sk-local-master
  database_url: postgres://xhub:xhub@127.0.0.1:5432/xhub?sslmode=disable
```

```bash
make test
make run
make ui
```

`make test` 跑 Go 测试。`make e2e` 跑浏览器测试；第一次先执行 `cd frontend && npx playwright install chromium`。端到端测试会使用上面的 `xhub-postgres` 容器。

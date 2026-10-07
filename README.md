# XHub

自托管 AI 网关。控制台和网关是两个进程：

| | 地址 | 做什么 |
| --- | --- | --- |
| 控制台 | http://localhost:3000 | 登录、模型、密钥、Playground。页面在 Next.js 上，`/ui/...` 只属于这个端口 |
| 网关 API | http://localhost:4000 | OpenAI 兼容接口和管理接口。客户端 `base_url` 用这个地址 |

Playground 复制出来的调用示例，以及控制台自己的请求，都指向网关，不指向 `:3000`。网关不提供页面。

## 读哪份文档

[docs/README.md](docs/README.md) 把文档分成三层，不要混读：

- `docs/current/` 是现在跑在进程里的规则（团队、用户、权限、测试）。和代码冲突时以代码为准。
- `docs/design/` 是目标方案，多数还没实现。里面写着的预算预占、结算账本、统一异步媒体状态机，不是今天的行为。
- 各目录的 `readme.md` / `readme_cn.md` 描述该目录代码现在做什么。例如 `internal/hooks` 只数本进程在途调用，不查预算；`internal/provider/qiniu` 的内容生成路径是 `/v3/contents/generations/tasks`，`internal/provider/volcengine` 是 `/api/v3/contents/generations/tasks`，两者不是同一套接口。

## 一次调用怎么走进网关

客户端把 `base_url` 设为 `:4000`。`engine.go` 的 `Handler` 先看幂等键，再在 Gin 之前做 bypass 匹配（`provider.Match`）。命中则 `dataplane.ServeBypass`：创建官方任务不扣费，任务 id 钉 7 天（`official_task:v1:`），第一次带 usage 的后续查询才记一次账（`official_billed:v1:`）。没匹配上的路径进 Gin。普通推理最后到 `dataplane.Serve`：先身份和预算，再护栏（仅聊天），再 `hooks.Begin` 计在途，再非流式缓存，再按部署 id `api_base|model` 选上游。密钥 RPM/TPM 用的是令牌哈希 `Principal.Hash`，Redis 键是 `xhub:rpm:` 和 `xhub:tpm:`，不是 `api_base|model`。

## 权限模型

资源层级是**组织 → 团队 → 项目**。角色只有两个维度，没有第三个：

| 作用域 | 取值 | 怎么得到 |
| --- | --- | --- |
| 账号 | `admin`（平台管理员）、`user` | 由平台管理员创建，不开放自助注册 |
| 团队 | `team_admin`、`member` | 加进团队时指定 |

用户**只通过团队**获得权限：没有组织成员，没有项目管理员，没有访问组角色。团队管理员在本团队内能改团队资料、管成员、建项目和项目下的服务密钥，但不能删团队、不能改团队预算上限、不能给别人平台管理员。组织只由平台管理员维护。

几条容易踩的规则：

- 成员只能从**本团队**里选，且必须输入完整邮箱；邮箱不存在和已禁用返回同一个错误，防止拿它枚举全平台账号。
- 一个团队必须始终有至少一位团队管理员，所以移除最后一位会被拒绝。
- 服务密钥属于团队或项目。团队管理员只能**收窄**它的可用模型，不能超出团队范围。
- 逐条调用日志带请求/响应内容，所以只有本人、本团队管理员和平台管理员能看，且平台管理员每次查看都会写审计。

密钥分两种：**个人密钥**属于某个人，绑自己所在的团队/项目，只有自己能看；**服务密钥**属于团队或项目，团队管理员能轮换、禁用、删除。调用的归属（谁、哪个团队、哪个项目）在写入时就快照到用量事件上，所以成员离开团队不会带走他已经产生的用量，删团队也不会抹掉历史。

`master_key` 不是账号，也不能当密码登录。它只能调 `/bootstrap` 和应急管理接口；模型、密钥、团队这些管理接口都要管理员会话，用主密钥访问返回 403。

### 登录

控制台：http://localhost:3000/login ，用 `general_settings.admin_email` 和 `admin_password` 配置的账号登录。默认是 `admin@xhub.local` / `admin-pass-1234`，**上线前必须改掉**。

管理员账号在启动时按 `admin_email` 创建，只在数据库里还没有这个邮箱时才建。配置里的密码是**初始密码**：之后改配置**不会**重置已存在的账号密码，改线上密码要走控制台。没配这两项时不创建，此时用 `master_key` 调 `POST /bootstrap` 手工建第一个管理员。

调用网关：

```python
import openai

client = openai.OpenAI(
    api_key="虚拟密钥",
    base_url="http://localhost:4000",
)
```

公网部署时，网关用环境变量 `XHUB_PUBLIC_ORIGIN`（例如 `https://api.example.com`）作为对外 API 地址。控制台要连别的网关时，构建前设置 `NEXT_PUBLIC_BASE_URL`。

## 安装

数据库必须**全新**。这次身份与权限是重建的 schema（`internal/iam/schema.sql`），没有从旧库迁移的路径：旧表结构和新的对不上，进程启动时会直接报错退出（例如 `column "email" does not exist`），不会静默降级。请建一个新库，或先删掉旧库再启动。`users`、`teams`、`organizations`、`api_keys`、用量和审计这些表都由 `internal/iam` 拥有；`internal/store` 只留框架表（键值设置、UI 会话、花费队列），它们不带权限。

第一次启动只写入 `fennoai` 和 `qiniu` 两个凭证，不写入模型。`XHUB_BUILTIN_PROVIDERS` 默认开启；设为 `off` 则不安装。从供应商目录添加的模型和手填的一样，只保存模型名和凭证名。目录地址是 fennoai `https://api.fenno.ai/v1/models`、七牛 `https://api.qnaigc.com/v1/models`。调用地址在凭证上：fennoai `https://api.fenno.ai`，七牛 `https://api.qnaigc.com/bypass/openai/v1`。key 分别读 `FENNOAI_API_KEY` 和 `QINIU_API_KEY`。

`master_key` 现在可以留空；留空时没有应急凭据，一切按账号走。`redis_url` 也可以留空；留空时网关不连 Redis，花费日志只走 PostgreSQL。Docker 模式使用镜像里的 `configs/config.docker.yaml`。源代码模式复制 `configs/config.example.yaml` 为 `configs/config.yaml` 后再改。

生产环境不要把管理员密码写在配置文件里，用 `os.environ/` 从环境变量取：

```yaml
general_settings:
  admin_email: os.environ/XHUB_ADMIN_EMAIL
  admin_password: os.environ/XHUB_ADMIN_PASSWORD
```

变量没设时解析成空串，此时就不创建管理员，不会退化成用占位符当密码。账号建好之后想彻底关掉这条路径，设 `disable_env_credential_login: true`。

### Docker 模式

先在本机编译，再把产物打进镜像。`Dockerfile` 只复制 `bin/xhub-linux-amd64`、`bin/node-linux-amd64` 和已经构建好的控制台目录 `bin/console`。镜像里不编译，也不从 Docker Hub 拉 `alpine` 或 `node`。网关用 `scratch`，控制台跑在本机已有的 `postgres:16` 上（只借用它的 glibc）。PostgreSQL 和 Redis 也用本机已有的 `postgres:16`、`redis:7-alpine`。

本机需要 Go 1.25、Node.js `>=24.14.1` 和 Docker。产物跟本机 CPU 一致（Apple Silicon 是 `linux/arm64`）。Node 的 Linux 二进制由脚本从 npmmirror 下载。

```bash
sh deploy/build.sh
docker compose up -d
```

`docker-compose.yml` 拉起 PostgreSQL、Redis、网关和控制台。镜像内配置是 `configs/config.docker.yaml`，管理员是 `admin@xhub.local`。控制台 http://localhost:3000/login ，网关 http://localhost:4000 。PostgreSQL 映射在本机 `5433`，容器名 `xhub-postgres`。

### 源代码模式

依赖环境：

| 依赖 | 版本 | 用途 |
| --- | --- | --- |
| Go | 1.25 | 网关 `cmd/gateway` |
| Node.js | `>=24.14.1` | 控制台 `frontend/` |
| PostgreSQL | 16 即可 | 必填且必须全新。`general_settings.database_url` 必须是 `postgres://`，不支持 SQLite |
| Redis | 7 即可 | 可选。配置了 `redis_url` 才连接 |

本机装好 PostgreSQL 后建库。三份配置里的 `database_url` 不是同一个地址：

| 文件 | 谁读 | `database_url` |
| --- | --- | --- |
| `configs/config.yaml` | 源代码模式的 `make run` | `postgres://xhub:xhub_dev_password@127.0.0.1:5433/xhub?sslmode=disable` |
| `configs/config.example.yaml` | 只是模板，进程不读它 | `postgres://xhub:xhub@127.0.0.1:5432/xhub?sslmode=disable` |
| `configs/config.docker.yaml` | 容器里的网关 | `postgres://xhub:xhub_dev_password@postgres:5432/xhub?sslmode=disable` |

Compose 把容器端口 `5432` 映射到本机 `5433`，用户 `xhub`，密码 `xhub_dev_password`，库名 `xhub`。示例文件里的 `5432` 和密码 `xhub` 连不上这个库。`config.yaml` 和示例文件里的初始管理员都是 `admin@xhub.local` / `admin-pass-1234`，显示名 `Admin`。

```bash
make test
make run
make ui
```

`make test` 跑 Go 测试。涉及数据库的测试会在本机连不上库时**跳过**而不是失败，所以它绿不代表权限逻辑真的跑过；要真正执行，先起 `xhub-postgres` 容器（映射在本机 `5433`），或设 `XHUB_TEST_DATABASE_URL` 指向一个可用库。这些测试各自建独立的 schema 并在结束时删掉，不会碰你自己的数据。

`make e2e` 跑浏览器测试；第一次先执行 `cd frontend && npx playwright install chromium`。端到端测试会使用上面的 `xhub-postgres` 容器。


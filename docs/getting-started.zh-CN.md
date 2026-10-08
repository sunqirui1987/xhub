# XHub 安装与运行指南

[English](getting-started.md) | **简体中文** · [项目首页](../README.zh-CN.md)

XHub 包含两个服务：Go API 网关默认监听 `4000`，Next.js 控制台默认监听 `3000`。PostgreSQL 保存配置、身份和用量。源码运行可不使用 Redis，Docker Compose 栈包含 Redis。

## 环境要求

| 依赖 | 版本 | 用途 |
| --- | --- | --- |
| Go | 1.25 | 构建与运行网关 |
| Node.js | ≥24.14.1 | 构建与运行控制台 |
| npm | ≥11.10.0 | 安装控制台依赖 |
| PostgreSQL | 推荐 16 | 必需的持久存储 |
| Redis | 推荐 7 | 可选的共享运行状态 |
| Docker Compose | Docker 部署时需要 | 运行本地服务栈 |

使用**全新的 PostgreSQL 数据库**。当前身份结构不提供从早期结构迁移的路径。

## Docker Compose 部署

克隆仓库，在本机构建服务，再启动：

```bash
git clone https://github.com/sunqirui1987/xhub.git
cd xhub
bash deploy/build.sh
docker compose up -d
```

构建脚本需要 Go、Node.js、npm、Docker，以及可读取的 `/etc/ssl/cert.pem`。脚本从 npmmirror 下载 Linux Node.js，目标架构跟随主机（amd64 或 arm64）。运行镜像只包含预构建产物，不在容器内编译。控制台使用 `postgres:16` 作为提供 glibc 的运行基础镜像；Docker 需要能获取 `postgres:16` 和 `redis:7-alpine`。

| 服务 | 地址 |
| --- | --- |
| 控制台登录 | [localhost:3000/login](http://localhost:3000/login) |
| 网关 API | [localhost:4000](http://localhost:4000) |
| 主机访问 PostgreSQL | `127.0.0.1:5433` |

本地初始管理员为 `admin@xhub.local` / `admin-pass-1234`。对外开放前修改密码。仅在该邮箱尚无账号时创建初始管理员，修改配置里的密码**不会**重置已有账号；已有账号通过控制台修改密码。

## 从源码运行

### 1. 启动 PostgreSQL

可使用仓库中的 Compose 数据库：

```bash
docker compose up -d postgres
cp configs/config.example.yaml configs/config.yaml
```

把 `configs/config.yaml` 中的 `general_settings` 替换为以下配置，保留 `model_list` 和 `router_settings`：

```yaml
general_settings:
  database_url: postgres://xhub:xhub_dev_password@127.0.0.1:5433/xhub?sslmode=disable
  redis_url: ""
  store_model_in_db: true
  admin_email: os.environ/XHUB_ADMIN_EMAIL
  admin_password: os.environ/XHUB_ADMIN_PASSWORD
  disable_env_credential_login: false
```

也可以连接自己的全新 PostgreSQL 数据库。示例模板使用端口 `5432`、密码 `xhub`；Compose 数据库使用主机端口 `5433`、密码 `xhub_dev_password`。

`redis_url` 为空时关闭 Redis。启用时填写网关进程可访问的地址；Compose Redis 默认没有向主机发布端口。

### 2. 启动网关

在运行网关的同一终端设置初始管理员：

```bash
export XHUB_ADMIN_EMAIL="admin@example.com"
export XHUB_ADMIN_PASSWORD="replace-with-a-strong-password"
make run
```

网关读取 `configs/config.yaml`，监听 `4000`。缺失环境变量解析为空字符串，未提供上述凭证时不会创建初始账号。账号已存在后，通过控制台修改密码。

### 3. 启动控制台

在另一终端进入仓库根目录运行：

```bash
make ui
```

打开 [localhost:3000/login](http://localhost:3000/login)，使用上面设置的账号登录。

## 接入模型并完成首次调用

全新安装没有模型部署。除非设置 `XHUB_BUILTIN_PROVIDERS=off`，启动会登记 FennoAI 和七牛两个供应商凭证条目；这些条目不会自动授予权限或创建模型。

1. 在 **Models + Endpoints** 配置供应商凭证和 API 地址。内置目录凭证可从网关环境读取 `FENNOAI_API_KEY` 和 `QINIU_API_KEY`。
2. 手动或从供应商目录添加部署，设置公开模型名、上游模型及端点类型。
3. 创建组织和团队，配置团队模型范围并添加成员。个人推理也需要团队成员关系；项目按需创建。
4. 创建个人或服务虚拟密钥，在 Playground 测试模型，再从应用调用。

完整操作与权限说明见[客户使用与管理手册](user-guide.zh-CN.md)。通过 `pip install openai` 安装客户端，再使用虚拟密钥和公开模型名：

```python
from openai import OpenAI

client = OpenAI(
    api_key="YOUR_XHUB_VIRTUAL_KEY",
    base_url="http://localhost:4000/v1",
)
response = client.chat.completions.create(
    model="YOUR_PUBLIC_MODEL_NAME",
    messages=[{"role": "user", "content": "Hello, XHub!"}],
)
print(response.choices[0].message.content)
```

客户端连接网关地址，不能连接控制台地址。

## 部署配置

- 网关设置 `XHUB_PUBLIC_ORIGIN` 为公开 API 地址，例如 `https://api.example.com`。
- 浏览器使用其他网关地址时，在**构建控制台前**设置 `NEXT_PUBLIC_BASE_URL`。
- 控制台设置 `XHUB_GATEWAY_ORIGIN` 用于服务端访问网关，地址需要从控制台进程可达。
- 配置中的 `os.environ/变量名` 可从环境读取密钥。Docker 部署修改 `configs/config.docker.yaml` 后，为网关服务传入对应环境变量并重新构建镜像。
- 可选 `master_key` 用于初始化和应急访问，不能作为控制台登录凭证或普通应用密钥。

平台、组织、团队和成员范围见[权限参考](development/permissions.md)。

## 开发与验证

从[开发与 AI 编程指南](development/README.md)定位模块，再按修改选择[测试指南](development/testing.md)中的检查。Go 基础检查与模拟上游回归：

```bash
make test
make regression
```

数据库不可达时部分测试会跳过。启动本地 PostgreSQL 或设置 `XHUB_TEST_DATABASE_URL` 后检查测试输出；测试使用独立 schema。前端按受影响文件定向验证，避免无路径运行整个套件。

浏览器测试需要先安装 Chromium（`cd frontend && npx playwright install chromium`），再从根目录运行 `make e2e`；端口与服务要求见测试指南。`make regression-live` 会调用真实供应商并产生费用，需要另行配置凭证并确认运行范围。

## 文档与能力边界

客户操作见[客户手册](user-guide.zh-CN.md)，实现规则见[开发参考](development/README.md)，模块内的 `readme.md` / `readme_cn.md` 提供双语代码说明。文档记录当前行为，临时方案与执行记录不作为使用承诺。

当前没有预算预占、独立结算账本或通用异步媒体生命周期。七牛和火山引擎的供应商任务转发已实现，具体约束见[运行行为与限制](development/runtime.md)。

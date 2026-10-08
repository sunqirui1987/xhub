# 用户、团队与权限重构方案（重建版）

> **目标设计，不是当前行为。** 本文描述的是计划要实现的架构。现在实际运行的行为见 [docs/current/](../current/)。两份冲突时，以 `current/` 和代码为准。

状态：已确认，实施中。本文替代旧草稿；与 `docs/design/02-identity-and-authorization.md` 冲突时以本文为准。

## 一、目标与原则

1. 账号角色只有两种：`admin`（平台管理员）、`user`（普通用户）。团队内角色：`team_admin`（团队管理员）、`member`（成员）。组织管理员是 `organization_members` 里的成员关系，不是账号角色，也不是团队角色。不设项目管理员、访问组角色。
2. 资源层级 `平台 → 组织 → 团队 → 项目`。组织是团队的分组，由平台管理员维护；用户通过**团队成员关系**获得权限。
3. 个人归个人：个人资料、个人密钥、个人密钥产生的逐条日志只有本人可见；团队管理员也看不到；平台管理员可查但留审计。
4. 服务端唯一判权：每次请求按数据库实时关系判定；列表在 SQL 层限范围；依赖读取失败即拒绝。前端只做呈现。
5. 旧数据全部丢弃：用户、团队、密钥、用量、日志、配置清空，以新结构重建，不做兼容层或迁移。
6. **可复用的配置有归属。** 一份配置属于平台、某个组织或某个团队。归属决定谁能改；可见范围决定谁能用。两者不是一件事：一个团队可以用本组织另一个团队建的配置，但改不了它。

## 二、角色与权限矩阵

| 资源 / 操作 | 普通成员 | 团队管理员（本团队内） | 平台管理员 |
| --- | --- | --- | --- |
| 自己的账号资料、密码 | 本人 | 本人 | 全部（审计） |
| 他人账号（创建、禁用、重置密码、改角色） | ✗ | ✗ | ✓ |
| 组织（增删改） | 看所属团队的组织名 | 同成员 | ✓ |
| 团队（创建、删除、换组织、设团队预算） | 看自己所在团队 | 改团队名称/描述；不能删、不能改预算上限 | ✓ |
| 团队成员（增删、改团队角色） | 看成员列表（名称、角色） | ✓；按精确邮箱添加已存在账号；保护最后一位团队管理员 | ✓ |
| 项目（增删改、封禁） | 看 | ✓ | ✓ |
| 个人密钥 | 仅自己的；绑定自己所在团队/项目；模型只能在团队可用范围内选 | 仅自己的；看不到他人个人密钥 | 全部（审计） |
| 服务密钥（属于团队/项目） | ✗ 不可见 | 创建、轮换、禁用、删除本团队的 | 全部 |
| 逐条调用日志（含请求/响应内容） | 仅本人个人密钥产生的 | 本人的 + 本团队服务密钥的 | 全部（审计） |
| 用量汇总 | 本人明细；所在团队总量（按模型、项目） | 另加本团队按成员、按密钥、按项目的汇总（只有金额/次数，无内容） | 全局 |
| 预算 | 看本人与团队余量 | 在团队预算内给项目、服务密钥设上限 | 设平台、组织、团队、用户预算 |
| 访问组与模型授权 | 看自己可用模型 | 看团队可用范围；给项目、服务密钥收窄，不能扩大 | 创建访问组并分配给团队 |
| 路由模板（平台级） | 看 | 看 | 建、改、删 |
| 路由模板（本组织） | 看 | 看 | 建、改、删 |
| 路由模板（本团队） | 看 | 建、改、删 | 建、改、删 |
| 路由模板（其他组织的） | ✗ 不可见 | ✗ 不可见 | 全部 |
| 选用路由模板（组织/团队/密钥上） | 本团队、本组织、自己的个人密钥 | 同成员，另加本团队服务密钥 | 任意范围 |
| 审计日志 | ✗ | ✗ | ✓ |

补充：

- 团队管理员只能授予 `member`、`team_admin`，不能授予平台管理员。
- 成员选择器只返回本团队成员的 id、名称、邮箱；添加成员须输入精确邮箱，不存在或已禁用都返回同一错误，防止枚举全平台用户。
- 账号由平台管理员创建或邀请，不开放自助注册。
- **普通成员可以选用路由模板**，限自己所属的团队、自己所属的组织、以及自己的个人密钥。选一份模板是在已有配置里挑一份，不新建也不改内容；能挑的范围就是能受益的范围。
  代价要说清：成员选的这一份对该团队（或该组织的全部团队）的每一次调用都生效。团队规模小的场景下这是省事；如果哪一天想收紧，判权收回到 `ActionTeamWrite` 即可，是同一处的一个动作替换。
- **不能跨组织共享。** 一个组织建的模板只有该组织及其团队可见。想让另一个组织用同一套配置就各建一份——比多一维"发布给谁了"便宜，而那一维一旦存在，每次判定都要先问它。
- 组织管理员能管本组织下**任意团队**建的模板（`ActionTeamWrite` 的判定本来就含 `OrgAdminOf(orgID)`），但管不了别的组织。

## 三、数据模型（全新建库）

全部用显式 DDL 建表，带外键、唯一约束和 CHECK；去掉 xorm 自动同步和 `IgnoreConstrains`；去掉 `extra_json` 里的成员、角色、用户镜像字段。

```
users            id PK, email UNIQUE(lower), name, password_hash,
                 role CHECK IN ('admin','user'), status CHECK IN ('active','disabled'),
                 session_version INT, created_at, updated_at

organizations    id PK, name UNIQUE, status, created_at, updated_at

teams            id PK, organization_id FK NOT NULL, name, status, max_budget,
                 created_at, updated_at, UNIQUE(organization_id, name)
                 -- organization_id 只能通过受控迁移接口修改

team_members     team_id FK, user_id FK, role CHECK IN ('team_admin','member'),
                 created_at, PK(team_id, user_id)

projects         id PK, team_id FK NOT NULL, name, status, max_budget,
                 UNIQUE(team_id, name)       -- 组织由团队推导，不冗余存储

access_groups    id PK, organization_id FK NOT NULL, name, models JSONB,
                 status, UNIQUE(organization_id, name)

team_access_groups  team_id, access_group_id, organization_id, PK(team_id, access_group_id)
                    -- 复合外键保证访问组与团队同属一个组织

api_keys         id PK, token_hash UNIQUE, key_prefix,
                 owner_type CHECK IN ('personal','service'),
                 user_id FK NULL, team_id FK NOT NULL, project_id NULL,
                 created_by FK, name, models JSONB, access_group_ids JSONB,
                 max_budget, tpm, rpm, status, expires_at, last_used_at, created_at
                 CHECK (owner_type='personal' AND user_id IS NOT NULL
                     OR owner_type='service'  AND user_id IS NULL)
                 -- (project_id, team_id) 复合外键 → projects(id, team_id)

usage_events     id PK, request_id UNIQUE, ts, key_id, owner_type,
                 user_id, team_id, project_id, organization_id,   -- 写入时快照
                 model, call_type, status, prompt_tokens, completion_tokens, cost

request_logs     request_id PK FK→usage_events, request_body, response_body, error

usage_daily      day, organization_id, team_id, project_id, user_id, key_id,
                 owner_type, model, requests, tokens, cost

audit_logs       id, ts, actor_id, actor_kind, action, object_type, object_id,
                 team_id, detail JSONB

route_templates  id PK, name UNIQUE, body JSONB,
                 organization_id FK NULL, team_id FK NULL,
                 created_at, updated_at
                 CHECK (team_id IS NULL OR organization_id IS NOT NULL)
                 -- 两列都空 = 平台模板；只有 organization_id = 组织模板；
                 -- 两列都有 = 团队模板。team_id 单独存在没有意义，所以禁掉。
                 -- 归属用两列而不是 owner_type + owner_id：authz.Object 本来就带
                 -- OrgID 和 TeamID，OrgAdminOf / TeamAdminOf 直接吃这两列，
                 -- 换成枚举每次判定都要先解类型，多一步就能写错。

bootstrap_state  单行：是否已初始化
```

`route_templates` 的归属列是 `ON DELETE CASCADE`，和三个范围列上的 `SET NULL` 相反：那三列是"选了哪个"，模板没了等于没选；这两列是"属于谁"，归属方没了，这份配置就没有意义了。

用量事件写入时快照归属；后续成员变动不改历史账单归属。`team_members` 是判权和计费的唯一成员关系来源。

## 四、统一授权层

新包 `internal/authz`：

```go
type Actor  struct{ Kind (session|key|master); UserID; Role; KeyID; KeyTeamID; KeyProjectID; OwnerType }
type Action string
type Object struct{ Type; ID; OrgID; TeamID; ProjectID; OwnerUserID; OwnerType }

func Authorize(ctx, actor, action, object) error   // nil / ErrForbidden / ErrNotFound
func TeamFilter(ctx, actor, action) SQLScope       // 列表查询注入 WHERE
```

- 对象归属在数据库中解析后按矩阵判定；单请求内成员关系查一次，不跨请求缓存。
- 401 未认证；403 作用域内无权限；404 不存在或不可见；依赖读失败返回 500 并拒绝。
- 列表、详情、搜索、导出、统计、写入只能经 `Authorize` / `TeamFilter`。
- **选用的判权是成员级，不是管理员级。** 指向一份路由模板用的是一个独立动作（`route_template.select`），它的判定是：本团队、本组织、或自己的密钥；平台管理员任意范围。不挂在 `team.write` 上，因为那会把"能改团队设置"和"能改团队怎么路由"绑成同一件事，而这两件事的松紧程度不同。
- **可见性在写入路径上也要判。** 把一个范围指向一份配置时，必须校验这份配置对这个范围可见；不可见按 404 回答，不是 403——"存在但你没资格"本身就是信息泄露，和团队查询的既有做法一致。
  这条在只有平台能建配置的时候看不出差别；一旦组织能自己建，少了它就是跨租户越权：甲组织的团队管理员可以把乙组织的配置挂到自己团队上，然后按乙的配置路由。
- 删除 `auth.Scope`、`MemberTeamIDs`、`KeyVisibleTo`、`VisibleSpend`、`adminOrgIDs`，以及 `proxy_admin*`、`internal_user_viewer`、`org_admin` 等旧角色。

## 五、认证与会话

- UI 会话存 `user_id` + `session_version`；每次请求核对用户状态与版本；改角色、禁用、改密码时版本号 +1，旧会话立即失效。成员关系每次实时判定，无需失效会话。
- 虚拟密钥只能推理和读取自身信息，不论拥有者是谁都不带管理能力；取消 `management` 类型。
- 个人密钥每次请求校验：拥有者 active、仍是绑定团队成员、团队/项目未封禁。服务密钥校验团队/项目状态。
- 主密钥只用于首次 `/bootstrap`（创建首位平台管理员）和应急管理接口，全部审计；不能登录 UI，默认不能推理。
- 删除环境凭据自动建管理员、旧角色登录回退、无主密钥隐式提权等路径。

## 六、事务化写操作

| 操作 | 同一事务内 |
| --- | --- |
| 添加团队成员 | 校验用户 active → 插入 `team_members` → 审计 |
| 移除成员 / 降级管理员 | 检查最后一位团队管理员；吊销该用户绑定此团队的个人密钥；审计 |
| 禁用 / 删除用户 | 版本号 +1；吊销其全部个人密钥；删除时清成员关系；服务密钥不受影响 |
| 删除 / 封禁项目、团队 | 下属密钥失效；删团队级联项目、成员、访问组分配 |
| 撤销 / 删除访问组 | 删分配关系；推理时实时求交集，无需回写密钥 |
| 团队换组织 | 平台管理员专用接口；先解除跨组织访问组分配，再一次性完成，否则整体失败 |
| 批量操作 | 全成或全败 |

成员变更以 `SELECT … FOR UPDATE` 锁定团队行，串行化并发修改。

## 七、模型授权与推理

```
团队允许集 = ∪ 分配给该团队的有效访问组.models
项目允许集 = 团队允许集 ∩ 项目收窄列表（若有）
密钥允许集 = 项目或团队允许集 ∩ 密钥收窄列表（若有）
```

每把密钥只绑定一个团队（可选一个项目），不跨团队拼接；UI 会话查看模型目录时须选定团队/项目。`/v1/models` 与推理入口调用同一函数。创建密钥时所选模型、访问组必须是祖先允许集的子集。

## 七之二、路由模板：归属、可见与生效

路由设置不是全局一份。平台、组织、团队都能定义具名的**路由模板**，一个范围最多选一份。

### 归属决定谁能改，可见范围决定谁能用

| 归属 | 谁能建、改、删 | 谁可见可用 |
| --- | --- | --- |
| 平台模板（两列都空） | 平台管理员 | 所有人 |
| 组织模板（organization_id） | 该组织的管理员 | 该组织及其全部团队 |
| 团队模板（organization_id + team_id） | 该团队管理员，以及该组织的管理员 | 只有那个团队 |

**可见即可选，但不可改。** 一个组织能选平台模板和组织内任意团队建的模板，改不了别人的。这条的理由是不让每个团队都重配一份一样的东西；而改和删只能是归属方，跨组织可见是读不是写。

**不做跨组织发布。** 一个组织的模板不对其他组织可见。想让另一个组织用同一套配置，就按各自的组织建一份——这比多一维"谁能看到什么"便宜，而那条维度一旦存在，每次判定都要先问"发布给谁了"。

### 生效：窄的那一层赢，整份替换

```
密钥 > 团队 > 组织 > 平台默认
```

第一个选了模板的层整份生效，不和上一级按字段合并。合并要回答"两层的 num_retries 谁赢""权重按谁归一""策略 A 加策略 B 是什么"——这些问题没有好答案，而它们的答案还会让"组织改了设置、团队行为跟着变"。

因为整份替换，一份模板必须是一份**完整**文档。所以新建模板时用平台默认做底，而不是从空文档开始：模板里少写 num_retries 是"只尝试一次"，少写 timeout 是"没有超时"。这两个都可以配，但不该是新建一份模板时不小心得到的。

**归属不影响生效。** 一个团队绑了本组织另一个团队建的模板，生效的是那份模板的内容，来源标的是**绑定它的那一层**。谁建的、和它在谁的请求上生效，是两件事。代价是兄弟团队改了那份模板，这个团队的行为跟着变——这是共享配置的应有之义，所以界面上必须显示来源里的模板名。

### 会话调用也要解析

预算链只在有密钥行时才走，而控制台演练场是**会话**调用。会话按成员关系推团队：只属于一个团队就用那个团队，属于多个则不猜（用平台默认）。少了这一段，团队选了模板、演练场里的请求却仍然按平台默认走。

---

## 八、用量、日志与预算

写入：推理结束写 `usage_events` + `request_logs`（带归属快照），并累加 `usage_daily`。

| 接口 | 普通成员 | 团队管理员 | 平台管理员 |
| --- | --- | --- | --- |
| `GET /usage/me` | 本人明细与汇总 | 同左 | 同左 |
| `GET /usage/teams/{id}` | 团队总量（按模型、项目） | 另加按成员、按密钥 | 全部 |
| `GET /logs` | `user_id=本人 AND owner_type=personal` | 本人的 ∪ (`team_id∈管理团队 AND owner_type=service`) | 全部（查看内容审计） |
| `GET /logs/{request_id}` | 同上，否则 404 | 同上 | 同上 |
| 导出 | 同列表范围 | 同列表范围 | 同列表范围 |

普通成员的团队汇总不含按成员维度，也不能跳转他人明细。推理前依次检查用户、密钥、项目、团队、组织预算，任一超限即拒绝。

## 九、接口

- 认证：`/auth/login`、`/auth/logout`、`/auth/me`（当前用户、团队及团队角色、能力列表）。
- 初始化：`/bootstrap`（仅未初始化时、仅主密钥、一次性）。
- 平台管理：`/admin/users`、`/admin/organizations`、`/admin/teams`（创建、删除、换组织、预算）、`/admin/access-groups`、`/admin/audit`。
- 团队：`/teams`（我所在的）、`/teams/{id}`、`/teams/{id}/members`、`/teams/{id}/projects`、`/teams/{id}/service-keys`、`/teams/{id}/models`。
- 个人：`/me/keys`、`/me/profile`、`/me/password`。
- 用量与日志：见第八节。
- 路由模板：`/route_templates`（可见的那些）、`/route_templates/{id}`、`/route_templates/{id}/usage`，以及 `GET|POST /route_templates/binding`（在某个范围上读取或设置选择）。三个范围自己的更新接口也接这个字段。
- 推理：`/v1/*` 保持 OpenAI 兼容，判权换成第五、七节规则。
- 旧的 LiteLLM 兼容管理接口（`/key/*`、`/team/*`、`/user/*`、`/organization/*` 等）下线；确需保留的必须转到新 handler 与判权。

## 十、前端

- 用 `/auth/me` 返回的能力控制页面与按钮；`utils/roles.ts` 只认 `admin`、`user`，团队内认 `team_admin`。
- 普通用户：我的密钥、我的用量、我的团队（成员、项目、团队用量）、可用模型。
- 团队管理员：另加成员管理、项目管理、服务密钥、团队日志。
- 平台管理员：另加用户、组织、团队、访问组、全局用量、审计。
- 页面只请求当前角色有权访问的接口；顶部团队切换器决定用量、模型、项目的作用域。

## 十一、实施顺序

每阶段结束可编译、测试通过。

1. 建库：新 DDL + 约束，替换 xorm 自动同步；新 store 层；删除 JSON 成员与镜像字段。
2. `internal/authz` + 完整单元测试矩阵；重写 `auth`（会话版本、主密钥初始化、删旧角色）。
3. 身份与成员：用户、组织、团队、成员、项目接口，事务化。
4. 密钥：个人/服务区分，创建、轮换、吊销、推理时校验。
5. 访问组与模型：授权求交集，目录与推理共用一个函数。
6. 用量、日志、预算：写入快照、汇总表、范围查询与导出、日志查看审计。
7. 前端改造。
8. 路由模板：归属列；可见性校验（**这一步先做，它补的是一个已经存在的越权**）；按归属过滤列表与选择；界面分区。
9. 清理：删除旧接口、旧角色、死代码；修复 `logx` 测试；更新 `docs/design/02-identity-and-authorization.md`。

## 十二、发布（全库重建）

1. 隔离环境演练：空库 → 初始化 → 完整链路。
2. 生产执行前停写；备份并验证可恢复；**再次确认目标实例和清空范围**。
3. 清空旧表（含 `kv`、`proxy_config` 等平台配置），执行新 DDL，部署新版本。
4. 用主密钥 `/bootstrap` 创建首位平台管理员；重新配置模型、组织、团队、访问组。
5. 任一步失败不开放流量，用备份回滚旧版本。

生产清库由部署者执行。

## 十三、测试与验收

- 夹具：2 个组织 × 2 个团队 × 2 个项目；用户：平台管理员、A 团队管理员、A 成员兼 B 团队管理员、普通成员、已禁用用户。
- 每个角色 × 每类资源 × 列表/详情/搜索/导出/写入/统计/推理，结果与第二节一致。
- 越权：猜他人团队 ID、伪造 `team_id`/`project_id`、直接调 API，均返回 404/403。
- 个人隐私：团队管理员看不到成员的个人密钥和个人日志，只能看到汇总数字。
- 即时生效：移除成员、降级、禁用用户、撤销访问组、封禁项目/团队后，现有会话和密钥的下一次请求即被拒绝。
- 并发与失败：两人同时移除最后的团队管理员必有一方失败；批量中途失败整体回滚；数据库错误时拒绝。
- 模型目录与推理一致；不能跨团队拼接；普通密钥无法获得管理权限。
- 端到端：空库初始化 → 组织 → 团队 → 成员 → 项目 → 访问组 → 密钥 → 推理 → 计费 → 用量与日志，每个角色走一遍。
- 路由模板的越权：把本团队指向别的组织的模板必须被拒（404），且那一次写不能落库；组织管理员改不了别的组织或别的团队的模板；普通成员改不了任何模板的**内容**（改和选用是两件事），也不能在一个不属于自己的团队或组织上选用。
- 路由模板的选用按范围成立：普通成员能给自己的团队选、能给自己的组织选（如果他是组织成员）、能给自己的个人密钥选；对别人的团队、别人的组织、别人的密钥一律拒绝。
- 路由模板的继承：组织选了、团队没选 → 团队用组织那一份（不是跳回平台默认）；团队自己选了 → 只用团队那一份；清除选择 → 回到继承。
- 路由模板对计费生效：平台默认重试 1 次、模板 3 次，同一批失败请求的上游尝试次数分别是 1 和 3；会话（演练场）和密钥两条入口都要断言，因为预算链只覆盖后者。

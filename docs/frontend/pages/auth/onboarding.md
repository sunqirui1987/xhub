# 开通邀请

- Route: `/onboarding`
- Nav: AUTH
- Status: `specified`

## 目的

受邀用户用 invitation token 设密码并激活；也可走重置密码。成功后清旧 session、写入新 token，整页去 `/?login=success`（Keys）。无管理壳。

变体由 `?action=` 决定：缺省 **signup**（Claim your user account）；`action=reset_password` 为 **Reset Password**。

## 布局

居中 `max-w-md` 卡片（`mt-10`）。无侧栏。

### 顶栏

无 DashboardHeader。卡片内标题：产品名 + Sign Up 或 Reset Password + 一句说明。

### 筛选

无。

### 表

无。不是资源列表。

### Tab

无。signup / reset 是同一表单不同文案，靠 query，不是 Tab。

### 抽屉

无。

### 模态

无。失败用页内 Alert，不是模态。

### URL

| Query | 作用 |
|---|---|
| `invitation_id` | 必填。`GET /onboarding/get_token?invite_link=` |
| `action=reset_password` | 重置密码文案与按钮 |
| 其它 | 从 `/?invitation_id=` 整段转到本路由时原样带上 |

表单区：

1. signup 时一条 SSO 说明（企业能力；外链试用），reset 不显示。
2. Email Address：只读 disabled，值来自邀请 JWT 的 `user_email`。
3. Password：signup「Create a password」；reset「Enter your new password」。无单独 confirm 字段（源码单字段）。
4. 认领失败 Alert。
5. 提交按钮 Sign Up / Reset Password。

无效 / 过期邀请：**不是这张表**，见交互。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 读取邀请 | `GET /onboarding/get_token?invite_link={invitation_id}` | 解码 token，填只读 email；取出 `user_id` 与临时 `key` |
| 认领 / 重置 | `POST /onboarding/claim_token` body `{invitation_link, user_id, password}`，Authorization 为邀请里的临时 key | 清旧 cookie，`storeLoginToken`，整页去 `/?login=success` |

无 `invitation_id` 时不发 get_token。

## 字段

| 字段 | 含义 |
|---|---|
| `invitation_id` / `invitation_link` | 邀请 id（query 与 POST 字段名） |
| `user_email` | 只读邮箱 |
| `user_id` | 邀请 JWT 内，随 claim 提交 |
| `password` | 新密码，必填 |
| `token` | get_token 返回的邀请 JWT；claim 成功返回 session token |

## 交互

本页在建立自己的 session 之前，不套 Admin Viewer 规则。成功后的 view-only 由壳处理。

| 操作 | 结果 |
|---|---|
| 有 invite、凭据 loading | 卡片位置中央 spinner「Loading invitation」，不闪空表单。 |
| get_token 失败 / 过期 / 非法 | OnboardingErrorView：Failed to load invitation（link may be invalid or expired）+ Back to Login。无密码框。 |
| 缺 `invitation_id` | query 不启用，走错误/不提交。 |
| 空密码提交 | 字段「password required to sign up」，不发 claim。 |
| 点 Sign Up / Reset Password | 按钮 spinner、禁用；成功整页跳转 Keys。失败 Alert，可改密码重试。超时锁定原 payload。 |
| 认领成功但无 session token | 「Failed to start session. Please try again.」留在本页。 |
| 点 Back to Login | 去 `/login`。 |
| 无资源 empty | 不适用。过期邀请是 **forbidden/invalid**，不是「还没有用户」。 |
| 无 SecretOnce | 不展示 `sk-`。写入的是 session cookie，不是虚拟 Key。 |

## 验收

桌面与窄屏：有效邀请设密→进 Keys；过期邀请错误 + 回登录；reset_password 文案分支。不得 mock 表格。

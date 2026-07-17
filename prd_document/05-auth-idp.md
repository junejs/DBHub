# 05 — 认证与身份集成

系统必须与企业身份体系集成，使「人员身份」成为数据库访问授权的主体，而非散落的数据库账号。

---

## 1. 用户故事

- 作为平台负责人，我希望接入公司的 OIDC/LDAP，员工用统一账号登录。
- 作为平台负责人，我希望员工入职/离职时，权限随身份系统自动开通/回收。
- 作为员工，我希望用 SSO 一次登录即可，无需再记一套数据库平台密码。
- 作为安全管理员，我希望关键角色强制二次验证（MFA）。

---

## 2. 认证方式

| 方式 | 说明 | 本期 |
|---|---|---|
| **本地账号 / 密码** | 内置账号，bcrypt 存储；用于初始化与无 IdP 场景 | ✅ |
| **OIDC** | OpenID Connect（Keycloak、Okta、Entra ID、Authing 等） | ✅ |
| **LDAP** | 目录绑定 + 搜索认证（AD/OpenLDAP） | ✅ |
| **OAuth2** | 通用 OAuth2（Google/GitHub/自建） | 可选增强 |
| **邮箱验证码登录** | Passwordless，6 位码 | 可选 |
| **SCIM 2.0 目录同步** | Entra ID / Okta 自动同步用户与组 | 可选增强 |
| **SAML** | — | ❌ 不支持（Bytebase 亦未支持） |

> OIDC 与 LDAP 是企业集成的两条主路径，本期必须支持。

---

## 3. 身份提供商（IDP）配置模型

```
IdentityProvider {
  name    // idps/{id}
  title   // 显示名
  domain  // 邮箱域（用于登录时匹配/路由）
  type    // OIDC / OAUTH2 / LDAP
  config  // 协议特定配置（oneof）
}

OIDCConfig {
  issuer, client_id, client_secret,
  scopes, field_mapping, skip_tls_verify, auth_style
  // auth_endpoint 由 issuer/.well-known/openid-configuration 自动发现
}

LDAPConfig {
  host, port, security_protocol (START_TLS / LDAPS),
  bind_dn, bind_password, base_dn, user_filter,
  skip_tls_verify, field_mapping
}

FieldMapping {        // 把 IdP 返回字段映射到平台用户属性
  identifier (必填，通常邮箱), display_name, phone, groups
}
```

---

## 4. 登录流程

统一登录入口 `Login`，按请求形态分发：

1. **密码登录**：邮箱 + 密码（bcrypt 校验）。
2. **SSO（OIDC/OAuth2）**：前端跳转 IdP 授权 → 回调带 code → 后端换 token → 取 UserInfo → 匹配/创建用户。
3. **LDAP**：后端用服务账号 bind → 按 `user_filter` 搜索用户 DN → 用用户密码二次 bind 验证。
4. **邮箱验证码**：发 6 位码（HMAC 存储、限频、10 分钟有效）。
5. **MFA 二次验证**：若用户开启 TOTP 且工作区要求 2FA，第一步返回短期 `mfa_temp_token`，第二步提交 OTP/恢复码完成登录。

### 登录后处理
- **用户匹配/创建（JIT Provisioning）**：IdP 返回的 `identifier`（邮箱）匹配已有用户则登录；不存在则按策略自动创建（仅限允许的邮箱域）。单组织自部署，无席位/多租户概念（D1）。
- **组映射**：IdP 的 `groups` claim 与平台组按邮箱/名称匹配，自动加入/移除，IAM 缓存随之刷新。
- **发 token**：签发 JWT access token（默认 1h）+ 不透明 refresh token（默认 7d，SHA256 存储）。
- **Cookie/Body**：Web 端 HTTP-only Cookie；API 端返回 token。

---

## 5. 会话与令牌

| 令牌 | 说明 |
|---|---|
| **Access Token (JWT)** | HS256 签名；默认 1h；`Authorization: Bearer` 或 `access-token` Cookie |
| **Refresh Token** | 32 字节随机，SHA256 存储；旋转刷新（非滑动续期，保留原始绝对过期） |
| **MFA Temp Token** | 短期（5 min），登录两步之间的桥接 |
| **Service Account Token** | （v2）给自动化用的长期 token（API token） |

- **登出**：删除 refresh token、清 Cookie。
- **改密/重置密码**：吊销该用户所有 refresh token。
- **限频防爆破**：密码 10 次/10min、MFA 5 次/5min 失败锁定（参考审计日志计数）。

---

## 6. MFA（2FA）

- **TOTP**（RFC 6238，Google Authenticator 等）。
- **恢复码**：一次性、恒定时间比较。
- **两阶段启用**：先生成 temp secret → 用户验证一次 OTP → 提升为正式。
- **工作区强制 2FA**：可配置要求全部/关键角色开启；管理员可豁免自身。

---

## 7. 用户、组与权限联动

- 用户来源记录（本地 / OIDC / LDAP / SCIM）。
- 组（Group）= 成员集合，是 IAM 绑定中 `group:{email}` 的主体。
- IdP 组 claim → 平台组自动同步（增删成员）。
- 离职处理：IdP 侧禁用/删除用户后，登录即失效；建议配合 SCIM 或定期同步实现自动回收。

---

## 8. Service Account（服务账号）—— 不在 v1 范围

> v1 面向自然人用户，不提供 Service Account / API token（路线图见 [18 §1.6](./18-roadmap.md)）。

---

## 9. 安全要求

- 所有 IdP/业务库连接 TLS；LDAP 支持 StartTLS/LDAPS。
- IdP `client_secret`、`bind_password` 加密存储，禁止明文。
- 邮箱域白名单：限制可登录/可 JIT 创建的邮箱域。
- 登录失败/成功均审计（见 [06](./06-audit-log.md)）。
- 防 user enumeration：登录失败返回统一模糊错误。

---

## 10. 功能范围

- 本地账号/密码 + bcrypt。
- OIDC 集成（含 discovery、组映射、JIT）。
- LDAP 集成（StartTLS/LDAPS、bind+search）。
- MFA（TOTP + 恢复码 + 工作区强制 2FA）。
- 会话/JWT/refresh token、登出、限频。

**可选增强（不在 v1 范围，见 [18 §1.6](./18-roadmap.md)）：** 通用 OAuth2、邮箱验证码登录、SCIM 2.0 目录同步、Service Account/API token、密码策略增强、登录风险评分。

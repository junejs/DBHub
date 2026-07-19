# DBHUB 业务术语表（统一语言 / Ubiquitous Language）

> 本表是 DBHUB 的**统一语言**（DDD ubiquitous language）：代码命名、数据库表/字段设计、API 资源名都以本表的**英文术语**为准。所有人（产品/开发/AI）对同一概念用同一个词，杜绝同义词混用导致的歧义与返工。
>
> 与 PRD 冲突时以 PRD（`docs/prd/`）为准；本表是 PRD 概念的命名落地。数据模型见 [10-data-model.md](../prd/10-data-model.md)，API 资源见 [12-api-contract.md](../prd/12-api-contract.md)。

---

## 0. 怎么用

- **新增功能/表/字段前**，先在本文查"这个概念叫什么"；查不到再新增条目（并同步 PRD）。
- **命名优先级**：本表英文术语 → Go/TS 标识符规则（§2）→ DB 规范（§3）→ API 规范（§4）。
- **禁止**用同义词自创术语（例：统一用 `Worksheet`，不要又出现 `SavedQuery`/`Snippet`/`Note`）。

---

## 1. 命名总则

| 维度 | 规则 | 例 |
|---|---|---|
| 英文术语 | 单数、概念名、行业通用词 | `Project`、`Instance`、`Worksheet` |
| 资源集合（API 路径/列表） | 复数 | `/v1/projects`、`items: Project[]` |
| DB 表名 | 复数蛇形 | `projects`、`export_tasks`、`audit_logs` |
| DB 列名 | 蛇形 | `created_at`、`project_id`、`deleted_at` |
| Go 类型 | PascalCase | `type ProjectService struct` |
| Go 方法/字段 | PascalCase（导出）/ camelCase（私有） | `ListProjects`、`projectID` |
| TS 类型/组件 | PascalCase | `type Project`、`ProjectList` |
| TS 变量/函数 | camelCase | `listProjects`、`pageSize` |
| JSON / 查询参数 | **snake_case** | `next_page_token`、`page_size` |
| 常量 | Go: PascalCase；TS: UPPER_SNAKE | `DefaultPageSize`、`PAGE_SIZE` |
| 缩写 | 全大写当词、不拆点 | `ID`（非 `Id`）、`URL`、`API`、`IAM`、`OIDC` |

> 前端入参可 camelCase（`pageSize`），在 API 层映射为契约的 snake_case（见 `src/hooks/useProjects.ts`）。

---

## 2. 术语表

> 列说明：**术语**（EN，权威）/ **中文** / **定义** / **代码命名**（Go 类型 · TS）/ **DB**（表 · 关键列）/ **备注**（易混/禁忌）。

### 2.1 身份域（Identity）

| 术语 | 中文 | 定义 | 代码命名 | DB | 备注 |
|---|---|---|---|---|---|
| **User** | 用户 | 平台用户；含自然人与服务账号，以 `kind` 区分 | `User`(Go/TS) | `users` · `email`, `kind`, `status`, `source` | 不用 `Account`/`Member`（Member 仅指组成员关系） |
| **User kind** | 用户类型 | `user` \| `service` | `kind: user\|service` | `users.kind` | v1 实际只用 `user`；`service` 延后 |
| **Group** | 用户组 | 用户的集合，可与 IdP 组同步，用于 IAM 批量授权 | `Group` | `groups` · `group_members` | 不用 `Team`/`RoleGroup` |
| **GroupMember** | 组成员 | 用户与组的归属关系 | `GroupMember` | `group_members(group_id,user_id)` | 联结表 |
| **IdentityProvider (IdP)** | 身份提供商 | OIDC/LDAP/OAuth2 外部身份源 | `IdentityProvider` | `identity_providers` · `type`, `config`, `secret_ref` | 简称 IdP；不用 `SSO`(SSO 是行为不是实体) |
| **RefreshToken** | 刷新令牌 | 不透明刷新凭证，存 SHA256 | `RefreshToken` | `refresh_tokens` · `token_hash`, `expires_at` | 明文不存；DB 存 `token_hash` |
| **LoginAttempt** | 登录尝试 | 失败计数/锁定态（防暴力） | `LoginAttempt` | `login_attempts` · `identifier`, `channel`, `locked_until` | 与 AuditLog 不同：同步判锁定，不依赖审计 |

### 2.2 资源组织（Resource Organization）

| 术语 | 中文 | 定义 | 代码命名 | DB | 备注 |
|---|---|---|---|---|---|
| **Project** | 项目 | 产品团队的**逻辑隔离边界**（一等）；实例/库归属项目 | `Project` | `projects` · `key`, `name` | 不用 `Workspace`/`Tenant`/`Organization`；这是隔离单元 |
| **Environment** | 环境 | 一等策略维度：dev/test/stage/prod，驱动访问控制与护栏 | `Environment` | `environments` · `key`, `protection_level` | 不是标签/Tag；有 `protection_level` |
| **EnvironmentPolicy** | 环境策略 | 按环境差异化的查询/导出护栏 | `EnvironmentPolicy` | `environment_policies` · `query_row_limit`, `export_max_rows` | 与 IAM 条件不同（这是运行时护栏） |
| **Instance** | 实例 | 一个物理 DB 连接目标；**归属且仅归属一个 Project** | `Instance` | `instances` · `project_id`, `key`, `engine`, `environment_id` | 不是"数据库服务器"的同义词——指平台注册的一个连接目标 |
| **DataSource** | 数据源 | 实例下的连接配置，按角色：`admin` \| `readonly`（每实例各至多 1） | `DataSource` | `data_sources` · `role`, `host`, `connection`, `secret_ref` | `{dataSource}` 资源名段即角色；不用 `Connection`/`Credential` |
| **Database** | 数据库 | 实例内的**逻辑库**（业务数据所在）；项目归属由其实例决定 | `Database` | `databases` · `instance_id`, `name` | ≠ 平台元数据库；≠ Instance；见 §5 辨析 |
| **DatabaseSchema** | 库结构快照 | 某库的结构（表/列/视图…）反射快照，每库一行整体替换 | `DatabaseSchema` | `database_schemas` · `schema_json`, `sync_status` | ≠ Database；≠ 引擎 schema（见 §5） |
| **SyncHistory** | 同步历史 | 每次 schema 快照留档，用于结构变更对比 | `SyncHistory` | `sync_history` | 按时间清理 |

### 2.3 权限（Access Control / IAM）

| 术语 | 中文 | 定义 | 代码命名 | DB | 备注 |
|---|---|---|---|---|---|
| **Role** | 角色 | 权限集合；v1 仅预置（workspace/project 两层） | `Role` | `roles` · `key`, `scope`, `builtin` | v1 不做自定义角色（D35） |
| **Permission** | 权限 | 扁平字符串，命名 `db.<resource>.<verb>`（如 `db.sql.select`） | 常量集合（代码注册表） | `role_permissions(role_id,permission)` | 不用 `Ability`/`Action`/`Right` |
| **RoleAssignment (Binding)** | IAM 绑定 | 把角色授予成员（workspace/project 范围），可选条件 | `RoleAssignment` | `role_assignments` · `scope_type`, `member_kind`, `condition` | 类 GCP IAM Binding；"绑定"="授予" |
| **BindingCondition** | 绑定条件 | 结构化授权条件：environments/databases/schemas/tables | `BindingCondition`（JSONB） | `role_assignments.condition` | 不暴露表达式语言、不用 CEL（D34） |
| **Member** | 成员 | IAM 绑定的被授予方：`user:{email}` \| `group:{email}` \| `allUsers` | 字符串前缀 | — | ≠ GroupMember（那是组归属） |
| **Workspace** | 工作区 | v1 **退化为单一固定实例**（单组织自部署，D1） | 固定常量（`scope_id=0`） | 无独立表 | 概念上保留但**不暴露为 API 资源**，禁止当多租户用 |

### 2.4 查询与协作（SQL & Collaboration）

| 术语 | 中文 | 定义 | 代码命名 | DB | 备注 |
|---|---|---|---|---|---|
| **Worksheet** | 查询表/工作表 | 保存的查询（含 SQL 文本、可见性） | `Worksheet` | `worksheets` · `title`, `content`, `visibility`, `etag` | 不用 `SavedQuery`/`Snippet`/`Note`/`Script` |
| **QueryHistory** | 查询历史 | 仅本人的查询/导出执行记录 | `QueryHistory` | `query_history` · `statement`, `kind`, `duration_ms` | ≠ Worksheet（历史是自动日志，Worksheet 是人工保存） |
| **Favorite** | 收藏 | 指向 Worksheet 的个人书签（星标） | `Favorite` | `favorites(user_id,worksheet_id)` | v1 收藏即星标（D36）；不用 `Bookmark`/`Star` 作类型名 |

### 2.5 导出（Export）

| 术语 | 中文 | 定义 | 代码命名 | DB | 备注 |
|---|---|---|---|---|---|
| **ExportTask** | 导出任务 | 异步导出大结果集的任务 | `ExportTask` | `export_tasks` · `state`, `format`, `row_count` | state: CREATED\|RUNNING\|SUCCEEDED\|FAILED\|EXPIRED |
| **ExportArchive** | 导出产物 | 任务产出的加密 ZIP（存储抽象：local/S3…），有过期 | `ExportArchive` | `export_archives` · `storage`, `location`, `expires_at` | ZIP 密码用户自设、平台不存（D40） |

### 2.6 横切（Cross-cutting）

| 术语 | 中文 | 定义 | 代码命名 | DB | 备注 |
|---|---|---|---|---|---|
| **AuditLog** | 审计日志 | 请求/操作的不可变记录（含 SQL 字面量） | `AuditLog` | `audit_logs`（按月分区）· `method`, `actor_id`, `severity` | 只 INSERT；按月分区；读权限受控（D26） |
| **Notification** | 站内通知 | 用户站内消息（如 `export_done`） | `Notification` | `notifications` · `type`, `read_at` | v1 仅站内，不做邮件（D21） |
| **Setting** | 设置 | 平台 k/v 设置 | `Setting` | `settings(key,value)` | value 用 jsonb |

---

## 3. 数据库命名与设计规范

> 平台元数据库（PostgreSQL）。详见 [10-data-model.md](../prd/10-data-model.md) §1 设计原则。

1. **主键**：内部表统一 `bigint generated always as identity`；仅对外随机器密（刷新令牌、访问令牌、分享链接）用 `uuid`/哈希。
2. **表名**：复数蛇形（`users`、`role_assignments`、`export_tasks`）。
3. **列名**：蛇形；外键列 `_id` 后缀（`project_id`、`user_id`）；时间戳 `_at` 后缀（`created_at`、`updated_at`、`deleted_at`、`expires_at`）。
4. **布尔列**：`is_`/`has_` 前缀或状态语义名（`enabled`、`mfa_enabled`、`activation`）。
5. **软删除**：统一 `deleted_at timestamptz`（可空）+ `where deleted_at is null` 部分唯一索引；**不引入** `deleted bool`。
6. **审计列**：业务表带 `created_at`/`updated_at`；有创建/更新人语义的加 `created_by`/`updated_by`（bigint → users）。
7. **单组织**：**无** `tenant_id`/`workspace_id` 列（D1；未来需要再统一迁移）。
8. **引擎字段**：`engine` 存 `text`（`'postgresql'`），非数据库枚举——便于扩展不改表。
9. **枚举**：存 `text`（如 `state`、`status`、`kind`、`source`），不用 PG enum 类型（加值免迁移）。
10. **邮箱**：`citext`（大小写不敏感）+ `lower(email)` 部分唯一索引。
11. **JSONB 受控**：仅用于真正半结构化字段（`connection`、`schema_json`、`settings`、`condition`、`metadata`）；关系能表达的用关系表，不当万能仓库。
12. **凭据**：DB 不存明文；`connection.password_enc`/`secrets_enc` 用 `bytea`（AES-256-GCM）；`secret_ref text` 指向外部 Secret Manager。
13. **审计表分区**：`audit_logs` 按月 `partition by range (created_at)`。
14. **索引**：热路径必建（见 10-data-model §6）；部分唯一索引配合软删；`audit_logs` 多列 + scope 索引。
15. **命名禁忌**：避免 SQL 保留字作列名；联表中外键列建议带表前缀消歧（`p.key`、`i.project_id`）。

---

## 4. API 命名规范

> 详见 [12-api-contract.md](../prd/12-api-contract.md)。

1. **资源路径**：层级反映归属（`projects/{project}/instances/{instance}/databases/{database}`）；集合复数。
2. **路径段标识**：`{project}`/`{instance}`/`{environment}` 为小写 slug `[a-z][a-z0-9-]{1,62}`；库名/表名按引擎大小写规则原样保留。
3. **标准方法**：`Get`(GET)/`List`(GET)/`Create`(POST)/`Update`(PATCH)/`Delete`(DELETE)。
4. **自定义动词**：`:verb` 后缀（`:query`、`:export`、`:sync`、`:download`、`:test`、`:search`、`:undelete`、`:favorite`、`:markRead`）。
5. **字段大小写**：JSON 与查询参数一律 **snake_case**（`page_size`、`next_page_token`、`create_time`）。
6. **资源名**：完整资源名为字符串路径（`projects/{key}`、`users/{id}`、`.../dataSources/{role}`）；`{dataSource}` 取角色 `admin`/`readonly`。
7. **错误**：统一 `Error{code,message,details[]}`，业务码 `details[].reason`（稳定大写蛇形，如 `QUERY_ROW_LIMIT_EXCEEDED`）。
8. **分页**：`page_size` + 不透明 keyset `page_token`；响应 `items[]` + 可选 `next_page_token`；大表可不返回 `total_size`。
9. **声明式扩展**：每操作标 `x-requires-permission`（`db.<resource>.<verb>`）/ `x-audit` / `x-auth-method` / `x-allow-without-credential`。

---

## 5. 易混淆词辨析（必读）

| 易混点 | 正确理解 |
|---|---|
| **Platform DB vs Business DB** | 平台**元数据库**（PostgreSQL，存用户/权限/审计/任务）与用户查询的**业务数据库**（外部 PG/MySQL…）物理隔离；平台不持久化业务数据（导出产物除外且有保留期）。代码中 `db`/`pool` 指平台库；连业务库走 `DataSource` |
| **Database vs Instance vs DataSource** | `Instance`=注册的一个连接目标；`Database`=实例内的逻辑库；`DataSource`=实例下的连接配置（admin/readonly）。一个 Instance 可含多个 Database、最多 2 个 DataSource |
| **Database vs DatabaseSchema** | `Database` 是逻辑库资源；`DatabaseSchema` 是其结构的**快照**（反射结果）。引擎里的 schema（如 `public`）是 `DatabaseSchema.schema_json` 内的一个层级，不单独建表 |
| **Project vs Workspace vs Tenant** | v1 用 **Project** 做隔离；`Workspace` 退化为单一固定常量（不暴露、不建表）；**不做 Tenant**（单组织自部署，D1）。代码里禁用 tenant 概念 |
| **Role vs Permission vs RoleAssignment** | `Role`=权限集合；`Permission`=单个扁平能力串；`RoleAssignment`(=Binding)=把 Role 授予 Member（可带 Condition） |
| **Member vs GroupMember** | `Member`=IAM 绑定的被授方（`user:x@`/`group:y@`/`allUsers`，字符串）；`GroupMember`=用户在组内的归属记录（联结表行） |
| **Environment vs Label/Tag** | `Environment` 是一等策略维度（有 protection_level，驱动访问控制+护栏）；不是随意标签。标签用 `labels`（jsonb），二者不同 |
| **Worksheet vs QueryHistory** | `Worksheet`=人工保存的查询（可分享/收藏，带 etag）；`QueryHistory`=自动记录的执行日志（仅本人）。不混用 |
| **ExportTask vs ExportArchive** | `ExportTask`=导出任务（过程/状态）；`ExportArchive`=任务产出的加密 ZIP 文件（存储位置+过期） |
| **Sync vs Refresh** | 统一用 **sync**（`sync_status`、`last_sync_at`、`:sync`、`:syncSchema`）；不用 refresh |
| **admin/readonly (DataSource role)** | 数据源角色名固定这两值；`{dataSource}` 路径段即此。不用 `read`/`write`/`primary`/`replica` |

---

## 6. 弃用 / 禁用词

| 禁用 | 改用 | 原因 |
|---|---|---|
| `tenant` / `organization`（作隔离单元） | `project` | 单组织自部署，不做多租户（D1） |
| `workspace`（作可变实体/列/API 资源） | 固定常量（不暴露） | v1 退化为单一实例 |
| `account`（指用户） | `user` | 统一术语 |
| `saved query` / `snippet` / `note` / `script` | `worksheet` | 统一术语 |
| `bookmark` / `star`（类型名） | `favorite` | 统一术语 |
| `connection` / `credential`（指数据源） | `data source` | 区分连接配置与凭据 |
| `master/slave`（数据库主从） | `primary/replica` 或 `read-write/read-only` | 中性术语 |
| `blacklist` / `whitelist` | `blocklist` / `allowlist` | 中性术语 |
| `CEL` / 表达式策略语言 | 结构化条件（结构化字段） | 不暴露表达式语言（D34） |

---

## 7. 维护

- 新增领域概念时：先加 PRD（`docs/prd/`）→ 再加本表条目 → 再写代码/建表/加 API。
- 发现代码/DB/API 出现本表之外的术语，视为"术语漂移"——要么改回统一术语，要么在此登记新词并说明理由。

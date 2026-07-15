# 10 — 数据模型

> 平台自身元数据库（PostgreSQL）的表结构设计。原则：**简洁、规范、可扩展**。Bytebase 仅作思路参考——其 JSONB proto blob、store/API 双份 proto、workspace 多租户等历史包袱**一律不照搬**。这是一个新项目，无兼容性约束。

---

## 1. 设计原则

1. **主键**：内部表统一用 `bigint generated always as identity`；仅对外暴露的随机秘密（刷新令牌、访问令牌、分享链接）用 `uuid` / 哈希。
2. **软删除**：统一用 `deleted_at timestamptz`（可空），唯一约束用 `where deleted_at is null` 的部分索引实现。不引入 `deleted bool`。
3. **审计列**：所有业务表带 `created_at` / `updated_at`；带「创建/更新人」语义的额外加 `created_by` / `updated_by`（bigint，指向 users）。
4. **单组织**：不做多租户，**没有 `workspace_id` / `tenant_id` 列**。若将来确需多租户，再统一加列——届时是一次性迁移，而非现在背着它。
5. **引擎可扩展**：`engine` 存为 `text`（如 `'postgresql'`），不是数据库枚举。新增引擎 = 加插件代码 + 数据行，**无需改表结构**。
6. **JSONB 受控使用**：仅用于真正半结构化的字段（连接参数、schema 快照、设置、策略增量）。关系能表达的就用关系表，避免把 JSONB 当万能仓库。
7. **CEL 条件**：以 `text` 存储，在应用层用 `cel-go` 求值。
8. **命名**：表名复数蛇形（`users`、`role_assignments`）；外键列 `_id` 后缀；时间戳 `_at` 后缀。

---

## 2. ER 概览

```
身份域
  users(kind=user|service) ──< group_members >── groups
  users ──< refresh_tokens / access_tokens
  identity_providers

资源组织
  projects            environments(tag)
       │
       ▼
  instances ──< data_sources          （实例归属项目，由项目团队管理）
       │
       ▼
  databases                            （库的项目归属由实例决定）
       │
       ▼
  database_schemas (1:1)        column_annotations (per column)

权限
  roles ──< role_permissions
  role_assignments ──(scope: project|workspace)──> roles
  access_grants (JIT)

脱敏
  data_classifications      semantic_types(algorithm)
  masking_rules             masking_exemptions

查询/协作
  query_history             worksheets ──< favorites
                            worksheets ──< share_links

导出
  export_tasks ──> export_archives

横切
  audit_logs (按月分区)      settings(k/v)
```

---

## 3. DDL

### 3.1 身份与认证

```sql
-- 用户（含服务账号，用 kind 区分；避免单独建表）
create table users (
  id              bigint generated always as identity primary key,
  kind            text not null default 'user',      -- user | service
  email           citext not null,
  name            text not null,
  status          text not null default 'active',    -- active | disabled | deleted
  source          text not null default 'local',     -- local | oidc | ldap | oauth2 | scim
  password_hash   text,                              -- bcrypt；SSO-only 用户为 null
  mfa_secret_enc  bytea,                             -- TOTP secret（应用层加密）
  mfa_enabled     boolean not null default false,
  last_login_at   timestamptz,
  created_at      timestamptz not null default now(),
  updated_at      timestamptz not null default now(),
  deleted_at      timestamptz
);
create unique index users_email_uidx on users(lower(email)) where deleted_at is null;

-- 组（可与 IdP 组同步）
create table groups (
  id            bigint generated always as identity primary key,
  name          text not null,                       -- 如 analysts@corp
  display_name  text,
  source        text not null default 'manual',      -- manual | idp:<provider>
  created_at    timestamptz not null default now(),
  updated_at    timestamptz not null default now(),
  deleted_at    timestamptz
);
create unique index groups_name_uidx on groups(name) where deleted_at is null;

create table group_members (
  group_id  bigint not null references groups(id),
  user_id   bigint not null references users(id),
  added_at  timestamptz not null default now(),
  primary key (group_id, user_id)
);

-- 身份提供商
create table identity_providers (
  id             bigint generated always as identity primary key,
  name           text not null unique,               -- 'oidc-corp'
  title          text,
  type           text not null,                      -- oidc | oauth2 | ldap
  domain         text,                               -- 邮箱域，用于登录路由
  config         jsonb not null default '{}',        -- 协议特定（issuer/client_id 或 host/bind_dn/...）
  field_mapping  jsonb not null default '{}',        -- {identifier, display_name, phone, groups}
  enabled        boolean not null default true,
  created_at     timestamptz not null default now(),
  updated_at     timestamptz not null default now()
);

-- 刷新令牌（不透明，存 SHA256）
create table refresh_tokens (
  token_hash  bytea primary key,                     -- sha256(token)
  user_id     bigint not null references users(id),
  issued_at   timestamptz not null default now(),
  expires_at  timestamptz not null,
  revoked_at  timestamptz,
  ip          inet,
  user_agent  text
);
create index on refresh_tokens(user_id);

-- 访问令牌（服务账号 / 个人 PAT，长期）
create table access_tokens (
  id           bigint generated always as identity primary key,
  token_hash   bytea not null unique,                -- sha256(secret)
  user_id      bigint not null references users(id),
  name         text,                                 -- 标签
  scopes       text[],
  last_used_at timestamptz,
  expires_at   timestamptz,
  revoked_at   timestamptz,
  created_at   timestamptz not null default now()
);
```

### 3.2 资源组织

```sql
-- 项目（产品团队的逻辑隔离边界；成员/权限在项目内自治）
create table projects (
  id           bigint generated always as identity primary key,
  key          text not null unique,                 -- 'orders'
  name         text not null,
  description  text,
  settings     jsonb not null default '{}',          -- 项目级开关
  created_at   timestamptz not null default now(),
  updated_at   timestamptz not null default now(),
  deleted_at   timestamptz
);
-- 注：项目成员关系 = role_assignments(scope_type='project')，不再单独建 project_members 表。

-- 环境（一等的策略维度；dev/test/stage/prod 等）
create table environments (
  id              bigint generated always as identity primary key,
  key             text not null unique,              -- 'prod' | 'stage' | 'test' | 'dev'
  name            text,
  protection_level int not null default 0,           -- 越大越受保护（prod 高、dev 低），驱动默认策略强度
  color           text,                              -- 前端展示色，如 '#d4351c'
  description     text,
  rank            int not null default 0,            -- 排序
  created_at      timestamptz not null default now()
);

-- 环境级运营策略（按环境差异化：护栏/脱敏/导出）
create table environment_policies (
  environment_id        bigint primary key references environments(id),
  query_row_limit       int,                         -- 单次查询默认行数上限
  query_cost_threshold  jsonb,                       -- {"soft":1e5,"hard":1e6} EXPLAIN 成本阈值
  export_max_rows       bigint,                      -- 单次导出行数上限
  export_require_approval boolean not null default false,  -- 导出是否需审批/JIT
  masking_mode          text not null default 'strict',     -- strict | relaxed | off
  settings              jsonb not null default '{}'  -- 扩展位
);

-- 实例（一个物理 DB 连接；【归属且仅归属一个项目】，由该项目管理）
create table instances (
  id                    bigint generated always as identity primary key,
  project_id            bigint not null references projects(id),  -- 实例所属项目（团队）
  name                  text not null unique,
  engine                text not null,               -- 'postgresql'（text，可扩展）
  engine_version        text,
  environment_id        bigint not null references environments(id),  -- 必填：每个实例必须标注环境
  activation            boolean not null default true,
  sync_interval_seconds int not null default 900,
  settings              jsonb not null default '{}', -- sync_databases、labels 等
  last_sync_at          timestamptz,
  created_at            timestamptz not null default now(),
  updated_at            timestamptz not null default now(),
  deleted_at            timestamptz
);
create unique index instances_name_uidx on instances(name) where deleted_at is null;
create index on instances(project_id) where deleted_at is null;
create index on instances(environment_id) where deleted_at is null;
-- 注：实例归属项目且必须标注环境。同一物理库服务器若被多团队使用，按团队分别注册为不同实例（各自项目下、各自凭据），
--     因此实例是「团队自治的资源」，不再需要平台级管理员介入。

-- 数据源（实例下的连接；每实例 1 个 admin + 至多 1 个 readonly）
create table data_sources (
  id           bigint generated always as identity primary key,
  instance_id  bigint not null references instances(id),
  role         text not null,                        -- admin | readonly
  name         text,
  host         text not null,
  port         int,
  database     text,                                 -- 默认库
  username     text not null,
  secret_ref   text,                                 -- 外部 Secret Manager 引用（非空时优先用外部密钥，见 D25）
  connection   jsonb not null default '{}',          -- 含 password_enc(AES-256-GCM 密文)、ssl、ssh 隧道、引擎特有项
  -- 凭据加密（D25）：DB 不存明文。secret_ref 非空 → 走外部 Secret Manager；
  --   否则 connection.password_enc 用 AES-256-GCM 加密，主密钥由环境变量/KMS 注入（不入库）。
  created_at   timestamptz not null default now(),
  updated_at   timestamptz not null default now()
);
create unique index one_admin_ds on data_sources(instance_id) where role = 'admin';
create unique index one_readonly_ds on data_sources(instance_id) where role = 'readonly';

-- 数据库（实例内的逻辑库；项目归属【由其所属实例决定】，不在本表冗余 project_id）
--   - database 的 project_id = 其 instance.project_id（单一事实来源，避免不一致）。
--   - 因此「把库归属到哪个项目」不再是独立操作：团队注册实例时已确定项目，
--     同步发现的新库自动归属该实例（即该项目），无需任何人手动分配/转移。
create table databases (
  id             bigint generated always as identity primary key,
  instance_id    bigint not null references instances(id),
  name           text not null,
  environment_id bigint references environments(id), -- 为空则继承实例环境
  metadata       jsonb not null default '{}',        -- sync_status / last_sync / labels
  created_at     timestamptz not null default now(),
  updated_at     timestamptz not null default now(),
  deleted_at     timestamptz
);
create unique index databases_uidx on databases(instance_id, name) where deleted_at is null;
-- 按项目列库走 join：select d.* from databases d join instances i on d.instance_id=i.id
--   where i.project_id = ? and d.deleted_at is null;
```

### 3.3 元数据目录

```sql
-- schema 快照（每个数据库一行，整体替换）
create table database_schemas (
  database_id   bigint primary key references databases(id),
  schema_json   jsonb not null,                      -- 发现的结构树（schemas/tables/columns/views/...）
  raw_ddl       text,                                -- SDL/DDL dump
  sync_status   text not null,                       -- ok | failed
  sync_error    text,
  synced_at     timestamptz not null default now()
);

-- 同步历史（每次成功的 schema 快照留一份，用于结构变更对比；按时间清理）
create table sync_history (
  id            bigint generated always as identity primary key,
  database_id   bigint not null references databases(id),
  schema_json   jsonb not null,
  raw_ddl       text,
  synced_at     timestamptz not null default now()
);
create index on sync_history(database_id, synced_at desc);

-- 列级标注（脱敏语义类型、分类、标签）—— 规范化、可查询
create table column_annotations (
  id               bigint generated always as identity primary key,
  database_id      bigint not null references databases(id),
  schema_name      text not null,
  table_name       text not null,
  column_name      text not null,
  semantic_type    text,                             -- 'phone' / 'email' / 'id_card'
  classification_id bigint references data_classifications(id),
  labels           jsonb not null default '{}',
  created_at       timestamptz not null default now(),
  updated_at       timestamptz not null default now()
);
create unique index column_annotations_uidx
  on column_annotations(database_id, schema_name, table_name, column_name);
```

> 标注按 `(schema, table, column)` 名称匹配读取；schema 变更后旧标注自然失效或由后台清理，不阻塞查询。

### 3.4 权限（RBAC + IAM）

```sql
-- 角色（预置 + 自定义）
create table roles (
  id           bigint generated always as identity primary key,
  key          text not null unique,                 -- 'workspaceAdmin' / 'sqlEditorReadUser' / 自定义
  name         text not null,
  scope        text not null,                        -- workspace | project
  builtin      boolean not null default false,       -- 预置角色不可改不可删
  description  text,
  created_at   timestamptz not null default now(),
  updated_at   timestamptz not null default now()
);

-- 角色 → 权限（扁平字符串集，权限注册表在代码里）
create table role_permissions (
  role_id     bigint not null references roles(id),
  permission  text not null,                         -- 'db.sql.select'
  primary key (role_id, permission)
);

-- IAM 绑定：把角色授予成员（在 workspace 或 project 范围），可选 CEL 条件
create table role_assignments (
  id           bigint generated always as identity primary key,
  scope_type   text not null,                        -- workspace | project
  scope_id     bigint not null,                      -- project id；workspace 用固定常量 0
  role_id      bigint not null references roles(id),
  member_kind  text not null,                        -- user | group | all
  member_id    bigint,                               -- users/groups.id；all 为 null（无 FK，多态）
  condition    text,                                 -- CEL，可选
  created_at   timestamptz not null default now(),
  updated_at   timestamptz not null default now()
);
create unique index role_assignments_uidx
  on role_assignments(scope_type, scope_id, role_id, member_kind, coalesce(member_id,0));
create index on role_assignments(member_kind, member_id);
```

> `member_id` 故意不加外键（多态指向 users/groups）。`scope_id` 同理（workspace/project）。一致性由应用层保证。

### 3.5 脱敏

```sql
-- 数据分类（敏感等级）
create table data_classifications (
  id    bigint generated always as identity primary key,
  name  text not null unique,                        -- 'Confidential'
  level int not null,                                -- 数值越大越敏感
  description text
);

-- 语义类型 → 脱敏算法
create table semantic_types (
  type        text primary key,                      -- 'phone'
  algorithm   jsonb not null,                        -- {kind:'range', prefix:3, suffix:4} | full | md5 | inner_outer
  description text
);

-- 脱敏规则：CEL 条件 → 语义类型
create table masking_rules (
  id            bigint generated always as identity primary key,
  name          text,
  condition     text not null,                       -- CEL over resource.*
  semantic_type text not null references semantic_types(type),
  priority      int not null default 0,
  enabled       boolean not null default true,
  created_at    timestamptz not null default now(),
  updated_at    timestamptz not null default now()
);

-- 脱敏豁免：谁、何时可看明文
create table masking_exemptions (
  id           bigint generated always as identity primary key,
  member_kind  text not null,
  member_id    bigint,
  condition    text,                                 -- CEL over resource.* + request.time
  expires_at   timestamptz,
  created_at   timestamptz not null default now(),
  updated_at   timestamptz not null default now()
);
```

### 3.6 JIT 临时访问

```sql
create table access_grants (
  id           bigint generated always as identity primary key,
  state        text not null,                        -- pending | active | revoked | expired
  user_id      bigint not null references users(id),
  database_id  bigint not null references databases(id),
  statement    text,                                 -- 可选：绑定到精确 SQL
  unmask       boolean not null default false,
  allow_export boolean not null default false,
  reason       text,
  expire_at    timestamptz,
  approved_by  bigint references users(id),
  approved_at  timestamptz,
  created_at   timestamptz not null default now(),
  updated_at   timestamptz not null default now()
);
create index on access_grants(user_id, state);
create index on access_grants(database_id) where state = 'active';
```

### 3.7 查询、Worksheet、收藏、分享

```sql
-- 查询历史（仅本人）
create table query_history (
  id           bigint generated always as identity primary key,
  user_id      bigint not null references users(id),
  database_id  bigint not null references databases(id),
  statement    text not null,
  kind         text not null default 'query',        -- query | export
  duration_ms  int,
  error        text,
  created_at   timestamptz not null default now()
);
create index on query_history(user_id, created_at desc);

-- 保存的查询
create table worksheets (
  id           bigint generated always as identity primary key,
  project_id   bigint references projects(id),
  database_id  bigint references databases(id),
  title        text not null,
  content      text not null,
  visibility   text not null default 'private',      -- private | project_read | project_write | link
  created_by   bigint not null references users(id),
  created_at   timestamptz not null default now(),
  updated_at   timestamptz not null default now(),
  deleted_at   timestamptz
);
create index on worksheets(project_id) where deleted_at is null;

-- 收藏（用户私有书签，指向 worksheet）
create table favorites (
  id            bigint generated always as identity primary key,
  user_id       bigint not null references users(id),
  worksheet_id  bigint not null references worksheets(id),
  folder        text,                                -- 单级标签
  note          text,
  pinned        boolean not null default false,
  last_opened_at timestamptz,
  created_at    timestamptz not null default now(),
  unique (user_id, worksheet_id)
);
create index on favorites(user_id);

-- 分享链接（随机 token，哈希存储）
create table share_links (
  id            uuid primary key default gen_random_uuid(),
  token_hash    bytea not null unique,               -- sha256(token)
  worksheet_id  bigint not null references worksheets(id),
  created_by    bigint references users(id),
  expires_at    timestamptz,
  revoked_at    timestamptz,
  created_at    timestamptz not null default now()
);
```

### 3.8 导出

```sql
create table export_tasks (
  id                      bigint generated always as identity primary key,
  state                   text not null,             -- created | running | succeeded | failed | expired
  user_id                 bigint not null references users(id),
  database_id             bigint not null references databases(id),
  statement               text not null,
  format                  text not null,             -- csv | json | sql | xlsx
  row_count               bigint,
  size_bytes              bigint,
  error                   text,
  applied_access_grant_id bigint references access_grants(id),
  created_at              timestamptz not null default now(),
  started_at              timestamptz,
  completed_at            timestamptz
);
create index on export_tasks(user_id, created_at desc);

-- 导出产物（ZIP 由用户密码加密；密码不存储）
create table export_archives (
  id          bigint generated always as identity primary key,
  task_id     bigint not null references export_tasks(id),
  storage     text not null default 'local',         -- local | s3 | ...
  location    text not null,                         -- 路径 / key
  format      text not null,
  size_bytes  bigint,
  expires_at  timestamptz not null,                  -- 默认 24h
  created_at  timestamptz not null default now()
);
create index on export_archives(expires_at);
```

### 3.9 审计日志（按月分区）

```sql
create table audit_logs (
  id             bigint generated always as identity,
  created_at     timestamptz not null default now(),
  method         text not null,                      -- RPC 名
  actor_id       bigint,                             -- users.id；匿名/登录失败为 null
  actor_email    text,                               -- 冗余留存，防重命名后失真
  scope_type     text,                               -- workspace | project
  scope_id       bigint,
  resource       text,                               -- 受影响资源名
  severity       text not null default 'info',
  status_code    int,
  status_message text,
  latency_ms     int,
  request        text,                               -- 已脱敏 JSON
  response       text,                               -- 已脱敏 JSON
  ip             inet,
  user_agent     text,
  service_data   jsonb,                              -- 如 policy_delta
  primary key (id, created_at)
) partition by range (created_at);
-- 按月建分区，例如：
-- create table audit_logs_2026_07 partition of audit_logs
--   for values from ('2026-07-01') to ('2026-08-01');
create index on audit_logs(created_at desc);
create index on audit_logs(method);
create index on audit_logs(actor_id);
create index on audit_logs(resource);
```

> 审计只 INSERT、不 UPDATE/DELETE（仅身份重命名等维护路径重写 `actor_*`）。按月分区便于归档与过期清理。SQL 字面量按决策原样留存于 `request`。

### 3.10 通知（站内通知中心，D21）

```sql
create table notifications (
  id          bigint generated always as identity primary key,
  user_id     bigint not null references users(id),       -- 接收人
  type        text not null,                              -- export_done | jit_pending | jit_resolved | system
  title       text not null,
  body        text,
  link        text,                                       -- 点击跳转的资源/页面
  ref_id      bigint,                                     -- 关联对象 id（如 export_task_id / access_grant_id）
  read_at     timestamptz,                                -- null = 未读
  created_at  timestamptz not null default now()
);
create index on notifications(user_id, read_at, created_at desc);
```

> 触发点：异步导出完成（→ 发起人 `export_done`）、JIT 申请提交（→ 审批人 `jit_pending`）、JIT 审批结果（→ 申请人 `jit_resolved`）。MVP 仅站内，不做邮件（D21）。

### 3.11 设置

```sql
create table settings (
  key        text primary key,
  value      jsonb not null,
  updated_at timestamptz not null default now()
);
```

---

## 4. 种子数据（初始化）

```sql
-- 角色
insert into roles(key,name,scope,builtin) values
  ('workspaceAdmin','Workspace Admin','workspace',true),
  ('securityAdmin','Security Admin','workspace',true),
  ('workspaceMember','Workspace Member','workspace',true),
  ('projectOwner','Project Owner','project',true),
  ('projectDBA','Project DBA','project',true),
  ('sqlEditorUser','SQL Editor User','project',true),
  ('sqlEditorReadUser','SQL Editor Read User','project',true);

-- 环境（protection_level 越大越受保护）
insert into environments(key,name,protection_level,color,rank) values
  ('prod', 'Production',  40, '#d4351c', 40),
  ('stage','Staging',     30, '#f47738', 30),
  ('test', 'Test',        20, '#1d70b8', 20),
  ('dev',  'Development', 10, '#00703c', 10);

-- 环境级策略（示例：prod 最严，dev 最宽松）
insert into environment_policies(environment_id, query_row_limit, query_cost_threshold, export_max_rows, export_require_approval, masking_mode) values
  ((select id from environments where key='prod'),  1000, '{"soft":100000,"hard":1000000}'::jsonb, 100000, true,  'strict'),
  ((select id from environments where key='stage'), 5000, '{"soft":500000,"hard":5000000}'::jsonb, 500000, false, 'strict'),
  ((select id from environments where key='test'),  10000,'{"soft":1000000,"hard":10000000}'::jsonb,1000000,false,'relaxed'),
  ((select id from environments where key='dev'),   10000,'{"soft":10000000,"hard":100000000}'::jsonb,1000000,false,'relaxed');

-- 分类
insert into data_classifications(name,level) values
  ('Public',10),('Internal',20),('Confidential',30),('Restricted',40);

-- 语义类型与算法
insert into semantic_types(type,algorithm) values
  ('phone',   '{"kind":"range","prefix":3,"suffix":4}'::jsonb),
  ('email',   '{"kind":"inner_outer","prefix":1,"suffix":0,"keep_domain":true}'::jsonb),
  ('id_card', '{"kind":"range","prefix":4,"suffix":2}'::jsonb),
  ('bank_card','{"kind":"range","prefix":4,"suffix":4}'::jsonb),
  ('full',    '{"kind":"full"}'::jsonb);

-- role_permissions 由代码注册表灌入（权限常量集中维护）
```

---

## 5. 可扩展性要点

| 扩展场景 | 是否改表 | 做法 |
|---|---|---|
| 新增数据库引擎 | ❌ 不改 | `engine` 为 text；加 driver/parser 插件代码即可 |
| 新增 IdP 协议 | ❌ 不改 | 加 idp 插件代码；`identity_providers.config` 容纳协议参数 |
| 新增脱敏算法 | ❌ 不改 | `semantic_types.algorithm` 的 `kind` 由代码识别 |
| 新增角色/权限 | ❌ 不改 | 插入 `roles` / `role_permissions` |
| 新增敏感列标注 | ❌ 不改 | 插入 `column_annotations` |
| 多租户（未来） | ⚠️ 加列 | 统一加 `tenant_id`（一次性迁移，现在不背） |
| 变更管理（未来） | ➕ 加表 | 新增 `change_issues`/`change_tasks` 等表，不动现有表 |
| 行级权限（未来） | ➕ 加表 | 新增 `row_filter_policies` 表，不改现有 |

---

## 6. 关键约束与索引策略

- **Project 逻辑隔离（核心）**：实例与库都归属项目（`instances.project_id`；库的项目由实例决定）。所有数据访问（查询/导出/JIT/审计 parent）在执行前解析目标库 → 其实例的 `project_id`，并校验调用者在**该项目**内的角色绑定（`role_assignments(scope_type='project', scope_id=<project>)`）。未在该项目获得授权 → 默认拒绝，跨 Project 不可见、不可访问。`instances(project_id)` 索引支撑按项目列实例/库。
- **环境标注（必须）**：每个实例必须标注 `environment_id`（NOT NULL）；数据库可显式标注，否则继承其实例的环境（`effective_environment`）。环境是 CEL 条件与 `environment_policies` 的策略维度：访问控制（`resource.environment_id=='prod'`）、脱敏强度、查询/导出护栏均按环境差异化。
- **每实例恰好 1 个 admin 数据源**：`one_admin_ds` 部分唯一索引。
- **软删除下的唯一性**：所有带历史名的唯一约束用 `where deleted_at is null` 部分索引。
- **审计查询性能**：`audit_logs` 按 `(created_at desc)` + `method` / `actor_id` / `resource` 索引；按月分区。
- **权限判定热路径**：`role_assignments(member_kind, member_id)` 索引支撑「取该用户/组的所有绑定」。
- **JIT 命中**：`access_grants(database_id) where state='active'` 部分索引。
- **导出清理**：`export_archives(expires_at)` 支撑过期扫描删除。

---

## 7. 与 PRD 各模块的对应

| PRD 模块 | 主要表 |
|---|---|
| [05 认证/IDP](./05-auth-idp.md) | users, groups, group_members, identity_providers, refresh_tokens, access_tokens |
| [07 资源管理](./07-resource-management.md) | projects, environments, environment_policies, instances, data_sources, databases, database_schemas, column_annotations |
| [02 权限与访问](./02-permission-and-access.md) | roles, role_permissions, role_assignments, access_grants, masking_rules, masking_exemptions, semantic_types, data_classifications, environment_policies |
| [03 SQL 查询](./03-sql-query.md) | query_history, worksheets |
| [04 数据导出](./04-data-export.md) | export_tasks, export_archives |
| [09 收藏与分享](./09-sql-favorite-share.md) | favorites, share_links |
| [06 审计日志](./06-audit-log.md) | audit_logs |
| 横切 | settings, notifications（站内通知，D21） |

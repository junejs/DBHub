-- 0001_v1_schema.sql — DBHUB v1 platform metadata schema.
-- Source of truth: docs/prd/10-data-model.md §3.1–§3.11.
-- All tables / indexes / partial-unique-indexes transcribed verbatim.
-- Applied once at startup; subsequent boots are a no-op (golang-migrate tracks state).

-- Required extensions (citext for case-insensitive emails; uuid-ossp not needed — internal ids are bigint identity).
CREATE EXTENSION IF NOT EXISTS citext;

-- =====================================================================
-- 3.1 身份与认证
-- =====================================================================

-- 用户（含服务账号，用 kind 区分；避免单独建表）
CREATE TABLE users (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  kind            TEXT NOT NULL DEFAULT 'user',      -- user | service
  email           CITEXT NOT NULL,
  name            TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'active',    -- active | disabled（删除即 disabled，见 D28；deleted_at 仅物理清理用）
  source          TEXT NOT NULL DEFAULT 'local',     -- local | oidc | ldap | oauth2 | scim
  password_hash   TEXT,                              -- bcrypt；SSO-only 用户为 null
  mfa_secret_enc  BYTEA,                             -- TOTP secret（应用层加密）
  mfa_enabled     BOOLEAN NOT NULL DEFAULT FALSE,
  last_login_at   TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at      TIMESTAMPTZ
);
CREATE UNIQUE INDEX users_email_uidx ON users (LOWER(email)) WHERE deleted_at IS NULL;

-- 组（可与 IdP 组同步）
CREATE TABLE groups (
  id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name          TEXT NOT NULL,                       -- 如 analysts@corp
  display_name  TEXT,
  source        TEXT NOT NULL DEFAULT 'manual',      -- manual | idp:<provider>
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at    TIMESTAMPTZ
);
CREATE UNIQUE INDEX groups_name_uidx ON groups (name) WHERE deleted_at IS NULL;

CREATE TABLE group_members (
  group_id  BIGINT NOT NULL REFERENCES groups(id),
  user_id   BIGINT NOT NULL REFERENCES users(id),
  added_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (group_id, user_id)
);

-- 身份提供商
CREATE TABLE identity_providers (
  id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name           TEXT NOT NULL UNIQUE,               -- 'oidc-corp'
  title          TEXT,
  type           TEXT NOT NULL,                      -- oidc | oauth2 | ldap
  domain         TEXT,                               -- 邮箱域，用于登录路由
  config         JSONB NOT NULL DEFAULT '{}',        -- 协议特定【非敏感】参数（issuer/client_id/host/base_dn/...）
  secret_ref     TEXT,                                -- 外部 Secret Manager 引用（非空时优先，见 D25）；与 data_sources 对齐
  secrets_enc    BYTEA,                               -- AES-256-GCM 密文（client_secret/bind_password）；secret_ref 非空时为 null
  field_mapping  JSONB NOT NULL DEFAULT '{}',        -- {identifier, display_name, phone, groups}
  enabled        BOOLEAN NOT NULL DEFAULT TRUE,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 刷新令牌（不透明，存 SHA256）
CREATE TABLE refresh_tokens (
  token_hash  BYTEA PRIMARY KEY,                     -- sha256(token)
  user_id     BIGINT NOT NULL REFERENCES users(id),
  issued_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at  TIMESTAMPTZ NOT NULL,
  revoked_at  TIMESTAMPTZ,
  ip          INET,
  user_agent  TEXT
);
CREATE INDEX refresh_tokens_user_id_idx ON refresh_tokens (user_id);

-- 登录锁定态（同步写、同步判锁定；**不依赖** fail-open 的审计日志，见 11 D30）
--   密码 10/10min、MFA 5/5min 失败锁定（见 05 §5、14-F）。
CREATE TABLE login_attempts (
  identifier     TEXT NOT NULL,             -- 归一化邮箱(lower)或 ip
  channel        TEXT NOT NULL,             -- password | mfa
  fail_count     INT NOT NULL DEFAULT 0,    -- 当前窗口失败次数
  last_fail_at   TIMESTAMPTZ,               -- 最近一次失败时间
  locked_until   TIMESTAMPTZ,               -- 非空=锁定中
  PRIMARY KEY (identifier, channel)
);

-- =====================================================================
-- 3.2 资源组织
-- =====================================================================

-- 项目（产品团队的逻辑隔离边界；成员/权限在项目内自治）
CREATE TABLE projects (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  key          TEXT NOT NULL UNIQUE,                 -- 'orders'
  name         TEXT NOT NULL,
  description  TEXT,
  settings     JSONB NOT NULL DEFAULT '{}',          -- 项目级开关
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at   TIMESTAMPTZ
);
-- 注：项目成员关系 = role_assignments(scope_type='project')，不再单独建 project_members 表。

-- 环境（一等的策略维度；dev/test/stage/prod 等）
CREATE TABLE environments (
  id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  key              TEXT NOT NULL UNIQUE,              -- 'prod' | 'stage' | 'test' | 'dev'
  name             TEXT,
  protection_level INT NOT NULL DEFAULT 0,           -- 越大越受保护（prod 高、dev 低），驱动默认策略强度
  color            TEXT,                              -- 前端展示色，如 '#d4351c'
  description      TEXT,
  rank             INT NOT NULL DEFAULT 0,            -- 排序
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 环境级运营策略（按环境差异化：查询/导出护栏）
CREATE TABLE environment_policies (
  environment_id   BIGINT PRIMARY KEY REFERENCES environments(id),
  query_row_limit  INT,                         -- 单次查询默认行数上限
  export_max_rows  BIGINT,                      -- 单次导出行数上限
  settings         JSONB NOT NULL DEFAULT '{}'  -- 扩展位（含结果字节上限、并发查询数等运行时策略）
);

-- 实例（一个物理 DB 连接；【归属且仅归属一个项目】，由该项目管理）
CREATE TABLE instances (
  id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id            BIGINT NOT NULL REFERENCES projects(id),  -- 实例所属项目（团队）
  key                   TEXT NOT NULL,                 -- 资源名段(slug)，如 'pg-prod'；映射 API 路径 {instance}
  name                  TEXT NOT NULL,                 -- 展示名；映射 API Instance.title
  engine                TEXT NOT NULL,               -- 'postgresql'（text，可扩展）
  engine_version        TEXT,
  environment_id        BIGINT NOT NULL REFERENCES environments(id),  -- 必填：每个实例必须标注环境
  activation            BOOLEAN NOT NULL DEFAULT TRUE,
  sync_interval_seconds INT NOT NULL DEFAULT 900,
  settings              JSONB NOT NULL DEFAULT '{}', -- sync_databases、labels 等
  last_sync_at          TIMESTAMPTZ,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at            TIMESTAMPTZ
);
CREATE UNIQUE INDEX instances_key_uidx ON instances (project_id, key) WHERE deleted_at IS NULL;
CREATE INDEX instances_project_id_idx ON instances (project_id) WHERE deleted_at IS NULL;
CREATE INDEX instances_environment_id_idx ON instances (environment_id) WHERE deleted_at IS NULL;

-- 数据源（实例下的连接；每实例 1 个 admin + 至多 1 个 readonly）
CREATE TABLE data_sources (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  instance_id  BIGINT NOT NULL REFERENCES instances(id),
  role         TEXT NOT NULL,                        -- admin | readonly（资源名段 {dataSource} 即此角色）
  host         TEXT NOT NULL,
  port         INT,
  database     TEXT,                                 -- 默认库
  username     TEXT NOT NULL,
  secret_ref   TEXT,                                 -- 外部 Secret Manager 引用（非空时优先用外部密钥，见 D25）
  connection   JSONB NOT NULL DEFAULT '{}',          -- 含 password_enc(AES-256-GCM 密文)、ssl、ssh 隧道、引擎特有项
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX one_admin_ds ON data_sources (instance_id) WHERE role = 'admin';
CREATE UNIQUE INDEX one_readonly_ds ON data_sources (instance_id) WHERE role = 'readonly';

-- 数据库（实例内的逻辑库；项目归属【由其所属实例决定】，不在本表冗余 project_id）
CREATE TABLE databases (
  id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  instance_id    BIGINT NOT NULL REFERENCES instances(id),
  name           TEXT NOT NULL,
  environment_id BIGINT REFERENCES environments(id), -- 为空则继承实例环境
  metadata       JSONB NOT NULL DEFAULT '{}',        -- sync_status / last_sync / labels
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at     TIMESTAMPTZ
);
CREATE UNIQUE INDEX databases_uidx ON databases (instance_id, name) WHERE deleted_at IS NULL;

-- =====================================================================
-- 3.3 元数据目录
-- =====================================================================

-- schema 快照（每个数据库一行，整体替换）
CREATE TABLE database_schemas (
  database_id   BIGINT PRIMARY KEY REFERENCES databases(id),
  schema_json   JSONB NOT NULL,                      -- 发现的结构树（schemas/tables/columns/views/...）
  raw_ddl       TEXT,                                -- SDL/DDL dump
  sync_status   TEXT NOT NULL,                       -- ok | failed
  sync_error    TEXT,
  synced_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 同步历史（每次成功的 schema 快照留一份，用于结构变更对比；按时间清理）
CREATE TABLE sync_history (
  id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  database_id   BIGINT NOT NULL REFERENCES databases(id),
  schema_json   JSONB NOT NULL,
  raw_ddl       TEXT,
  synced_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX sync_history_database_id_synced_at_idx ON sync_history (database_id, synced_at DESC);

-- =====================================================================
-- 3.4 权限（RBAC + IAM）
-- =====================================================================

-- 角色（预置 + 自定义）
CREATE TABLE roles (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  key          TEXT NOT NULL UNIQUE,                 -- 'workspaceAdmin' / 'sqlEditorReadUser' / 自定义
  name         TEXT NOT NULL,
  scope        TEXT NOT NULL,                        -- workspace | project
  builtin      BOOLEAN NOT NULL DEFAULT FALSE,       -- 预置角色不可改不可删
  description  TEXT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 角色 → 权限（扁平字符串集，权限注册表在代码里）
CREATE TABLE role_permissions (
  role_id     BIGINT NOT NULL REFERENCES roles(id),
  permission  TEXT NOT NULL,                         -- 'db.sql.select'
  PRIMARY KEY (role_id, permission)
);

-- IAM 绑定：把角色授予成员（在 workspace 或 project 范围），可选结构化条件
CREATE TABLE role_assignments (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  scope_type   TEXT NOT NULL,                        -- workspace | project
  scope_id     BIGINT NOT NULL,                      -- project id；workspace 用固定常量 0
  role_id      BIGINT NOT NULL REFERENCES roles(id),
  member_kind  TEXT NOT NULL,                        -- user | group | all
  member_id    BIGINT,                               -- users/groups.id；all 为 null（无 FK，多态）
  condition    JSONB,                                -- 结构化条件 {environments,databases,schemas,tables}，可选
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX role_assignments_uidx
  ON role_assignments (scope_type, scope_id, role_id, member_kind, COALESCE(member_id, 0));
CREATE INDEX role_assignments_member_idx ON role_assignments (member_kind, member_id);

-- =====================================================================
-- 3.7 查询、Worksheet、收藏
-- =====================================================================

-- 查询历史（仅本人）
CREATE TABLE query_history (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id      BIGINT NOT NULL REFERENCES users(id),
  database_id  BIGINT NOT NULL REFERENCES databases(id),
  statement    TEXT NOT NULL,
  kind         TEXT NOT NULL DEFAULT 'query',        -- query | export
  duration_ms  INT,
  error        TEXT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX query_history_user_id_created_at_idx ON query_history (user_id, created_at DESC);

-- 保存的查询
CREATE TABLE worksheets (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id   BIGINT REFERENCES projects(id),
  database_id  BIGINT REFERENCES databases(id),
  title        TEXT NOT NULL,
  content      TEXT NOT NULL,
  visibility   TEXT NOT NULL DEFAULT 'private',      -- private | project
  created_by   BIGINT NOT NULL REFERENCES users(id),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  etag         TEXT NOT NULL DEFAULT '',             -- 乐观锁；每次 Update 刷新（见 12 §3.5、14-B）
  deleted_at   TIMESTAMPTZ
);
CREATE INDEX worksheets_project_id_idx ON worksheets (project_id) WHERE deleted_at IS NULL;

-- 收藏（用户私有书签，指向 worksheet）
CREATE TABLE favorites (
  id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id       BIGINT NOT NULL REFERENCES users(id),
  worksheet_id  BIGINT NOT NULL REFERENCES worksheets(id),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (user_id, worksheet_id)
);
CREATE INDEX favorites_user_id_idx ON favorites (user_id);

-- =====================================================================
-- 3.8 导出
-- =====================================================================

CREATE TABLE export_tasks (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  state        TEXT NOT NULL,             -- created | running | succeeded | failed | expired
  user_id      BIGINT NOT NULL REFERENCES users(id),
  project_id   BIGINT NOT NULL REFERENCES projects(id),  -- 冗余：支撑项目级列表，避免删库后 join 断链
  database_id  BIGINT NOT NULL REFERENCES databases(id),
  statement    TEXT NOT NULL,
  format       TEXT NOT NULL,             -- csv | json（v1）
  row_count    BIGINT,
  size_bytes   BIGINT,
  error        TEXT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  started_at   TIMESTAMPTZ,
  completed_at TIMESTAMPTZ
);
CREATE INDEX export_tasks_user_id_created_at_idx ON export_tasks (user_id, created_at DESC);
CREATE INDEX export_tasks_project_id_created_at_idx ON export_tasks (project_id, created_at DESC);

-- 导出产物（ZIP 由用户密码加密；密码不存储）
CREATE TABLE export_archives (
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  task_id     BIGINT NOT NULL REFERENCES export_tasks(id),
  storage     TEXT NOT NULL DEFAULT 'local',         -- local | s3 | ...
  location    TEXT NOT NULL,                         -- 路径 / key
  format      TEXT NOT NULL,
  size_bytes  BIGINT,
  expires_at  TIMESTAMPTZ NOT NULL,                  -- 默认 24h
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX export_archives_expires_at_idx ON export_archives (expires_at);

-- =====================================================================
-- 3.10 通知（站内通知中心，D21）
-- =====================================================================

CREATE TABLE notifications (
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id     BIGINT NOT NULL REFERENCES users(id),       -- 接收人
  type        TEXT NOT NULL,                              -- export_done | system
  title       TEXT NOT NULL,
  body        TEXT,
  link        TEXT,                                       -- 点击跳转的资源/页面
  ref_id      BIGINT,                                     -- 关联对象 id（如 export_task_id）
  read_at     TIMESTAMPTZ,                                -- null = 未读
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX notifications_user_id_read_at_created_at_idx ON notifications (user_id, read_at, created_at DESC);

-- =====================================================================
-- 3.11 设置
-- =====================================================================

CREATE TABLE settings (
  key        TEXT PRIMARY KEY,
  value      JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
# 07 — 资源管理

本文档定义数据库资源（Instance / Database / Schema / Table / View / Column）的模型、组织方式，以及元数据的发现与同步机制。

> 参考 Bytebase 的资源层级与「后台同步 + 缓存」元数据模型。

---

## 1. 资源层级

```
Workspace（工作区，单组织自部署下为单一固定实例，非多租户）
  ├── Environment（环境，软标签：prod/test/dev，支持继承）—— 用于策略选择
  └── Project（项目，硬归属容器）—— 权限授予的基本单元
        └── Instance（实例 = 一个物理 DB 连接：host:port + 引擎 + DataSources）
              └── Database（逻辑库；归属于一个 Project）
                    └── Schema（PG 概念；MySQL 无此层）
                          ├── Table
                          │     └── Column (+ Index, ForeignKey, Check, Trigger, Partition...)
                          ├── View / MaterializedView
                          ├── Function / Procedure
                          └── Sequence / ...
```

关键点：
- **Instance 与 Database 是持久化的一等资源**（关系表行）。
- **Schema / Table / Column 不是独立行**，而是作为嵌套元数据结构（`DatabaseSchemaMetadata`）整体存储 + 缓存。
- **Project = 硬归属**（Database 必属一个 Project，外键）；**Environment = 软标签**（可挂在 Instance 或 Database 上，Database 未设则继承 Instance 的）。

---

## 2. Instance（实例）模型

```
Instance {
  title            // 显示名
  engine           // 引擎：POSTGRES/MYSQL/ORACLE/REDSHIFT/CLICKHOUSE/...
  engine_version   // 发现的版本（输出）
  activation       // 是否启用
  environment      // 环境标签
  data_sources[]   // 连接配置：1 个 ADMIN + 0/1 个 READ_ONLY
  sync_interval    // 自动同步周期
  sync_databases[] // 限定同步的库；空=全部
  last_sync_time
  labels           // 自由标签
  external_link    // 外部控制台链接
}
```

**DataSource（数据源）**：
- `type`：ADMIN（管理，用于同步/变更）/ READ_ONLY（只读，用于查询）。
- `username/password`（或外部 Secret Manager 引用）、`host/port/database`。
- SSL/TLS、SSH 隧道、IAM 认证（RDS IAM 等）、引擎特有项（Oracle SID/service_name、MongoDB replica_set 等）。
- 校验：**必须恰好 1 个 ADMIN**，至多 1 个 READ_ONLY。

---

## 3. Database（数据库）模型

```
Database {
  name              // instances/{i}/databases/{d}
  project           // 归属项目
  environment       // 显式环境标签（可空）
  effective_environment // 继承后的有效环境（输出）
  instance_resource // 所属实例快照（输出）
  sync_status       // OK / FAILED
  sync_error
  labels
  // 元数据见 db_schema
}
```

- 复合主键 `(instance, database)`；外键到 Instance 与 Project。
- 新发现的库自动归属到创建实例时指定的 `initial_database_project`（默认项目兜底）。

---

## 4. Schema / Table / Column 元数据

以嵌套结构存储于 `db_schema`：

```
db_schema {
  instance, db_name,
  metadata   // DatabaseSchemaMetadata（发现的结构树）
  raw_dump   // SDL/DDL 文本
  config     // DatabaseConfig（平台侧标注：分类/语义类型/标签）
}

DatabaseSchemaMetadata
  └─ schemas[]
        ├─ tables[]   → columns[], indexes[], foreign_keys[], ...
        ├─ views[] / materialized_views[]
        ├─ functions[] / procedures[]
        └─ sequences[] / ...

TableMetadata  → name, columns[], indexes[], foreign_keys[], row_count, data_size, comment, partitions, triggers, ...
ColumnMetadata → name, position, type, nullable, default, comment, is_identity, generation(virtual/stored), ...
```

- **config（Catalog 标注）** 平行结构：`DatabaseCatalog → SchemaCatalog → TableCatalog → ColumnCatalog { semantic_type, labels, classification }`。
- 两套结构在内存合并为统一可查询视图（`DatabaseMetadata`），供查询/补全/脱敏使用。
- 大小写敏感性按引擎处理（MySQL 默认大小写不敏感等）。

---

## 5. 元数据发现与同步

**模型：后台定时同步 + 缓存 + 手动刷新**（非实时反射，避免每次查询都打业务库）。

### 同步器（Schema Syncer）
- 两个循环：
  - 实例循环（如 15min）：遍历实例，发现库列表/版本/角色。
  - 数据库循环（如 10s）：处理待同步库队列。
- 单库同步流程（`doSyncDatabaseSchema`）：
  1. 用 ADMIN 数据源打开驱动。
  2. `driver.SyncDBSchema()` 反射结构 → `DatabaseSchemaMetadata`。
  3. `driver.Dump()` 生成 SDL/DDL。
  4. 写 `db_schema`（metadata/raw_dump/config）+ 更新库 `last_sync_time`/`sync_status`。
  5. 可选写快照（`sync_history`）用于结构变更对比。
- 失败：`sync_status=FAILED` + `sync_error`。

### 缓存
- 进程内 LRU（key=instance+database），读多写少，命中率极高。
- 写入/更新时失效缓存。

### 手动同步
- `SyncInstance` / `SyncDatabase` RPC：DBA 手动触发（含 `validate_only` 连通性测试）。
- `enable_full_sync`：是否对每个库做完整结构同步。

---

## 6. 管理操作需求

| 操作 | 说明 | 角色 |
|---|---|---|
| 注册实例 | 填写引擎、连接、ADMIN/RO 数据源、环境、初始项目 | workspaceDBA |
| 测试连接 | `validate_only` 不落库，仅验证连通 | workspaceDBA |
| 更新/删除实例 | 含数据源增删改（RO 可独立增删；ADMIN 随实例） | workspaceDBA |
| 数据源密码轮换 | 支持；旧密码失效 | workspaceDBA |
| 手动同步 | 触发实例/库同步 | workspaceDBA / projectOwner |
| 库归属调整 | 将库移到其他 Project（影响权限） | workspaceAdmin |
| 环境标签 | 挂在实例或库上 | workspaceDBA |
| Catalog 标注 | 为列设置 semantic_type/classification/labels | securityAdmin / DBA |
| 查看结构 | 浏览 schema/table/column 定义、DDL | 授权用户 |

---

## 7. 可见性与展示

- **资源树**：按 Project → Environment → Instance → Database → Schema → Table/View → Column 层级展示。
- 仅展示用户有访问权的库（IAM 过滤）。
- 表详情：列定义、索引、外键、行数/大小、DDL、数据预览（受权限/脱敏）。
- 支持结构搜索（按表名/列名）。

---

## 8. 引擎能力矩阵

集中声明每个引擎支持的能力，前端/后端据此降级：
- 是否支持查询新 ACL（QuerySpan 细粒度授权）
- 是否支持脱敏
- 是否支持自动补全

---

## 9. 功能范围

- 实例注册/测试连接/更新/删除/数据源管理（**当前版本支持 PostgreSQL 引擎**；架构支持扩展其他引擎）。
- 库自动发现 + 归属初始项目。
- 后台元数据同步 + 缓存 + 手动刷新。
- 资源树浏览 + 结构详情 + DDL。
- Catalog 标注（列级 semantic_type/classification/labels；**手工标注 + 按列名/类型正则规则批量**，不含自动扫描发现）。
- 环境与项目管理。

**可选增强（不在本期必须范围）：** 结构变更对比（diff）、结构变更历史、库结构搜索增强、跨实例资源视图、更多引擎（MySQL/Oracle/Redshift/ClickHouse…）。

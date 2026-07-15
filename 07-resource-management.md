# 07 — 资源管理

本文档定义数据库资源（Instance / Database / Schema / Table / View / Column）的模型、组织方式，以及元数据的发现与同步机制。

> 参考 Bytebase 的资源层级与「后台同步 + 缓存」元数据模型。

---

## 1. 资源层级

```
Workspace（工作区，单组织自部署下为单一固定实例）
  │
  ├── Environment（一等策略维度：dev/test/stage/prod；实例必标，库可继承/覆盖）—— 驱动访问控制/脱敏/护栏
  │
  └── Project（项目 = 产品团队的逻辑隔离边界）── 成员/权限自治
        │
        └── Instance（实例 = 一个物理 DB 连接：host:port + 引擎 + DataSources）── 归属该项目，由项目团队管理
              └── Database（逻辑库；项目归属由其 Instance 决定）
                    └── Schema（PG 概念；MySQL 无此层）
                          ├── Table → Column (+ Index, ForeignKey, Check, Trigger, Partition...)
                          ├── View / MaterializedView
                          ├── Function / Procedure
                          └── Sequence / ...
```

关键点：
- **Instance 归属 Project**（产品团队）：实例与库都在项目内，由项目角色（`projectOwner` / `projectDBA`）管理，**无需平台级管理员**。同一物理库服务器若被多团队使用，按团队分别注册为各自项目下的实例。
- **Database 的项目归属由其实例决定**——因此「把库归属到哪个项目」不再是独立操作：团队注册实例时已确定项目，同步发现的新库自动归属该实例（即该项目）。
- **Project 是逻辑隔离边界**：跨 Project 默认隔离（见 [02 §2.4](./02-permission-and-access.md)），Project A 的成员看不到也访问不了 Project B 的实例与库。
- **Instance 与 Database 是持久化的一等资源**（关系表行）。
- **Schema / Table / Column 不是独立行**，而是作为嵌套元数据结构（`DatabaseSchemaMetadata`）整体存储 + 缓存。
- **Environment = 一等策略维度（重要）**：每个 Instance **必须标注** environment（dev/test/stage/prod 等）；Database 可显式覆盖，否则继承其实例的环境（`effective_environment`）。环境不只是展示标签，而是**驱动访问控制（CEL `resource.environment_id`）、脱敏强度、查询/导出护栏**的策略维度，由 `environment_policies` 表按环境差异化配置（见 [02 §3.2](./02-permission-and-access.md)、[10 environment_policies](./10-data-model.md)）。环境的增删与策略配置由 `securityAdmin`/`workspaceAdmin` 管理（平台级），实例上的标注由项目角色填写。

---

## 2. Instance（实例）模型

```
Instance {
  project          // 归属项目（必填，团队自治范围）
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
  project           // 归属项目（= 其 instance.project_id，输出，只读）
  environment       // 显式环境标签（可空）
  effective_environment // 继承后的有效环境（输出）
  instance_resource // 所属实例快照（输出）
  sync_status       // OK / FAILED
  sync_error
  labels
  // 元数据见 db_schema
}
```

- 复合主键 `(instance, database)`；外键到 Instance（项目归属经 Instance 决定，不在 Database 上冗余）。
- **库的项目归属由其实例决定**：同步在实例上发现的新库，自动归属该实例所在的项目——无需 `initial_database_project`、无需默认项目兜底、无需手动分配。

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
| 注册实例 | 在项目内填写引擎、连接、ADMIN/RO 数据源、环境 | projectOwner / projectDBA |
| 测试连接 | `validate_only` 不落库，仅验证连通 | projectOwner / projectDBA |
| 更新/删除实例 | 含数据源增删改（RO 可独立增删；ADMIN 随实例） | projectOwner / projectDBA |
| 数据源密码轮换 | 支持；旧密码失效 | projectOwner / projectDBA |
| 手动同步 | 触发实例/库同步 | projectOwner / projectDBA |
| 库发现→归属 | 新发现的库**自动归属该实例所在项目**，无需手动分配 | （自动） |
| 实例迁移项目 | 把实例连同其库迁移到另一项目（影响权限，需审计） | workspaceAdmin |
| 环境标签 | 挂在实例或库上 | projectOwner / projectDBA |
| Catalog 标注 | 为列设置 semantic_type/classification/labels | projectDBA（语义类型）；分类由 securityAdmin 定 |
| 查看结构 | 浏览 schema/table/column 定义、DDL | 授权用户 |

---

## 7. 可见性与展示

- **资源树**：按 **Project**（团队隔离边界）→ Instance → Database → Schema → Table/View → Column 层级展示；用户只能看到自己所属 Project 下的实例与库（跨 Project 默认不可见，见 [02 §2.4](./02-permission-and-access.md)）。
- **环境色标**：实例与库旁边显示环境色标（`environments.color`，如 prod 红、dev 绿），便于一眼区分；查询工作台顶部也标注当前库的 effective environment。
- 仅展示用户有访问权的实例/库（Project 成员关系 + IAM 过滤）。
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

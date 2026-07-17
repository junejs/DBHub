# 02 — 权限与访问控制

权限体系是本系统的核心。目标：**从库级到表级，实现完整、可组合、可审计的访问控制**。

> 设计参考 Bytebase 的 IAM 式模型（`角色 × 成员 × 条件`）。数据级条件采用**结构化字段**（环境/库/表），不暴露表达式语言。**行级权限（Row-Level Security）不做**：其 SQL 改写注入的正确性风险高、安全代价大；未来若出现明确业务场景，优先以数据库原生 RLS 形式引入。**列级动态脱敏与 JIT 延后到 v2**（见 §4、§6）。

---

## 1. 模型总览

权限控制分为两层，逐层收紧：

```
第 1 层：粗粒度 RBAC（功能权限）
   └─ 角色(Role) = 权限(Permission) 集合，如 db.sql.select / db.instances.create
   └─ 通过 IAM 绑定把角色授予 成员(Member)

第 2 层：数据级授权（IAM 绑定 + 结构化条件）
   └─ 把 db.sql.select 等数据权限，用结构化条件（环境/库/表）限定范围
   └─ 这是「库级 / 表级 / 环境级」授权的主要手段
```

> **v1 不做列级动态脱敏与谓词列保护**（延后 v2）。敏感列的可见性由数据库授权（GRANT）决定；平台对全部访问做审计。

---

## 2. 角色（RBAC）

### 2.1 预置角色

权限分 **Workspace（工作区）级** 与 **Project（项目）级** 两层。成员通过 IAM 绑定获得角色。

| 层级 | 角色 | 定位 | 关键权限 |
|---|---|---|---|
| Workspace | `workspaceAdmin` | 平台管理员 | 平台级：项目管理（建/删 project）、环境、IdP、全局设置、成员；可见全部项目 |
| Workspace | `securityAdmin` | 安全管理员（**职责分离**） | 审计查看、环境策略、访问护栏、监督授权合规；**不含**连接凭据 |
| Workspace | `workspaceMember` | 普通成员 | 浏览、查询自身历史 |
| Project | `projectOwner` | 项目所有者 | 项目治理：项目 IAM 管理 + 项目内一切操作；含实例/库管理；**可读本项目审计**（D26） |
| Project | `projectDBA` | 项目 DBA | **项目内实例/数据源/库管理、元数据同步**；不含项目 IAM 管理 |
| Project | `sqlEditorUser` | SQL 编辑者（读写） | `db.sql.select/ddl/dml`（**v1 占位、暂不授予**：DDL/DML 属变更管理，不在 v1 范围；v1 实际只读路径用 `sqlEditorReadUser`，此角色随变更管理在 v2 启用） |
| Project | `sqlEditorReadUser` | SQL 只读者 | `db.sql.select/explain/info` |
| Project | `projectViewer` | 项目只读者 | 浏览库表 schema，不可查询数据 |

> 实例/库的日常管理下放到 **Project 级**（由 `projectOwner` / `projectDBA` 在项目内自治），不再需要平台级 DBA。`workspaceDBA` 角色因此**不再保留**——平台只保留 `workspaceAdmin`（治理）与 `securityAdmin`（安全/合规）。

### 2.2 权限命名与分类

权限为扁平字符串集，命名规范 `db.<resource>.<verb>`：

| 类别 | 示例权限 |
|---|---|
| SQL 执行 | `db.sql.select / dml / ddl / explain / info` |
| 导出 | `db.exports.create`（异步导出任务）；同步导出走 `db.sql.select` |
| 数据库/Schema | `db.databases.get / getSchema / list / sync / update` |
| 实例管理（项目内） | `db.instances.create / update / delete / sync`（项目级，由 projectOwner / projectDBA 执行） |
| 权限管理 | `db.projects.getIamPolicy / setIamPolicy` |
| 审计 | `db.auditLogs.search / export` |
| 身份管理 | `db.users.* / db.groups.* / db.identityProviders.*` |

### 2.3 Project：产品团队的逻辑隔离边界（核心概念）

> 公司按**产品团队**划分，每个团队在自己的 **Project** 内管理自己的数据库、成员与权限。**Project 是平台的一等逻辑隔离单元。**

- **资源归属**：**实例（Instance）与数据库（Database）都归属且仅归属一个 Project**（`instances.project_id`；库的项目由其实例决定）。公司数据库实例不跨团队共享——同一物理服务器若被多团队使用，分别按团队注册为各自项目下的实例。
- **团队自治**：实例/库/数据源/同步由项目内角色（`projectOwner` / `projectDBA`）管理，**无需平台级管理员介入**。
- **成员制**：用户通过在 Project 上获得角色而成为该 Project 的成员（即「项目角色绑定 = 成员关系」）。一个用户可属于多个 Project（跨团队人员）。
- **默认跨 Project 隔离（Default Deny）**：用户对 Project A 的实例/库没有任何访问权，除非他被授予了 Project A 的角色（或 `workspaceAdmin`）。查询/导出在执行前会解析目标库所属 Project，并强制校验调用者在**该 Project** 内的访问权。
- **可见性隔离**：用户能看到的 Project 列表 = 他持有角色的 Project（+ `workspaceAdmin`/`securityAdmin` 可见全部）。Project A 的成员在资源树/工作台里看不到 Project B 的实例与库。
- **两层角色作用域**：
  - **Project 角色**（`projectOwner` / `projectDBA` / `sqlEditorUser` / `sqlEditorReadUser` / 自定义）：仅在所属 Project 内生效，是该 Project 实例/数据访问权的来源。
  - **Workspace 角色**（`workspaceAdmin` / `securityAdmin`）：跨 Project 的平台级权限（建项目、IdP、审计、环境策略）。

---

## 3. IAM 绑定（数据级授权）

### 3.1 绑定结构

```
IamPolicy {
  bindings: [
    {
      role: "roles/sqlEditorReadUser",
      members: ["user:alice@corp.com", "group:analysts@corp.com"],
      condition: {              // 可选，结构化条件（不暴露表达式语言）
        environments: ["prod"],      // 限定环境；空=全部
        databases:   ["orders_db"],  // 限定库；空=全部
        tables:       ["customers", "orders"]  // 限定表；空=全部
      }
    }
  ]
}
```

- **members** 类型：`user:{email}`、`group:{email}`、`allUsers`。
- **两层策略**：每个 Workspace 一份工作区策略；**每个 Project 一份项目策略**——这是各团队自治设置自己成员与权限的载体。
- **判定规则**：
  - `workspaceAdmin` / `securityAdmin` 拥有跨 Project 的平台级权限。
  - Project 资源（实例/库）的访问与管理权**要求调用者在该 Project 内被放行**——这是 §2.3 跨 Project 隔离的执行点。
  - 结构化条件在 Project 内进一步把权限收敛到具体环境/库/表。

### 3.2 环境作为访问控制维度（重要）

环境（dev/test/stage/prod）不仅是标签，更是**访问授权的关键维度**。典型治理：开发/测试库对较多成员开放，**生产库默认收紧**——通过结构化条件把对 prod 的访问单独授予少数人。

```
// 普通分析师：授予 dev/test/stage（不在 prod 上授予）
role=sqlEditorReadUser, members=[group:analysts],
condition={ environments: ["dev","test","stage"] }

// DBA：仅在 prod 上授予只读
role=sqlEditorReadUser, members=[group:prod-dbas],
condition={ environments: ["prod"] }
```

环境还是**查询/导出护栏**的差异化维度（行数上限、导出审批等，见 [03](./03-sql-query.md)、[04](./04-data-export.md)），由 `environment_policies` 表集中配置（prod 最严、dev 最宽松）。

### 3.3 结构化条件维度

| 维度 | 含义 | 典型用途 |
|---|---|---|
| `environments` | 环境（dev/test/stage/prod） | **按环境隔离访问**（如仅 dev/test/stage） |
| `databases` | 数据库名 | 限定到某库 |
| `schemas` | schema | 限定到某 schema |
| `tables` | 表名 | 限定到某表 |

> 条件以结构化字段表达（多选/包含语义），由 UI 提供下拉选择，不暴露表达式语言，避免误配=越权。实例维度由「绑定所属 Project」天然限定，不再单列。

### 3.4 库级 / 表级 / 环境级授权示例

```
// Alice 可读 prod 环境 orders_db 全部表
role=sqlEditorReadUser, members=[user:alice],
condition={ environments:["prod"], databases:["orders_db"] }

// Analysts 组只能读 customers、orders 两张表（任意非 prod 环境）
role=sqlEditorReadUser, members=[group:analysts],
condition={ environments:["dev","test","stage"], databases:["orders_db"], tables:["customers","orders"] }
```

---

## 4. 列级动态脱敏 —— 不在 v1 范围

> **v1 不实现列级动态脱敏、谓词列保护、脱敏豁免**（路线图见 [18 §1.1](./18-roadmap.md)）。

**v1 的敏感数据保护方式：**
- 敏感列的可见性由**数据库授权（GRANT）**决定：DBA 为平台的只读查询角色只授予非敏感列（或整表），敏感列干脆不可见。
- 平台对**全部查询/导出做全量审计**（见 [06](./06-audit-log.md)），任何访问可回溯。
- 临时放行敏感数据通过 DBA 调整授权实现（v1 不做平台侧 JIT）。

---

## 5. 行级权限（Row-Level Security）—— 不在本期范围

> 经评估，**行级权限本期不实现**，未来视业务需要再引入。

**结论与理由：**
- 行级权限（同一张表、不同用户看到不同行子集）只有当数据呈多租户/地区隔离/"我的数据"形态时才真正需要；若数据主要按角色共享整表，**表级授权 + 数据库列授权**已覆盖绝大多数查询/导出场景。
- 平台侧实现行级过滤的主流做法是「SQL 改写注入」（把 `AND predicate` 注入到每张被约束表），但其**正确性风险高**：子查询、CTE、各类 JOIN、表别名、UNION 等任一未覆盖，轻则查询报错、重则**越权泄露数据**；且 bug 即安全事故（非普通功能缺陷）。
- 该能力还需**每引擎单独实现解析改写**，维护成本随引擎数线性增长。

**未来引入路径（届时优先）：**
- **数据库原生 RLS**：PostgreSQL Row Security Policy、Oracle VPD、Snowflake Row Access Policy——策略下推到数据库侧强制，绕不过去，最可靠；平台仅做策略管理与下发。
- 仅当引擎不支持原生 RLS 且确有强需求时，才评估 SQL 改写注入方案，并配套失败关闭 + 大规模测试。

---

## 6. JIT 临时访问 —— 不在 v1 范围

> **v1 不实现平台侧 JIT 临时访问**（路线图见 [18 §1.2](./18-roadmap.md)）。临时放行由 DBA 在数据库侧调整授权（GRANT）+ 全量审计实现，不经过平台审批流。

---

## 7. 权限执行点（汇总）

| 操作 | 检查内容 |
|---|---|
| 任意 RPC | ACL 拦截器：粗粒度 `db.<resource>.<verb>` + 项目隔离 |
| SQL SELECT | 库/表结构化条件（环境/库/表）+ 只读强制 |
| SQL DML/DDL（变更管理，不在本期范围） | 解析写目标 → 逐目标库/表/环境校验 |
| 导出 | 同查询 + 导出权限 |
| 资源管理 | 实例/库等各自 `db.*` 权限 |
| 权限变更 | `setIamPolicy` 记变更审计 |

---

## 8. 与 Bytebase 的差异小结

| 维度 | Bytebase | 本系统（v1） |
|---|---|---|
| RBAC + IAM 绑定 | ✅ | ✅ 沿用 |
| 数据级条件 | CEL 表达式 | **结构化条件**（环境/库/表，下拉选择，不暴露表达式语言） |
| 列级（动态脱敏） | ✅ | ❌ **延后 v2**；v1 靠数据库授权 + 审计 |
| 谓词列保护 | ✅ | ❌ **延后 v2**（随脱敏） |
| JIT 临时访问 | ✅ | ❌ **延后 v2**（随脱敏） |
| 自定义角色 | ✅ | ❌ v1 仅预置角色 |
| 行级权限 | ❌ 无 | ❌ 不做（未来优先走数据库原生 RLS） |
| 职责分离（DBA vs Security） | 部分 | ✅ **强化**：`securityAdmin`（审计/环境策略）与 `projectDBA`（实例/库管理）分离 |

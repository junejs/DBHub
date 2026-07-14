# 02 — 权限与访问控制

权限体系是本系统的核心。目标：**从库级到列级，实现完整、可组合、可审计的细粒度访问控制**。

> 设计大量参考 Bytebase 的 GCP-IAM 式模型（`角色 × 成员 × CEL 条件`）与动态脱敏机制。经评估，**行级权限（Row-Level Security）不在本期范围**：其 SQL 改写注入的正确性风险高、安全代价大，且需每引擎单独实现解析改写；未来若出现明确业务场景，再优先以「数据库原生 RLS（PostgreSQL Row Security Policy / Oracle VPD）」形式引入。

---

## 1. 模型总览

权限控制分为三层，逐层收紧：

```
第 1 层：粗粒度 RBAC（功能权限）
   └─ 角色(Role) = 权限(Permission) 集合，如 bb.sql.select / bb.instances.create
   └─ 通过 IAM 绑定把角色授予 成员(Member)

第 2 层：数据级授权（IAM 绑定 + CEL 条件）
   └─ 把 bb.sql.select 等数据权限，用 CEL 条件限定到 具体库/表/环境
   └─ 这是「库级 / 表级」授权的主要手段

第 3 层：数据内容级控制
   └─ 列级：动态脱敏（Data Masking）— 敏感列在结果中掩码
   └─ 谓词列保护：敏感列出现在 WHERE/JOIN 时默认拒绝，防止通过过滤推断明文
```

---

## 2. 角色（RBAC）

### 2.1 预置角色

权限分 **Workspace（工作区）级** 与 **Project（项目）级** 两层。成员通过 IAM 绑定获得角色。

| 层级 | 角色 | 定位 | 关键权限 |
|---|---|---|---|
| Workspace | `workspaceAdmin` | 系统管理员 | 全部权限（含 IAM、设置、IdP、订阅） |
| Workspace | `workspaceDBA` | DBA | 实例/库/策略/连接管理、元数据同步；**不含**用户管理与脱敏明文查看 |
| Workspace | `securityAdmin` | 安全管理员（**职责分离**） | 审计查看、数据分类、脱敏规则、JIT 审批；**不含**连接凭据与 DBA 权限 |
| Workspace | `workspaceMember` | 普通成员 | 浏览、申请权限（JIT）、查询自身历史 |
| Project | `projectOwner` | 项目所有者 | 项目内全部数据权限 + 项目 IAM 管理 |
| Project | `sqlEditorUser` | SQL 编辑者（读写） | `bb.sql.select/ddl/dml`（注：本期以查询为主；DDL/DML 属变更管理，不在本期范围） |
| Project | `sqlEditorReadUser` | SQL 只读者 | `bb.sql.select/explain/info` |
| Project | `projectViewer` | 项目只读者 | 浏览库表 schema，不可查询数据 |

> 默认核心角色：`workspaceAdmin` / `workspaceDBA` / `securityAdmin` / `workspaceMember` / `projectOwner` / `sqlEditorReadUser` / `sqlEditorUser`。

### 2.2 权限命名与分类

权限为扁平字符串集，命名规范 `db.<resource>.<verb>`：

| 类别 | 示例权限 |
|---|---|
| SQL 执行 | `db.sql.select / dml / ddl / explain / info` |
| 数据库/Schema | `db.databases.get / getSchema / list / sync / update` |
| Catalog 标注 | `db.databaseCatalogs.get / update`（配置脱敏语义类型） |
| 实例管理 | `db.instances.create / update / delete / sync` |
| 权限管理 | `db.projects.getIamPolicy / setIamPolicy` |
| 审计 | `db.auditLogs.search / export` |
| 脱敏策略 | `db.policies.{MaskingRule, MaskingExemption}.{get,update}` |
| 身份管理 | `db.users.* / db.groups.* / db.identityProviders.*` |
| JIT 访问 | `db.accessGrants.create / activate / revoke` |

### 2.3 自定义角色（可选扩展）
- 支持创建自定义角色（权限子集组合），预置角色不可改不可删。

### 2.4 Project：产品团队的逻辑隔离边界（核心概念）

> 公司按**产品团队**划分，每个团队在自己的 **Project** 内管理自己的数据库、成员与权限。**Project 是平台的一等逻辑隔离单元。**

- **资源归属**：每个**数据库**必属且仅属一个 Project（`databases.project_id`）。实例（Instance）是平台级共享资源——一个实例可同时承载多个 Project 的数据库。
- **成员制**：用户通过在 Project 上获得角色而成为该 Project 的成员（即「项目角色绑定 = 成员关系」）。一个用户可属于多个 Project（跨团队人员）。
- **默认跨 Project 隔离（Default Deny）**：用户对 Project A 的数据库没有任何访问权，除非他被授予了 Project A 的角色（或工作区级跨项目角色）。查询/导出/JIT 在执行前会解析目标库所属 Project，并强制校验调用者在**该 Project** 内的访问权。
- **可见性隔离**：用户能看到的 Project 列表 = 他持有角色的 Project（+ 工作区角色可见全部）。Project A 的成员在资源树/工作台里看不到 Project B 的库。
- **两层角色作用域**：
  - **Project 角色**（`projectOwner` / `sqlEditorUser` / `sqlEditorReadUser` / 自定义）：仅在所属 Project 内生效，是该 Project 数据访问权的来源。
  - **Workspace 角色**（`workspaceAdmin` / `workspaceDBA` / `securityAdmin`）：跨 Project 的平台级权限（管理实例、IdP、审计、全局脱敏策略等）。

---

## 3. IAM 绑定（数据级授权）

### 3.1 绑定结构（类 GCP IAM）

```
IamPolicy {
  bindings: [
    {
      role: "roles/sqlEditorReadUser",
      members: ["user:alice@corp.com", "group:analysts@corp.com"],
      condition: "resource.environment_id == 'prod' && resource.database == 'orders_db'"  // 可选 CEL
    }
  ]
}
```

- **members** 类型：`user:{email}`、`group:{email}`、`serviceAccount:{email}`、`allUsers`。
- **两层策略**：每个 Workspace 一份工作区策略；**每个 Project 一份项目策略**——这是各团队自治设置自己成员与权限的载体。
- **判定规则**：
  - Workspace 角色授予平台级权限（跨 Project）。
  - Project 资源的数据访问权（查询/导出等）**要求调用者在该 Project 内被放行**——这是 §2.4 跨 Project 隔离的执行点。
  - CEL 条件在 Project 内进一步把权限收敛到具体库/表。

### 3.2 CEL 条件可用的资源变量

| 变量 | 含义 | 典型用途 |
|---|---|---|
| `resource.environment_id` | 环境（prod/test） | 限制只能查测试库 |
| `resource.instance_id` | 实例 ID | 限定到某实例 |
| `resource.database` | 数据库名 | 限定到某库 |
| `resource.schema_name` | schema | 限定到某 schema |
| `resource.table_name` | 表名 | 限定到某表 |
| `request.time` | 当前时间 | 时间窗授权（如仅工作时间） |

### 3.3 库级 / 表级授权示例

```
// Alice 可读 prod 环境 orders_db 全部表
role=sqlEditorReadUser, members=[user:alice], condition="resource.environment_id=='prod' && resource.database=='orders_db'"

// Analysts 组只能读 customers、orders 两张表
role=sqlEditorReadUser, members=[group:analysts], condition="resource.database=='orders_db' && (resource.table_name=='customers' || resource.table_name=='orders')"
```

---

## 4. 列级权限 — 动态脱敏（Data Masking）

> 设计原则：**列级权限不通过 grant/deny，而通过「结果层动态掩码」实现**。用户能查到行，但敏感列被掩码；通过 JIT 去脱敏或脱敏豁免按需放开。

### 4.1 组成要素

1. **数据分类（Data Classification）**
   - 定义敏感等级（如 `Public / Internal / Confidential / Restricted`，级别递增）与分类项（如「手机号」「身份证」「银行卡」）。
   - 项目选择一套分类配置。

2. **列 Catalog 标注（ColumnCatalog）**
   - 为列打标：`semantic_type`（语义类型，如 `PHONE`）、`labels`、`classification`（分类 ID）。
   - 由 DBA/安全管理员在 catalog 页面或通过规则批量标注。

3. **脱敏规则策略（MaskingRulePolicy）**
   - 规则用 CEL 条件匹配列，命中后赋予 `semantic_type`：
     `condition: "resource.classification_level >= CONFIDENTIAL || resource.column_name matches '.*phone.*'"`
   - 规则可按 环境差异 化（如 prod 全脱敏，dev 不脱敏）。

4. **脱敏豁免策略（MaskingExemptionPolicy）**
   - 指定成员/组在（可选）特定列、（可选）时间窗内可查看明文。

5. **脱敏算法（Algorithm）**
   - `FullMask`（全掩码，如 `***`）
   - `RangeMask`（首尾保留，如 `138****1234`）
   - `MD5Mask`（加盐哈希）
   - `InnerOuterMask`（首尾保留、中间掩码）
   - 每种 `semantic_type` 映射到一个算法。

### 4.2 求值与执行

```
查询返回结果前：
  对结果集中每一列：
    1. 跑脱敏规则 CEL → 得 semantic_type → 得算法
    2. 若无命中，看列 catalog 直配 semantic_type
    3. 检查豁免：成员+列+时间 命中 → 跳过脱敏
    4. 否则按算法逐行掩码
  返回掩码后结果
```

- 脱敏在**服务端、结果返回前**强制执行，前端拿到的就是掩码数据。
- 若 SQL 报错但触及敏感列，错误信息也要脱敏/截断，防止通过错误泄露。

### 4.3 谓词列保护（Predicate Column Check）
- 若敏感列出现在 `WHERE`/`JOIN ON` 等谓词位置，掩码结果仍可通过「能否过滤命中」推断明文值。
- 默认策略：**拒绝**此类查询；或强制谓词列也按掩码值比较（语义上有损，谨慎）。

### 4.4 引擎能力
- 引擎是否支持脱敏由能力矩阵声明。不支持脱敏的引擎，若含敏感列则禁止查询/导出。

---

## 5. 行级权限（Row-Level Security）—— 不在本期范围

> 经评估，**行级权限本期不实现**，未来视业务需要再引入。

**结论与理由：**
- 行级权限（同一张表、不同用户看到不同行子集）只有当数据呈多租户/地区隔离/"我的数据"形态时才真正需要；若数据主要按角色共享整表，**表级 + 列级脱敏**已覆盖绝大多数查询/导出场景。
- 平台侧实现行级过滤的主流做法是「SQL 改写注入」（把 `AND predicate` 注入到每张被约束表），但其**正确性风险高**：子查询、CTE、各类 JOIN、表别名、UNION 等任一未覆盖，轻则查询报错、重则**越权泄露数据**；且 bug 即安全事故（非普通功能缺陷）。
- 该能力还需**每引擎单独实现解析改写**，维护成本随引擎数线性增长。

**未来引入路径（届时优先）：**
- **数据库原生 RLS**：PostgreSQL Row Security Policy、Oracle VPD、Snowflake Row Access Policy——策略下推到数据库侧强制，绕不过去，最可靠；平台仅做策略管理与下发。
- 仅当引擎不支持原生 RLS 且确有强需求时，才评估 SQL 改写注入方案，并配套失败关闭 + 大规模测试。

---

## 6. JIT 临时访问（Access Grant）

用于「按需、限期、可去脱敏」的临时授权，避免长期授予高权限。

### 6.1 模型
```
AccessGrant {
  state: PENDING → ACTIVE → REVOKED
  targets: ["instances/{i}/databases/{d}"]   // 目标库
  query: "SELECT ... WHERE id=123"           // 精确语句（可选，绑定到具体 SQL）
  unmask: true/false                          // 是否去脱敏
  export: true/false                          // 是否允许导出
  reason: "客诉处理"
  expire_time / ttl                           // 有效期
  issue_id                                    // 关联审批单
}
```

### 6.2 流程
1. 成员（需 `db.accessGrants.create`）发起申请：选择目标库/列、事由、有效期、是否去脱敏、是否导出。
2. 审批人（`securityAdmin`/`projectOwner`，需 `db.accessGrants.activate`）批准 → 状态 ACTIVE。
3. 期间命中目标的查询自动放行（去脱敏若申请）；过期或手动 REVOKE 后失效。
4. 选择「能力最高」的授权生效（unmask=true 优先）。

### 6.3 安全
- JIT 授权的目标库仍受跨库引用的 IAM 检查（防止以 JIT 为跳板读取未授权库）。
- 所有 JIT 授权下的查询/导出，审计记录中标记 `applied_access_grant`，便于专项复查。

---

## 7. 权限执行点（汇总）

| 操作 | 检查内容 |
|---|---|
| 任意 RPC | ACL 拦截器：粗粒度 `db.<resource>.<verb>` + workspace 隔离 |
| SQL SELECT | QuerySpan → 库/表 CEL 条件 + 列脱敏 + 谓词列保护 |
| SQL DML/DDL（变更管理，不在本期范围） | 解析写目标 → 逐目标库/表/环境校验 |
| 导出 | 同查询，额外校验 `export=true`（JIT）/ 导出权限 |
| 资源管理 | 实例/库/catalog 等各自 `db.*` 权限 |
| 权限变更 | `setIamPolicy` 记 PolicyDelta 审计 |

---

## 8. 与 Bytebase 的差异小结

| 维度 | Bytebase | 本系统 |
|---|---|---|
| RBAC + IAM 绑定 | ✅ | ✅ 沿用 |
| CEL 条件到库/表 | ✅ | ✅ 沿用 |
| 列级（动态脱敏） | ✅ | ✅ 沿用 |
| 谓词列保护 | ✅ | ✅ 沿用 |
| JIT 临时访问 | ✅ | ✅ 沿用 |
| 行级权限 | ❌ 无 | ❌ 不在本期范围（同 Bytebase 取舍；未来优先走原生 RLS） |
| 职责分离（DBA vs Security） | 部分 | ✅ **强化**，显式 `securityAdmin` 与 `workspaceDBA` 分离 |

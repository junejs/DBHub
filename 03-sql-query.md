# 03 — SQL 查询工作台

SQL 查询是系统最高频场景。本文档定义查询工作台的完整需求：从查询执行、自动补全，到结果展示、查询历史与安全控制。

---

## 1. 用户故事

- 作为分析师，我希望打开工作台，选择数据库，编写 SQL 时能得到表/列/函数的自动补全。
- 作为分析师，我希望执行查询后快速看到结果，并能分页/翻看。
- 作为分析师，我希望能查看自己的历史查询，一键重跑。
- 作为安全管理员，我希望所有查询都受权限/脱敏约束，且全程审计。

---

## 2. 功能需求

### 2.1 数据库与连接选择
- 工作台左侧展示「我可见的数据库」（按 Project/Environment 分组，仅展示 IAM 授权的库）。
- 选择库后，系统自动选择该实例的只读数据源（READ_ONLY）；无只读则回退 ADMIN 连接但强制只读模式。
- 支持选择 schema / search_path（PostgreSQL）、当前 schema（Oracle）等。

### 2.2 SQL 编辑器
- 基于 **Monaco Editor**，支持语法高亮、括号匹配、多光标、查找替换。
- **多语句编辑**：编辑区可写多条语句，分号分隔；执行时按光标位置或全选执行。
- **自动补全**（见 §3）。

### 2.3 执行与结果
- 执行按钮：执行全部 / 执行选中 / 执行当前语句。
- 结果区：表格展示，支持列类型感知（时间、JSON、二进制等）。
- **分页**：默认带行数上限（见 §5），支持「加载更多」；大结果引导用户走导出。
- **多结果集**：多语句时按语句分组展示，各自带状态/耗时/错误。
- **错误信息**：引擎原始错误结构化（PG 错误码、语法错误位置等），但涉敏感列时脱敏。

### 2.4 EXPLAIN
- 支持 `EXPLAIN` / `EXPLAIN ANALYZE`，按引擎能力展示执行计划（文本/可视化）。

### 2.5 查询历史
- 每次查询自动记录：数据库、语句、耗时、错误、时间、创建人。
- 用户可查看/搜索/重跑自己的历史；按 CEL 过滤（库、语句包含等）。
- 仅本人可见（auth_method=CUSTOM）。

### 2.6 保存的查询（Worksheet）
- 支持保存 SQL 为 Worksheet：标题、内容、可见性（私有/项目读/项目写）、收藏、文件夹组织。
- 项目内可共享，便于团队复用。
- 收藏与分享的完整设计见 [09-sql-favorite-share.md](./09-sql-favorite-share.md)。

### 2.7 引擎消息
- 展示引擎消息（PG `RAISE NOTICE`、MSSQL `PRINT`、Oracle `DBMS_OUTPUT`）。

---

## 3. SQL 自动补全（核心特性）

### 3.1 目标
补全质量接近 IDE：基于**真实 schema 元数据**的上下文相关补全。

### 3.2 架构：LSP over WebSocket

```
Monaco Editor (前端)
   │  textDocument/completion (JSON-RPC over WS)
   ▼
LSP Server (后端 /lsp)
   │  根据 engine 调用对应 parser.Completion
   ▼
Parser 插件层（按引擎）
   │  从元数据缓存取 schema（表/列/视图/函数）
   ▼
平台元数据库（同步后的 DatabaseSchemaMetadata）
```

- 前端 Monaco 作为 LSP 客户端（基于 `vscode-languageclient` / `monaco-languageclient`），通过 WebSocket 连后端 LSP 服务。
- 后端 LSP 服务按**引擎**分发到对应 `parser/<engine>/completion` 实现。
- 补全候选来源：**同步后的真实 catalog**（表、列、视图、函数、关键字）。

### 3.3 补全候选与排序
- 候选类型：关键字、Schema、表、视图、列、函数、过程。
- **上下文相关**：`FROM` 后优先表/视图；`SELECT` 后优先列；`.` 触发 schema/table 内成员。
- 排序优先级：列 > schema > 表 > 视图 > 函数 > 关键字；`SELECT/SHOW/SET/FROM/WHERE` 关键字加权。
- 节流：hover 300ms、completion 200ms；触发字符 `. , ( 空格` 或显式调用。

### 3.4 引擎支持
- 由能力矩阵 `EngineSupportAutoComplete` 声明。**当前版本实现 PostgreSQL**；架构支持扩展到 MySQL、Oracle、MSSQL、ClickHouse、Snowflake、Redshift 等（新增引擎仅需实现其 parser completion 插件）。
- 不支持的引擎：编辑器仍可用，但仅关键字补全或关闭补全。

### 3.5 其他 LSP 能力（可选/后续）
- `textDocument/hover`：列/表的文档与类型提示。
- `textDocument/diagnostic`：实时语法诊断（标记语法错误）。
- 自定义 command：如「插入列清单」「格式化」。

---

## 4. API 设计（关键 RPC）

| RPC | 路径 | 权限 | 审计 | 说明 |
|---|---|---|---|---|
| `Query` | `POST /v1/{name=instances/*/databases/*}:query` | `db.sql.select` | ✅ | 执行只读查询 |
| `AdminExecute` | `GET /v1:adminExecute`（流） | `db.sql.admin` | ✅ | 管理连接执行（高权限，慎开；可选增强） |
| `DiffMetadata` | schema diff | 公开工具 | ❌ | 两个 catalog 的结构差异 |
| `SearchQueryHistories` | `db.sql.*`（CUSTOM） | ❌ | 自身历史 |
| `GetQueryHistory` | 同上 | ❌ | 单条历史 |
| Worksheet CRUD | `db.worksheets.*` | 部分 | 保存的查询 |

### 4.1 `QueryRequest` 主要字段
```
name          // instances/{i}/databases/{d}
statement     // SQL 文本
limit         // 行数上限
data_source_id // 可选，指定数据源
explain       // 是否 EXPLAIN
schema        // 可选，search_path / current schema
query_option  // 引擎相关选项
```

### 4.2 `QueryResult` 主要字段
```
column_names / column_type_names
rows[]             // 每行带类型化值（null/bool/int/float/string/bytes/time/struct）
rows_count
error              // 引擎错误（结构化）
detailed_error     // 细分：SyntaxError(位置)/PermissionDenied/CommandError(DDL/DML/非只读)
latency
statement
engine_messages    // NOTICE/PRINT 等
masking_reasons    // 每列脱敏原因（便于前端标注「已脱敏」）
applied_access_grant // 若经 JIT 授权
```

---

## 5. 安全与限制（与 [02](./02-permission-and-access.md) 联动）

| 控制项 | 说明 |
|---|---|
| 权限校验 | 粗粒度 `db.sql.select` + 细粒度 CEL（库/表）+ 列脱敏 |
| 只读强制 | 只读数据源下拒绝 DDL/DML/非只读语句 |
| 谓词列保护 | 敏感列出现在 WHERE/JOIN 时默认拒绝 |
| 行数上限 | 单次结果默认上限（如 1000 行），可由策略调整；超过引导导出 |
| 结果大小上限 | 单结果集字节上限（防 OOM/滥用） |
| 超时 | 单语句超时（如 30s），可按引擎/策略配置 |
| 重试与停止 | 默认遇错停止；支持语句级重试 |
| 并发限制 | 单用户/单库并发查询数限制 |
| **查询成本护栏** | 见 §5.6，执行前 EXPLAIN 估算成本/行数，超阈值拦截或告警 |

---

## 5.6 查询成本护栏（Query Cost Guardrail）

> 防止"一条全表扫描拖垮业务库"。这是本系统相对 Bytebase 的差异化能力。

**机制：**
1. SELECT 执行前，先对语句跑 `EXPLAIN`（PostgreSQL 可用 `EXPLAIN`，无需 ANALYZE 避免实际执行开销）。
2. 从执行计划估算**预估行数（rows）** 与**成本（cost）**。
3. 与阈值比较——阈值取自目标库所属环境的 `environment_policies.query_cost_threshold`：
   - **硬阈值**（如 prod cost > 1e6）：**拦截**，拒绝执行并提示"查询开销过大，请加过滤条件或走导出"。
   - **软阈值**（如 prod cost > 1e5）：放行但**告警**（结果区标注、记录到审计）。
4. 同一条查询在 prod 会被严格限制、在 dev 则宽松放行（阈值按环境差异化，见 [10 environment_policies](./10-data-model.md)）。同时行数上限也由 `environment_policies.query_row_limit` 决定。

**设计要点：**
- EXPLAIN 本身有开销，仅在 SELECT 且未命中结果缓存时触发；可配置开关。
- 对无法 EXPLAIN 的语句（DDL/非 SELECT）跳过。
- 拦截决策与原因写入审计。
- 与行数上限/超时叠加，形成"事前估算拦截 + 事中行数/超时兜底"的双重防护。

---

## 6. 非功能需求

- **补全响应**：首字符候选 P95 ≤ 200ms。
- **查询首屏**：常见查询 P95 ≤ 2s（不含网络到业务库的固有延迟）。
- **元数据新鲜度**：补全依赖的 catalog 由后台同步（≤15min）+ 手动刷新保证；补全取缓存即可。
- **可观测**：查询耗时、行数、错误率埋点。

---

## 7. 功能范围

- 库选择、SQL 编辑、多语句执行、结果分页、错误展示。
- LSP 自动补全（**当前版本引擎：PostgreSQL**）。
- 查询历史（自身，可搜索重跑）。
- 保存查询（Worksheet，项目内共享）。
- EXPLAIN。
- 查询成本护栏（EXPLAIN 估算 + 阈值拦截/告警）。
- 权限/脱敏/审计联动。

**可选增强（不在本期必须范围）：** AdminExecute 管理连接模式、AI 辅助生成 SQL、跨库查询。

# 04 — 数据导出

数据导出是第二大核心场景。本文档定义导出任务的模型、格式、生命周期，以及与权限/审计的联动。

---

## 1. 用户故事

- 作为分析师，我希望把查询结果导出为 CSV/Excel 用于报表。
- 作为分析师，当结果很大时，我希望能提交后台导出任务，完成后下载。
- 作为安全管理员，我希望每一次导出都被强审计，并能按环境限制导出规模与是否需要审批。

---

## 2. 功能需求

### 2.1 导出方式

| 方式 | 适用 | 行为 |
|---|---|---|
| **同步导出（Ad-hoc）** | 预估 ≤ 1 万行（EXPLAIN 估算，D22） | 工作台直接导出，立即返回带密码的压缩包 |
| **异步导出（任务）** | 预估 > 1 万行（强制异步） | 提交任务，后台执行，完成后通知下载；产物有保留期 |

> 系统对目标语句跑 EXPLAIN 估算行数，超过 1 万行**强制走异步**（D22），不允许同步导出大结果。

### 2.2 导出格式
- **CSV**、**JSON**。（XLSX、SQL(`INSERT`) 格式延后，见 §7）
- 每条语句的结果独立成文件，打包进一个**密码加密 ZIP**。

### 2.3 导出来源
- **来自工作台 SQL**：用户在工作台填入导出 SQL（一条或多条），选择格式与密码。
- **来自保存的查询（Worksheet）**：选定已保存 SQL 发起导出。
- **来自审批单（可选增强）**：审批通过的批量导出任务，可走 ADMIN 连接。

### 2.4 导出任务生命周期（异步）
```
CREATED → RUNNING → SUCCEEDED → DOWNLOADABLE → EXPIRED
                  ↘ FAILED
```
- 任务记录：发起人、目标库、SQL、格式、状态、进度、产物 ID、创建/完成时间、错误。
- 完成后产物经**存储抽象**写入（MVP 本地盘，未来可切 S3/OSS，D20）；`export_archives(storage,location)` 已预留。
- 保留期默认 **24h**，过期自动清理。
- 完成后向发起人推**站内通知**（`export_done`，D21）；用户在「导出中心」查看任务列表、下载产物；产物需密码解压。

### 2.5 下载
- 下载产物需校验：发起人或具备下载权限的成员。
- 产物 ZIP 密码由用户在发起时设定（强密码校验），或由系统生成并通过安全渠道下发。
- 异步任务如基于审批单，下载时按需重新打包（多任务合并）。

---

## 3. 安全与合规（与 [02](./02-permission-and-access.md) 联动）

| 控制项 | 说明 |
|---|---|
| 权限 | 需 `db.sql.select`（导出走查询路径）或专门的 `db.exports.create` |
| 敏感列 | v1 不做应用层脱敏；敏感列可见性由数据库授权决定（导出与查询共用同一只读连接） |
| **环境差异（重要）** | 导出护栏按环境差异化：行数上限取 `environment_policies.export_max_rows`；prod 默认 `export_require_approval=true`（导出需审批），dev/test 宽松 |
| 产物密码 | 必须密码加密，防止产物在传输/存储中被读取 |
| 审计 | 导出行为强审计：语句摘要、行数、产物 ID、**环境** |
| 行数/大小上限 | 单任务上限（按环境配），防全表导出滥用 |
| 下载限制 | 产物保留期内可下载；过期不可恢复 |

---

## 4. API 设计（关键 RPC）

| RPC | 路径 | 权限 | 审计 | 说明 |
|---|---|---|---|---|
| `Export`（同步） | `POST .../:export` | `db.sql.select` | ✅ | 工作台同步导出，返回 ZIP bytes |
| `CreateExportTask`（异步） | `POST .../exportTasks` | `db.exports.create` | ✅ | 创建异步导出任务 |
| `GetExportTask` / `ListExportTasks` | — | 发起人/权限 | ❌ | 查询任务状态 |
| `DownloadExport` | `GET .../exportTasks/{id}:download` | 发起人 | ✅ | 下载产物（校验密码） |

### 4.1 导出请求主要字段
```
name             // projects/{p}/instances/{i}/databases/{d}（同步）或 projects/{p}（任务）
statement        // 导出 SQL
format           // CSV/JSON（v1）
password         // ZIP 密码
data_source_id   // 可选
limit            // 可选行数上限
```

### 4.2 异步任务模型（ExportTask）
```
state            // CREATED/RUNNING/SUCCEEDED/FAILED/EXPIRED
database, statement, format
creator, create_time, complete_time
row_count, size_bytes
error
export_archive_id  // 产物引用
```

---

## 5. 实现要点（参考 Bytebase `backend/component/export`）

- 格式写入器按格式独立实现，**流式写入**（避免大结果集驻留内存）：
  - CSV：流式、引号转义、二进制 hex、空值处理。
  - JSON：流式对象/数组。
  - （SQL/XLSX 延后；v1 仅 CSV/JSON）
- ZIP 加密使用支持密码的 zip 库；每个语句产出一对文件：`statement-N.sql` + `statement-N.result.<ext>`。
- 异步执行器（Task Runner）调用 `Driver.QueryConn`（带超时、流式），写入产物存储，记录 `export_archive`，任务状态置 SUCCEEDED。
- 清理器（Data Cleaner）周期删除过期产物（默认 24h）。

---

## 6. 非功能需求

- 大数据量导出：异步任务支持 **百万级行**（流式，分批提交产物）。
- 资源隔离：导出任务限并发，避免拖垮业务库（建议使用只读副本，并设置语句超时）。
- 产物存储：本地盘；未来可扩展到对象存储（支持更大容量与保留）。

---

## 7. 功能范围

- 同步导出（CSV/JSON）+ 密码 ZIP。
- 异步导出任务（创建/查询/下载/过期清理）。
- 导出权限、审计联动；敏感列由数据库授权控制。
- 导出中心（任务列表 + 状态 + 下载）。

**可选增强（不在 v1 范围，见 [18 §1.7](./18-roadmap.md)）：** XLSX/SQL 导出格式、审批制批量导出、导出到外部对象存储。

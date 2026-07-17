# 06 — 审计日志

审计日志是合规与安全回溯的基石。目标：**所有关键操作自动、完整、不可篡改地记录，且业务代码无法绕过**。

> 设计参考 Bytebase 的「注解驱动 + 拦截器统一写入 + 敏感字段脱敏 + 不可取消 context」机制。

---

## 1. 用户故事

- 作为安全管理员，我希望查询「某分析师上月在 prod 库的所有导出操作」。
- 作为合规人员，我希望导出一段时间的审计记录用于审计报告。
- 作为平台负责人，我希望审计记录不可被任何人（含管理员）篡改或删除。
- 作为安全管理员，我希望审计中的密码、查询结果、token 等敏感内容被自动脱敏。

---

## 2. 审计日志数据模型

```
AuditLog {
  parent          // 作用域：projects/{p} 或 workspaces/{w}
  method          // RPC 名，如 /dbh.SQLService/Query
  resource        // 受影响的资源名（实例/库/邮箱等）
  user            // 操作者 users/{email}
  severity        // DEBUG..EMERGENCY（默认 INFO）
  request         // 请求 JSON（已脱敏）
  response        // 响应 JSON（已脱敏；结果行/导出内容等大字段被丢弃）
  status          // google.rpc.Status（成功为 nil）
  latency         // 耗时
  service_data    // 附加上下文（如权限变更前后差异）
  request_metadata {
    caller_ip
    caller_supplied_user_agent
  }
  name            // {parent}/auditLogs/{uid}（输出）
  create_time     //（输出）
}
```

存储表见 [10 §3.9 audit_logs](./10-data-model.md)：列式、按月分区、**无 workspace/tenant 列**（D1），scope 由 `scope_type`/`scope_id` 表达。关键字段：`method`、`actor_id`/`actor_email`、`scope_type`/`scope_id`、`resource`、`severity`、`status_code`/`status_message`、`latency_ms`、`request`/`response`（脱敏后）、`ip`、`user_agent`、`service_data`。

---

## 3. 被审计的事件（声明式）

通过 RPC 注解 `audit = true` 声明。本期至少覆盖以下：

| 类别 | 事件 |
|---|---|
| **认证** | Login、Logout、Refresh、RequestPasswordReset、ResetPassword（对齐 [12 AuthService](./12-api-contract.md)）；启用邮箱验证码登录时含 SendEmailLoginCode |
| **数据访问** | SQL.Query、SQL.Export（流式按消息对审计）；AdminExecute（启用时） |
| **权限变更** | SetIamPolicy（workspace/project，记录前后差异） |
| **用户/身份** | CreateUser/UpdateUser/DeleteUser、组增删、IDP 增删改 |
| **资源管理** | CreateInstance/Update/Delete、AddDataSource/Update/Remove、UpdateDatabase |
| **策略** | 环境策略（environment_policies）的增删改 |
| **审计** | SearchAuditLogs、ExportAuditLogs（审计的审计） |
| **设置** | 关键 Setting 更新（记前值） |

---

## 4. 写入机制

1. **拦截器统一写入**：`AuditInterceptor` 包装每个 RPC。
2. **声明驱动**：是否审计由 RPC 的 `audit` 注解决定（在认证阶段读入上下文）。
3. **脱敏**：写入前对 request/response 做字段级脱敏——密码、token、连接凭据（SSL/SSH key）、查询结果行、导出内容、登录 OTP/MFA 码 → 替换为空/掩码。
   - **SQL 语句按字面量原样记录（已确认决策）**：审计保留完整 SQL 文本，**不做字面量参数化/脱敏**（即 `WHERE ssn='110...'` 中的敏感值会被原样留存），以完整支持事后回溯与取证。
   - ⚠️ **残余风险与控制**：因字面量可能含 PII，审计日志本身即为敏感对象。读权限按 **D26** 收紧：`securityAdmin`/`workspaceAdmin` 可读全部、`projectOwner` 可读本项目（服务端强制按 scope 过滤）；审计访问行为本身也被审计；按保留期到期清理或转冷存储。
4. **操作者解析**：从上下文取用户；登录类 RPC 特殊处理（从请求/MFA token 解析邮箱）。
5. **作用域（parent）解析**：从资源名解析所属 project/workspace；登录类回退到工作区。
6. **逐 parent 写一条**：若一次操作涉及多个 project，每个 project 各写一条审计。
7. **写入不随请求取消（语言中立）**：审计落库使用一个**不随请求取消**的执行上下文（请求取消/客户端断连不传播到审计写入），确保即便客户端断开，审计仍落库。具体实现（如派生独立任务/线程、解绑取消信号）由各语言自选。
8. **stdout 镜像（可选）**：可配置把审计同步输出为结构化日志（截断 100KB），便于外部 SIEM 采集。

---

## 5. 查询与导出

### 5.1 SearchAuditLogs
- `POST /v1/{parent}/auditLogs:search`，权限 `db.auditLogs.search`。
- **读权限范围（D26）**：
  - `securityAdmin` / `workspaceAdmin`：可查全部审计（任意 parent）。
  - `projectOwner`：仅可查**自己所属项目**的审计（parent=`projects/{自己的项目}`，服务端强制按项目过滤，无法越界）。
  - 审计含 PII 字面量（D5），读权限严格收紧；**查看审计行为本身也被审计**。
- **结构化过滤**（UI 下拉/日期选择，不暴露表达式语言）：操作类型(method)、用户、资源、严重级别、时间范围。
- 分页（page_size + page_token，上限如 5000）。
- 默认按 `create_time DESC`。

### 5.2 ExportAuditLogs
- `POST .../auditLogs:export`，权限 `db.auditLogs.export`。
- 复用 Search 流程，渲染为 CSV/JSON/XLSX，列：`time, user, method, severity, resource, request, response, status`。

### 5.3 典型查询示例
```
// 某分析师上月所有导出：操作类型=SQL.Export，用户=alice@corp.com，时间=2026-06
// 所有权限变更：操作类型=SetIamPolicy
```

---

## 6. 不可变性与保留

| 维度 | 策略 |
|---|---|
| **不可变** | 应用层只有 INSERT，无 UPDATE/DELETE（仅在身份重命名等维护路径下重写 `user` 引用）。生产建议进一步加 PG 侧防护（行保护、WORM 存储、独立审计库账号） |
| **保留期** | 单一可配置保留期（无多租户分级，D1）；查询期 `created_at >= cutoff` 过滤，老数据物理留存（或迁移冷存储） |
| **防篡改** | 推荐生产部署：审计库独立、仅追加权限；可选对审计做哈希链/签名，或导出到外部不可变存储（S3 Object Lock） |

---

## 7. 审计内容字段说明（对前述模型补充）

- **method**：便于按操作类型筛选。
- **resource**：受影响资源（库/表/实例/用户邮箱）。
- **user**：操作者。
- **request/response**：脱敏后的请求/响应摘要（密码/token/凭据等敏感字段置空）；查询类只保留列名/语句/错误，丢弃结果行；导出类丢弃内容但保留行数/产物引用。
- **service_data**：权限变更带前后差异（增删了哪些绑定）；设置更新带前值。
- **latency / status / ip / ua**：运维与取证所需。

---

## 8. 非功能需求

- 审计写入不阻塞主流程（异步落库 + 不可取消 context）。
- 查询性能：常用过滤（method/user/time）有索引；千万级审计可接受秒级查询。
- 审计量预估：高频查询会产生大量审计，需规划存储与归档（冷热分层）。

---

## 9. 功能范围

- 注解驱动 + 拦截器统一审计，覆盖 §3 事件清单。
- 完整数据模型 + 脱敏 + 不可取消写入。
- 结构化查询 + 导出。
- 保留期策略（默认配置）。

**可选增强（不在本期必须范围）：** 哈希链/签名防篡改、外部不可变存储、SIEM 实时投递（stdout/kafka）、可视化审计看板。

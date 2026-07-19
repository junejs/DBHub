# 16 — 部署、安全参数、容量与测试

> 运维/QA/安全交付依据。把"方向级"落到**可执行数值与清单**。配合 [08 NFR](./08-nfr.md)、[11 决策](./11-decisions.md)、[14 边界](./14-edge-cases.md)。

---

## 1. 部署形态

- **MVP：单节点**（D24）：一个应用进程 + 一个平台 PostgreSQL（单实例）。
- 后续演进：无状态多副本 + 平台 PG 主从 + 对象存储（导出产物）。
- 交付形态：**docker-compose**（首选）/ 单二进制 / Helm Chart（后续）。
- 进程模型：HTTP/JSON + WebSocket(LSP) 同进程；后台 Runner（同步/导出/清理）同进程（任务调度机制由实现语言自选）。

### 健康检查
- `GET /healthz` → liveness（进程存活）。
- `GET /readyz` → readiness（平台 PG 可连、缓存就绪）。
- 启动：平台 PG 迁移（migrate）完成后才 ready。

---

## 2. 配置项清单（环境变量 / config.yaml）

| 配置 | 环境变量 | 默认 | 说明 |
|---|---|---|---|
| 监听 | `PORT` | 8080 | HTTP/JSON；LSP 走同端口 WS 升级 |
| 平台 PG DSN | `DB_DSN` | — | `postgres://...` |
| **主密钥** | `MASTER_KEY` | — | AES-256-GCM 主密钥（base64，32B）；**不入库不入仓** |
| **JWT 密钥** | `JWT_SECRET` | — | HS256 签名密钥 |
| 外部 Secret Manager | `SECRET_MANAGER` | 空 | `vault`/`aws`/`gcp`/`aliyun`；为空则用应用层加密(D25) |
| 首次管理员 | `BOOTSTRAP_ADMIN_EMAIL` / `_PASSWORD` | — | 首启创建；之后失效 |
| 同步周期 | `SYNC_INTERVAL` | 15min | D19 |
| 懒刷新 TTL | `SCHEMA_CACHE_TTL` | 15min | 缓存过期阈值 |
| 查询超时 | `QUERY_TIMEOUT` | 30s | 单语句 |
| 异步导出阈值 | `EXPORT_SYNC_ROW_LIMIT` | 10000 | D22 |
| 产物保留 | `EXPORT_RETENTION` | 24h | |
| 限流：查询 | `RATE_QUERY_PER_USER_PER_MIN` | 60 | |
| 限流：登录 | `RATE_LOGIN_PER_IP_PER_MIN` | 10 | |
| 日志 | `LOG_LEVEL` / `LOG_FORMAT` | info / json | slog |
| 审计 stdout 镜像 | `AUDIT_STDOUT_MIRROR` | false | 开后按 100KB 截断输出 |
| 默认语言 | `DEFAULT_LOCALE` | zh | D23（zh/en） |

> 所有密钥类变量必须通过环境注入（容器编排/K8s Secret），禁止写入镜像或配置文件。

---

## 3. 安全参数（具体数值）

| 项 | 值 | 说明 |
|---|---|---|
| bcrypt cost | **12** | 密码哈希 |
| access token TTL | **1h** | JWT |
| refresh token TTL | **7d** | 不透明，SHA256 存；旋转刷新（非滑动） |
| MFA temp token TTL | **5min** | 登录两步桥接 |
| 登录失败锁定 | 密码 **10 次/10min**；MFA **5 次/5min** | 超限 `AUTH_ACCOUNT_LOCKED` |
| 密码策略 | 最少 **12 位**，含大小写+数字+符号；**180 天可选轮换** | 可配 |
| 邮箱验证码 | 6 位、**10min** 有效、**60s** 重发冷却、5 次尝试 | HMAC-SHA256 存 |
| TOTP | RFC 6238，30s 步长，±1 窗口 | Google Authenticator 等 |
| Cookie 属性 | `HttpOnly; Secure; SameSite=Lax`；跨站点敏感操作 `Strict` | |
| TLS | **强制**（平台入站 + 到业务库 + 到 IdP） | |
| CORS | 仅允许可信前端域；`Authorization` 受控 | |
| CSP | `default-src 'self'`；Monaco/WS 放行所需 | |
| 凭据加密 | **AES-256-GCM**（D25）；主密钥环境注入 | |
| 导出 ZIP 密码 | 强密码校验（≥12 位复杂度） | |
| 审计 stdout 截断 | 100KB | CloudTrail 对齐 |

---

## 4. 容量假设（规模预估，用于选型与默认值）

> 用于指导限流默认值、连接池、平台 PG 容量。实际以压测校准。

| 维度 | 预估（中型组织） |
|---|---|
| 产品团队（项目） | 20–50 |
| 数据库实例 | 100–300 |
| 数据库 | 500–2000 |
| 用户 | 200–1000 |
| 并发查询 QPS | 峰值 50–100 |
| 并发异步导出 | 峰值 10 |
| 单库表数 | 数百～数千 |

**平台 PG 容量（大头是审计）：**
- 估算：1000 用户、人均日 20 次操作 → 2 万条/日 → ~730 万/年；每条 ~2KB → ~15GB/年（审计）。
- 建议：平台 PG 起步 50GB；审计按月分区，老分区归档/清理（保留期按合规）。
- 元数据（用户/权限/catalog）相对小，<1GB 量级。

**到业务库的连接池：** 每实例独立池；readonly 池大小按并发上限配（如每实例 10）；超限排队+超时。

---

## 5. 可观测性

**日志（结构化 slog / JSON）：**
- 业务日志 + 审计镜像（`AUDIT_STDOUT_MIRROR=true` 时，`log_type=audit`）。
- 关键字段：`trace_id`、`user`、`project`、`method`、`latency_ms`、`status_code`。

**指标（Prometheus）：**
| 指标 | 说明 |
|---|---|
| `dbh_query_duration_seconds`（histogram） | 查询耗时 |
| `dbh_query_rows` | 返回行数 |
| `dbh_export_task_state`（gauge by state） | 导出任务状态分布 |
| `dbh_sync_failed_total` | 同步失败 |
| `dbh_audit_write_failed_total` | 审计写入失败（D30 告警源） |
| `dbh_db_pool_inuse` | 业务库连接池占用 |
| `dbh_lsp_completion_latency` | 补全延迟 |

**链路追踪：** 可选；Trace ID 贯穿 RPC → Driver（建议接 OpenTelemetry）。

**告警（最低集）：**
- 审计写入失败率 > 阈值（D30）。
- 同步失败持续堆积。
- 业务库连接失败率升高。
- 平台 PG 磁盘/连接耗尽。

---

## 6. 备份与恢复

- **MVP：不做备份**（D24，已知风险 D-risk-1）。
- **上线前最低要求：** 平台 PG 定时 `pg_dump`（如每日，保留 7–30 天）+ 异地存放。
- **恢复目标（建议）：** RTO ≤ 4h，RPO ≤ 24h（按定时备份粒度）。
- 审计为合规数据，建议额外将审计分区/stdout 归档到不可变存储（WORM）。

---

## 7. 测试策略

> 测试是 v1 交付质量的核心保障。以单元测试为主，集成/安全/性能/审计测试为辅。

### 7.1 单元测试（第一优先级）

**目标：** 所有业务逻辑可在不依赖外部服务的情况下被快速、稳定地测试。

**后端（Go）：**
- **框架**：`stretchr/testify` 断言 + `gomock`/`mockery` 生成 mock。
- **设计约束**：依赖外部资源（DB、IdP、存储、缓存）的组件必须通过 interface 注入，禁止在业务逻辑中直接创建 `sql.DB` 或 HTTP client。
- **必测模块与场景**：
  - **IAM 引擎**：角色解析、权限集合展开、结构化条件（环境/库/表）匹配、跨项目隔离判定。
  - **审计拦截器**：事件生成、请求/响应脱敏、状态码分类、不可取消 context 保证审计落库。
  - **SQL Parser**：多语句切分、只读判断、语法错误定位、DDL/DML 拒绝。
  - **导出格式化器**：CSV 引号/转义/空值/二进制 hex、JSON 流式结构、大字段截断。
  - **环境策略解析**：行数上限、字节上限、并发限制、导出上限按环境差异化生效。
  - **登录锁定**：失败窗口、阈值触发、锁定解锁、多 channel（password/mfa）隔离。
- **表驱动测试**：对组合爆炸场景（如权限判定矩阵、导出格式边界）使用表驱动，覆盖正例/反例/边界。
- **覆盖率门槛**：
  - 核心业务逻辑（IAM、审计、导出、查询、同步、登录）≥ **80%**
  - 工具类/胶水代码 ≥ **60%**
  - 关键路径未覆盖不允许合并。

**前端（React + TypeScript）：**
- **框架**：Vitest + `@testing-library/react` + `@testing-library/user-event`。
- **测试重点**：
  - 组件渲染与交互（表单校验、权限按钮显隐、表格分页/排序、错误状态）。
  - Zustand store 状态转换。
  - TanStack Query hook 封装（请求/缓存/重试/错误处理）。
  - API 层错误码映射与提示。
- **API Mock**：MSW（Mock Service Worker）统一拦截 REST/WebSocket，避免测试依赖后端。
- **不测试第三方库**：Monaco、虚拟化库、图表库等不做单元测试，由 e2e/手动覆盖。

### 7.2 集成测试（testcontainers + 真 PostgreSQL）
- 端到端：登录 → 查询 → 审计；同步 → 补全；导出同步/异步。
- 边界用例（取自 [14](./14-edge-cases.md)）：删除级联、ETag 并发、断连恢复、限流。

### 7.3 安全测试（越权矩阵）
- **越权矩阵**：每个角色 × 每个关键操作的预期放行/拒绝（含跨项目 `PROJECT_ISOLATION`）。
- **SQL 注入**：参数化、语句边界。
- **令牌**：过期/吊销/重放/越权 scope。

### 7.4 性能测试
- 补全 P95 ≤ 200ms；查询首屏 P95 ≤ 2s；异步导出百万行吞吐；审计写入不阻塞主流程（D30 验证）。

### 7.5 审计完整性测试
- 所有关键操作（[06 §3](./06-audit-log.md) 清单）均产生审计；绕过路径为 0；审计敏感字段脱敏正确；字面量留存（D5）。

### 7.6 关键用例清单（摘）
| 用例 | 预期 |
|---|---|
| 跨项目查库 | `PROJECT_ISOLATION` 拒绝 |
| 只读连接执行 DDL | `NON_READONLY_STATEMENT` |
| 同步导出 >1万行 | `EXPORT_TOO_LARGE_FOR_SYNC` |
| 并发改 worksheet | `CONCURRENT_MODIFICATION` |
| 审计 DB 抖动时查询 | fail-open，查询不受影响 + 告警 |

---

## 8. 上线检查清单（Go-Live Checklist）

- [ ] 平台 PG **定时备份**已开（pg_dump + 异地）
- [ ] `MASTER_KEY` / `JWT_SECRET` 经编排/K8s Secret 安全注入，未入镜像
- [ ] TLS 全链路（入站 + 业务库 + IdP）
- [ ] 首启管理员账号已改密、`BOOTSTRAP_*` 已失效
- [ ] 限流参数按容量配好
- [ ] 审计 stdout/SIEM 已对接（或 `AUDIT_STDOUT_MIRROR` 已评估）
- [ ] 告警（审计写入失败、同步失败、连接失败）已接
- [ ] 环境策略（行数/导出护栏）已配
- [ ] 至少一个 OIDC/LDAP IdP 已配并验证
- [ ] 安全测试（越权矩阵）已通过
- [ ] 容量/性能压测达标

---

## 9. 交付度收尾说明

至此，PRD 已覆盖：产品/架构/权限/查询/导出/认证/审计/资源/数据模型/收藏分享/决策/API 契约/时序/边界/UI/部署运维。**前后端 + DBA + 安全 + 运维 + QA 均具备零沟通开工所需信息**；剩余仅视觉设计稿（[15](./15-ui.md) 已给结构线框）与压测校准（容量假设见 §4）。

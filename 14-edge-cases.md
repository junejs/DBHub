# 14 — 边界条件与异常处理矩阵

> 后端开发必读。把"踩坑点"提前拍板:删除级联、并发、分页、断连恢复、会话边界、输入/配额、审计容错。每条给出**默认行为**与**错误码**;标 ⚠️ 的为建议默认值,你可覆盖。

---

## A. 删除与级联

原则:**统一软删除**(`deleted_at`);删除主资源时,依赖资源按"级联软删 / 保留为孤儿(隐藏) / 阻塞"三选一。

| 删除对象 | 依赖资源 | 默认行为 | 说明 |
|---|---|---|---|
| **Instance** | 其 databases、data_sources | **级联软删**(连同库) | 库无实例无意义;操作需二次确认 + 审计 |
| | 进行中的 sync | 取消该实例的同步任务 | |
| | 引用该库的 worksheet/历史/导出 | **保留为孤儿**(UI 隐藏,不报错) | 历史数据不丢;库恢复后自动重现 |
| **Database** | database_schemas / sync_history | **级联软删/清理** | 元数据随库走 |
| | 引用的 worksheet/历史/导出 | **保留为孤儿** | 同上 |
| **User** | (默认)**禁用**(`status=disabled`)而非硬删 | 保留 user 行,便于审计回溯 | 离职标准动作 |
| | role_assignments、group_members | 移除 | |
| | refresh_tokens / access_tokens | 全部吊销 | 立即踢下线 |
| | 其创建的 worksheet | **保留**,creator 标记为已禁用用户 | 不转让,除非手动 |
| | favorites / notifications / query_history | 保留(本人已不可见) | |
| **Group** | group_members | 级联删成员关系 | |
| | role_assignments 中 `group:x@` | 保留绑定但永不命中(组成员为空)(D31) | 后续可做同步清理 |
| **Worksheet** | favorites | **级联删** | 收藏指向的对象没了 |
| **Project** | instances/databases/worksheets/export_tasks | **非空阻塞**:需先清理或显式 `force=true` 级联(D27) | 防误删整团队资源 |
| **Environment** | 被引用的 instances/databases | **阻塞**:仍有资源引用时不允许删/停用 | 先迁移标注 |
| **Undelete** | — | 软删资源可恢复;唯一名冲突时返回 `RESOURCE_ALREADY_EXISTS` | |

---

## B. 并发与一致性

| 场景 | 默认行为 | 错误码 |
|---|---|---|
| 多人同时编辑同一 worksheet | ETag 乐观锁;后写者失败 | `ABORTED CONCURRENT_MODIFICATION` |
| 并发 SetIamPolicy | IamPolicy 带 etag;冲突失败 | `ABORTED CONCURRENT_MODIFICATION` |
| 同库并发 schema 同步 | **advisory lock**(PG `pg_advisory_xact_lock`),第二个直接跳过本次 | 不报错,日志记录 |
| 重复提交导出/创建任务 | `request_id` 幂等,返回首次结果 | — |
| 权限缓存失效时机 | 改 IAM 绑定后**同步失效**该 scope 缓存;其余 ≤1s 最终一致 | — |
| 引擎元数据(库列表)变更 与 缓存 | 懒刷新:发现过期后台异步刷新,当前用旧值 | — |

---

## C. SQL 与脱敏/谓词列 —— 不在 v1 范围

> v1 不做应用层脱敏与谓词列保护（路线图见 [18 §1.1](./18-roadmap.md)；另见 [02 §4](./02-permission-and-access.md)）。敏感列可见性由数据库授权（GRANT）决定，平台不解析 SQL 做列级改写，因此**不存在**谓词列推断问题——能查到就可见，查不到（未授权）就不可见。

---

## D. 结果集与分页

| 场景 | 默认行为 |
|---|---|
| 分页协议 | **keyset 游标**(非 offset),游标不透明、不可构造;保证翻页稳定 |
| 行数上限 | 取 `environment_policies.query_row_limit`;超限**截断**并标注"已截断,请导出" |
| 结果字节上限 | 单结果集字节上限(防 OOM);超限**截断**或引导异步 |
| 大结果传输 | **流式**(gRPC server-streaming `QueryResult`),前端边收边渲染 |
| 客户端慢/背压 | 服务端限缓冲;超时取消 |
| 二进制/JSON 列 | 按类型渲染;超大单值截断 |
| 空结果 / 仅元数据 | 正常返回,rows 为空 |

---

## E. 任务与断连恢复

| 场景 | 默认行为 |
|---|---|
| 导出任务跑到一半业务库断连 | `state=FAILED` + error;**不自动重试**(数据可能部分写入);用户手动重试 |
| 导出流式写入中途进程崩 | 产物不完整;清理器按 `expires_at` 回收;任务标 FAILED |
| schema 同步失败 | `sync_status=FAILED` + `sync_error`;退避重试(如 1/5/15min);查询用旧缓存 |
| 长查询客户端断开 | 服务端 `context` 取消→取消 DB 查询;**审计仍写**(非取消 context) |
| 元数据同步与查询并发 | 查询读缓存版本号;同步完成才切换缓存(原子替换) |
| 连接池耗尽 | 排队 + 超时 → `UNAVAILABLE DB_CONNECTION_FAILED` |

---

## F. 会话与令牌边界

| 场景 | 默认行为 | 错误码 |
|---|---|---|
| access token 过期,refresh 有效 | 前端自动 `Refresh` → 续期 | `AUTH_TOKEN_EXPIRED`(触发刷新) |
| refresh 过期/被吊销 | 重定向登录 | `UNAUTHENTICATED` |
| 改密 / 重置密码 | 吊销该用户所有 refresh token | — |
| 登出 | 删 refresh + 清 Cookie | — |
| MFA temp token 过期(>5min) | 要求重新登录第一步 | `AUTH_MFA_INVALID` |
| 登录失败次数超限 | 锁定(密码 10/10min,MFA 5/5min) | `AUTH_ACCOUNT_LOCKED` |
| 查询/导出触发限流 | 拒绝 + `Retry-After` | `RESOURCE_EXHAUSTED RATE_LIMITED` |

---

## G. 数据/输入边界

| 场景 | 默认行为 |
|---|---|
| 空语句 / 仅空白 | `INVALID_ARGUMENT` |
| SQL 文本过大(如 >1MB) | `INVALID_ARGUMENT`(限值可配) |
| 多语句数量超限 | `INVALID_ARGUMENT`(默认上限如 50 条) |
| 引擎不支持多语句 | `MULTI_STATEMENT_NOT_SUPPORTED` |
| 时区 | **存 UTC**,展示按用户/组织时区 |
| 标识符大小写 | 按引擎规则(PG 折叠小写、MySQL 看 `lower_case_table_names`);catalog 匹配做归一化 |
| 资源名非法字符 | `INVALID_ARGUMENT` |
| 配额(每项目实例数、每用户 worksheet 数等) | 超限 `RESOURCE_EXHAUSTED`;配额值入 `settings` 可配 |

---

## H. 审计与容错

| 场景 | 默认行为 |
|---|---|
| 审计写入失败(平台 DB 抖动) | **fail-open**:不阻塞用户请求 + stdout 镜像(尽力)+ 告警;后台补写重试(D30) |
| 审计含 PII 字面量 | 按 D5 原样留存;读权限收紧(D26) |
| 审计按月分区 | 分区不存在时自动建(或 pg_partman);老分区按保留期归档/清理 |
| 审计读权限越界 | 服务端强制按 scope 过滤(projectOwner 只能读本项目) | `PERMISSION_DENIED` |

---

## 已确认的边界决策

1. **删项目**:非空阻塞;需显式 `force=true` 才级联软删(D27)。✅
2. **删用户**:禁用(soft),worksheet 保留为孤儿(creator 标记已禁用)(D28)。✅
3. **审计写入失败**:fail-open(不阻塞用户)+ stdout 镜像 + 告警 + 后台补写重试(D30)。✅
4. **删 group 后 `group:x@` 历史绑定**:保留(永不命中);如需可后续做同步清理(D31)。✅

其余均为无明显歧义的工程默认。

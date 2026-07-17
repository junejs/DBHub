# 集中式数据库管理系统 PRD（产品需求文档）

本目录是「集中式数据库管理系统」（下文简称 **DBHUB**）的产品需求文档集。系统定位为一个 **企业内部的统一数据库访问与数据查询/导出平台**，把分散在多种数据库引擎中的数据资产收敛到一个受控、可审计、可细粒度授权的工作台。

---

## 1. 一句话定位

> **让组织里的每一个人，在统一的工作台中安全地查询和导出任意数据库的数据——所有访问都被精确授权、全程可审计。**

---

## 2. 文档导航

| 文档 | 主题 | 说明 |
|---|---|---|
| [00-overview.md](./00-overview.md) | 产品总览 | 愿景、背景、目标用户、核心使用场景、范围与边界、术语表 |
| [01-architecture.md](./01-architecture.md) | 系统架构 | 总体架构、技术选型、模块划分（PostgreSQL 原生） |
| [02-permission-and-access.md](./02-permission-and-access.md) | 权限与访问控制 | RBAC 角色体系、项目隔离、库/表级结构化授权、环境护栏 |
| [03-sql-query.md](./03-sql-query.md) | SQL 查询工作台 | 查询执行、SQL 自动补全（LSP）、结果展示、查询历史、行数/超时控制 |
| [04-data-export.md](./04-data-export.md) | 数据导出 | 导出任务模型、格式、异步生命周期、权限与审计约束、归档与保留 |
| [05-auth-idp.md](./05-auth-idp.md) | 认证与身份集成 | 本地账号、LDAP、OIDC、SSO、用户/组映射、JIT、会话、MFA |
| [06-audit-log.md](./06-audit-log.md) | 审计日志 | 审计数据模型、被审计事件清单、不可变性、查询导出、保留策略 |
| [07-resource-management.md](./07-resource-management.md) | 资源管理 | instance/database/schema/table/view/column 资源模型、元数据发现与同步 |
| [08-nfr.md](./08-nfr.md) | 非功能性需求、风险与验收 | 性能/可用性/安全/合规、风险对策、验收标准 |
| [09-sql-favorite-share.md](./09-sql-favorite-share.md) | SQL 收藏与分享 | 个人收藏（星标）、基于 Worksheet 可见性的安全分享 |
| [10-data-model.md](./10-data-model.md) | 数据模型 | 平台元数据库表结构（简洁可扩展，PostgreSQL DDL） |
| [11-decisions.md](./11-decisions.md) | 决策记录 | 所有关键产品/架构决策及理由（交付团队必读） |
| [12-api-contract.md](./12-api-contract.md) | API 契约 | 资源命名、通用约定、错误码目录、各服务 proto（前后端并行依据） |
| [13-sequences.md](./13-sequences.md) | 核心时序图 | 登录、查询执行、异步导出三条端到端流程（Mermaid；JIT 延后 v2） |
| [14-edge-cases.md](./14-edge-cases.md) | 边界条件与异常处理 | 删除级联、并发、分页、断连恢复、会话边界、审计容错 |
| [15-ui.md](./15-ui.md) | UI 页面清单与线框 | 页面清单、核心页 ASCII 线框、交互状态约定（前端依据） |
| [16-ops.md](./16-ops.md) | 部署/安全/容量/测试 | 配置项、安全参数数值、容量假设、可观测、备份、测试策略、上线清单 |
| [17-bytebase-tech-stack.md](./17-bytebase-tech-stack.md) | Bytebase 技术选型（参考） | Bytebase 实际技术栈清单 + 对我们系统的借鉴取舍 |
| [18-roadmap.md](./18-roadmap.md) | 路线图（v2 及未来） | 不在 v1 范围的能力统一说明：v2 方向、未来延伸、永久边界 |

---

## 3. 产品范围（摘要）

本 PRD 描述一个**完整的集中式数据库查询与导出平台**，不分阶段交付，整体作为一个产品：

- 集中式 SQL 查询工作台（含自动补全）+ 数据导出
- 完善的权限体系（RBAC + 项目隔离 + 库/表级结构化授权 + 环境护栏）
- 审计日志（全量关键操作、不可篡改）
- LDAP/OIDC 身份集成 + MFA
- SQL 收藏与分享

**关于数据库引擎支持：**
> 本系统**当前版本仅实现 PostgreSQL 一种引擎**，核心代码按 PostgreSQL 原生实现，不预设多引擎插件抽象。未来若确需接入其他引擎（MySQL/Oracle 等），届时再抽取驱动/解析器抽象——不为尚不存在的第二个引擎提前支付抽象成本。

**不在 v1 范围、计划 v2 或未来引入的能力**（列级脱敏/谓词列保护、JIT、成本护栏、多引擎、Service Account、XLSX/SQL 导出、分享增强、自定义角色等）统一见 [18-roadmap.md](./18-roadmap.md)。v1 期间敏感数据保护依赖组织现有数据库授权（GRANT）+ 全量审计。

**明确不做（产品边界）：**
- 数据库变更管理（schema 变更审批、迁移、版本管理、SQL Review、回滚）。
- 行级权限（Row-Level Security）。其 SQL 改写注入正确性风险高、安全代价大，未来若确有需求，优先以数据库原生 RLS（PostgreSQL Row Security Policy / Oracle VPD）形式引入。

---

## 4. 参考来源

- 业界成熟开源产品 **Bytebase** 的代码与官方文档作为架构与功能的重要参考：
  - 代码目录：`/Users/zhangjun/Documents/code/opensource/bytebase`
  - 官方文档：https://docs.bytebase.com/introduction/what-is-bytebase
- 本 PRD 借鉴其成熟设计（RBAC + IAM 绑定、LSP 自动补全、注解驱动审计拦截器），并按"实用优先、不过度设计"的原则做了大幅取舍（见上节与 [11-决策记录](./11-decisions.md)）：不预设多引擎插件、不引入 CEL 作为对外策略语言、v1 不做应用层脱敏。

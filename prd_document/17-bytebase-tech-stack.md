# 17 — Bytebase 技术选型清单（参考）

> 本文档记录 Bytebase 开源项目实际使用的技术栈（取自其 `go.mod` / `frontend/package.json`），供我们做技术选型时参考。**Bytebase 是参考对象，不是抄写对象**——我们按自己的规模与诉求取舍（见末节）。
禁止修改次文档，READ ONLY!!
---

## 1. 总览

| 维度 | Bytebase 选型 |
|---|---|
| 后端语言 | **Go 1.26**（单体优先 modular monolith） |
| API 协议 | **gRPC + Protobuf**，经 **Connect-RPC** 暴露 HTTP/JSON |
| 前端框架 | **React 19**（已从 Vue 迁移完成） |
| 平台元数据库 | **PostgreSQL**（JSONB 存半结构化策略） |
| SQL 编辑器 | **Monaco + LSP over WebSocket** |
| 部署 | Docker / Helm Chart |

---

## 2. 后端（Go）

| 关注点 | 选型 | 说明 |
|---|---|---|
| RPC 框架 | **Connect-RPC**（`connectrpc.com/connect`）+ gRPC + grpc-gateway | Connect 让同一套 proto 同时出 gRPC 与 HTTP/JSON |
| Protobuf 工具 | **buf** + `bufbuild/protovalidate` | proto 校验/代码生成/字段校验注解 |
| HTTP 服务 | **labstack/echo v5** | 部分 HTTP 路由（`echo_routes.go`） |
| WebSocket | **gorilla/websocket** + `tmc/grpc-websocket-proxy` | LSP / 流式查询 |
| CEL 求值 | **google/cel-go** | IAM 条件、脱敏规则、审计过滤 |
| Postgres 驱动 | **jackc/pgx v5**（Bytebase 自有 fork） | 平台元数据库 + PG 引擎 |
| MySQL 驱动 | **go-sql-driver/mysql** | MySQL/MariaDB 引擎 |
| 认证/IDP | **coreos/go-oidc**、**go-ldap/ldap/v3**、**golang-jwt/v5**、**golang.org/x/oauth2** | OIDC/LDAP/OAuth2、JWT 签发 |
| 密钥管理 | **AWS SDK v2**（SecretsManager/RDS IAM/DynamoDB）、**Azure KeyVault SDK**、**HashiCorp Vault API** | 外部 Secret Manager 集成 |
| 缓存 | **redis/go-redis v9** | 元数据/会话缓存 |
| 云连接 | **cloud.google.com/go/cloudsqlconn** | GCP Cloud SQL IAM 认证 |
| 导出 | **alexmullins/zip**（密码 ZIP）、**xuri/excelize**（XLSX） | 数据导出产物 |
| CLI | **spf13/cobra** | 命令行入口 |
| 测试 | **stretchr/testify** | 单元/集成测试 |

> 后端是典型的 Go modular monolith：`backend/api`（服务层）、`backend/store`（数据层）、`backend/plugin/{db,idp,parser}`（插件层）、`backend/runner`（后台任务）、`backend/component`（IAM/脱敏/dbfactory 等组件）。

---

## 3. 前端（React 19）

| 关注点 | 选型 | 说明 |
|---|---|---|
| 框架 | **React 19** + **react-router-dom v7** | Vue→React 迁移已完成 |
| 类型/构建 | **TypeScript 7** + **Vite 8** | dev/build |
| RPC 客户端 | **@connectrpc/connect** + **connect-web** + **@bufbuild/protobuf** | 由 proto 生成强类型客户端 |
| 编辑器 | **@codingame/monaco-vscode-api** + **monaco-editor** + SQL 扩展 | Monaco + 真实 LSP |
| 样式 | **Tailwind CSS v4**（@tailwindcss/vite）+ **@stylexjs/stylex** + **base-ui/react** + class-variance-authority | 双样式方案（迁移中） |
| 虚拟化/树/拖拽 | **@tanstack/react-virtual**、**react-arborist**（资源树）、**@dnd-kit**、**react-resizable-panels** | 大表/树/分栏 |
| 执行计划可视化 | **pev2** + **html-query-plan** | EXPLAIN 图形化 |
| i18n | **i18next** + **react-i18next** | 多语言 |
| 其他 | **immer**、**rxjs**、**dayjs**、**sql-formatter**、**lodash-es**、**elkjs**（图）、**lucide-react**（图标）、**pouchdb** | 状态/工具/离线 |
| 测试/Lint | **Vitest 4** + **@testing-library** + **Playwright**（e2e）+ **Biome**（lint/format） | |
| 包管理 | **pnpm** | |

---

## 4. 平台与工具链

| 维度 | 选型 |
|---|---|
| 平台元数据库 | **PostgreSQL**（审计按月分区、JSONB 表达式索引） |
| Proto 代码生成 | **buf**（Go server/client + 前端 TS + OpenAPI） |
| 容器/编排 | **Docker** + **Helm Chart**（`helm-charts/`） |
| CI | GitHub Actions（`.github/`） |
| Lint（Go） | golangci-lint（`.golangci.yaml`） |

---

## 5. 对我们系统的借鉴（取舍）

我们（DBHUB）的技术选型见 [01 §4](./01-architecture.md)。与 Bytebase 的对齐与差异：

| 维度 | Bytebase | 我们 | 理由 |
|---|---|---|---|
| 后端 Go + Connect-RPC + proto | ✅ | ✅ **沿用** | 注解驱动权限/审计，前后端类型共享，已验证 |
| buf + protovalidate | ✅ | ✅ **沿用** | proto 单一来源 + 字段校验 |
| cel-go | ✅ | ❌ **不引入** | 数据级条件改结构化字段（环境/库/表），不暴露表达式语言 |
| Monaco + LSP over WS | ✅ | ✅ **沿用** | 高质量 SQL 补全 |
| 前端 React + Vite + TS + Tailwind | ✅ | ✅ **沿用** | 现代主流栈 |
| 平台元数据库 PostgreSQL | ✅ | ✅ **沿用** | JSONB + 分区 + 表达式索引 |
| 元数据缓存 Redis | ✅ | ⚠️ **MVP 先进程内 LRU** | 单节点规模够用；不引入 Redis 依赖（D 简洁） |
| 外部 Secret Manager（Vault/AWS/Azure） | ✅ | ⚠️ **MVP 应用层 AES-256-GCM + 可选 secret_ref**（D25） | 默认能跑，不强制外部依赖 |
| 多引擎 PGX/MySQL 等驱动 | ✅ 多引擎 | ⚠️ **当前仅 PG（pgx）**（D4） | 聚焦；v1 不预设插件抽象，未来按需抽取 |
| 单体 monolith | ✅ | ✅ **沿用** | 规模无需微服务 |
| pgx 用 Bytebase fork | ✅ | ❌ 用官方 pgx | 我们无历史包袱，不背 fork |
| 前端双样式（Stylex + Tailwind）迁移中 | ✅ | ❌ **只选 Tailwind 一套** | 新项目不背双方案 |

**一句话：后端协议/补全/平台 DB 这些"骨架"沿用 Bytebase 验证过的选型；权限条件(改结构化)、脱敏(v1 不做)、多引擎/缓存/密钥这些"扩展面"按 MVP 简化;凡涉及历史包袱(fork、双样式)一律不背。**

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

**DBHUB** — a centralized database query & export platform. Single-organization, self-hosted (decision **D1**: no multi-tenancy). The platform metadata DB (users / IAM / audit / tasks, PostgreSQL) is physically separate from the business databases users query.

The PRD, CODING_STANDARDS, and GLOSSARY are written in **Chinese**; code/comments are bilingual (English doc-comments are the norm in the Go/TS layers). When the docs conflict, precedence is: **PRD (`prd_document/`) > [CODING_STANDARDS.md](prd_document/CODING_STANDARDS.md) > [GLOSSARY.md](prd_document/GLOSSARY.md)**. For naming, GLOSSARY is authoritative.

## Authoritative docs (read before non-trivial work)

- [CODING_STANDARDS.md](prd_document/CODING_STANDARDS.md) — hard rules for how to write code (layering, error handling, security floor, test strategy). The single most important file.
- [GLOSSARY.md](prd_document/GLOSSARY.md) — ubiquitous language. Code/DB/API naming must use these exact terms (e.g. `Worksheet` not `SavedQuery`; `Instance`/`Database`/`DataSource` are distinct; `tenant`/`workspace`-as-entity are **banned**).
- `prd_document/` — feature & architecture spec (full catalog below). Decision IDs like D1/D38/D50 are cited everywhere; `11-decisions.md` is the index.

## PRD 文档清单 (`prd_document/`)

完整产品需求文档集,定位为「企业内部统一数据库访问与查询/导出平台」。改功能前先定位到对应文档。索引见 [`prd_document/README.md`](prd_document/README.md)。

| 文档 | 路径 | 内容用途 |
|---|---|---|
| 产品总览 | [`prd_document/00-overview.md`](prd_document/00-overview.md) | 愿景、背景、目标用户、核心使用场景、范围与边界、术语表 |
| 系统架构 | [`prd_document/01-architecture.md`](prd_document/01-architecture.md) | 总体架构、技术选型、模块划分(PostgreSQL 原生) |
| 权限与访问控制 | [`prd_document/02-permission-and-access.md`](prd_document/02-permission-and-access.md) | RBAC 角色体系、项目隔离、库/表级结构化授权、环境护栏 |
| SQL 查询工作台 | [`prd_document/03-sql-query.md`](prd_document/03-sql-query.md) | 查询执行、SQL 自动补全(LSP)、结果展示、查询历史、行数/超时控制 |
| 数据导出 | [`prd_document/04-data-export.md`](prd_document/04-data-export.md) | 导出任务模型、格式、异步生命周期、权限与审计约束、归档与保留 |
| 认证与身份集成 | [`prd_document/05-auth-idp.md`](prd_document/05-auth-idp.md) | 本地账号、LDAP、OIDC、SSO、用户/组映射、JIT、会话、MFA |
| 审计日志 | [`prd_document/06-audit-log.md`](prd_document/06-audit-log.md) | 审计数据模型、被审计事件清单、不可变性、查询导出、保留策略 |
| 资源管理 | [`prd_document/07-resource-management.md`](prd_document/07-resource-management.md) | instance/database/schema/table/view/column 资源模型、元数据发现与同步 |
| 非功能性/风险/验收 | [`prd_document/08-nfr.md`](prd_document/08-nfr.md) | 性能/可用性/安全/合规、风险对策、验收标准 |
| SQL 收藏与分享 | [`prd_document/09-sql-favorite-share.md`](prd_document/09-sql-favorite-share.md) | 个人收藏(星标)、基于 Worksheet 可见性的安全分享 |
| 数据模型 | [`prd_document/10-data-model.md`](prd_document/10-data-model.md) | 平台元数据库表结构(简洁可扩展,PostgreSQL DDL) |
| 决策记录 | [`prd_document/11-decisions.md`](prd_document/11-decisions.md) | 所有关键产品/架构决策及理由(交付团队必读,D## 编号的来源) |
| API 契约 | [`prd_document/12-api-contract.md`](prd_document/12-api-contract.md) | 资源命名、通用约定、错误码目录、OpenAPI 路径与 JSON Schema(前后端并行依据) |
| 核心时序图 | [`prd_document/13-sequences.md`](prd_document/13-sequences.md) | 登录、查询执行、异步导出三条端到端流程(Mermaid;JIT 延后 v2) |
| 边界条件与异常处理 | [`prd_document/14-edge-cases.md`](prd_document/14-edge-cases.md) | 删除级联、并发、分页、断连恢复、会话边界、审计容错 |
| UI 页面清单与线框 | [`prd_document/15-ui.md`](prd_document/15-ui.md) | 页面清单、核心页 ASCII 线框、交互状态约定(前端依据) |
| 部署/安全/容量/测试 | [`prd_document/16-ops.md`](prd_document/16-ops.md) | 配置项、安全参数数值、容量假设、可观测、备份、测试策略、上线清单 |
| 路线图(v2 及未来) | [`prd_document/18-roadmap.md`](prd_document/18-roadmap.md) | 不在 v1 范围的能力统一说明:v2 方向、未来延伸、永久边界 |
| 技术选型决策 | [`prd_document/19-tech-stack.md`](prd_document/19-tech-stack.md) | v1 完整技术栈、选型理由、风险与取舍 |

> 注:编号无 `17`(原列表即缺)。v1 仅实现 PostgreSQL 一种引擎;列级脱敏/谓词保护、JIT、成本护栏、多引擎、Service Account 等不在 v1 范围,统一见 `18-roadmap.md`。

## Commands

Backend and frontend are **fully independent** (separate modules, deps, scripts) — always `cd` into one first.

### Backend (`cd backend`)

```bash
make gen        # regenerate internal/oas from ../openapi.yaml  (after editing the contract)
make dev        # go run main.go  (PORT env, default 8080)
make test       # go test -race -coverprofile=coverage.out ./...
make lint       # golangci-lint run ./...   (needs golangci-lint >= v1.62.1 for Go 1.25)
make build      # go build -o bin/server main.go
```

Single test / package:

```bash
go test -run TestListProjects ./internal/service/...
go test -race -run TestName -v ./internal/api/
```

Pre-commit: `go fmt ./... && go vet ./... && go test -race ./... && golangci-lint run ./...`

### Frontend (`cd frontend`)

```bash
pnpm install
pnpm dev         # Vite, port 3000, proxies /v1 -> http://localhost:8080
pnpm typecheck   # tsc -b   (build also runs this first)
pnpm lint        # biome check .
pnpm lint:fix    # biome check . --write
pnpm test        # vitest run
pnpm test:watch  # vitest
pnpm build       # tsc -b && vite build
```

Single test: `pnpm test src/api/client.test.ts` or `pnpm test -t "test name"`.

### Local stack

```bash
docker compose up db -d      # PostgreSQL 17 (platform metadata DB)
# terminal 2
cd backend && cp .env.example .env && make dev
# terminal 3
cd frontend && cp .env.example .env && pnpm install && pnpm dev
```

## Architecture (the big picture)

### API-first / contract-driven (D38, D44)

`openapi.yaml` is the **single source of truth** for the API. Each side derives its transport from it differently:

- **Backend**: `ogen` generates a full Go server (router, `Handler` interface, request/response types, validation, security hooks) into `backend/internal/oas/`. This is the standard **"change the contract → `make gen` → implement"** loop. Never edit code first and back-fill the contract.
- **Frontend**: types are **hand-written** in `src/api/types.ts` and synced manually. Reason: TypeScript 7 is too new for `openapi-typescript` / `@hey-api/openapi-ts` (decision **D51**). When `openapi.yaml` changes, update `types.ts` by hand.

Cross-cutting security is **declarative**: each operation carries `x-requires-permission` / `x-audit` / `x-auth-method` / `x-allow-without-credential` extensions, enforced by middleware — business code must not gate access itself.

### Backend layering (strict, one-way, irreversible)

```
main → api → service → (repo/store interfaces)
              oas (generated) is implemented by api, assembled by main
```

- `internal/oas/` — **generated, never hand-edit** (lint-excluded, regen overwrites it). `make gen` only.
- `internal/api/` — thin adapter. `handler.go` embeds `oas.UnimplementedHandler` (returns 501 for every op) and overrides only implemented operations. Its job is decode → call service → encode, plus domain↔contract type mapping (e.g. `toOasProject`). **No business logic here.**
- `internal/service/` — the core. Business/domain logic, pure Go, unit-testable. External deps (DB, clock) are behind interfaces (`ProjectRepo`) injected via constructors, faked in tests (D50).
- `main.go` — the **only** wiring point (repo → service → handler/security → ogen server) and where chi cross-cutting middleware (request-id, recoverer, timeout; **TODO: auth / ACL / audit**) wraps the generated server.

Key rule: `service` must not import `api` or `oas` (domain independent of transport). Currently `MemoryProjectRepo` is a dev/demo stand-in (it ignores `pageToken`) until bun/pgx is wired in.

### Frontend layering

```
components → hooks → api → fetch (client.ts)
```

- `src/api/client.ts` — the only place that calls `fetch`. Handles `/v1` base, `credentials: 'include'` (cookie session), CSRF double-submit token on writes, JSON, and normalizes errors into `ApiRequestError` (exposes `.reason` = the stable business code from `details[].reason`).
- `src/api/types.ts` — contract types (JSON fields are **snake_case**, matching the contract).
- `src/api/<resource>.ts` — one function per endpoint (e.g. `listProjects`).
- `src/hooks/use*.ts` — TanStack Query hooks. Each resource owns a **query-key factory** (e.g. `projectsKeys`) for cache invalidation/prefetch by domain. Server state stays in React Query; client UI state goes in Zustand.
- **Components never call `fetch` directly** and never reimplement server state in local state. Frontend code uses camelCase params; the hook/API layer maps them to the contract's snake_case query params.

Path alias: `@` → `src`. Dev requests to `/v1` are proxied to the backend at `:8080`.

## Critical conventions (easy to get wrong)

- **JSON / query params are snake_case** (`page_size`, `next_page_token`, `create_time`); time is RFC3339 UTC over the wire, `timestamptz` in DB. Frontend code stays camelCase and maps at the API boundary.
- **Pagination**: `page_size` (default 50) + opaque keyset `page_token`; responses are `items[]` + optional `next_page_token`. Large tables may omit `total_size`.
- **Errors**: uniform `Error{code, message, details[]}`; the stable business code lives in `details[].reason` as UPPER_SNAKE (e.g. `QUERY_ROW_LIMIT_EXCEEDED`). Frontend branches on `err.reason`.
- **Soft delete** = `deleted_at` (nullable) + partial unique index. Never a `deleted` boolean.
- **Naming is locked by GLOSSARY** — don't invent synonyms. `Instance` (a registered connection target) ≠ `Database` (a logical DB inside it) ≠ `DataSource` (connection config, role `admin`/`readonly`). `Project` is the isolation unit; there is no `tenant`/`workspace` entity (single-org, D1).
- **DB design**: `bigint identity` PKs, `text` for enums (not PG enum types), `citext` + `lower(email)` partial unique index for emails, AES-256-GCM `bytea` for encrypted credentials. No `tenant_id`/`workspace_id` columns.
- **Generated code is off-limits**: editing `internal/oas/*` by hand is wasted work.
- **Security is not yet implemented**: `internal/api/security.go` is a **passthrough placeholder that accepts any credential**, and the auth/ACL/audit chi middleware is a TODO. The declarative `x-requires-permission`/`x-audit` enforcement layer must exist before any deployment.

## Git

Conventional Commits (`feat:`/`fix:`/`docs:`/`refactor:`/`test:`/`chore:`), first line ≤ 72 chars, body explains *why*. Branch from `main` (`feat/…`, `fix/…`, `prd/…`); don't push directly to `main`. Keep commits atomic (one concern each); generated `internal/oas` changes can be split from hand-written logic to ease review.

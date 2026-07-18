# DBHUB

集中式数据库查询与导出平台。

## 技术栈

- **后端**：Go 1.25 / chi 5.3 / ogen 1.23 / pgx 5.10 / bun 1.2
- **前端**：React 19.2 / Vite 8.1 / Tailwind CSS 4.3 / TanStack Query 5.101 / Zustand 5.0
- **数据库**：PostgreSQL 17
- **部署**：Docker Compose

## 项目结构

```
dbhub/
├── backend/                      # Go 后端 API（独立 Go module，make 管理）
│   ├── internal/
│   │   ├── oas/                  # ogen 从 openapi.yaml 生成（契约 → 代码，已提交）
│   │   ├── api/                  # Handler 适配层 + SecurityHandler（薄壳，调 service）
│   │   └── service/              # 手写业务逻辑（领域层，可单测）
│   ├── main.go                   # chi 横切中间件包住 ogen 服务端
│   └── ogen.yml                  # ogen 代码生成配置
├── frontend/                     # React 前端（独立 pnpm 项目）
│   └── src/
│       ├── api/                  # 手写类型 + 薄类型化 fetch（client.ts / types.ts）
│       └── hooks/                # TanStack Query hook
├── openapi.yaml                  # API 唯一事实来源
├── docker-compose.yml
└── README.md
```

> 后端与前端的构建、依赖、脚本完全独立，互不耦合。

## 契约 → 代码（API-first）

`openapi.yaml` 是唯一事实来源。前后端各自从它派生传输层，业务逻辑手写。

**后端（ogen 生成）**：契约 → Go 服务端（路由 / Handler 接口 / 请求响应类型 / 校验 / 安全钩子）。

```bash
cd backend
make gen          # 从 ../openapi.yaml 重新生成 internal/oas/
```

- 生成的 `Handler` 接口由 `internal/api` 实现（嵌入 `UnimplementedHandler`，按需 override）。
- 业务逻辑在 `internal/service`（领域层，interface + mock 注入，D50）。
- chi 横切中间件（认证 / ACL / 审计，待实现）包在生成的服务端外层。

**前端（手写类型）**：前端不自动生成类型——TypeScript 7 太新，`openapi-typescript` / `@hey-api/openapi-ts` 均不支持（见决策 D51）。改为手写请求/响应类型（`src/api/types.ts`），契约更新时手动同步；fetch 层为 `src/api/client.ts`（Cookie 会话 + CSRF 双提交 + 统一错误）。

## 开发环境

### 依赖

- Go 1.25+
- Node.js 24+
- pnpm 9+
- Docker & Docker Compose

### 启动

```bash
# 启动平台数据库
docker compose up db -d

# 后端（另一个终端）
cd backend
cp .env.example .env
make dev

# 前端（另一个终端）
cd frontend
cp .env.example .env
pnpm install
pnpm dev
```

### 测试

```bash
# 后端测试
cd backend
make test

# 前端测试
cd frontend
pnpm test
```

## 文档

详细设计见 `prd_document/` 目录，关键文档：

- [19-tech-stack.md](prd_document/19-tech-stack.md) — 技术选型决策
- [12-api-contract.md](prd_document/12-api-contract.md) — API 契约
- [10-data-model.md](prd_document/10-data-model.md) — 数据模型
- [16-ops.md](prd_document/16-ops.md) — 部署、安全与测试策略

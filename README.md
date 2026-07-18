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
├── backend/            # Go 后端 API（独立 Go module，make 管理）
├── frontend/           # React 前端（独立 pnpm 项目）
├── openapi.yaml        # API 唯一事实来源
├── docker-compose.yml
└── README.md
```

> 后端与前端的构建、依赖、脚本完全独立，互不耦合。`openapi.yaml` 作为契约由双方各自生成代码。

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

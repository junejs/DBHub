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
├── openapi.yaml                  # API 唯一事实来源（入口，paths/components 为 $ref）
├── openapi/
│   ├── components.yaml           # 组件：parameters + responses + schemas
│   └── paths/                    # 按资源领域拆分的路径定义
│       ├── auth.yaml, project.yaml, instance.yaml … （共 16 个）
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

### OpenAPI 文件拆分

为避免单文件膨胀（原 3080 行），`openapi.yaml` 按资源领域拆分为多文件，`ogen` 跨文件 `$ref` 解析通过 `ogen.yml` 中 `parser.allow_remote: true` 开启。

```
openapi.yaml                     (入口：header + tags + paths $ref + components $ref)
├── openapi/
│   ├── components.yaml          (parameters / responses / schemas 合并一处)
│   └── paths/
│       ├── system.yaml          # 健康检查
│       ├── auth.yaml            # 认证/会话
│       ├── sql.yaml             # SQL 查询/导出
│       ├── export.yaml          # 异步导出任务
│       ├── instance.yaml        # 实例/数据源管理
│       ├── database.yaml        # 数据库/schema
│       ├── iam.yaml             # 权限绑定
│       ├── audit.yaml           # 审计日志
│       ├── project.yaml         # 项目
│       ├── environment.yaml     # 环境/策略
│       ├── user.yaml            # 用户
│       ├── group.yaml           # 用户组
│       ├── idp.yaml             # 身份提供商
│       ├── worksheet.yaml       # 保存的查询/收藏
│       ├── notification.yaml    # 站内通知
│       └── setting.yaml         # 平台设置
```

**对应关系**：每个 `paths/*.yaml` 对应原始文件中 `# ──── <Topic> ────` 注释分隔的一个节，按 `tags` 中的领域名划分。

**编辑原则**：

| 要改什么 | 编辑哪个文件 | 注意事项 |
|---|---|---|
| 已有路径（参数/响应/方法） | `openapi/paths/<topic>.yaml` | 内部 `$ref` 自动指向 `components.yaml` |
| 新增路径 | 对应 topic 文件 + `openapi.yaml` 加一行 `$ref` | 需确保 `openapi.yaml` `paths:` 下不重复 |
| 新增/改 Schema / Parameter / Response | `openapi/components.yaml` 对应节 | 内部 `$ref` 用 `#/components/...`（同文件内） |
| 改安全方案 | `openapi.yaml` `securitySchemes` 节 | 体量小，留在入口文件 |

**添加新路径的步骤**：

1. 在 `openapi/paths/` 对应 topic 文件中写入 `  /v1/...:` 路径块
2. 替换内部 `#/components/...` 引用为 `../components.yaml#/components/...`（若手写）
3. 在 `openapi.yaml` `paths:` 下添加：`$ref: 'openapi/paths/<topic>.yaml#/~1v1~1...'`
   （`~1` 是 JSON Pointer 对 `/` 的转义，如 `/v1/projects` → `~1v1~1projects`）
4. 跑 `cd backend && make gen` 验证

**如需恢复单文件**：`cp openapi.yaml.bak openapi.yaml`（拆分脚本自动备份）。

**拆分脚本**：`_scripts/split_openapi.py`（幂等，从原始 3080 行文件按注释节自动拆分，备复用）。

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

工程规范（项目根目录）：

- [CODING_STANDARDS.md](CODING_STANDARDS.md) — 前后端编码规范
- [GLOSSARY.md](GLOSSARY.md) — 业务术语表（统一语言，命名/DB 设计参考）

详细设计见 `docs/prd/` 目录，关键文档：

- [19-tech-stack.md](docs/prd/19-tech-stack.md) — 技术选型决策
- [12-api-contract.md](docs/prd/12-api-contract.md) — API 契约
- [10-data-model.md](docs/prd/10-data-model.md) — 数据模型
- [16-ops.md](docs/prd/16-ops.md) — 部署、安全与测试策略

# 19 — 技术选型决策

> 本文档记录 DBHUB v1 的完整技术选型及决策理由。所有选择均基于 PRD 约束：前后端分离、monorepo、REST API（OpenAPI 3 / HTTP / JSON）、v1 仅 PostgreSQL、单组织自部署、MVP 实用优先。

---

## 1. 选型总览

| 层级 | 选型 | 版本 / 关键库 |
|---|---|---|
| 后端语言 | **Go** | 1.25+ |
| HTTP 框架 | **chi** | `github.com/go-chi/chi/v5` v5.3.1 |
| API 契约 | **OpenAPI 3** | 手写 `openapi.yaml`，用 **ogen** v1.23.0 生成服务端/客户端 |
| 数据库驱动 | **pgx/v5** | `github.com/jackc/pgx/v5` v5.10.0 |
| ORM / Query Builder | **bun** | `github.com/uptrace/bun` v1.2.18 |
| 任务队列（v1） | **进程内 Runner** | 自研，预留 `Queue` 接口，v2 可切 Redis + asynq |
| 认证/会话 | **JWT + OIDC/LDAP** | `golang-jwt/jwt/v5`、`coreos/go-oidc`、`go-ldap/ldap/v3` |
| JWT 签名算法 | **HS256** | v1 单服务，对称密钥足够；未来多服务再切 RS256 |
| 密码哈希 | **bcrypt** | 本地账号密码存储 |
| ZIP 加密 | **alexmullins/zip** | Go 标准 zip 不支持密码加密 |
| 前端框架 | **React** | 19.2.7 |
| 前端运行时 | **Node.js** | 24+ |
| 前端构建 | **Vite** | 8.1.5 |
| 样式/UI | **Tailwind CSS 4.3 + @tailwindcss/vite** | shadcn/ui 按需复制 |
| 前端状态管理 | **TanStack Query 5.101 + Zustand 5.0** | Query 管服务端状态，Zustand 管本地状态 |
| i18n | **react-i18next 17.0** | 中英双语（D23） |
| SQL 编辑器 | **Monaco Editor** | `@codingame/monaco-vscode-api` + `monaco-editor` |
| LSP 客户端 | **monaco-languageclient** | 连接 Monaco 到后端 LSP over WebSocket |
| LSP 协议 | **WebSocket** | 标准库 `net/http` + `gorilla/websocket` |
| 平台数据库 | **PostgreSQL** | 17+ |
| Monorepo 组织 | **简单目录结构** | 不用 Turborepo/nx |
| 包管理 | **pnpm** | 9+ |
| 部署 | **Docker Compose** | v1 唯一部署方式 |

---

## 2. 后端：Go

### 2.1 决策
**后端语言使用 Go。**

### 2.2 理由
1. **与 PRD 原生 PostgreSQL 路线高度契合**：Go 的 `pgx/v5` 是 PostgreSQL 生态最成熟、性能最好的驱动之一，且社区维护活跃。
2. **部署简单**：Go 可编译为单二进制文件，配合 scratch/alpine 镜像极小，非常符合"单组织自部署"的诉求（运维负担低）。
3. **并发模型适合查询/导出/同步任务**：goroutine + channel 非常适合管理大量数据库连接、流式导出、后台同步任务，而不会因为事件循环或 GIL 限制导致长查询/大导出阻塞。
4. **类型安全与可维护性**：静态类型在权限引擎、审计拦截器、IAM 策略求值等安全关键路径上能减少运行时错误。
5. **Bytebase 已验证**：参考对象 Bytebase 同样使用 Go + PostgreSQL 驱动，证明该组合能支撑类似的数据库管理场景。

### 2.3 放弃的备选
- **Node.js/TypeScript**：前后端同语言、开发速度快，但长连接/大结果集导出时对事件循环和 GC 更敏感，且 TypeORM/Prisma 在复杂 PG 查询上不如 pgx/bun 直接。
- **Java/Spring Boot**：生态成熟但启动慢、镜像大、配置重，与 v1 "简洁、单节点自部署" 的目标不符。
- **Python/FastAPI**：开发极快，但并发与类型系统弱于 Go/TS，不适合高并发查询工作台。

---

## 3. HTTP 框架：chi

### 3.1 决策
**HTTP 路由与中间件使用 chi。**

### 3.2 理由
1. **标准库风格**：chi 基于 `net/http`，中间件写法与标准库一致，学习成本低，迁移风险小。
2. **轻量且成熟**：没有 gin 的魔法路由语法，也没有 echo 的额外抽象，适合长期维护。
3. **中间件链清晰**：认证、ACL、审计、限流等横切层天然以中间件形式插入，与 PRD 的"网关层拦截器"设计一致。

### 3.3 放弃的备选
- **echo/v4**：文档更全、功能更多，但相比 chi 略显厚重；chi 的标准库风格更贴合 Go 习惯。
- **fiber**：基于 fasthttp，性能更高，但 fasthttp 与标准库 `net/http` 不完全兼容，某些中间件需要重写，增加不必要的复杂度。

---

## 4. API 契约：OpenAPI 3 + ogen

### 4.1 决策
**API 以手写 OpenAPI 3 YAML 为唯一事实来源，服务端/客户端代码由 ogen 生成。**

### 4.2 理由
1. **满足"编程语言无关"要求**：OpenAPI 3 是语言中立的契约，任何语言都能生成客户端/服务端，PRD 明确 API 契约以 OpenAPI 3 为唯一事实来源（D38）。
2. **强类型与契约一致性**：ogen 由 OpenAPI 生成 Go 的 handler 接口、请求/响应结构体、路由注册，避免手写时出现路径/字段不一致。
3. **声明式权限/审计**：OpenAPI 的 `x-requires-permission` 和 `x-audit` 扩展可以随着生成代码被横切层读取，确保权限与审计声明无法被业务代码遗漏。
4. **拒绝 gRPC/proto**：PRD 已明确 v1 不走 gRPC/Connect-RPC（D38），因此不引入 proto、buf、Connect-RPC。

### 4.3 放弃的备选
- **手写 handler + kin-openapi 校验**：更灵活，但需要手动维护路径、结构体、校验逻辑，容易与契约脱节。
- **gRPC + grpc-gateway / Connect-RPC**：Bytebase 采用此方案，但 PRD 已决定 v1 使用 HTTP/JSON，故不采纳。

---

## 5. 数据库访问：pgx/v5 + bun

### 5.1 决策
**底层驱动使用 pgx/v5，ORM/查询构建使用 bun。**

### 5.2 理由
1. **pgx/v5 是 Go PG 驱动的最佳实践**：支持 `COPY`、预编译语句、通知监听、`pgtype` 高级类型，性能与功能都优于 `lib/pq`。
2. **bun 轻量且基于 pgx**：bun 不是全功能 ORM，更像类型安全的查询构建器，能直接复用 pgx 连接池，学习曲线平缓，同时保留手写复杂 SQL 的灵活性。
3. **与 sqlc 的权衡**：sqlc 从 SQL 生成类型安全 Go 代码，适合复杂查询；但 bun 在 CRUD、关联查询、迁移友好度上更灵活，对 v1 的快速迭代更友好。
4. **PostgreSQL 原生**：v1 仅 PG，不引入多引擎抽象，因此不需要 GORM 那种跨数据库兼容层。

### 5.3 放弃的备选
- **sqlc**：类型安全、查询清晰，但每改一次查询就要重新生成，且对动态条件（如 IAM 条件过滤）支持不如 bun 灵活。
- **gorm**：功能全但隐式魔法多（钩子、自动迁移、关联预加载），在审计/权限这类需要精确控制 SQL 的场景容易埋坑。
- **手写 pgx + stdlib**：最灵活，但 CRUD 样板代码多，bun 能显著减少重复。

---

## 6. 任务队列：进程内 Runner（预留抽象）

### 6.1 决策
**v1 使用进程内任务调度器，抽象出 `Queue` 接口，v2 可替换为 Redis + asynq。**

### 6.2 理由
1. **MVP 零外部依赖**：v1 功能只有导出、schema 同步、清理，任务量不大，引入 Redis 增加部署复杂度。
2. **单节点部署优先**：PRD 明确 v1 单节点、单组织自部署，进程内队列足够支撑。
3. **预留抽象防止返工**：定义 `Queue` / `Worker` 接口，进程内实现只需一个 goroutine + PostgreSQL 任务状态表；v2 引入 Redis 时替换实现即可，业务代码不感知。
4. **任务状态持久化在 PG**：`export_tasks`、`sync_history` 等表本身就是任务持久化，进程内 Runner 只负责调度和执行，重启后可恢复待执行/失败任务。

### 6.3 放弃的备选
- **Redis + asynq**：功能更强（持久化、重试、延迟任务、分布式），但 v1 不需要分布式，Redis 是额外运维负担。
- **PostgreSQL 任务表 + 轮询**：也可接受，但进程内 channel + 表状态更新更简单，延迟更低。

---

## 7. 认证与身份集成

### 7.1 决策
**本地账号使用 bcrypt；OIDC 使用 `coreos/go-oidc`；LDAP 使用 `go-ldap/ldap/v3`；JWT 使用 `golang-jwt/jwt/v5`；导出 ZIP 密码加密使用 `alexmullins/zip`。**

### 7.2 理由
1. **库选择标准**：选择社区维护活跃、被大量生产环境验证的库。
2. **OIDC 需要 discovery**：`coreos/go-oidc` 原生支持 `.well-known/openid-configuration` 自动发现，减少配置错误。
3. **JWT 签名算法**：v1 使用 **HS256**（单服务、对称密钥足够），未来多服务时再考虑 RS256 + JWK。
4. **密码哈希**：**bcrypt**，Go 标准库支持，抗彩虹表与暴力破解。
5. **ZIP 加密**：Go 标准库 `archive/zip` 不支持传统 ZIP 密码加密；`alexmullins/zip` 提供 AES/ZipCrypto 加密，满足导出产物密码保护需求。
6. **凭据加密**：业务库密码/IdP secret 使用 AES-256-GCM，主密钥由环境变量/KMS 注入，符合 PRD D25。

---

## 8. 前端：React 19 + Vite 8 + Tailwind CSS 4

### 8.1 决策
**前端框架使用 React 19.2，构建工具 Vite 8.1，样式使用 Tailwind CSS 4.3 + @tailwindcss/vite，组件库使用 shadcn/ui。**

### 8.2 理由
1. **React 适合复杂工作台**：SQL 编辑器、资源树、结果表格、导出中心、审计筛选等交互密集场景，React 的组件生态（虚拟化、拖拽、分栏、表格）更成熟。
2. **Vite 8 构建快**：冷启动、HMR、构建速度都显著优于 CRA/Webpack；v8 使用 Lightning CSS，构建性能进一步提升。
3. **Tailwind CSS 4 + @tailwindcss/vite**：Tailwind v4 改用 CSS-first 配置，通过 Vite 插件集成，构建更快、配置更简洁。
4. **状态管理**：**TanStack Query 5.101** 负责服务端状态（缓存、重试、失效），**Zustand 5.0.14** 负责本地 UI 状态，分工清晰。
5. **i18n**：**react-i18next 17.0** 成熟稳定，满足 PRD 中英双语要求（D23）。
6. **Bytebase 已验证**：Bytebase 从 Vue 迁移到 React 19 + Vite + Tailwind，证明该组合能支撑数据库管理这类复杂后台。

### 8.3 放弃的备选
- **Vue 3**：上手快、模板友好，但复杂工作台的状态管理、组件生态、TypeScript 深度集成不如 React 成熟。
- **Angular**：太重，不符合 v1 快速迭代诉求。
- **Redux**：相比 Zustand 样板代码多，v1 状态复杂度不需要。

---

## 9. SQL 编辑器与自动补全

### 9.1 决策
**编辑器使用 Monaco Editor，通过 WebSocket 连接后端 LSP 服务实现 SQL 自动补全。**

### 9.2 理由
1. **Monaco 是行业标准**：VS Code 同款编辑器，支持自定义语言、主题、LSP、错误高亮，用户熟悉度高。
2. **LSP over WebSocket**：PRD 明确 LSP 通过 WebSocket 暴露（[12 §6.15](./12-api-contract.md)），与 REST API 分离，避免把 WebSocket 塞进 OpenAPI。
3. **后端解析器**：PostgreSQL 原生解析器用于多语句切分、语法诊断、schema 反射；自动补全由 LSP 服务驱动。

---

## 10. 平台数据库：PostgreSQL 17+

### 10.1 决策
**平台自身元数据库使用 PostgreSQL 17+。**

### 10.2 理由
1. **PRD 已定**：v1 仅支持 PostgreSQL 引擎，平台元数据库也使用 PG，统一技术栈。
2. **JSONB 存储半结构化策略**：`environment_policies.settings`、`role_assignments.condition`、`identity_providers.config` 等字段用 JSONB，灵活且可建 GIN/表达式索引。
3. **审计按月分区**：PostgreSQL 原生表分区 + `pg_partman` 支持审计日志按月归档。
4. **开发/测试/生产版本一致**：避免版本差异导致 JSONB、分区行为不一致。

---

## 11. Monorepo 组织：简单目录结构，不用 Turborepo/nx

### 11.1 决策
**采用简单目录结构实现 monorepo，不引入 Turborepo 或 nx。**

### 11.2 理由
1. **MVP 规模小**：v1 只有 web、server 两个应用和少量共享包，Turborepo/nx 的缓存、任务编排收益不明显。
2. **减少工具链复杂度**：pnpm workspaces 已足够管理依赖；构建脚本用根目录 `package.json` 的 scripts 串联即可。
3. **保留扩展性**：未来应用/包增多时，再引入 Turborepo 只需改 `turbo.json`，目录结构无需大动。

### 11.3 目录结构
```
dbhub/
├── apps/
│   ├── web/              # React 前端
│   └── server/           # Go 后端
├── packages/
│   ├── api/              # OpenAPI 契约 + ogen 生成代码
│   └── shared/           # 前后端共享常量/类型（未来）
├── openapi.yaml          # API 唯一事实来源
├── docker-compose.yml
└── README.md
```

### 11.4 放弃的备选
- **Turborepo**：构建缓存、pipeline 编排强大，但 v1 应用少，收益不足以抵消复杂度。
- **nx**：功能更重，适合大型企业 monorepo，v1 不需要。

---

## 12. 部署：Docker Compose

### 12.1 决策
**v1 唯一部署方式使用 Docker Compose。**

### 12.2 理由
1. **单组织自部署友好**：Docker Compose 是大多数团队最容易上手的自部署方式，一条命令启动平台 PG + 后端 + 前端。
2. **避免 Kubernetes 复杂度**：v1 单节点，不需要 K8s 的编排、服务发现、配置管理。
3. **可扩展到 Helm**：未来多副本/高可用时，再把 Compose 配置迁移到 Helm Chart，v1 不提前支付成本。

### 12.3 镜像选择
| 服务 | 基础镜像 | 理由 |
|---|---|---|
| Go 后端 | **distroless** 或 **alpine** | 极小攻击面；distroless 更轻，alpine 调试用 shell 更方便 |
| 前端 | **node:24-alpine**（构建阶段）+ **nginx:alpine** 或 **distroless**（运行阶段） | 构建用 Node，运行用静态服务器 |
| 平台数据库 | **postgres:17-alpine** | 与开发/生产版本一致 |

---

## 13. 开发工具链

| 工具 | 选型 | 理由 |
|---|---|---|
| 包管理 | **pnpm** | monorepo 依赖去重、安装快、磁盘占用小 |
| Go 迁移 | **golang-migrate** / **goose** | 标准迁移工具，支持版本化 SQL 迁移 |
| Go Lint | **golangci-lint** | 行业标准，规则丰富 |
| 前端 Lint/Format | **Biome** | 速度快，统一 lint + format |
| 测试（后端） | **testify** + **gomock** / **mockery** + **testcontainers-go** | testify 标准断言；gomock/mockery 生成接口 mock；testcontainers 跑真 PG 集成测试 |
| 测试（前端） | **Vitest** + **@testing-library/react** + **MSW** + **Playwright** | Vitest 单元/组件测试；MSW 模拟 API；Playwright e2e |
| 测试原则 | **单元测试优先**；关键路径覆盖率 ≥ 80%；权限/审计/导出必须单测覆盖 | 见 [16 §7](./16-ops.md) |
| 日志 | **slog**（Go 标准库） | 结构化日志，无需第三方依赖 |
| 配置 | **koanf** | 轻量，支持 env/file/flag 多层配置 |

### 13.1 单元测试策略（重点）

> 单元测试是 v1 质量保障的第一道防线。所有业务逻辑必须可在不依赖外部服务（数据库、IdP、文件系统）的情况下进行单元测试。

**后端单元测试原则：**

1. **接口驱动设计**：所有依赖外部资源（DB、IdP、存储、缓存）的组件通过 Go interface 注入，测试时使用 mock 实现。
2. **表驱动测试**：对权限判定、SQL 多语句切分、环境策略解析、导出格式转换等组合爆炸场景，使用 testify + 表驱动覆盖正例/反例/边界。
3. **必测模块**：
   - IAM 引擎（角色解析、权限集合、结构化条件匹配）
   - 审计拦截器（事件生成、敏感字段脱敏、不可取消 context）
   - SQL parser（多语句切分、只读判断、语法诊断）
   - 导出格式化器（CSV/JSON 转义、空值、二进制、大字段截断）
   - 环境策略解析（行数/字节/并发/导出上限）
   - 登录锁定逻辑（窗口、阈值、解锁）
4. **Mock 工具**：`gomock` 或 `mockery` 自动生成接口 mock；避免手写大量 mock 样板。
5. **覆盖率门槛**：
   - 核心业务逻辑（IAM、审计、导出、查询、同步）≥ **80%**
   - 工具类/胶水代码 ≥ **60%**
   - 不追求 100%，但关键路径必须覆盖。

**前端单元测试原则：**

1. **组件级测试**：使用 `@testing-library/react` + Vitest 测试表单校验、权限按钮显隐、表格渲染、错误状态。
2. **状态逻辑测试**：Zustand store、TanStack Query hook 封装单独测试。
3. **API 层测试**：MSW（Mock Service Worker）拦截 HTTP/WebSocket，测试请求/响应处理、错误码映射。
4. **不测试第三方库**：Monaco、图表库等不做单元测试，依赖 e2e/手动验证。

**测试组织：**
- 单测文件与源码同目录，命名 `*_test.go` / `*.test.tsx`。
- CI 中 `go test ./...` 与 `vitest run` 必须全部通过，覆盖率未达标阻塞合并。

---

## 14. 与 Bytebase 的对齐与取舍

| 维度 | Bytebase | 本系统（v1） | 理由 |
|---|---|---|---|
| 后端语言 | Go | **Go** | 对齐，已验证 |
| API 协议 | gRPC + Connect-RPC | **OpenAPI 3 + HTTP/JSON** | PRD 明确要求语言无关（D38） |
| 前端框架 | React 19 | **React 19** | 对齐 |
| 平台数据库 | PostgreSQL | **PostgreSQL** | 对齐 |
| ORM | GORM 等 | **bun** | 更轻量、类型安全 |
| 任务队列 | 自建 | **进程内 Runner** | v1 零外部依赖 |
| 缓存 | Redis | **进程内 LRU** | v1 单节点够用 |
| 多引擎 | PG/MySQL/Oracle 等 | **仅 PG** | PRD D4 |
| CEL | cel-go | **不引入** | 结构化条件替代 CEL（D34） |

---

## 15. 关键风险与应对

| 风险 | 应对 |
|---|---|
| ogen 生成的代码过于僵化 | 手写 `openapi.yaml` 时保持精简，复杂查询用通用 schema；必要时 fallback 到 kin-openapi 做补充校验 |
| bun 生态不如 GORM 丰富 | v1 需求简单（CRUD + 中等复杂查询），bun 足够；复杂 SQL 可手写 |
| 进程内 Runner 重启丢任务 | 任务状态持久化在 `export_tasks` 等表，重启后恢复待执行/失败任务；v2 切 Redis |
| 前端 shadcn/ui 组件复制带来维护成本 | 只复制必要组件，避免过度碎片化 |

---

## 16. 决策记录索引

| 决策 | 文件/位置 | 说明 |
|---|---|---|
| API 以 OpenAPI 3 为唯一事实来源 | [11-decisions.md](./11-decisions.md) D38 | 语言/框架无关 |
| 查询结果用 unary + keyset 分页 | [11-decisions.md](./11-decisions.md) D39 | 与 HTTP/JSON 主契约一致 |
| 导出 ZIP 密码用户自设 | [11-decisions.md](./11-decisions.md) D40 | 平台不存储密码 |
| v1 仅 PostgreSQL，不预设多引擎 | [11-decisions.md](./11-decisions.md) D4 | 聚焦与简化 |
| 数据级条件用结构化字段 | [11-decisions.md](./11-decisions.md) D34 | 不引入 CEL |
| 技术选型最终方案 | 本文档 | 供实现阶段遵循 |

---

> 本文档已锁定 v1 技术栈。后续实现阶段如需调整，应在此更新决策记录并说明理由。

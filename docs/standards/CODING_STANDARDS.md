# DBHUB 编码规范（Coding Standards）

> 本规范约束 DBHUB 前后端的代码风格、结构与质量底线，结合行业最佳实践与本项目实际（技术栈见 [../prd/19-tech-stack.md](../prd/19-tech-stack.md)、决策见 [11-decisions.md](../prd/11-decisions.md)）。
>
> 命名所用业务术语一律以 [GLOSSARY.md](./GLOSSARY.md)（统一语言）为准。本文与 PRD 冲突时，PRD 优先；PRD 未覆盖处，本文为准。

---

## 0. 阅读对象与定位

- **对象**：所有向本仓库提交代码的工程师（含 AI 协作）。
- **定位**：是"怎么写代码"的硬约束，不是"做什么功能"的需求文档。功能与架构以 `docs/prd/` 为准。
- **原则层级**：安全/正确性 > 可读性 > 一致性 > 简洁性 > 性能（性能优化须有依据，不臆测）。

---

## 1. 通用原则

1. **API-first / 契约驱动**（D38/D44）：改 API 必先改 `openapi.yaml`，后端用 `make gen` 重新生成，前端手写类型随之同步（D51）。禁止"先改代码、后补契约"。
2. **YAGNI + 不提前抽象**：只为已存在的第二个数据点抽象（见 D4：v1 只 PG，不预设多引擎）。死代码/未用功能不写。
3. **关注点分离 / 分层**：传输层（ogen 生成 / 手写 fetch）只做编解码；业务逻辑在 service 层；持久化在 repo 层。见 §2 结构。
4. **显式优于隐式**：不依赖"魔法"、全局可变状态、隐式时序。依赖通过构造函数注入。
5. **失败要显式**：错误必须处理或显式向上传递，禁止吞错；安全相关路径见 §6。
6. **可测试性优先**（D50）：任何依赖外部资源（DB、HTTP、时钟）的组件必须通过 interface 注入，以便 mock。
7. **小步提交**：一次提交解决一个问题，可独立通过测试与审查。

---

## 2. 仓库结构与分层

```
backend/                         # 独立 Go module，make 管理
  internal/
    oas/                         # [生成·勿手改] ogen 从 ../openapi.yaml 生成（路由/handler 接口/类型/校验/安全钩子）
    api/                         # [手写] 适配层：实现 oas.Handler，薄壳，调 service
    service/                     # [手写] 业务/领域逻辑（核心），interface + mock 注入
    (infra / store / iam / ...)  # 后续按需新增，遵循分层
  main.go                        # 装配 + chi 横切中间件包住 ogen server
  ogen.yml                       # 代码生成配置
frontend/                        # 独立 pnpm 项目
  src/
    api/                         # [手写] 契约类型(types.ts) + 薄类型化 client.ts + 各资源 API 函数
    hooks/                       # [手写] TanStack Query hook（query key 工厂模式）
    (components/ pages/ stores/) # 后续按需新增
openapi.yaml                     # 唯一事实来源（契约）
```

**依赖方向（单向、不可逆）**：

```
main → api → service → (store/repo 接口)
                  ↑
            oas(生成) 被 api 实现、被 main 装配
```

- `service` 不得 import `api` 或 `oas`（领域层不依赖传输层）。
- `api` 负责领域类型 ↔ 契约类型(oas) 的映射。
- `main` 负责装配与横切（认证/ACL/审计中间件）。
- 前端 `hooks` → `api` →（fetch）；`components` → `hooks`。禁止组件直接调 fetch。

> **前后端严格分离**（D48）：无共享代码包，构建/脚本/依赖互不耦合，仅以 `openapi.yaml` 为共同契约。

---

## 3. 后端（Go）编码规范

### 3.1 基本风格
- **格式**：`gofmt`/`goimports` 强制；提交前 `go fmt ./...`。
- **包名**：小写、单词、无下划线、无复数（`service` 非 `services`）。
- **文件名**：小写蛇形（`project_service.go`）；测试文件 `xxx_test.go` 与被测同包。
- **导出标识符**：大写驼峰（`ProjectService`、`ListProjects`）；导出项必须有文档注释（以名字开头）。
- **接收者名**：类型名首字母小写、全处一致（`func (s *ProjectService)`）。
- **接口名**：行为型用 `-er`（`ProjectRepo`→方法 `List`）；接口在**消费者包**定义、小而专（Go 习惯）。
- **错误字符串**：小写开头、无标点结尾（`"project not found"`）。

### 3.2 错误处理
- **永远不要忽略 error**：`errcheck` 强制；`_ = f()` 需有注释说明为何忽略。
- **错误包装**：跨包边界用 `fmt.Errorf("list projects: %w", err)` 保留链；顶层映射为 HTTP 响应。
- **哨兵错误 / 类型化错误**：业务可判定的错误用 `errors.Is/As`，定义在 service 层（如 `var ErrNotFound = errors.New(...)`)；`api` 层将其映射为契约错误响应（含 `reason`）。
- **panic 只用于不可恢复的编程错误**（如 nil 解引用、不变量被破坏）；业务错误一律返回 `error`，不得 panic 跨包边界。

### 3.3 context
- **`ctx context.Context` 永远是首个参数**，逐层传递，禁止存入 struct 字段长期持有（除显式的 per-request handler 对象）。
- 长操作（DB 查询、HTTP 调用）必须响应 `ctx.Done()`；审计写入用不可取消的派生 context（见 PRD §6 安全边界）。

### 3.4 并发
- 共享状态用 channel 或 `sync` 保护；优先"不共享"（每请求独立）。
- 启动 goroutine 须明确其生命周期（随 ctx 取消 / WaitGroup 等待），禁止裸 `go f()` 无回收。

### 3.5 依赖注入与构造
- 依赖通过构造函数注入：`func NewProjectService(repo ProjectRepo) *ProjectService`。
- `main.go` 是唯一"组装点"；其余层不直接 `new` 具体依赖。

### 3.6 生成代码（`internal/oas`）
- **禁止手改** `internal/oas/*`；改契约 → `make gen` 重新生成。
- 该包被 lint 排除（见 `.golangci.yml`），不参与覆盖率统计。
- 在 `api` 层实现 `oas.Handler` 接口：嵌入 `oas.UnimplementedHandler`，按需 override（其余返回 501 直到实现）。

### 3.7 日志
- 用 `log/slog` 结构化日志（JSON handler，main 中装配）。
- 日志键值用蛇形常量：`logger.Info("query executed", "database", name, "rows", n)`。
- **禁止日志中打印明文凭据/令牌/完整 PII SQL**（审计另有脱敏路径，见 PRD §6）。

### 3.8 测试（后端，D50）
- **表驱动**优先；断言用 `testify/assert`、`require`。
- 包内白盒测试（`package service`）测私有逻辑；跨包用导出 API。
- 外部依赖（DB/HTTP/时钟）必须 interface + fake/mock；单测不得依赖真实 Postgres。
- 覆盖率：核心业务（service / iam / 解析 / 导出）≥ 80%；运行 `make test`（`-race -coverprofile`）。
- 一个操作至少有：成功路径、主要错误路径、边界值。

### 3.9 提交前（后端）
```bash
go fmt ./... && go vet ./... && go test -race ./... && golangci-lint run ./...
```
> `golangci-lint` 需 ≥ v1.62.1 以支持 Go 1.25（当前若为旧版会报 typecheck version 错误，请升级）。

---

## 4. 前端（React + TypeScript）编码规范

### 4.1 基本风格
- **格式与 lint**：Biome 强制（`pnpm lint` / `pnpm lint:fix`）；双引号、分号、2 空格、行宽 100、尾逗号。
- **命名**：组件/类型 PascalCase（`ProjectList`、`type Project`）；变量/函数 camelCase；常量全大写蛇形（`PAGE_SIZE`）；hook 以 `use` 开头（`useProjects`）。
- **文件名**：组件文件与默认导出同名（`ProjectList.tsx`）；普通模块 camelCase 或 kebab。
- **类型优先 interface**：对象形状用 `interface`（可扩展、报错友好）；联合/工具用 `type`。

### 4.2 TypeScript
- **strict** 模式必开；禁止 `any`（必要时 `unknown` 并收窄）；禁止 `!` 非空断言（显式判空）。
- **API 类型手写同步契约**（D51）：`src/api/types.ts` 对应 `openapi.yaml` 的 schemas，JSON 字段沿用 **snake_case**；契约变更必须同步。
- **可辨识联合**表达多态响应/状态（`type Status = { kind: 'loading' } | { kind: 'ok', data }`）。
- 导入类型用 `import type`。

### 4.3 组件与状态
- **函数组件 + hooks**；遵守 hooks 规则（顶层、不条件调用）。
- **服务端状态**一律 TanStack Query（`useQuery`/`useMutation`），不要塞进本地 state 再手动同步。
- **客户端 UI 状态**用 Zustand；**派生数据不存储**（计算得出）。
- props 类型显式；组件保持纯展示 + 数据获取分离（容器/展示分层，轻重按需）。
- 列表渲染必须 stable `key`（用业务 id，不用数组下标）。

### 4.4 数据获取（API 层约定）
- 所有请求经 `src/api/client.ts`（`api.get/post/...`），它统一处理：baseURL `/v1`、Cookie 会话（`credentials: 'include'`）、CSRF 双提交（写操作注入 `X-Csrf-Token`）、JSON、错误 → `ApiRequestError`。
- 禁止组件/页面直接 `fetch`；通过 `src/api/<resource>.ts` 函数 + `src/hooks/use*.ts` 暴露。
- **query key 工厂**（见 `useProjects` 的 `projectsKeys`）：失效/预取按资源域聚合。

### 4.5 样式
- Tailwind utility-first；复杂/复用样式抽组件而非写自定义 CSS 类名地狱。
- 设计 token（颜色/间距）走 Tailwind 配置，避免散落魔法值。

### 4.6 可访问性（a11y）
- 语义化标签（`button`/`nav`/`main`）；交互元素可键盘到达；图标按钮带 `aria-label`；表单 `label` 关联。
- 颜色对比达标；不仅靠颜色传达状态（配图标/文字）。

### 4.7 测试（前端，D50）
- Vitest + Testing Library；按"用户视角"断言（`getByRole`/`getByText`），少断实现细节。
- mock 网络用 `vi.stubGlobal('fetch', ...)`（见 `client.test.ts`）。
- hook 测试用 `renderHook` + `QueryClientProvider` 包装（见 `useProjects.test.tsx`）。

### 4.8 提交前（前端）
```bash
pnpm lint && pnpm typecheck && pnpm test && pnpm build
```

### 4.9 XSS 防护
- React 默认对插值文本转义，正常渲染用户/数据库内容即安全。
- **禁止**使用任何绕过 React 转义、直接注入原始 HTML 的 API 渲染不可信内容；确需渲染富文本时必须经 DOMPurify 等净化，并经安全评审。

---

## 5. 共享约定（跨前后端）

| 关注点 | 约定 |
|---|---|
| **契约** | 改 API 必先改 `openapi.yaml`；字段 **snake_case**；资源名 `projects/{key}/instances/{instance}/...` |
| **命名源** | 领域术语以 [GLOSSARY.md](./GLOSSARY.md) 为唯一来源 |
| **错误** | 统一 `Error{code,message,details[]}`；业务码放 `details[].reason`（稳定字符串，见 12-api-contract §4.3）；前端用 `err.reason` 分支 |
| **时间** | RFC3339（UTC）字符串传输；后端 `timestamptz` 存储 |
| **分页** | `page_size`（默认 50）+ 不透明 keyset `page_token`；响应 `items[]` + 可选 `next_page_token` |
| **ID** | 对外资源名用字符串路径段；内部主键 `bigint identity`（见 GLOSSARY §DB 规范） |
| **软删** | `deleted_at`（可空）+ 部分唯一索引；Delete 默认软删 |
| **i18n** | 中英双语（D23）；用户可见文案走 i18next，不硬编码 |

---

## 6. 安全编码底线

1. **鉴权不可绕过**：权限/审计由横切层据契约 `x-requires-permission`/`x-audit` 强制（D38）；业务代码不得自行放行。
2. **输入校验**：所有外部输入（路径/查询/请求体）经 ogen 校验 + service 层语义校验；禁止拼接 SQL，一律参数化（pgx/bun）。
3. **凭据**：DB/IdP 凭据 AES-256-GCM 加密存储（D25），内存中按需解密、用后即清；禁止入日志/审计明文。
4. **输出编码**：前端渲染防 XSS（见 §4.9）。
5. **CSRF**：Cookie 写操作双提交令牌；纯 Bearer 不受影响（PRD §3.1）。
6. **密钥/机密**：不入库不入仓；走环境变量/KMS/`secret_ref`。
7. **审计完整性**：审计写失败 fail-open + 补写（D30）；审计记录不可 UPDATE/DELETE（维护路径除外）。

---

## 7. 测试策略要点（详见 [16-ops.md](../prd/16-ops.md)）

- **单测优先**（D50）：核心逻辑 ≥80%，外部依赖 interface+mock。
- **契约测试**：后端实现须满足 `openapi.yaml`（ogen 类型即契约）；前端类型手写但须与契约一致。
- **集成测试**：关键路径（查询/导出/鉴权）用真实 Postgres（docker）跑，置 CI。
- 每个操作至少覆盖：**成功 / 主错误 / 边界**（如空、超限、并发冲突）。

---

## 8. Git / 提交 / 分支（详见 [BRANCH_STRATEGY.md](./BRANCH_STRATEGY.md)）

> 分支模型、PR 流程、合并方式、`main` 保护规则的权威来源是 [BRANCH_STRATEGY.md](./BRANCH_STRATEGY.md)。Git 相关冲突以该文为准。

- **分支模型**：GitHub Flow——唯一长期分支 `main`（受保护、始终可部署）；从最新 `main` 拉 `<type>/<scope>-<desc>` 工作分支（`feat/...`、`fix/...`、`prd/...`、`docs/...`、`refactor/...`、`test/...`、`chore/...`、`hotfix/...`）；**不引入 `develop`/`release` 分支**。
- **提交信息**：Conventional Commits（`feat:`/`fix:`/`docs:`/`refactor:`/`test:`/`chore:`），首行 ≤ 72 字符，正文说明"为什么"。
- **原子提交**：一次提交一个关注点；生成代码（`internal/oas`）与手写逻辑可分提交以便审查。
- **合并**：默认 **squash merge**（一 PR 一语义提交，`main` 历史线性）；不用 merge commit。
- **PR**：须通过两侧 lint + test + build；描述含动机、测试方式、影响面。

---

## 9. 工具链速查

| 操作 | 命令 |
|---|---|
| 后端生成（契约→代码） | `cd backend && make gen` |
| 后端测试 | `cd backend && make test` |
| 后端 lint | `cd backend && make lint`（需 golangci-lint ≥ v1.62.1） |
| 后端运行 | `cd backend && make dev` |
| 前端 dev | `cd frontend && pnpm dev` |
| 前端类型检查 | `cd frontend && pnpm typecheck` |
| 前端 lint / 修复 | `cd frontend && pnpm lint` / `pnpm lint:fix` |
| 前端测试 | `cd frontend && pnpm test` |
| 前端构建 | `cd frontend && pnpm build` |
| 全栈起 | `docker compose up -d db` + `make dev` + `pnpm dev` |

---

## 10. 清单（提交前自检）

- [ ] 改 API 是否先改 `openapi.yaml` 并 `make gen` + 同步前端类型？
- [ ] 领域命名是否与 GLOSSARY 一致？
- [ ] 外部依赖是否 interface 注入、有单测？
- [ ] 错误是否都被处理或显式上抛？安全路径是否经横切层？
- [ ] 无明文凭据/PII 进入日志？
- [ ] `lint` + `test` + `build` 两端皆绿？

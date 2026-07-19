# 分支策略（GitHub Flow）

> 本文规定 DBHUB 仓库的分支模型、PR 流程、合并方式与 `main` 保护规则。是 [CODING_STANDARDS.md §8 Git](./CODING_STANDARDS.md) 的展开与权威来源；冲突时以本文为准（Git 相关）。命名术语以 [GLOSSARY.md](./GLOSSARY.md) 为准。
>
> 适用对象：所有向本仓库提交代码的工程师（含 AI 协作 agent）。

---

## 0. 定位

- **工程流程文档**，不是产品需求。回答「怎么协作、怎么合代码」，不回答「做什么功能」。
- 与 [CODING_STANDARDS.md](./CODING_STANDARDS.md) 配套：后者管「代码怎么写」，本文管「代码怎么进仓」。
- 单仓 monorepo（D48）：`backend/` 与 `frontend/` 独立模块，共用同一套分支与 PR 规则。

---

## 1. 为什么选 GitHub Flow

| 诉求 | GitHub Flow 的对应 |
|---|---|
| 小团队、单一组织自部署（D1） | 无需 `develop`/`release` 长分支的复杂度 |
| 单节点 + Docker Compose（D24/D49） | `main` 始终可部署即可，无需多环境分支编排 |
| 持续交付、快速反馈 | 分支短命、合并即上线，流转最快 |
| 已采用 Conventional Commits + 原子提交 | 与 squash merge 天然契合（一 PR 一语义提交） |
| AI agent 协作多、PR 多 | 流程轻、规则少、易自动化 |

**核心约束：`main` 永远是可部署的（always deployable）、永远绿的（always green）。** 任何改动都经 feature 分支 + PR + CI + 评审后合入 `main`。

---

## 2. 工作流总览

```mermaid
gitGraph
   commit id: "init"
   commit id: "..."
   branch feat/query-timeout
   checkout feat/query-timeout
   commit id: "feat: query timeout"
   commit id: "test: timeout cases"
   checkout main
   merge feat/queryout type: SQUASH
   branch fix/export-csv
   checkout fix/export-csv
   commit id: "fix: csv quoting"
   checkout main
   merge fix/export-csv type: SQUASH
   commit id: "release: deploy"
```

**循环（每天重复）：**

1. 从最新 `main` 拉分支 → 2. 本地开发 + 提交 → 3. 推远端、开 PR → 4. CI（lint/test/build）→ 5. Code Review → 6. **Squash 合并**到 `main` → 7. 删除分支 → 8. 从 `main` 部署。

---

## 3. 分支模型

### 3.1 唯一长期分支：`main`

- **`main` 是受保护的、唯一长期分支。** 不接受直接 push。
- `main` 的 HEAD = 当前生产可部署状态（即便不每次都发布，也必须保证可发布）。
- 版本号（tag）打在 `main` 上，见 §8。

### 3.2 短命工作分支

从 `main` 拉，命名采用 `<type>/<scope>-<short-desc>`：

| 前缀 | 用途 | 示例 |
|---|---|---|
| `feat/` | 新功能 / 新端点 / 新页面 | `feat/project-list-api` |
| `fix/` | 缺陷修复 | `fix/export-csv-quoting` |
| `prd/` | PRD 文档新增 / 修订 | `prd/v1-export-cleanup` |
| `docs/` | 非需求文档（README、本文件、ARCHITECTURE 等） | `docs/branch-strategy` |
| `refactor/` | 不改行为的重构 | `refactor/extract-project-repo` |
| `test/` | 仅补测试 | `test/iam-matrix` |
| `chore/` | 构建/依赖/脚手架/生成物同步 | `chore/upgrade-ogen` |
| `hotfix/` | 紧急修复（走快速 PR，见 §8.2） | `hotfix/login-lockout` |
| `change/` | **OpenSpec change 驱动的端到端切片**（跨 stage 多 agent 接力，见 §4.1） | `change/add-instance-management` |

**规则：**

- 全小写、kebab-case 分隔；前缀对应 Conventional Commits 的 `type`（§6）。
- 一个分支**只做一件事**（一个关注点）。跨多关注点 → 拆多个 PR。
- 分支名要能自解释；避免 `wip`、`tmp`、`x` 这类无意义名。
- 命名中的业务术语用 [GLOSSARY.md](./GLOSSARY.md) 的统一语言（如 `feat/worksheet-share`，不是 `feat/saved-query-share`）。
- 单仓前后端改动可放同一分支（见 §7），除非改动很大需拆分。

> **OpenSpec change 驱动的改动一律用 `change/` 前缀**（不用 `feat/`/`fix/`），分支名 = `change/<change-name>`，从 change name 派生，agent 间流转无需单独告知分支名。详见 §4.1。

### 3.3 不引入的分支

- **无 `develop`、无 `release/*`、无 `staging`**（这是 GitHub Flow 与 GitFlow 的关键差异）。
- 环境差异靠配置（环境变量，见 [16-ops.md §2](./16-ops.md)），不靠分支。
- 实验性大改用 `feat/` 长分支时，定期 rebase `main` 保持新鲜，避免发散。

---

## 4. 分支生命周期规则

| 规则 | 说明 |
|---|---|
| **起点** | `git switch -c feat/xxx main`，确保基于最新 `main`（先 `git fetch` + 同步） |
| **保鲜** | 开 PR 后若 `main` 有新提交，**rebase 上游**而非堆积 merge commit：`git fetch origin && git rebase origin/main` |
| **冲突** | 由分支作者负责解决；冲突未解的 PR 不合入 |
| **命名稳定** | 开 PR 后不改分支名（会断 CI / 审查链接） |
| **收尾** | 合并后**立即删除分支**（PR 合并时勾选「Delete branch」） |
| **长寿阈值** | 单分支超过 **3 个工作日**未合并需在 PR 说明原因；超 **1 周**强制 rebase 或拆分 |

> 不要在他人分支上直接 push；要改就开新分支或用建议（suggestion）/评论。

### 4.1 OpenSpec change 分支的接力生命周期

一个 OpenSpec change = 一个 `change/<name>` 分支 = 一个 PR。多个 agent 在**同一分支**上接力提交，不互相拆 PR。分支名从 change name 派生，agent 间流转只传 change name。

| 阶段 | 负责人 | 动作 | 分支状态 |
|---|---|---|---|
| stage 1（按需） | Solution Design Agent | 首个动代码者：`git switch -c change/<name> main`；commit `openapi.yaml` 改动 + `make gen` 产物 | 已建，无 PR |
| stage 2 | Implementation Agent | 接力同分支：checkout 现有分支（无则自建）；commit 后端+前端+两端单测；创建 draft PR；stage 2 done 时标记 ready for review | draft → ready |
| stage 3 | Test Agent | 接力同分支：checkout；commit 集成/e2e/越权测试 | ready，CI 跑 |
| stage 3 | CodeReview Agent | 在 PR 上做 review；approve 或打回；**不直接 commit**（意见交 Implementation 改） | ready，待 review |
| merge | Implementation Agent | CodeReview approve + Test 绿 → 执行 squash merge；删分支 | merged |
| archive | PM Agent | merge 后跑 `openspec archive` | — |

**约束：**

- 分支**只在第一个动代码的 agent 处创建**（stage 1 跑→Solution Design；跳过→Implementation），后续 agent `git switch change/<name>` 或 `git fetch && git switch change/<name>`（远端协作时）。
- 所有改动**同一分支同一 PR**，不拆 backend/frontend PR（违反 fullstack 切片纪律，见 `docs/agents/PM_AGENT.md` §2.1）。
- PR 由 Implementation 创建为 **draft**（stage 2 进行中），stage 2 done 改 **ready**（可审查）；CodeReview 才有正式 review 对象。
- merge 权归 **Implementation（PR 所有者）**；CodeReview 只 approve、不 merge。
- PR 描述引用 change name + OpenSpec change 目录路径。

---

## 5. Pull Request 流程

### 5.1 开 PR 前（本地自检）

```bash
# 后端（cd backend）
go fmt ./... && go vet ./... && go test -race ./... && golangci-lint run ./...

# 前端（cd frontend）
pnpm lint && pnpm typecheck && pnpm test && pnpm build
```

改了 API 契约的，额外自检：

```bash
cd backend && make gen      # 重新生成 internal/oas
# 并手动同步 frontend/src/api/types.ts（D51，前端类型手写）
```

### 5.2 PR 描述模板

```markdown
## 动机 / Why
（为什么改；关联 issue / 决策编号 D## 或 PRD 章节）

## 改了什么 / What
- 后端：…
- 前端：…
- 契约：openapi.yaml 是否变更（若变更，是否已 `make gen` + 同步前端类型）

## 测试方式 / How Tested
- 新增/修改的测试用例
- 手动验证步骤

## 影响面 / Impact
- 是否触及安全/审计/权限路径
- 是否含破坏性变更（Breaking）
- 是否需要迁移 / 配置项变更
```

### 5.3 CI 门禁（必须全绿才能合）

CI 在每个 PR（含 `main` 的 push）上跑：

| 作业 | 命令 | 阻断合并 |
|---|---|---|
| 后端 lint | `make lint` | ✅ |
| 后端测试 | `make test`（`-race -cover`） | ✅ |
| 前端 lint | `pnpm lint` | ✅ |
| 前端类型检查 | `pnpm typecheck` | ✅ |
| 前端测试 | `pnpm test` | ✅ |
| 前端构建 | `pnpm build` | ✅ |
| 契约一致 | `make gen` 后 `git diff --exit-code internal/oas`（生成物未落后） | ✅ |

> CI 未全绿的 PR 不合入；`internal/oas` 落后于 `openapi.yaml` 一律打回（契约驱动 D38/D44/D51）。

### 5.4 评审（Code Review）

- **至少 1 人批准**（approve）方可合并；触及安全/审计/IAM/查询执行/导出的改动建议 **2 人**。
- 评审关注：分层是否破坏（service 不依赖 transport）、错误是否被吞、凭据是否泄漏、命名是否符合 GLOSSARY、是否有冗余抽象（YAGNI）。
- 生成代码 `internal/oas` 的 diff 可折叠/跳读，重点看 `openapi.yaml` 与手写层。
- 小 PR 优先（建议 < 400 行 diff，超过在描述说明并考虑拆分）。

---

## 6. 合并方式：默认 Squash Merge

| 方式 | 是否采用 | 理由 |
|---|---|---|
| **Squash and merge** | ✅ **默认** | 一 PR 一提交，`main` 历史干净；与 Conventional Commits 一一对应 |
| Rebase and merge | ⚠️ 仅当 PR 恰好是单一干净提交且需保留多提交时 | 少用 |
| Create merge commit | ❌ 不用 | `main` 会出现 merge commit 噪声，违背 GitHub Flow 的线性历史 |

**Squash 合并的提交信息规则：**

- 标题用 Conventional Commits：`<type>(<scope>): <summary>`，首行 ≤ 72 字符。
- `type` 与分支前缀一致（`feat/` → `feat:`）。
- 正文保留「动机 / 测试 / 影响面」（从 PR 描述精简），写清楚**为什么**。
- 多关注点的 PR 在 squash 正文里用 `- ` 列要点（而非把开发期的 WIP 提交全塞进去）。

**示例：**

```
feat(query): enforce per-statement row limit

- add QUERY_ROW_LIMIT_EXCEEDED mapping in api layer
- guard execution in service with configurable limit
- cover success / over-limit / boundary in tests

Refs: PRD 03-sql-query §3.3; D8
```

### Conventional Commits 类型对照

| type | 含义 | 触发发布物变化 |
|---|---|---|
| `feat` | 新功能 | minor / 可部署 |
| `fix` | 缺陷修复 | patch |
| `docs` | 文档 | — |
| `refactor` | 重构（不改行为） | patch（若触及构建产物） |
| `test` | 测试 | — |
| `chore` | 构建/依赖/脚手架/生成物同步 | patch |
| `perf` | 性能优化 | patch |
| `build` | 构建系统/依赖 | patch |
| `ci` | CI 配置 | — |
| `revert` | 回滚 | patch |
| `BREAKING CHANGE`（正文或 `!`） | 破坏性变更 | major |

> 破坏性变更在标题加 `!`（`feat(api)!: ...`）并在正文写 `BREAKING CHANGE: ...`，需在 PR 描述显式标注并经评审确认。

---

## 7. 与 API-first / 契约驱动的配合（D38/D44/D51）

契约是 `openapi.yaml`，前后端唯一共同事实来源。分支策略上的约束：

1. **改契约 = 改 `openapi.yaml`**，禁止先改代码再补契约。
2. 同一 PR 内完成：`openapi.yaml` 改动 + `cd backend && make gen` + `frontend/src/api/types.ts` 手动同步 + 后端 handler/service 实现 + 前端调用方。
   - 仅当改动确实过大时，才允许拆为「契约 PR（含 `make gen` + 类型同步，标记 WIP/先行评审）」+「实现 PR」，且契约 PR 必须先合。
3. 声明式安全扩展（`x-requires-permission`/`x-audit`/`x-auth-method`/`x-allow-without-credential`）随契约一起改，业务代码不得自行放行（见 CODING_STANDARDS §6）。
4. **生成代码 `internal/oas` 的 diff 可与手写逻辑分两个提交**以便审查（CODING_STANDARDS §8）；squash 合并后不影响 `main` 线性。

### 前后端独立模块、同一分支

- D48：`backend/` 与 `frontend/` 独立 module/依赖/脚本，但**共享分支与 PR**。
- 纯后端 / 纯前端改动：分支仅动一侧即可，CI 仍两端都跑（保持整体绿）。
- 跨端改动（契约驱动的端到端功能）放同一分支同一 PR，保证原子性与一致性。

---

## 8. 发布与热修复

GitHub Flow 下「发布」就是「从 `main` 构建并部署」，不另开 release 分支。

### 8.1 正常发布

1. 确保 `main` CI 全绿。
2. 在 `main` HEAD 打版本 tag：`v<major>.<minor>.<patch>`（语义化版本）。
   ```bash
   git tag -a v0.1.0 -m "release: v0.1.0"
   git push origin v0.1.0
   ```
3. CI/部署流水线由 tag 触发，构建镜像并按 [16-ops.md](./16-ops.md) 部署（Docker Compose）。
4. tag 描述附 changelog（可由 Conventional Commits 自动生成）。

### 8.2 紧急热修复（hotfix）

```mermaid
gitGraph
   commit id: "v1.2.0"
   commit id: "feat"
   branch hotfix/login-lockout
   checkout hotfix/login-lockout
   commit id: "fix: lockout"
   checkout main
   merge hotfix/login-lockout type: SQUASH
   commit id: "v1.2.1"
```

1. 从 `main` 拉 `hotfix/<desc>` 分支。
2. 最小化修复 + 补回归测试 + 走 PR（CI 全绿 + 至少 1 人加急批准）。
3. Squash 合并回 `main`。
4. 在 `main` 新 HEAD 打 patch tag `v1.2.1`，触发部署。
5. （若曾从旧 tag 拉过发布分支，则 cherry-pick 该 squash 提交过去——v1 暂无此场景。）

> 不要在 `main` 上直接修 bug 再 push；即便是热修复也走 PR，保留评审与审计。

---

## 9. `main` 分支保护规则（配置清单）

在仓库 Settings → Branches → `main` 上启用：

- [ ] **Require a pull request before merging**：要求 PR；最少 **1** 个 review approval。
- [ ] **Dismiss stale pull request approvals when new commits are pushed**：新提交使旧批准失效，强制复审。
- [ ] **Require status checks to pass before merging**：勾选下列必需检查：
  - 后端 lint / test
  - 前端 lint / typecheck / test / build
  - 契约一致（`internal/oas` 未落后）
- [ ] **Require branches to be up to date before merging**：合并前须基于最新 `main`（鼓励 rebase）。
- [ ] **Require conversation resolution before merging**：所有讨论标记为已解决。
- [ ] **Do not allow bypassing the above settings**：管理员也不绕过（安全敏感仓库强烈建议）。
- [ ] （可选）**Require linear history**：配合 squash merge，禁止 merge commit。
- [ ] **Restrict who can push**：不允许任何人直接 push（PR only）。

---

## 10. 协作速查 / 清单

### 开新功能
```bash
git fetch origin
git switch -c feat/my-feature origin/main
# ...开发，原子提交...
git push -u origin feat/my-feature
# GitHub 上开 PR，等 CI + review
```

### 开 OpenSpec change（change/ 前缀）
```bash
# stage 1 跑：Solution Design 首个动代码
git switch -c change/<change-name> main
# ...commit openapi 改动 + make gen...
git push -u origin change/<change-name>

# stage 2：Implementation 接力
git fetch origin && git switch change/<change-name>   # 远端协作；本地接力直接 git switch
# ...commit 后端+前端+单测...
gh pr create --draft --title "<conventional-summary>" --body "OpenSpec change: <change-name>"
# stage 2 done:
gh pr ready <PR-NUMBER>

# stage 3：Test 接力
git switch change/<change-name>
# ...commit 集成测试...

# CodeReview 在 PR 上 approve；Implementation 执行 merge：
gh pr merge <PR-NUMBER> --squash --delete-branch

# PM: merge 后 archive
openspec archive "<change-name>"
```

### 同步上游（保持新鲜）
```bash
git fetch origin
git rebase origin/main          # 不要 git pull --merge 堆 merge commit
# 解冲突后 git push --force-with-lease
```

### 合并后清理
- GitHub 勾选「Delete branch」自动删除远端；
- 本地：`git switch main && git pull --ff-only && git branch -d feat/my-feature`。

### 提交前自检（强制）
- [ ] 改 API 是否先改 `openapi.yaml` 并 `make gen` + 同步前端类型？
- [ ] 后端 `fmt+vet+test+lint` 全绿？前端 `lint+typecheck+test+build` 全绿？
- [ ] 分支只含一个关注点、命名符合前缀表？
- [ ] 提交信息 Conventional Commits、首行 ≤ 72？
- [ ] 无明文凭据 / PII 进入日志或提交？
- [ ] PR 描述含动机 / 测试方式 / 影响面？

---

## 11. 决策记录

本策略对应可在 [11-decisions.md](./11-decisions.md) 追加一条工程决策（建议）：

| # | 决策 | 理由 |
|---|---|---|
| D52 | **采用 GitHub Flow**：唯一长期分支 `main`（受保护、始终可部署），feature 分支 + PR + squash merge；不引入 develop/release 分支 | 小团队、单组织自部署（D1）、单节点 Docker Compose（D24/D49）；流程轻、流转快，与 Conventional Commits + 契约驱动（D38/D44）契合 |

> 该编号为占位建议；最终以 `11-decisions.md` 实际追加为准。

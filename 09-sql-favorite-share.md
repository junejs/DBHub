# 09 — SQL 收藏与分享

> 本文档定义 SQL 的「收藏（Favorite）」与「分享（Share）」能力。它们建立在 [03-sql-query.md](./03-sql-query.md) 的「保存的查询（Worksheet）」之上，是提升查询复用与团队协作的一等公民功能。

---

## 1. 背景与目标

实际工作中，高频痛点是：
- **复用**：同一段排查 SQL、报表 SQL 反复写，需要快速找回。
- **协作**：一位工程师写好的取数 SQL，想直接给同事/分析师用，而不是贴在聊天里。

目标：
- 让用户一键**收藏**任意 SQL，建立个人/团队的查询知识库。
- 让用户安全地**分享** SQL 给他人，且**分享只传递「SQL 文本」，不传递任何数据访问权限**——执行时仍按接收者本人的权限/脱敏/审计约束。

---

## 2. 概念关系：收藏 vs 保存的查询（Worksheet）

为避免概念混淆，明确两者：

| 对象 | 定位 | 是否持久 SQL | 归属 |
|---|---|---|---|
| **Worksheet（保存的查询）** | SQL 的一等存储对象：标题、内容、关联库、可见性 | ✅ | 创建者 / 项目 |
| **Favorite（收藏）** | 指向某个 Worksheet 的快捷书签（类似浏览器的收藏夹） | 否，引用 Worksheet | 单个用户私有 |

**核心设计：收藏始终指向一个 Worksheet。** 用户「收藏一段 SQL」时：
- 若该 SQL 还不是 Worksheet → 系统自动创建一个 **PRIVATE** Worksheet（默认仅自己可见），再收藏它。
- 若已是 Worksheet（自己的或他人分享可见的）→ 直接加为收藏。

这样「SQL 内容」只有一份（Worksheet），收藏只是个人视角的快捷入口，不会产生内容副本与同步问题。

> 这与 Bytebase 的 `WorksheetOrganizer`（per-user 的 `starred` / `folders`）模型一致：star/folder 是「用户对 Worksheet 的个人组织关系」，Worksheet 本身是共享内容。

---

## 3. 收藏（Favorite）

### 3.1 用户故事
- 作为分析师，我希望在工作台一键收藏当前 SQL，下次秒级找回。
- 作为分析师，我希望把收藏分组（如「日常排查」「月报」）、置顶常用的。
- 作为分析师，我希望给收藏加备注（如「注意：用前改日期」）。

### 3.2 功能需求
- **收藏入口**：工作台编辑区、查询历史列表、Worksheet 列表上均有「收藏」按钮。
- **收藏夹**：侧边栏「我的收藏」聚合视图；支持：
  - 文件夹/分组（单级标签）。
  - 置顶（pin）常用项。
  - 备注（note，个人视角，不影响 Worksheet 内容）。
  - 排序：最近使用 / 收藏时间 / 名称 / 自定义。
- **一键打开**：点击收藏 → 在工作台打开对应 Worksheet（带上关联库），可直接执行或编辑后另存。
- **搜索**：按标题、备注、SQL 内容、关联库过滤收藏。
- **归属**：收藏为**用户私有**，他人不可见；删除收藏不影响底层 Worksheet。

### 3.3 数据模型（Favorite）
```
Favorite {
  user            // 收藏者 users/{email}
  worksheet       // 指向的 Worksheet
  folder          // 分组/标签（可选）
  note            // 个人备注（可选）
  pinned          // 是否置顶
  create_time, last_open_time
}
// 唯一约束：(user, worksheet) 防止重复收藏
```

---

## 4. 分享（Share）

### 4.1 用户故事
- 作为工程师，我希望把写好的取数 SQL 分享给项目内的分析师。
- 作为工程师，我希望能生成一个分享链接发给同事，点开即在工作台打开。
- 作为安全管理员，我希望分享**只传 SQL、不传权限**——同事执行时仍走他自己的权限和脱敏。

### 4.2 分享模型：基于 Worksheet 可见性

分享通过 **Worksheet 的可见性（Visibility）** 实现，不引入独立「分享对象」：

| 可见性 | 含义 |
|---|---|
| `PRIVATE` | 仅创建者可见（默认） |
| `PROJECT_READ` | 同项目成员可查看/执行/收藏 |
| `PROJECT_WRITE` | 同项目成员可编辑内容 |
| `LINK`（可选增强） | 持有分享链接者可查看（仍需是登录用户 + 通过权限校验） |

- 改可见性即「分享」：把 Worksheet 从 PRIVATE 改为 PROJECT_READ，项目内同事即刻在「项目查询」里看到。
- `LINK` 模式生成一个**带 token 的分享链接**，便于聊天/邮件传递；支持**过期时间**与**手动撤销**（吊销 token）。

### 4.3 分享的安全边界（关键）

> **分享是「SQL 的传递」，不是「数据权限的传递」。**

- 接收者打开分享的 Worksheet 并执行时：
  - 走**接收者本人**的 IAM 权限（`db.sql.select` + 库/表 CEL）。
  - 走**接收者本人**的脱敏策略与谓词列保护。
  - 若接收者无权访问该库/表 → 执行被拒绝（提示无权限），但可查看 SQL 文本。
- 分享者把 SQL 改写为「越权查询」也无法让接收者越权——权限校验在执行时按接收者身份强制。
- 分享行为本身**审计**：谁把哪个 Worksheet 分享给了什么范围/生成了链接。

### 4.4 协作增强
- **评论/备注**（可选）：项目成员可在 Worksheet 下评论（类似代码评审），辅助说明用法。
- **派生另存**：接收者可「另存为」自己的 Worksheet（如需改库/改条件），避免多人改同一份。
- **归属与变更感知**：分享者修改内容后，收藏了它的他人自动看到最新版（因收藏引用同一 Worksheet）。

---

## 5. API 设计（关键 RPC）

| RPC | 路径 | 说明 |
|---|---|---|
| `CreateWorksheet` / `UpdateWorksheet` | — | 保存/编辑 SQL（含 visibility 字段） |
| `FavoriteWorksheet` | `POST .../worksheets/{id}:favorite` | 收藏（带 folder/note/pinned） |
| `UnfavoriteWorksheet` | `POST .../worksheets/{id}:unfavorite` | 取消收藏 |
| `ListMyFavorites` | `GET .../me/favorites` | 我的收藏夹（支持过滤/排序） |
| `UpdateFavoriteOrganizer` | `PATCH .../me/favorites/{id}` | 改分组/备注/置顶 |
| `CreateShareLink` | `POST .../worksheets/{id}:createShareLink` | 生成带 token 的链接（可选过期） |
| `RevokeShareLink` | `POST .../worksheets/{id}:revokeShareLink` | 吊销链接 |
| `OpenSharedWorksheet` | `GET .../shared/{token}` | 打开分享链接（需登录 + 权限） |

> 多数 RPC 采用 `auth_method=CUSTOM`（按创建者/归属判定，非 IAM 权限），与 Bytebase 的 Worksheet/WorksheetOrganizer 一致。

---

## 6. 与权限/脱敏/审计的联动

| 维度 | 约束 |
|---|---|
| 数据权限 | **执行分享/收藏的 SQL 时，按执行者本人权限校验**；分享不授予任何数据访问 |
| 脱敏 | 结果按执行者本人脱敏策略；分享者去脱敏的 JIT 不传递给接收者 |
| 审计 | 收藏行为不审计（个人操作）；**分享、改可见性、生成/吊销链接需审计** |
| 可见性边界 | PRIVATE 仅本人；PROJECT_* 受项目成员关系约束；LINK 仍要求登录 |

---

## 7. 非功能需求
- 收藏夹打开 P95 ≤ 300ms（个人数据，量小）。
- 分享链接打开 → 工作台就绪 ≤ 1s。
- 分享 token 不可枚举（高熵随机），带过期与吊销。

---

## 8. 功能范围
- 收藏（一键收藏、收藏夹、分组/置顶/备注、搜索、一键打开）。
- 分享（基于 Worksheet 可见性：PRIVATE / PROJECT_READ / PROJECT_WRITE）。
- 分享链接（带 token、过期、吊销）。
- 安全边界：执行按接收者本人权限/脱敏；分享行为审计。

**可选增强（不在本期必须范围）：** 评论/协作、派生另存为模板、跨项目分享、收藏推荐。

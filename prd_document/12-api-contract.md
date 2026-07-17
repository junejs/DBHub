# 12 — API 契约

> 本文档是系统对外 API 的**唯一事实来源**,以 **OpenAPI 3(HTTP/JSON)** 描述。任何后端语言/框架均可据此实现客户端或服务端,**不预设特定编程语言或框架**。交付团队据此即可前后端并行开发。
>
> 传输:**HTTP/JSON**(REST 友好,采用 Google AIP 风格的资源命名与标准方法)。版本前缀 `/v1`;不兼容变更升 `/v2`。

---

## 1. 总则

- **声明式权限 / 审计**:每个操作通过 OpenAPI 扩展声明 `x-requires-permission`(驱动 ACL 中间件)与 `x-audit`(驱动审计中间件),业务代码无法绕过(见 [01](./01-architecture.md)、[06](./06-audit-log.md))。这些扩展是**与语言无关的元数据**,由各实现的横切层(中间件 / 拦截器 / 装饰器 / 过滤器)在请求处理前读取并强制——具体用何种语言/框架实现该横切层,本文不规定。
- **资源命名**:路径层级反映归属与隔离(项目 → 实例 → 库)。
- **标准方法**:`Get`(GET)/ `List`(GET)/ `Create`(POST)/ `Update`(PATCH)/ `Delete`(DELETE) + 自定义动词(`:query`、`:export`、`:search`、`:sync`、`:activate`、`:download`、`:testConnection` 等,以 POST 调用)。
- **实现无关**:本文不规定服务端语言、Web 框架或 codegen 工具。`openapi.yaml` 可直接由各类工具(openapi-generator 等)生成任意语言的客户端 / 服务端桩与文档。

---

## 2. 资源命名约定

| 资源 | 名称模式 | 说明 |
|---|---|---|
| Project | `projects/{project}` | 团队隔离边界 |
| Environment | `environments/{environment}` | 平台级(dev/test/stage/prod) |
| Instance | `projects/{project}/instances/{instance}` | 归属项目 |
| Database | `projects/{project}/instances/{instance}/databases/{database}` | 项目归属由实例决定 |
| DatabaseCatalog | `.../databases/{database}/catalog` | 列级标注(**v2**,v1 不提供) |
| DataSource | `.../instances/{instance}/dataSources/{dataSource}` | 实例下连接(键=admin/readonly) |
| Worksheet | `projects/{project}/worksheets/{worksheet}` | 保存的查询 |
| Favorite | `users/{user}/favorites/{favorite}` | 个人收藏 |
| ExportTask | `projects/{project}/exportTasks/{task}` | 异步导出 |
| User / Group | `users/{user}` / `groups/{group}` | 平台级身份 |
| IDP | `identityProviders/{idp}` | 身份提供商 |
| AuditLog | `projects/{project}/auditLogs/{id}` 或 `auditLogs/{id}` | 项目级 / 全局 |
| Notification | `users/{user}/notifications/{id}` | 站内通知 |

> ID 规范:`{project}`/`{instance}` 等为小写 `[a-z][a-z0-9-]{1,62}`。数据库/表名按引擎大小写规则原样保留。`{dataSource}` 取数据源角色(`admin`/`readonly`,每实例各至多一个)。

---

## 3. 通用约定

### 3.1 鉴权
- Web 端:HTTP-only Cookie(`access-token` / `refresh-token`),`SameSite=Lax`。
- API 端(后续支持客户端凭证):`Authorization: Bearer <jwt>`。
- 未鉴权操作(登录类)标注 `x-allow-without-credential: true`。
- 用户私有数据(查询历史、收藏、通知)标注 `x-auth-method: CUSTOM`,按归属判定而非 ACL 权限。
- **CSRF 防护**(Cookie 场景必做):写操作(POST/PATCH/DELETE)要求 `SameSite=Lax` Cookie + **双提交 CSRF 令牌**(请求头 `X-CSRF-Token`,首屏由 `GET /v1/auth/csrf` 下发);或写操作改走 `Authorization: Bearer`(纯 Bearer 不受 CSRF 影响)。

### 3.2 分页
- 入参:`page_size`(默认 50;审计等大表上限 5000)、`page_token`(不透明 keyset 游标,不可构造)。
- 响应:`items[]`、`next_page_token`(无更多页时省略)、可选 `total_size`(**大表如 audit 可不返回**,避免昂贵 COUNT)。
- 服务端用「limit+1」判定是否有下一页;游标为 keyset(非 offset),保证翻页稳定。

### 3.3 更新
- `Update*` 用 `PATCH` + JSON Merge Patch(或 `update_mask` 查询参数声明改哪些字段)。
- 部分资源支持 `allow_missing=true`(upsert)与 `validate_only=true`(只校验不落库,如测试连接)。

### 3.4 软删除
- `Delete` 默认软删(`deleted_at`);`:undelete` 可恢复。硬删仅平台维护操作。

### 3.5 乐观锁(并发更新)
- 可被多人编辑的资源(如 `Worksheet`)带 `etag`;`PATCH` 带 `If-Match: <etag>`,服务端不匹配 → `409 CONCURRENT_MODIFICATION`。`IamPolicy` 同理。

### 3.6 幂等创建
- `Create*` 支持可选请求头 `X-Request-Id`(客户端生成);相同 ID 重试返回首次结果,防重复创建。

### 3.7 List 的鉴权过滤(重要)
- 所有 `List*` 返回**仅含调用者可见**的资源:项目级资源按「项目成员关系 + 结构化条件(环境/库/表)」二次裁剪,跨项目默认不可见(见 [02 §2.3](./02-permission-and-access.md))。即 List 结果受 IAM 过滤,而非返回项目内全部。

---

## 4. 错误模型

### 4.1 错误响应(JSON)

```json
{
  "code": 412,
  "message": "结果超行数上限,请加过滤条件或改用导出",
  "details": [
    { "@type": "dbh.v1.ErrorInfo",
      "reason": "QUERY_ROW_LIMIT_EXCEEDED",
      "domain": "dbh",
      "metadata": { "limit": "1000" } }
  ]
}
```

- `code`:HTTP 状态码。`message`:面向人。`details[]`:结构化,面向程序。
- 业务错误码以 `ErrorInfo.reason`(稳定字符串标识)携带,附 `metadata` 上下文(位置、阈值、资源名)。
- 字段级校验错误用 `BadRequest` detail(每字段 `field` + `description`)。
- 引擎原始错误(如 PG 错误码)作为 `metadata.engine_error` 透传。

### 4.2 HTTP 状态码

| HTTP | 含义 |
|---|---|
| 400 | 参数错误 |
| 401 | 未登录 / token 失效 |
| 403 | 无权限 |
| 404 | 资源不存在或无权可见 |
| 409 | 资源已存在 / 并发冲突(ETag) |
| 412 | 前置条件不满足(如只读连接执行 DDL) |
| 429 | 限流 / 配额耗尽 |
| 499 | 客户端取消(记审计) |
| 500 | 内部错误 |
| 503 | 服务不可用(业务库连不上) |
| 504 | 语句超时 |

### 4.3 业务错误码目录(reason)

| reason | HTTP | 触发场景 |
|---|---|---|
| `AUTH_INVALID_CREDENTIALS` | 401 | 账号/密码错误 |
| `AUTH_MFA_REQUIRED` | 401 | 需二次验证,返回 mfa_temp_token |
| `AUTH_MFA_INVALID` | 401 | OTP/恢复码错误 |
| `AUTH_ACCOUNT_LOCKED` | 401 | 失败次数过多被锁定 |
| `AUTH_TOKEN_EXPIRED` | 401 | access token 过期(提示 refresh) |
| `PERMISSION_DENIED` | 403 | 无对应权限 |
| `PROJECT_ISOLATION` | 403 | 跨项目访问被拒(资源不在调用者项目) |
| `EXPORT_PASSWORD_INVALID` | 403 | 导出产物 ZIP 密码错误 |
| `RESOURCE_NOT_FOUND` | 404 | 资源不存在或无权可见 |
| `RESOURCE_ALREADY_EXISTS` | 409 | 唯一冲突(如同名实例) |
| `CONCURRENT_MODIFICATION` | 409 | ETag 不匹配 |
| `INVALID_SQL` | 400 | 语法错误(details.metadata.position) |
| `NON_READONLY_STATEMENT` | 412 | 只读连接执行了 DDL/DML |
| `MULTI_STATEMENT_NOT_SUPPORTED` | 400 | 该引擎不支持多语句 |
| `QUERY_ROW_LIMIT_EXCEEDED` | 412 | 结果超行数上限 |
| `QUERY_TIMEOUT` | 504 | 语句超时 |
| `EXPORT_TOO_LARGE_FOR_SYNC` | 412 | 同步导出预估 >1 万行,强制异步(D22) |
| `EXPORT_ARCHIVE_EXPIRED` | 404 | 产物过期已清理 |
| `RATE_LIMITED` | 429 | 触发限流(查询/登录) |
| `DB_CONNECTION_FAILED` | 503 | 连不上业务库 |
| `VALIDATE_ONLY_FAILED` | 412 | 测试连接失败 |

---

## 5. 公共 schema(OpenAPI components)

> 下列为 JSON Schema 片段,可直接并入 `openapi.yaml` 的 `components.schemas`。时间戳为 RFC 3339 字符串。

```yaml
ExportFormat:
  type: string
  enum: [CSV, JSON]      # v1;SQL/XLSX 延后 v2

QueryResponse:
  type: object
  properties:
    results:             # 多语句:每条语句一个结果,按序
      type: array
      items: { $ref: "#/components/schemas/QueryResult" }

QueryResult:             # 查询/导出共用
  type: object
  properties:
    column_names:        { type: array, items: { type: string } }
    column_type_names:   { type: array, items: { type: string } }
    rows:                { type: array, items: { $ref: "#/components/schemas/QueryRow" } }
    rows_count:          { type: integer, format: int64 }
    next_page_token:     { type: string }   # 该结果集下一页游标;无则省略
    error:               { $ref: "#/components/schemas/QueryError" }
    latency:             { type: string }   # 如 "0.42s"
    statement:           { type: string }
    engine_messages:     { type: array, items: { type: string } }  # NOTICE/PRINT

QueryRow:
  type: object
  properties:
    values:
      type: array
      items: { $ref: "#/components/schemas/RowValue" }

RowValue:                # 列值;实际取一种 kind(由字段非空判定)
  type: object
  properties:
    null_value:   { type: boolean, nullable: true }   # 用 true 表示 NULL
    bool_value:   { type: boolean }
    int_value:    { type: integer, format: int64 }
    double_value: { type: number }
    string_value: { type: string }
    bytes_value:  { type: string, format: byte }       # base64
    time_value:   { type: string, format: date-time }
    struct_value: { type: string }                      # JSON 文本

QueryError:
  type: object
  properties:
    message: { type: string }
    syntax_error:        { $ref: "#/components/schemas/SyntaxError" }
    permission_denied:   { type: object }              # 命中结构化条件拒绝
    command_error:       { $ref: "#/components/schemas/CommandError" }   # DDL/DML/非只读
    engine_error:        { $ref: "#/components/schemas/EngineError" }

SyntaxError:    { type: object, properties: { position: { type: integer }, message: { type: string } } }
CommandError:   { type: object, properties: { kind: { type: string }, message: { type: string } } }   # kind: ddl|dml|non_read_only
EngineError:    { type: object, properties: { code: { type: string }, message: { type: string } } }
```

---

## 6. 各资源 API

> 下列为路径与扩展声明。标准 CRUD(未展开字段)按 Google AIP 与 §3 约定补齐。`x-requires-permission`/`x-audit`/`x-auth-method`/`x-allow-without-credential` 为自定义 OpenAPI 扩展,由服务端横切层强制。

### 6.1 AuthService — 认证

```
POST /v1/auth:login                    x-allow-without-credential:true  x-audit:true
POST /v1/auth:logout                   x-audit:true
POST /v1/auth:refresh                  x-allow-without-credential:true
POST /v1/auth:passwordReset:request    x-allow-without-credential:true
POST /v1/auth:passwordReset:confirm    x-allow-without-credential:true  x-audit:true
GET  /v1/auth/csrf                     (下发 CSRF token,供 Cookie 写操作双提交)
```

```yaml
LoginRequest:
  type: object
  properties:
    email:    { type: string }
    password: { type: string }
    # SSO:  idp / code          (OIDC/OAuth2 回调)
    # MFA:  mfa_temp_token / otp_code / recovery_code
LoginResponse:
  type: object
  properties:
    access_token:   { type: string }   # API 端;Web 端走 Cookie
    refresh_token:  { type: string }
    mfa_temp_token: { type: string }   # 需 MFA 时返回,access_token 为空
    user:           { $ref: "#/components/schemas/User" }
    workspace:      { type: string }
```

### 6.2 SQLService — 查询(核心)

```
POST /v1/{name=projects/*/instances/*/databases/*}:query
  x-requires-permission: db.sql.select   x-audit:true
POST /v1/{name=projects/*/instances/*/databases/*}:export        # 同步导出
  x-requires-permission: db.sql.select   x-audit:true
GET  /v1/queryHistories:search
  x-auth-method: CUSTOM                  # 自身历史
```

```yaml
QueryRequest:
  type: object
  properties:
    name:           { type: string }    # projects/{p}/instances/{i}/databases/{d}
    statement:      { type: string }    # 多语句分号分隔;带 page_token 时忽略
    limit:          { type: integer }   # 行数上限;默认取 environment_policies.query_row_limit
    schema:         { type: string }    # search_path / current schema
    data_source_id: { type: string }    # 可选
    explain:        { type: boolean }
    page_token:     { type: string }    # 翻某结果集下一页(语句序号+游标已编码入 token)
```

- **多语句**:首请求执行全部语句,响应 `results[]` 按序返回**每条语句的首页**;取某条语句结果集的更多行 → 带 `results[i].next_page_token` 再次 POST `:query`(此时 `statement` 忽略,服务端按 token 续读该结果集)。
- **同步导出**:预估 >1 万行(EXPLAIN 估算,D22)→ `412 EXPORT_TOO_LARGE_FOR_SYNC`,改走 ExportTask。

```yaml
ExportRequest:                          # 同步导出
  type: object
  properties:
    name:      { type: string }
    statement: { type: string }
    format:    { $ref: "#/components/schemas/ExportFormat" }
    password:  { type: string }         # 用户自设 ZIP 密码
    limit:     { type: integer }
# 响应:application/octet-stream(密码加密 ZIP 字节流)
```

### 6.3 ExportTaskService — 异步导出

```
POST   /v1/projects/{project}/exportTasks               x-requires-permission: db.exports.create  x-audit:true
GET    /v1/projects/{project}/exportTasks/{task}        x-auth-method: CUSTOM
GET    /v1/projects/{project}/exportTasks               x-auth-method: CUSTOM   (List, 鉴权过滤)
POST   /v1/projects/{project}/exportTasks/{task}:download   x-auth-method: CUSTOM  x-audit:true
```

```yaml
ExportTask:
  type: object
  properties:
    name:             { type: string }    # projects/{p}/exportTasks/{id}
    database:         { type: string }
    statement:        { type: string }
    format:           { $ref: "#/components/schemas/ExportFormat" }
    state:            { type: string }    # CREATED|RUNNING|SUCCEEDED|FAILED|EXPIRED
    creator:          { type: string }
    create_time:      { type: string, format: date-time }
    complete_time:    { type: string, format: date-time }
    row_count:        { type: integer, format: int64 }
    size_bytes:       { type: integer, format: int64 }
    error:            { type: string }
    export_archive_id:{ type: string }
CreateExportTaskRequest:
  type: object
  properties:
    parent:     { type: string }          # projects/{p}
    task:       { $ref: "#/components/schemas/ExportTask" }
# 幂等:请求头 X-Request-Id
DownloadExportRequest:
  type: object
  properties:
    password: { type: string }            # 校验 ZIP 密码;错 → 403 EXPORT_PASSWORD_INVALID
# 响应:application/octet-stream(流式解密下发)
```

### 6.4 InstanceService — 实例(项目内)

```
POST   /v1/projects/{project}/instances                  x-requires-permission: db.instances.create  x-audit:true
GET    /v1/projects/{project}/instances/{instance}       x-requires-permission: db.instances.get
GET    /v1/projects/{project}/instances                  x-requires-permission: db.instances.list   (鉴权过滤)
PATCH  /v1/projects/{project}/instances/{instance}       x-requires-permission: db.instances.update  x-audit:true
DELETE /v1/projects/{project}/instances/{instance}       x-requires-permission: db.instances.delete  x-audit:true
POST   /v1/projects/{project}/instances/{instance}:sync  x-requires-permission: db.instances.sync    x-audit:true   (即时返回·异步)
POST   /v1/projects/{project}/instances/{instance}:testConnection  x-requires-permission: db.instances.get  (validate_only)
# DataSource 子资源(admin/readonly)
POST   /v1/.../instances/{instance}/dataSources/{dataSource}        x-requires-permission: db.instances.update  x-audit:true
PATCH  /v1/.../instances/{instance}/dataSources/{dataSource}        x-requires-permission: db.instances.update  x-audit:true
DELETE /v1/.../instances/{instance}/dataSources/{dataSource}        x-requires-permission: db.instances.update  x-audit:true
```

```yaml
Instance:
  type: object
  properties:
    name:                 { type: string }    # projects/{p}/instances/{i}
    project:              { type: string }
    title:                { type: string }    # 展示名
    engine:               { type: string }
    engine_version:       { type: string }    # OUTPUT_ONLY
    environment:          { type: string }    # environments/{env},必填
    activation:           { type: boolean }
    sync_interval_seconds:{ type: integer }
    last_sync_time:       { type: string, format: date-time }   # OUTPUT_ONLY
    data_sources:         { type: array, items: { $ref: "#/components/schemas/DataSource" } }
CreateInstanceRequest:
  type: object
  properties:
    parent:        { type: string }          # projects/{p}
    instance_id:   { type: string }
    instance:      { $ref: "#/components/schemas/Instance" }
    validate_only: { type: boolean }
```

> `:sync` / `:syncSchema` **即时返回**(202 Accepted + 当前 `sync_status` + 触发时间),后台异步执行;完成后写 `last_sync_time`/`sync_status`,可选推站内通知。**不引入长任务(Operation)轮询模型**。

### 6.5 DatabaseService & DatabaseCatalogService

```
GET    /v1/.../databases/{database}          x-requires-permission: db.databases.get
GET    /v1/.../databases                      x-requires-permission: db.databases.list   (鉴权过滤)
PATCH  /v1/.../databases/{database}           x-requires-permission: db.databases.update  x-audit:true
POST   /v1/.../databases/{database}:syncSchema x-requires-permission: db.databases.sync   (即时返回·异步)
```

```yaml
Database:
  type: object
  properties:
    name:        { type: string }    # projects/{p}/instances/{i}/databases/{d}
    project:     { type: string }    # OUTPUT_ONLY(= instance.project)
    instance:    { type: string }
    environment: { type: string }    # effective_environment,OUTPUT_ONLY
    sync_status: { type: string }
    sync_error:  { type: string }
    labels:      { type: object, additionalProperties: { type: string } }
```

> DatabaseCatalog(列级语义类型/分类标注)**不在 v1 范围**(见 [18 §1.1](./18-roadmap.md))。

### 6.6 IamService — 权限绑定(类 GCP IAM)

```
POST /v1/{resource}:getIamPolicy          x-requires-permission: db.projects.getIamPolicy
POST /v1/{resource}:setIamPolicy          x-requires-permission: db.projects.setIamPolicy  x-audit:true   (If-Match etag;记前后差异)
POST /v1/{resource}:testIamPermissions    (按调用者本人权限判定)
```

```yaml
IamPolicy:
  type: object
  properties:
    bindings: { type: array, items: { $ref: "#/components/schemas/Binding" } }
    etag:     { type: string }
Binding:
  type: object
  properties:
    role:     { type: string }            # roles/sqlEditorReadUser
    members:  { type: array, items: { type: string } }   # user:x@ / group:y@ / allUsers
    condition: { type: object }           # 结构化条件 {environments,databases,schemas,tables}
SetIamPolicyRequest:
  type: object
  properties:
    resource: { type: string }
    policy:   { $ref: "#/components/schemas/IamPolicy" }
    etag:     { type: string }            # 乐观锁
```

### 6.7 AccessGrantService(JIT)— 不在 v1 范围

> v1 不提供平台侧 JIT(路线图见 [18 §1.2](./18-roadmap.md))。

### 6.8 AuditLogService

```
POST /v1/{parent}/auditLogs:search    x-requires-permission: db.auditLogs.search  x-audit:true   (审计的审计)
POST /v1/{parent}/auditLogs:export    x-requires-permission: db.auditLogs.export  x-audit:true
```

```yaml
AuditLog:
  type: object
  properties:
    name:          { type: string }
    create_time:   { type: string, format: date-time }
    method:        { type: string }
    user:          { type: string }
    resource:      { type: string }
    severity:      { type: string }
    status_code:   { type: integer }
    status_message:{ type: string }
    latency:       { type: string }
    request:       { type: string }    # 脱敏后
    response:      { type: string }    # 脱敏后
    ip:            { type: string }
    user_agent:    { type: string }
SearchAuditLogsRequest:
  type: object
  properties:
    parent:     { type: string }       # projects/{p} 或省略(全局,securityAdmin)
    filter:     { type: object }       # 结构化过滤:method/user/resource/severity/create_time
    page_size:  { type: integer }
    page_token: { type: string }
# 读权限范围见 [06 §5.1](./06-audit-log.md) D26
```

### 6.9 ProjectService & EnvironmentService

```
POST   /v1/projects                           x-requires-permission: db.projects.create (workspaceAdmin)  x-audit:true
GET    /v1/projects/{project}                 (按项目角色)
PATCH  /v1/projects/{project}                 x-audit:true
DELETE /v1/projects/{project}                 x-audit:true   (非空阻塞;force=true 级联,D27)
POST   /v1/projects/{project}:undelete
GET    /v1/projects                           (鉴权过滤:仅可见本人所属项目)
GET    /v1/environments                       (公开读)
POST   /v1/environments/{environment}         (workspaceAdmin/securityAdmin)  x-audit:true
PATCH  /v1/environments/{environment}         x-audit:true
DELETE /v1/environments/{environment}         (被引用时阻塞)
GET    /v1/environments/{environment}/policy
PATCH  /v1/environments/{environment}/policy  x-audit:true   (environment_policies)
```

### 6.10 UserService & GroupService

```
POST   /v1/users            x-requires-permission: db.users.create (workspaceAdmin)  x-audit:true
GET    /v1/users/{user}     x-requires-permission: db.users.get
GET    /v1/users            x-requires-permission: db.users.list
PATCH  /v1/users/{user}     x-requires-permission: db.users.update  x-audit:true
DELETE /v1/users/{user}     x-requires-permission: db.users.delete  x-audit:true   (默认禁用,D28)
POST   /v1/users/{user}:undelete
POST   /v1/users/me:password        x-auth-method: CUSTOM   (自身改密)
POST   /v1/users/me:mfa:regenerate  x-auth-method: CUSTOM
# Groups
POST   /v1/groups  / GET / PATCH / DELETE        x-requires-permission: db.groups.*  x-audit:true
POST   /v1/groups/{group}/members                x-audit:true
DELETE /v1/groups/{group}/members/{member}       x-audit:true
GET    /v1/groups/{group}/members
```

### 6.11 IDPService

```
POST   /v1/identityProviders              x-requires-permission: db.identityProviders.create  x-audit:true
GET    /v1/identityProviders/{idp}
GET    /v1/identityProviders              (可 x-allow-without-credential:登录页渲染 SSO 按钮)
PATCH  /v1/identityProviders/{idp}        x-audit:true
DELETE /v1/identityProviders/{idp}        x-audit:true
POST   /v1/identityProviders/{idp}:test   (用授权码测试,返回 IdentityProviderUserInfo)
```

### 6.12 WorksheetService / FavoriteService(收藏与分享)

```
POST   /v1/projects/{project}/worksheets                 x-auth-method: CUSTOM
GET    /v1/projects/{project}/worksheets/{worksheet}     x-auth-method: CUSTOM
GET    /v1/projects/{project}/worksheets:search          x-auth-method: CUSTOM
PATCH  /v1/projects/{project}/worksheets/{worksheet}     x-auth-method: CUSTOM  (If-Match etag)
DELETE /v1/projects/{project}/worksheets/{worksheet}     x-auth-method: CUSTOM
POST   /v1/projects/{project}/worksheets/{worksheet}:favorite    x-auth-method: CUSTOM
POST   /v1/projects/{project}/worksheets/{worksheet}:unfavorite  x-auth-method: CUSTOM
GET    /v1/users/me/favorites                            x-auth-method: CUSTOM
```

```yaml
Worksheet:
  type: object
  properties:
    name:       { type: string }
    project:    { type: string }
    database:   { type: string }
    title:      { type: string }
    content:    { type: string }
    visibility: { type: string }    # PRIVATE|PROJECT
    creator:    { type: string }
    etag:       { type: string }
Favorite:
  type: object
  properties:
    name:      { type: string }
    worksheet: { type: string }
```

> ShareLink(带 token 分享链接)不在 v1 范围(见 [18 §1.8](./18-roadmap.md));v1 通过 `visibility=PROJECT` 实现项目内分享。分享只传递 SQL 文本,执行仍按接收者本人权限(D18)。

### 6.13 NotificationService(站内通知)

```
GET   /v1/users/me/notifications                 x-auth-method: CUSTOM
POST  /v1/users/me/notifications/{id}:markRead   x-auth-method: CUSTOM
POST  /v1/users/me/notifications:markAllRead     x-auth-method: CUSTOM
GET   /v1/users/me/notifications:unreadCount     x-auth-method: CUSTOM
```

```yaml
Notification:
  type: object
  properties:
    name:        { type: string }
    type:        { type: string }    # export_done|system
    title:       { type: string }
    body:        { type: string }
    link:        { type: string }
    read_time:   { type: string, format: date-time }   # null=未读
    create_time: { type: string, format: date-time }
```

### 6.14 SettingService

```
GET   /v1/settings/{key}
PATCH /v1/settings/{key}    x-audit:true   (记前值)
```

> MaskingPolicyService(脱敏规则/豁免/分类/语义类型)不在 v1 范围(见 [18 §1.1](./18-roadmap.md))。

### 6.15 LSP — SQL 自动补全(核心,WebSocket)

> 自动补全为 V1 核心特性(成功指标:首字符候选 P95 ≤ 200ms,见 [00](./00-overview.md)、[03 §3](./03-sql-query.md))。协议为 **JSON-RPC 2.0 over WebSocket**(语言中立),与上述 REST 契约并行,不纳入 OpenAPI 的 REST 路径清单。

- **端点**:`wss://<host>/v1/lsp`(按库上下文:可接 `?database=projects/{p}/instances/{i}/databases/{d}`)。
- **鉴权**:WS 握手阶段以**首帧**或**子协议**携带 access token(校验逻辑同 REST);补全候选须限定在调用者**对该库持有 `db.sql.select`** 的范围内(按资源名解析的库做 ACL + 项目隔离)。
- **方法**:`textDocument/completion`(核心)、`textDocument/hover`、`textDocument/diagnostic`。
- **候选来源**:同步后的真实 catalog(表/列/视图/函数/关键字),取自元数据缓存(见 [07 §5](./07-resource-management.md))。
- 此端点为有状态长连接,服务端实现自选(语言不限);前端以标准 LSP 客户端接入。

---

## 7. 示例:一次查询的完整交互

**请求(多语句,首页)**
```
POST /v1/projects/orders/instances/pg-prod/databases/orders_db:query
Authorization: Bearer <jwt>
X-CSRF-Token: <token>
{ "statement": "SELECT id, region FROM customers LIMIT 100;", "limit": 100 }
```

**成功**(unary,一个结果集的首页)
```json
{
  "results": [{
    "column_names": ["id","region"],
    "rows": [{"values":[{"int_value":1},{"string_value":"east"}]}],
    "rows_count": 100,
    "next_page_token": "opaque-cursor-...",
    "latency": "0.42s"
  }]
}
```

**结果超行数上限**
```json
{ "code":412, "reason":"QUERY_ROW_LIMIT_EXCEEDED",
  "message":"结果超行数上限,请加过滤条件或改用导出" }
```

---

## 8. 交付说明

- `openapi.yaml` 为**唯一事实来源**,可直接由 openapi-generator 等工具生成**任意语言**的客户端 / 服务端桩与文档;前端据此生成 TS 类型与请求层。
- 自定义扩展 `x-requires-permission` / `x-audit` / `x-auth-method` / `x-allow-without-credential` 在 `openapi.yaml` 的 `components`/操作级声明,由服务端横切层在请求处理前读取并强制;具体实现语言与框架不限。
- LSP 补全为独立 WebSocket 契约(§6.15),语言中立。
- 尚未细化的点(各 CRUD 的完整字段、所有 List/Search 的 filter 形状)按 Google AIP 与本文约定补齐即可,不构成长期歧义。

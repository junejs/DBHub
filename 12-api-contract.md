# 12 — API 契约

> 本文档是系统对外 API 的**唯一事实来源**。proto 为源头,自动生成 Go server/client 与 OpenAPI/REST。交付团队据此即可前后端并行开发。
>
> 协议:**gRPC + Protobuf**,经 **Connect-RPC** 同时暴露 HTTP/JSON(REST 友好)。采用 Google AIP 风格的资源命名与标准方法。

---

## 1. 总则

- **proto 驱动**:每个 RPC 通过注解声明 `permission`(驱动 ACL 拦截器)与 `audit`(驱动审计拦截器),业务代码无法绕过(见 [01](./01-architecture.md)、[06](./06-audit-log.md))。
- **资源命名**:层级反映归属与隔离(项目 → 实例 → 库)。
- **标准方法**:`Get` / `List` / `Create` / `Update` / `Delete` + 自定义动词(`:query`、`:export`、`:search`、`:sync`、`:activate`)。
- **HTTP 映射**:Connect-RPC 自动映射,gRPC 状态码 ↔ HTTP 状态码(见 §4)。
- **版本**:所有路径前缀 `/v1`。不兼容变更升 `/v2`。

---

## 2. 资源命名约定

| 资源 | 名称模式 | 说明 |
|---|---|---|
| Project | `projects/{project}` | 团队隔离边界 |
| Environment | `environments/{environment}` | 平台级(dev/test/stage/prod) |
| Instance | `projects/{project}/instances/{instance}` | 归属项目 |
| Database | `projects/{project}/instances/{instance}/databases/{database}` | 项目归属由实例决定 |
| DatabaseCatalog | `.../databases/{database}/catalog` | 列标注 |
| DataSource | `.../instances/{instance}/dataSources/{dataSource}` | 实例下连接 |
| Worksheet | `projects/{project}/worksheets/{worksheet}` | 保存的查询 |
| Favorite | `users/{user}/favorites/{favorite}` | 个人收藏 |
| ShareLink | `worksheets/{worksheet}/shareLinks/{id}` | 分享链接 |
| AccessGrant | `projects/{project}/accessGrants/{grant}` | JIT |
| ExportTask | `projects/{project}/exportTasks/{task}` | 异步导出 |
| User / Group | `users/{user}` / `groups/{group}` | 平台级身份 |
| IDP | `identityProviders/{idp}` | 身份提供商 |
| AuditLog | `projects/{project}/auditLogs/{id}` 或 `auditLogs/{id}` | 项目级 / 全局 |
| Notification | `users/{user}/notifications/{id}` | 站内通知 |

> ID 规范:`{project}`/`{instance}` 等为小写 `[a-z][a-z0-9-]{1,62}`。数据库/表名按引擎大小写规则原样保留。

---

## 3. 通用约定

### 3.1 鉴权
- Web 端:HTTP-only Cookie(`access-token` / `refresh-token`)。
- API 端 / Service Account:`Authorization: Bearer <jwt>`。
- 未鉴权 RPC(登录类)注解 `allow_without_credential = true`。
- 用户私有数据(查询历史、收藏、通知)注解 `auth_method = CUSTOM`,按归属判定而非 IAM 权限。

### 3.2 分页
```proto
message PageRequest  { int32 page_size = 1; string page_token = 2; }
message ListResponse { repeated X items = 1; string next_page_token = 2; int32 total_size = 3; }
```
- `page_size` 默认 50、上限 5000(审计)。
- `page_token` 为不透明游标(不可构造)。
- 服务端用「limit+1」判定是否有下一页。

### 3.3 更新
- 所有 `Update*` 用 `google.protobuf.FieldMask update_mask` 声明改哪些字段。
- 部分资源支持 `allow_missing`(upsert 语义)与 `validate_only`(只校验不落库,如测试连接)。

### 3.4 软删除
- `Delete` 默认软删(`deleted_at`);`Undelete` 可恢复。硬删仅平台维护操作。

### 3.5 乐观锁(并发更新)
- 可被多人编辑的资源(如 `Worksheet`)带 `etag`;`Update` 时若服务端 etag 不匹配 → 返回 `ABORTED`(err `CONCURRENT_MODIFICATION`)。

### 3.6 幂等创建
- `Create*` 支持可选 `request_id`(客户端生成 UUID);相同 `request_id` 重试返回首次结果,防重复创建。

---

## 4. 错误模型与错误码目录

### 4.1 错误格式(Connect/gRPC)

错误 = **gRPC Code** + **message**(面向人)+ **details**(结构化,面向程序)。details 携带业务错误码与上下文:

```proto
// errors.proto
message ErrorInfo {
  string reason = 1;        // 业务错误码,如 "QUERY_COST_EXCEEDED"
  string domain = 2;        // "dbh"
  map<string, string> metadata = 3;  // 额外上下文(位置、阈值、资源名)
}
message FieldViolation { string field = 1; string description = 2; }
message BadRequest { repeated FieldViolation field_violations = 1; }
```

REST 端映射为:`{ "code": <int>, "message": "...", "details": [{ "@type": "...ErrorInfo", "reason": "..." }] }`。

### 4.2 gRPC Code ↔ HTTP

| gRPC Code | HTTP | 含义 |
|---|---|---|
| `INVALID_ARGUMENT` | 400 | 参数错误 |
| `UNAUTHENTICATED` | 401 | 未登录 / token 失效 |
| `PERMISSION_DENIED` | 403 | 无权限 |
| `NOT_FOUND` | 404 | 资源不存在 |
| `ALREADY_EXISTS` | 409 | 资源已存在 |
| `ABORTED` | 409 | 并发冲突(ETag) |
| `FAILED_PRECONDITION` | 412 | 前置条件不满足(如只读连接执行 DDL) |
| `RESOURCE_EXHAUSTED` | 429 | 限流 / 配额耗尽 |
| `CANCELLED` | 499 | 客户端取消 |
| `INTERNAL` | 500 | 内部错误 |
| `UNAVAILABLE` | 503 | 服务不可用(业务库连不上) |

### 4.3 业务错误码目录(reason)

| reason | gRPC Code | 触发场景 |
|---|---|---|
| `AUTH_INVALID_CREDENTIALS` | UNAUTHENTICATED | 账号/密码错误 |
| `AUTH_MFA_REQUIRED` | UNAUTHENTICATED | 需二次验证,返回 mfa_temp_token |
| `AUTH_MFA_INVALID` | UNAUTHENTICATED | OTP/恢复码错误 |
| `AUTH_ACCOUNT_LOCKED` | UNAUTHENTICATED | 失败次数过多被锁定 |
| `AUTH_TOKEN_EXPIRED` | UNAUTHENTICATED | access token 过期(提示 refresh) |
| `PERMISSION_DENIED` | PERMISSION_DENIED | 无对应权限 |
| `PROJECT_ISOLATION` | PERMISSION_DENIED | 跨项目访问被拒(资源不在调用者项目) |
| `PREDICATE_COLUMN_REJECTED` | PERMISSION_DENIED | 敏感列出现在 WHERE/JOIN |
| `RESOURCE_NOT_FOUND` | NOT_FOUND | 资源不存在或无权可见 |
| `RESOURCE_ALREADY_EXISTS` | ALREADY_EXISTS | 唯一冲突(如同名实例) |
| `CONCURRENT_MODIFICATION` | ABORTED | ETag 不匹配 |
| `INVALID_SQL` | INVALID_ARGUMENT | 语法错误(details.metadata.position) |
| `NON_READONLY_STATEMENT` | FAILED_PRECONDITION | 只读连接执行了 DDL/DML |
| `MULTI_STATEMENT_NOT_SUPPORTED` | INVALID_ARGUMENT | 该引擎不支持多语句 |
| `QUERY_COST_EXCEEDED` | FAILED_PRECONDITION | 成本超硬阈值(被拦截);metadata 带 threshold |
| `QUERY_ROW_LIMIT_EXCEEDED` | FAILED_PRECONDITION | 结果超行数上限 |
| `QUERY_TIMEOUT` | DEADLINE_EXCEEDED(504) | 语句超时 |
| `EXPORT_TOO_LARGE_FOR_SYNC` | FAILED_PRECONDITION | 同步导出预估 >1 万行,强制异步(D22) |
| `EXPORT_ARCHIVE_EXPIRED` | NOT_FOUND | 产物过期已清理 |
| `RATE_LIMITED` | RESOURCE_EXHAUSTED | 触发限流(查询/登录) |
| `DB_CONNECTION_FAILED` | UNAVAILABLE | 连不上业务库 |
| `ENGINE_NOT_SUPPORTED` | INVALID_ARGUMENT | 引擎能力不支持(如不支持脱敏的引擎查敏感列) |
| `VALIDATE_ONLY_FAILED` | FAILED_PRECONDITION | 测试连接失败 |

> 引擎原始错误(如 PG 错误码)作为 `ErrorInfo.metadata.engine_error` 透传,但涉敏感列时脱敏。

---

## 5. 公共类型(`common.proto`)

```proto
syntax = "proto3";
package dbh.v1;
import "google/protobuf/timestamp.proto";
import "google/protobuf/duration.proto";
import "google/protobuf/field_mask.proto";

enum ExportFormat { EXPORT_FORMAT_UNSPECIFIED = 0; CSV = 1; JSON = 2; SQL = 3; XLSX = 4; }

// 查询结果(查询/导出共用)
message QueryResult {
  repeated string column_names = 1;
  repeated string column_type_names = 2;
  repeated QueryRow rows = 3;
  int64 rows_count = 4;
  QueryError error = 5;
  google.protobuf.Duration latency = 6;
  string statement = 7;
  repeated string engine_messages = 8;        // NOTICE/PRINT
  repeated MaskingReason masking_reasons = 9;  // 每列脱敏说明
  string applied_access_grant = 10;           // 命中的 JIT
}
message QueryRow { repeated RowValue values = 1; }
message RowValue { oneof kind {
  google.protobuf.NullValue null_value = 1; bool bool_value = 2;
  int64 int_value = 3; double double_value = 4; string string_value = 5;
  bytes bytes_value = 6; google.protobuf.Timestamp time_value = 7;
  string struct_value = 8;  // JSON 字符串
}}
message QueryError {
  string message = 1;
  oneof detail {
    SyntaxError syntax_error = 2;       // 含位置
    PermissionDenied permission_denied = 3;
    CommandError command_error = 4;     // DDL/DML/非只读
    EngineError engine_error = 5;       // 引擎原始错误(脱敏后)
  }
}
message SyntaxError { int64 position = 1; string message = 2; }
message CommandError { string kind = 1; string message = 2; }   // ddl|dml|non_read_only
message EngineError { string code = 1; string message = 2; }
message MaskingReason { string column = 1; string semantic_type = 2; string algorithm = 3; }
```

---

## 6. 服务定义

> 下列 proto 为核心契约。`option (dbh.permission)`/`option (dbh.audit)` 为自定义注解(见 [02](./02-permission-and-access.md)、[06](./06-audit-log.md))。字段标注 OUTPUT_ONLY 的为服务端生成。

### 6.1 AuthService — 认证

```proto
service AuthService {
  rpc Login(LoginRequest) returns (LoginResponse)         { option (dbh.allow_without_credential)=true; option (dbh.audit)=true; }
  rpc Logout(LogoutRequest) returns (google.protobuf.Empty){ option (dbh.audit)=true; }
  rpc Refresh(RefreshRequest) returns (LoginResponse)      { option (dbh.allow_without_credential)=true; }
  rpc RequestPasswordReset(...) returns (...);
  rpc ResetPassword(...) returns (...);
}
message LoginRequest {
  string email = 1; string password = 2;
  // SSO: string idp = 3; string code = 4;  // OIDC/OAuth2 回调
  // MFA: string mfa_temp_token = 5; string otp_code = 6; string recovery_code = 7;
}
message LoginResponse {
  string access_token = 1; string refresh_token = 2;     // API 端;Web 端走 Cookie
  string mfa_temp_token = 3;                              // 需 MFA 时返回,access_token 为空
  User user = 4; string workspace = 5;
}
```

### 6.2 SQLService — 查询(核心)

```proto
service SQLService {
  rpc Query(QueryRequest) returns (stream QueryResult)    // 流式返回多语句结果
    { option (dbh.permission)="db.sql.select"; option (dbh.audit)=true; }
  rpc Export(ExportRequest) returns (ExportResponse)
    { option (dbh.permission)="db.sql.select"; option (dbh.audit)=true; }
  rpc SearchQueryHistories(SearchQueryHistoriesRequest) returns (ListQueryHistoriesResponse)
    { option (dbh.auth_method)=CUSTOM; }
}
message QueryRequest {
  string name = 1;            // projects/{p}/instances/{i}/databases/{d}
  string statement = 2;
  int32 limit = 3;            // 行数上限;默认取 environment_policies.query_row_limit
  string data_source_id = 4;  // 可选
  bool explain = 5;
  string schema = 6;          // search_path / current schema
}
// QueryResult 见 §5;流式时每个 statement 产出一个 QueryResult

message ExportRequest {
  string name = 1; string statement = 2; ExportFormat format = 3;
  string password = 4; int32 limit = 5;
}
message ExportResponse { bytes content = 1; string applied_access_grant = 2; }
// 预估 >1 万行 → 返回 EXPORT_TOO_LARGE_FOR_SYNC,改走 ExportTaskService
```

### 6.3 ExportTaskService — 异步导出

```proto
service ExportTaskService {
  rpc CreateExportTask(CreateExportTaskRequest) returns (ExportTask)
    { option (dbh.permission)="db.exports.create"; option (dbh.audit)=true; }
  rpc GetExportTask(GetExportTaskRequest) returns (ExportTask)   { option (dbh.auth_method)=CUSTOM; }
  rpc ListExportTasks(ListExportTasksRequest) returns (ListExportTasksResponse) { option (dbh.auth_method)=CUSTOM; }
  rpc DownloadExport(DownloadExportRequest) returns (stream bytes)
    { option (dbh.auth_method)=CUSTOM; option (dbh.audit)=true; }
}
message ExportTask {
  string name = 1;                  // projects/{p}/exportTasks/{id}
  string database = 2; string statement = 3; ExportFormat format = 4;
  string state = 5;                 // CREATED|RUNNING|SUCCEEDED|FAILED|EXPIRED
  string creator = 6;
  google.protobuf.Timestamp create_time = 7;
  google.protobuf.Timestamp complete_time = 8;
  int64 row_count = 9; int64 size_bytes = 10;
  string error = 11; string export_archive_id = 12; string applied_access_grant = 13;
}
message CreateExportTaskRequest {
  string parent = 1;                // projects/{p}
  ExportTask task = 2; string request_id = 3;  // 幂等
}
message DownloadExportRequest { string name = 1; string password = 2; }  // 校验 ZIP 密码
```

### 6.4 InstanceService — 实例(项目内)

```proto
service InstanceService {
  rpc CreateInstance(CreateInstanceRequest) returns (Instance)
    { option (dbh.permission)="db.instances.create"; option (dbh.audit)=true; }
  rpc GetInstance(...) returns (Instance)       { option (dbh.permission)="db.instances.get"; }
  rpc ListInstances(...) returns (ListInstancesResponse) { option (dbh.permission)="db.instances.list"; }
  rpc UpdateInstance(...) returns (Instance)    { option (dbh.permission)="db.instances.update"; option (dbh.audit)=true; }
  rpc DeleteInstance(...) returns (Instance)    { option (dbh.permission)="db.instances.delete"; option (dbh.audit)=true; }
  rpc SyncInstance(SyncInstanceRequest) returns (Operation)  // 手动同步
    { option (dbh.permission)="db.instances.sync"; option (dbh.audit)=true; }
  rpc AddDataSource(...) / UpdateDataSource(...) / RemoveDataSource(...)  // readonly 数据源增删改
  rpc TestConnection(TestConnectionRequest) returns (TestConnectionResponse)  // validate_only
    { option (dbh.permission)="db.instances.get"; }
}
message Instance {
  string name = 1;                  // projects/{p}/instances/{i}
  string project = 2;
  string title = 3; string engine = 4; string engine_version = 5;  // 5 OUTPUT_ONLY
  string environment = 6;           // environments/{env},必填
  bool activation = 7;
  int32 sync_interval_seconds = 8;
  google.protobuf.Timestamp last_sync_time = 9;  // OUTPUT_ONLY
  repeated DataSource data_sources = 10;
}
message CreateInstanceRequest {
  string parent = 1;                // projects/{p}
  string instance_id = 2; Instance instance = 3; bool validate_only = 4;
}
```

### 6.5 DatabaseService & DatabaseCatalogService

```proto
service DatabaseService {
  rpc GetDatabase(...) returns (Database)        { option (dbh.permission)="db.databases.get"; }
  rpc ListDatabases(...) returns (...)           { option (dbh.permission)="db.databases.list"; }
  rpc UpdateDatabase(...) returns (Database)     { option (dbh.permission)="db.databases.update"; option (dbh.audit)=true; }
  rpc SyncDatabaseSchema(...) returns (Operation){ option (dbh.permission)="db.databases.sync"; }
}
message Database {
  string name = 1;                  // projects/{p}/instances/{i}/databases/{d}
  string project = 2;               // OUTPUT_ONLY(= instance.project)
  string instance = 3; string environment = 4;  // 4 effective_environment OUTPUT_ONLY
  string sync_status = 5; string sync_error = 6;
  map<string,string> labels = 7;
}

service DatabaseCatalogService {
  rpc GetDatabaseCatalog(...) returns (DatabaseCatalog)    { option (dbh.permission)="db.databaseCatalogs.get"; }
  rpc UpdateDatabaseCatalog(...) returns (DatabaseCatalog) { option (dbh.permission)="db.databaseCatalogs.update"; option (dbh.audit)=true; }
}
message DatabaseCatalog { string name = 1; repeated SchemaCatalog schemas = 2; }
message ColumnCatalog { string name=1; string semantic_type=2; map<string,string> labels=3; string classification=4; }
```

### 6.6 IamService — 权限绑定(类 GCP IAM)

```proto
service IamService {
  rpc GetIamPolicy(GetIamPolicyRequest) returns (IamPolicy)
    { option (dbh.permission)="db.projects.getIamPolicy"; }   // 通用:scope 分 project/workspace
  rpc SetIamPolicy(SetIamPolicyRequest) returns (IamPolicy)
    { option (dbh.permission)="db.projects.setIamPolicy"; option (dbh.audit)=true; }  // 记 PolicyDelta
  rpc TestIamPermissions(...) returns (TestIamPermissionsResponse);
}
message IamPolicy { repeated Binding bindings = 1; string etag = 2; }
message Binding {
  string role = 1;                 // roles/sqlEditorReadUser
  repeated string members = 2;     // user:x@ / group:y@ / allUsers
  string condition = 3;            // CEL
}
message SetIamPolicyRequest { string resource = 1; IamPolicy policy = 2; string etag = 3; }  // 乐观锁
```

### 6.7 AccessGrantService — JIT 临时访问

```proto
service AccessGrantService {
  rpc CreateAccessGrant(...) returns (AccessGrant)  { option (dbh.permission)="db.accessGrants.create"; option (dbh.audit)=true; }
  rpc ActivateAccessGrant(...) returns (AccessGrant){ option (dbh.permission)="db.accessGrants.activate"; option (dbh.audit)=true; }  // 审批
  rpc RevokeAccessGrant(...) returns (AccessGrant)  { option (dbh.permission)="db.accessGrants.revoke"; option (dbh.audit)=true; }
  rpc ListAccessGrants(...) returns (...);
}
message AccessGrant {
  string name = 1;                 // projects/{p}/accessGrants/{id}
  string state = 2;                // PENDING|ACTIVE|REVOKED|EXPIRED
  string user = 3; string database = 4;
  string statement = 5;            // 可选,绑定精确 SQL
  bool unmask = 6; bool allow_export = 7;
  string reason = 8;
  google.protobuf.Timestamp expire_time = 9;
  string approved_by = 10; google.protobuf.Timestamp approved_time = 11;
}
// Create 后推 jit_pending 通知给审批人;Activate 后推 jit_resolved 给申请人(D21)
```

### 6.8 AuditLogService

```proto
service AuditLogService {
  rpc SearchAuditLogs(SearchAuditLogsRequest) returns (ListAuditLogsResponse)
    { option (dbh.permission)="db.auditLogs.search"; option (dbh.audit)=true; }  // 审计的审计
  rpc ExportAuditLogs(ExportAuditLogsRequest) returns (QueryResult)  // CSV/JSON/XLSX
    { option (dbh.permission)="db.auditLogs.export"; option (dbh.audit)=true; }
}
message AuditLog {
  string name = 1; google.protobuf.Timestamp create_time = 2;
  string method = 3; string user = 4; string resource = 5;
  string severity = 6; int32 status_code = 7; string status_message = 8;
  google.protobuf.Duration latency = 9; string request = 10; string response = 11;
  string ip = 12; string user_agent = 13;
}
message SearchAuditLogsRequest {
  string parent = 1;                // projects/{p} 或省略(全局,securityAdmin)
  string filter = 2;                // CEL: method/user/resource/severity/create_time
  int32 page_size = 3; string page_token = 4;
}
// 读权限范围见 [06 §5.1](./06-audit-log.md) D26
```

### 6.9 ProjectService & EnvironmentService

```proto
service ProjectService {
  rpc CreateProject / GetProject / ListProjects / UpdateProject / DeleteProject / UndeleteProject
  // CreateProject 需 workspaceAdmin;其余按项目角色
}
service EnvironmentService {
  rpc ListEnvironments(...) returns (...);     // 公开读
  rpc CreateEnvironment / UpdateEnvironment / DeleteEnvironment  // workspaceAdmin/securityAdmin
  rpc GetEnvironmentPolicy / UpdateEnvironmentPolicy             // environment_policies CRUD
}
```

### 6.10 UserService & GroupService

```proto
service UserService {
  rpc CreateUser / GetUser / ListUsers / UpdateUser / DeleteUser / UndeleteUser
  // 用户管理需 workspaceAdmin;自身可改密码/MFA(auth_method=CUSTOM)
  rpc RegenerateOwnMfa(...) / UpdateOwnPassword(...)
}
service GroupService {
  rpc CreateGroup / GetGroup / ListGroups / UpdateGroup / DeleteGroup
  rpc AddGroupMember / RemoveGroupMember / ListGroupMembers
}
```

### 6.11 IDPService

```proto
service IDPService {
  rpc CreateIdentityProvider / Get / List / Update / Delete
  rpc TestIdentityProvider(TestIdentityProviderRequest) returns (IdentityProviderUserInfo)  // 用授权码测试
  // ListIdentityProvider 可 allow_without_credential(登录页渲染 SSO 按钮)
}
```

### 6.12 WorksheetService / FavoriteService / ShareLinkService(收藏与分享)

```proto
service WorksheetService {
  rpc CreateWorksheet / GetWorksheet / SearchWorksheets / UpdateWorksheet / DeleteWorksheet
  // auth_method=CUSTOM;Update 带 etag(乐观锁)
}
message Worksheet {
  string name = 1; string project = 2; string database = 3;
  string title = 4; string content = 5; string visibility = 6;  // PRIVATE|PROJECT_READ|PROJECT_WRITE|LINK
  string creator = 7; string etag = 8;
}
service FavoriteService {
  rpc FavoriteWorksheet(FavoriteRequest) returns (Favorite)        // user_id+worksheet_id 唯一
  rpc UnfavoriteWorksheet(...) rpc ListMyFavorites(...)
  rpc UpdateFavoriteOrganizer(...)                                   // folder/note/pinned
}
message Favorite { string name=1; string worksheet=2; string folder=3; string note=4; bool pinned=5; }
service ShareLinkService {
  rpc CreateShareLink(...) returns (ShareLink)    // 生成随机 token;仅 visibility=LINK 时
  rpc RevokeShareLink(...) rpc OpenSharedWorksheet(...)   // 按 token 打开,仍校验登录+权限
}
message ShareLink { string worksheet=1; string token=2; google.protobuf.Timestamp expire_time=3; }  // token 仅创建时返回明文
```

> 分享只传递 SQL 文本,执行仍按接收者本人权限/脱敏(D18)。

### 6.13 NotificationService(站内通知)

```proto
service NotificationService {
  rpc ListNotifications(...) returns (...)       // auth_method=CUSTOM
  rpc MarkNotificationRead(...) / MarkAllRead(...)
  rpc GetUnreadCount(...) returns (Int32Value)
}
message Notification {
  string name = 1; string type = 2;   // export_done|jit_pending|jit_resolved|system
  string title = 3; string body = 4; string link = 5;
  google.protobuf.Timestamp read_time = 6; google.protobuf.Timestamp create_time = 7;
}
```

### 6.14 MaskingPolicyService & SettingService

```proto
service MaskingPolicyService {
  // 脱敏规则/豁免/数据分类/语义类型 的 CRUD;securityAdmin
  rpc GetMaskingRulePolicy / UpdateMaskingRulePolicy
  rpc GetMaskingExemptionPolicy / UpdateMaskingExemptionPolicy
  rpc ListSemanticTypes / ListDataClassifications
}
service SettingService {
  rpc GetSetting / UpdateSetting   // 平台级 key/value 配置;UpdateSetting 记前值审计
}
```

---

## 7. 示例:一次查询的完整交互

**请求**
```
POST /v1/projects/orders/instances/pg-prod/databases/orders_db:query
Authorization: Bearer <jwt>
{ "statement": "SELECT id, phone FROM customers WHERE region='east' LIMIT 100", "limit": 100 }
```

**成功**(流式,一个 QueryResult):
```json
{
  "column_names": ["id","phone"],
  "rows": [{"values":[{"int_value":1},{"string_value":"138****1234"}]}, ...],
  "rows_count": 100,
  "latency": "0.42s",
  "masking_reasons": [{"column":"phone","semantic_type":"phone","algorithm":"range"}]
}
```

**被成本护栏拦截**:
```json
{ "code":412, "message":"查询成本超阈值,请加过滤条件或改用导出",
  "details":[{"@type":"dbh.ErrorInfo","reason":"QUERY_COST_EXCEEDED",
              "metadata":{"threshold":"1000000","estimated":"5230000"}}] }
```

**谓词列被拒**(敏感列在 WHERE):
```json
{ "code":403, "reason":"PREDICATE_COLUMN_REJECTED",
  "message":"敏感列 phone 不可用于 WHERE/JOIN" }
```

---

## 8. 交付说明

- 本文档的 proto 可直接拆为 `.proto` 文件(`auth.proto`/`sql.proto`/...),`buf generate` 出 Go + OpenAPI。
- 自定义注解 `(dbh.permission)`/`(dbh.audit)`/`(dbh.auth_method)`/`(dbh.allow_without_credential)` 需在 `annotations.proto` 定义,拦截器在认证阶段读取。
- 尚未细化的点(各 CRUD 的完整字段、所有 List 的 filter 语法)按 Google AIP 与本文约定补齐即可,不构成长期歧义。

## 1. Dependencies & infra plumbing

- [ ] 1.1 Add `github.com/golang-migrate/migrate/v4` (with the pgx/v5 driver and file source) and `github.com/testcontainers/testcontainers-go/modules/postgres` to `backend/go.mod`; run `go mod tidy`; pin versions in `backend/internal/infra/infra.go`.
- [ ] 1.2 Add a new package `backend/internal/infra/db` exposing `func Open(ctx, dsn) (*bun.DB, error)` and `func Ping(ctx, *bun.DB) error` (bun on top of pgx/v5, single connection pool, no service-layer use).
- [ ] 1.3 Add a new package `backend/internal/infra/migrate` exposing `func Up(ctx, dsn) error` that runs `migrate.Up` against migrations embedded from `backend/internal/infra/migrations/*.sql` via `embed.FS`.

## 2. Migration SQL (DDL)

- [ ] 2.1 Create `backend/internal/infra/migrations/0001_v1_schema.sql` containing every CREATE TABLE / CREATE INDEX from PRD 10-data-model §3.1–§3.11 verbatim (users / groups / group_members / identity_providers / refresh_tokens / login_attempts / projects / environments / environment_policies / instances / data_sources / databases / database_schemas / sync_history / roles / role_permissions / role_assignments / query_history / worksheets / favorites / export_tasks / export_archives / notifications / settings) including the partial unique indexes (`users_email_uidx`, `instances_key_uidx`, `one_admin_ds`, `one_readonly_ds`, `databases_uidx`, etc.).
- [ ] 2.2 Create `backend/internal/infra/migrations/0002_audit_partitions.sql` declaring `audit_logs` as `PARTITION BY RANGE (created_at)` plus the current-month partition and a default catch-all partition.
- [ ] 2.3 Verify `golang-migrate` runs both files idempotently on a fresh DB and is a no-op on a populated DB.

## 3. Seed module (built-in roles / environments / policies)

- [ ] 3.1 Create `backend/internal/infra/permissions` holding a code-side registry of the v1 permission strings (read by both `role_permissions` seed and future IAM engine).
- [ ] 3.2 Create `backend/internal/infra/seed` exposing `func SeedBuiltin(ctx, *bun.DB) error` that upserts the four `environments`, their `environment_policies`, and the eight builtin `roles` with `ON CONFLICT DO NOTHING` on natural keys; then delete-inserts `role_permissions` from the registry. The function MUST be safe to call repeatedly.

## 4. First-admin bootstrap

- [ ] 4.1 Create `backend/internal/infra/bootstrap` exposing `func EnsureFirstAdmin(ctx, *bun.DB, email, password string) error`. Logic: `select 1 from users where deleted_at is null limit 1`; if any row, return nil. Otherwise insert a user with bcrypt(cost=12) `password_hash`, source=`local`, status=`active`. On unique-violation of `users_email_uidx` treat as already bootstrapped and return nil. MUST log only the user id + bootstrap status; the password MUST NEVER appear in logs.
- [ ] 4.2 Wire `EnsureFirstAdmin` into `main.go` startup between `migrate.Up` and `oas.NewServer`, reading `BOOTSTRAP_ADMIN_EMAIL` / `BOOTSTRAP_ADMIN_PASSWORD` from env (empty values: log warning, do not create).

## 5. PostgreSQL ProjectRepo

- [ ] 5.1 Create `backend/internal/service/project_postgres.go` with `BunProjectRepo` implementing `service.ProjectRepo` using bun. `List` uses keyset pagination on `projects.key`: `where projects.key > $cursor` ordered by `key asc`, returning `(items, next_key, err)`. Soft-deleted rows are excluded via `where deleted_at is null`.
- [ ] 5.2 Update `main.go` to construct `db` from `DB_DSN`, run `migrate.Up` + `seed.SeedBuiltin` + `bootstrap.EnsureFirstAdmin`, then wire `BunProjectRepo` into `ProjectService`. Remove the `NewMemoryProjectRepo(...)` call from production wiring.
- [ ] 5.3 Keep `MemoryProjectRepo` (rename / comment as test fixture allowed) so existing `project_test.go` and dev fixtures continue to work without a live PG.

## 6. Readyz gating

- [ ] 6.1 Add `Readyz` behavior so it pings the platform DB and returns 503 when the ping fails. `Healthz` remains a liveness probe (process up) and MUST NOT require DB.

## 7. Tests (unit + integration per 16-ops §7)

- [ ] 7.1 Add `backend/internal/infra/migrate/migrate_test.go` (testcontainers) — fresh DB applies all migrations; second run on the same DB is a no-op (no rows touched).
- [ ] 7.2 Add `backend/internal/infra/seed/seed_test.go` — first seed inserts the expected count of environments / roles / policies / permissions; second seed leaves the DB byte-identical (compare row counts).
- [ ] 7.3 Add `backend/internal/infra/bootstrap/bootstrap_test.go` — empty DB creates one user with bcrypt-hashed password; existing-admin DB does not create or modify; missing env vars do not create; capture stdout/stderr to assert the password never appears in any log line.
- [ ] 7.4 Add `backend/internal/service/project_postgres_test.go` (testcontainers) — `List` returns rows in `key` order; pagination with `page_token` resumes correctly across multiple pages; soft-deleted rows are excluded.
- [ ] 7.5 Existing `backend/internal/service/project_test.go` continues to pass using its fake repo (no behavior change at the service layer).

## 8. Configuration & docs

- [ ] 8.1 Update `backend/.env.example` to include `BOOTSTRAP_ADMIN_EMAIL` and `BOOTSTRAP_ADMIN_PASSWORD` (commented out by default), matching 16-ops §2.
- [ ] 8.2 Update `backend/Makefile`: `make migrate` now runs `go run ./cmd/migrate` (or equivalent) against the embedded migrations, replacing the current `TODO` placeholder.
- [ ] 8.3 Update `backend/internal/infra/infra.go` so that simply importing the infra package keeps all required deps (`pgx/v5`, `bun`, `migrate/v4`, `golang-jwt`) in `go.mod` and `go.sum`.

## 9. PR self-check (per IMPLEMENTATION_AGENT.md §1 step 5)

- [ ] 9.1 `cd backend && go fmt ./... && go vet ./... && go test -race ./... && golangci-lint run ./...` all pass.
- [ ] 9.2 `internal/oas/*` shows no diff (no contract change in this issue).
- [ ] 9.3 `main.go` no longer contains a `NewMemoryProjectRepo` call.
- [ ] 9.4 OpenSpec change `infra-platform-meta-db-foundation` is committed in this PR (on `git@github.com:junejs/DBHub.git`, NOT on the stale `git@gitee.com:junepy/dbhub-requirement.git` fork).
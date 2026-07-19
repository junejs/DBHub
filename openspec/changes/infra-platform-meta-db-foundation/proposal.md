## Why

DBHUB v1 is currently in-memory: `MemoryProjectRepo` ignores `pageToken` and the platform has no real persistence, no migration story, no seed data, and no first-run admin bootstrap. The PRD (10-data-model §3, 16-ops §1–2, decisions D1/D24/D45/D50) defines exactly what the v1 platform PostgreSQL must look like. This change establishes that foundation so every later feature (Instance / Database / IAM / query / export / audit) lands on real persistence with idempotent, testable bootstrap.

## What Changes

- Introduce PostgreSQL as the platform metadata store, accessed via `pgx/v5` + `bun` (D45), with a migration tool wired into startup (16-ops §1: migrate before ready).
- Author v1 DDL for every platform table (`users`, `groups`, `group_members`, `identity_providers`, `refresh_tokens`, `login_attempts`, `projects`, `environments`, `environment_policies`, `instances`, `data_sources`, `databases`, `database_schemas`, `sync_history`, `roles`, `role_permissions`, `role_assignments`, `query_history`, `worksheets`, `favorites`, `export_tasks`, `export_archives`, `audit_logs` partitioned by month, `notifications`, `settings`) plus all partial unique indexes from 10-data-model §3 / §6.
- Seed the four default `environments` (dev/test/stage/prod), their `environment_policies`, and the eight builtin `roles` from 10-data-model §4; insert role↔permission mappings from the v1 permission registry.
- Bootstrap the first admin idempotently from `BOOTSTRAP_ADMIN_EMAIL` / `BOOTSTRAP_ADMIN_PASSWORD` (16-ops §2): bcrypt cost 12, never logged, and ignored after the first admin exists.
- Replace `MemoryProjectRepo` with a bun-backed `ProjectRepo` so the existing `ProjectService.List` reads from real PostgreSQL; production wiring in `main.go` no longer constructs `MemoryProjectRepo`.
- Wire `GET /readyz` to verify platform PG connectivity before returning ready (16-ops §1).
- Provide automated tests: migration idempotency (re-run on populated DB is a no-op), seed idempotency, bcrypt first-admin behavior, and the new `ProjectRepo` against a real PostgreSQL via testcontainers (16-ops §7.2).

## Capabilities

### New Capabilities

- `platform-meta-db-foundation`: platform PostgreSQL persistence — migration mechanism, v1 DDL, partial unique indexes, audit monthly-partition bootstrap, idempotent first-admin creation, and the seeded builtin roles / environments / policies.
- `project-repo-postgresql`: bun-backed `ProjectRepo` that replaces `MemoryProjectRepo` for production wiring; keyset pagination on `projects.key`.

### Modified Capabilities

<!-- No existing spec files yet; left empty per OpenSpec guidance. -->

## Impact

- Backend (`backend/internal/infra`, `backend/internal/service/project.go`, `backend/internal/service/project_postgres.go`, `backend/internal/api/handler.go`, `backend/main.go`, `backend/Makefile`).
- Build: add `golang-migrate` (or equivalent) to dependencies; wire into `make migrate` and startup.
- Config: respect `DB_DSN`, `BOOTSTRAP_ADMIN_EMAIL`, `BOOTSTRAP_ADMIN_PASSWORD` from `16-ops` §2; remain backwards compatible with the existing `.env.example`.
- Observability: `/readyz` now blocks until PG is reachable and migrations applied; failure surfaces in slog.
- Frontend: no changes (this is infra-only).
- Repo: **THIS CHANGE RUNS ON `git@github.com:junejs/DBHub.git`** (the canonical DBHub monorepo). Previous runs landed on a stale fork (`git@gitee.com:junepy/dbhub-requirement.git`); those artifacts are abandoned — do not pull them forward.
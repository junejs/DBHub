## Context

DBHUB v1 currently ships with `MemoryProjectRepo` (dev/demo) and no real persistence layer. The PRD (10-data-model §3 + §4, 16-ops §1–2) and decisions D1/D24/D45/D50 together define exactly what v1 must look like: pgx/v5 + bun, all v1 tables with their constraints and partial unique indexes, an `audit_logs` table partitioned by month, four seeded `environments` + their `environment_policies`, eight builtin `roles`, and one idempotent first admin created from `BOOTSTRAP_ADMIN_EMAIL` / `BOOTSTRAP_ADMIN_PASSWORD`. Without this foundation, every later feature (Instance / Database / IAM / query / export / audit) cannot land — they all need real persistence with a working migration path.

**Repo scope**: this change runs on `git@github.com:junejs/DBHub.git`, the canonical DBHub monorepo. A previous run landed on a stale Gitee fork (`git@gitee.com:junepy/dbhub-requirement.git`); its artifacts are abandoned — do not pull them forward, do not rebase on its branch.

## Goals / Non-Goals

**Goals:**
- Establish the platform PostgreSQL as the single source of truth for v1 metadata.
- Embed an idempotent migration runner into the backend startup so an empty DB migrates + seeds automatically and a populated DB is a no-op.
- Author the full v1 DDL set (10-data-model §3.1–§3.11) with partial unique indexes, including the monthly `audit_logs` partition skeleton.
- Seed `environments` (dev/test/stage/prod) + `environment_policies` + 8 builtin `roles` + their `role_permissions` from a code-side permission registry.
- Bootstrap the first admin via bcrypt (cost 12, 16-ops §3) using `BOOTSTRAP_ADMIN_*` env vars; idempotent — never overwrites an existing admin.
- Provide a bun-backed `ProjectRepo` and rewire `main.go` to construct it from `DB_DSN`, removing `MemoryProjectRepo` from production wiring.
- Wire `GET /readyz` to require PG connectivity + applied migrations before returning ready.
- Provide automated tests covering migration idempotency, seed idempotency, first-admin bootstrap, and the new `ProjectRepo` (testcontainers per 16-ops §7.2).

**Non-Goals:**
- Implementing any v2 capability listed in `18-roadmap.md` (column masking, JIT, multi-engine, Service Account, custom roles, XLSX/SQL export, scheduled exports, link-sharing).
- Wiring real authentication / IAM engine / audit middleware — those are later issues; this one only plants the persistence foundation so they have something to read/write.
- Per-table repository implementations other than `projects` (Instance / Database / IAM repos land with their owning issues).
- Replacing `internal/infra/infra.go`'s "import-only" stub with a richer service container — keep it minimal: add `migrate`, `bootstrap`, `NewDB` constructors and have `main.go` call them.
- Touching `internal/oas/*` (generated, off-limits).

## Decisions

- **Migration tool**: use `github.com/golang-migrate/migrate/v4` with the `pgx/v5` driver and embed migration SQL via `embed.FS`. Rationale: simplest dependency-free driver for our stack; `embed.FS` ships migrations inside the binary so deploys do not need to copy SQL files alongside. Alternatives considered: `pressly/goose` (extra DSL we do not need), `bun.Migrator` (would force bun-specific migration conventions; less portable).
- **Migration entry point**: a new `internal/infra/migrate` package with `func Migrate(ctx, dsn) error` that runs `migrate.Up`. Called from `main.go` before the HTTP server starts so the process fails fast if the DB is unreachable or migrations are broken.
- **Schema source of truth**: a single migration file `0001_v1_schema.sql` (and `0002_audit_partition.sql` if needed) under `backend/internal/infra/migrations/` translated directly from 10-data-model §3.1–§3.11 — same DDL, same partial unique indexes, same audit monthly partitioning. The PRD is the design doc; this change just transcribes.
- **Bun vs raw SQL**: use bun for all application-layer queries (D45) but raw SQL for DDL migrations (migrations are DDL, not ORM work).
- **Audit partition strategy**: 16-ops §4 expects monthly partitions; v1 creates the partition for the current month on first migration and a default catch-all partition; full automation is out of scope (16-ops §6 acceptable: "分区不存在时自动建（或 pg_partman）").
- **First-admin bootstrap**: package `internal/infra/bootstrap` with `EnsureFirstAdmin(ctx, db, email, password)`. Logic: `select id from users where deleted_at is null order by id limit 1` — if a row exists, log and return; otherwise insert with bcrypt(cost=12). Uses `citext` + the partial unique index `users_email_uidx` from 10-data-model §3.1, which makes email collisions a no-op error that we treat as "already bootstrapped". Crucially: password never logged; we log only the user id and a success/failure status.
- **Seed mechanism**: same `bootstrap` package with `SeedBuiltin(ctx, db)`. Idempotent: every `INSERT` is guarded by an `ON CONFLICT DO NOTHING` keyed on the natural key (`roles.key`, `environments.key`). For `role_permissions` we delete-insert per role id (small fixed set, no production rows to disturb).
- **`ProjectRepo` rewrite**: keep `service.ProjectRepo` interface unchanged. New file `internal/service/project_postgres.go` with `BunProjectRepo`, using bun keyset pagination on `projects.key`. `MemoryProjectRepo` is kept temporarily (tests + future dev) but `main.go` no longer constructs it.
- **Config plumbing**: extend `.env.example` with `BOOTSTRAP_ADMIN_EMAIL` / `BOOTSTRAP_ADMIN_PASSWORD`. Reuse existing `DB_DSN`. No new env vars required.
- **Readyz gating**: a new helper `infra.Ping(ctx, dsn)` is called from `Readyz`; the handler returns 503 if ping fails. Migrations are run before the server starts, so by the time `Readyz` is reachable the schema is already applied.
- **Testing strategy**: per 16-ops §7.2, integration tests use testcontainers-go with PostgreSQL 17. Migration/seed/bootstrap tests hit a real container. Unit tests for `BunProjectRepo` use a fake `service.ProjectRepo`-shaped interface where possible; the bun-specific SQL is covered by integration tests only.

## Risks / Trade-offs

- [Migration runner brings a new dep] → pin the version in `go.mod` and add it to the backend `infra.go` lock-in list so the dependency cannot drift unnoticed.
- [First-admin idempotency relies on `users_email_uidx` partial unique index] → bootstrap wraps the insert in a transaction and treats unique-violation as success so two concurrent startups cannot double-create; covered by an explicit test.
- [Audit partition for current month only] → documented as a known gap; the next issue (audit middleware) must own partition rotation. Acceptable for v1 because 16-ops §1 only requires migrate-before-ready.
- [Removing `MemoryProjectRepo` from `main.go` while keeping the file] → some `go vet` warnings possible; tests in `project_test.go` continue to use fakes and `MemoryProjectRepo` is preserved as a test/dev fallback. Production wiring uses `BunProjectRepo` only.
- [DDL transcription errors] → every table/index is copied verbatim from PRD 10-data-model §3 and a follow-up review by CodeReviewer checks byte-for-byte alignment; tests pin key invariants (`instances_key_uidx`, `one_admin_ds`, `users_email_uidx`).
- [Bun vs pgx in different layers] → bun for service-layer reads/writes; pgx driver only for migrate (golang-migrate's pgx driver) and the connection pool. Single pool, single DSN, no drift.
- [Wrong-repo re-run risk] → previous run landed on a stale Gitee fork. To prevent recurrence: every agent (SolutionArchitect / Developer / Tester / CodeReviewer) MUST verify their `multica repo checkout` URL is `git@github.com:junejs/DBHub.git` before any work; if they detect a different URL, they abort and comment "wrong repo" back to PM rather than proceeding.

## Migration Plan

This change introduces persistence where there was none, so deploy ordering matters:

1. **Pre-deploy**: provision a PostgreSQL 17 instance reachable via `DB_DSN`; ensure `BOOTSTRAP_ADMIN_EMAIL` / `BOOTSTRAP_ADMIN_PASSWORD` are set in the deploy environment if first-run init is desired.
2. **Deploy**: rollout the new binary. On startup it runs migrations + seed + first-admin bootstrap before serving HTTP, so a fresh DB self-initializes.
3. **Verify**: `GET /readyz` returns 200; `psql` confirms expected tables; first admin can log in (post-auth issue).
4. **Rollback**: the previous binary had no DB dependency, so rollback is "stop the new binary, restart the old one". However, the new binary may have already created schema — that is harmless to the in-memory binary because nothing reads from it.
5. **No data migration** is required because there is no existing production data (D24: single-node MVP, no backup, no existing customers).

## Open Questions

- Should the first-admin bootstrap also seed a default `Project` for that admin (so the post-auth UI has somewhere to land)? → Decision: **no**. Project creation is its own feature; the admin lands on an empty project list, which matches the PRD.
- Should `MemoryProjectRepo` be deleted entirely once `BunProjectRepo` exists? → Decision: **no, keep it under `// +build dev` or rename to make clear it is a dev/demo fixture**, so `main.go`'s production wiring is the only thing that changes; existing tests still work without testcontainers.
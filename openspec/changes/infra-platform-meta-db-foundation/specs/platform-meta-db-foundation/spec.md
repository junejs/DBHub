## ADDED Requirements

### Requirement: Platform PostgreSQL is auto-migrated on startup
The system SHALL run all pending platform PostgreSQL migrations before the HTTP server becomes ready, so an empty database self-initializes on first boot.

#### Scenario: Fresh database self-initializes
- **WHEN** the backend process starts with an empty PostgreSQL reachable via `DB_DSN`
- **THEN** the system SHALL apply all migrations from `internal/infra/migrations/` and create every v1 table listed in PRD 10-data-model §3

#### Scenario: Populated database is a no-op
- **WHEN** the backend process starts with a PostgreSQL that already has all v1 tables applied
- **THEN** the system SHALL detect no pending migrations and start serving HTTP without altering existing data

#### Scenario: Migration failure blocks startup
- **WHEN** a migration fails (bad SQL, lost connection, permission error)
- **THEN** the system SHALL exit non-zero and SHALL NOT start the HTTP server

### Requirement: v1 schema matches PRD 10-data-model §3
The platform PostgreSQL schema MUST include every v1 table from PRD 10-data-model §3 with the exact columns, primary keys, foreign keys, default values, partial unique indexes, and regular indexes specified there. In particular the system SHALL create:
- `users` with `users_email_uidx` partial unique index on `lower(email)` where `deleted_at is null`
- `instances` with `instances_key_uidx` partial unique index on `(project_id, key)` where `deleted_at is null`
- `data_sources` with `one_admin_ds` partial unique index on `(instance_id)` where `role = 'admin'` and `one_readonly_ds` partial unique index on `(instance_id)` where `role = 'readonly'`
- `audit_logs` partitioned by range on `created_at` with at least the current-month partition and a default partition
- Every other table and index listed in 10-data-model §3.1–§3.11

#### Scenario: All v1 tables exist
- **WHEN** the platform DB is queried for the v1 table list
- **THEN** every table from PRD 10-data-model §3.1–§3.11 SHALL be present with the documented columns and indexes

#### Scenario: Partial unique indexes reject soft-deleted duplicates
- **WHEN** a soft-deleted row exists with key `K` and a new row is inserted with the same key `K` and `deleted_at is null`
- **THEN** the insert SHALL succeed because the partial unique index only covers `where deleted_at is null`

### Requirement: Seed data is idempotent
The system SHALL seed the four default `environments` (`dev`/`test`/`stage`/`prod`), their `environment_policies`, and the eight builtin `roles` (`workspaceAdmin`/`securityAdmin`/`workspaceMember`/`projectOwner`/`projectDBA`/`sqlEditorUser`/`sqlEditorReadUser`/`projectViewer`) together with their role↔permission mappings from the v1 permission registry. Re-running the seed MUST NOT duplicate rows or alter existing data.

#### Scenario: First seed inserts rows
- **WHEN** `SeedBuiltin` runs on an empty platform DB
- **THEN** the system SHALL insert the four environments, their policies, the eight roles, and the role↔permission mappings

#### Scenario: Re-running seed is a no-op
- **WHEN** `SeedBuiltin` runs on a platform DB that already contains the seed rows
- **THEN** the system SHALL make no changes (idempotent on the natural keys)

### Requirement: First admin is bootstrapped from environment variables
When the system starts and no active user exists, the system SHALL create a first admin user using `BOOTSTRAP_ADMIN_EMAIL` and `BOOTSTRAP_ADMIN_PASSWORD`. The password MUST be hashed with bcrypt cost 12 (16-ops §3) and MUST NEVER be written to logs. After the first admin exists, the env vars SHALL be ignored.

#### Scenario: Empty DB creates first admin
- **WHEN** the backend starts on an empty DB with `BOOTSTRAP_ADMIN_EMAIL=root@example.com` and a valid `BOOTSTRAP_ADMIN_PASSWORD`
- **THEN** the system SHALL create a user with that email, a bcrypt-hashed password, and a successful bootstrap log entry that does NOT contain the password

#### Scenario: Existing admin is not overwritten
- **WHEN** the backend starts on a DB that already contains at least one active user
- **THEN** the system SHALL NOT create, update, or delete that user regardless of the `BOOTSTRAP_ADMIN_*` env vars

#### Scenario: Missing env vars do not create an admin
- **WHEN** the backend starts on an empty DB without `BOOTSTRAP_ADMIN_EMAIL` or `BOOTSTRAP_ADMIN_PASSWORD`
- **THEN** the system SHALL NOT create a user, SHALL log a clear warning, and SHALL continue startup so an operator can create the admin manually

### Requirement: Readyz reports PG connectivity
`GET /readyz` SHALL return 200 only when the platform PostgreSQL is reachable AND all migrations are applied. If the DB is unreachable or migrations are pending, the endpoint SHALL return 503.

#### Scenario: Ready when DB is reachable
- **WHEN** the platform PostgreSQL responds to a ping
- **THEN** `/readyz` SHALL return 200

#### Scenario: Not ready when DB is unreachable
- **WHEN** the platform PostgreSQL does not respond to a ping
- **THEN** `/readyz` SHALL return 503
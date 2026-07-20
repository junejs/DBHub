# project-repo-postgresql Specification

## Purpose
TBD - created by archiving change infra-platform-meta-db-foundation. Update Purpose after archive.
## Requirements
### Requirement: ProjectRepo is backed by PostgreSQL in production
The production wiring in `main.go` SHALL construct `ProjectRepo` from `pgx/v5` + `bun` (D45), reading from the `projects` table. `MemoryProjectRepo` SHALL NOT be instantiated by `main.go`.

#### Scenario: ListProjects reads from PostgreSQL
- **WHEN** `/v1/projects` is called against the production binary
- **THEN** the system SHALL read rows from the `projects` table via bun, returning projects ordered by `key` ascending

#### Scenario: Pagination uses keyset cursor
- **WHEN** `ListProjects` is called with a non-empty `page_token`
- **THEN** the system SHALL resume pagination from the row after the supplied key using `where projects.key > $token` and SHALL return the next page's opaque token in `next_page_token`

#### Scenario: Empty page yields empty items
- **WHEN** `ListProjects` is called with no rows in `projects`
- **THEN** the system SHALL return `items = []` and an empty `next_page_token`

### Requirement: MemoryProjectRepo remains a test/dev fixture
`MemoryProjectRepo` MAY remain in the codebase as a test fixture or dev-mode fallback, but `main.go`'s production assembly MUST NOT use it.

#### Scenario: Production main does not construct MemoryProjectRepo
- **WHEN** `go build` produces the production binary
- **THEN** a `grep` for `NewMemoryProjectRepo` inside `main.go` SHALL return zero matches


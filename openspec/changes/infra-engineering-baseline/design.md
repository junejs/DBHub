## Context

ZZZ-85 targets the engineering infrastructure layer between the raw platform persistence (1.01) and the first business-domain feature. The current `main.go` has basic chi middleware (RequestID, RealIP, Logger, Recoverer, Timeout) and an ogen server with a passthrough security handler. Errors are returned as raw JSON without the PRD-prescribed `ErrorInfo.reason` structure. `/readyz` and `/healthz` are ogen-generated stubs that always return 200. There is no CI workflow in the repo, and no mechanism to detect OpenAPI/ogen drift.

This change transforms the skeleton into a hardened runtime that every subsequent Developer issue will build upon.

**Repo**: `git@github.com:junejs/DBHub.git` (verified; NOT the stale Gitee fork).

## Goals / Non-Goals

**Goals:**
- Provide a `Config` struct that loads all 16-ops §2 env vars, validates required fields at startup, and fails fast without leaking secrets in error messages.
- Provide a unified error response builder (`internal/service/errors.go` or equivalent) that emits `Error{code, message, details[{@type:"dbh.v1.ErrorInfo", reason, domain, metadata}]}` matching 12-api-contract §4. Include a mapping helper from domain errors to `reason` strings.
- Provide pagination helper functions (parse `page_size` with default 50, encode/decode opaque keyset `page_token`, build `LIMIT+1` queries) reusable across all future List endpoints.
- Wire `X-Request-Id` from chi middleware through slog context so every log line carries the request ID.
- Wire `/readyz` to ping the platform DB (reuse `infra/db.Ping`) and return 503 when unreachable; `/healthz` stays liveness-only.
- Add SIGTERM graceful shutdown (already partially present in main.go — verify 10s timeout works, add connection pool close).
- Add OpenAPI lint step (redocly or equivalent), ogen stale check (diff `internal/oas` against `make gen` output), and a CI workflow (`.github/workflows/ci.yml`) running backend lint/test and frontend lint/typecheck/test/build.
- Polish `docker-compose.yml` and `.env.example` for one-command local startup.
- Unit tests for error builder, pagination helper, config validation, and /readyz handler.

**Non-Goals:**
- Implementing auth, ACL, or audit middleware (later issues).
- Adding new API endpoints or DB tables.
- Editing `internal/oas/*` by hand.
- Any v2 capability from `18-roadmap.md`.

## Decisions

- **Error builder location**: `internal/service/` — the service layer already owns domain logic; a shared `errors.go` with `NewError(code, reason, msg)`, `WithMetadata(key, value)`, and `WithErrorInfo(reason, domain, meta)` keeps the builder accessible to all services without importing `api` or `oas`.
- **Pagination helper location**: `internal/service/pagination.go` — reusable `ParsePageSize(r *http.Request, default, max)`, `EncodeCursor(key string)`, `DecodeCursor(token string)` functions. Keyset uses base64-encoded JSON for now; opaque to clients.
- **Config struct location**: `internal/infra/config/config.go` — infrastructure concern, loaded once at startup, passed via constructor injection. Main.go calls `config.Load()` first; any validation failure exits with a user-friendly message that omits secret values.
- **CI tooling**: GitHub Actions (`.github/workflows/ci.yml`) with two jobs: `backend` (lint + test -race) and `frontend` (lint + typecheck + test + build). A third lightweight job `contract-check` runs `make gen` and asserts `internal/oas` has no diff. OpenAPI lint via `redocly lint openapi.yaml` if available, or skip for v1 (ogen already validates on gen).
- **OpenAPI stale check**: `cd backend && make gen && git diff --exit-code internal/oas/` in CI — if `openapi.yaml` was edited without regenerating, CI fails.
- **Docker Compose**: the existing `docker-compose.yml` already starts PG; verify it works with the current `main.go` startup sequence. Add comments showing the full local dev flow.

## Risks / Trade-offs

- [redocly not in go.mod] → CI contract-check job can rely solely on `make gen` + git diff for now; redocly lint is a nice-to-have that can be added later without breaking anything.
- [Pagination cursor format locked to base64-JSON] → if a more compact format is needed later, the encode/decode is centralized in one file; migration is a single PR.
- [Config validation order] → required-field checks happen before DB connection, matching the "fail fast" principle from the issue's acceptance criteria.

## Migration Plan

No data migration. This is pure code/infra plumbing on top of the existing platform PG from 1.01.

1. **Deploy**: merge the PR; the new binary starts with config validation + all existing 1.01 behavior.
2. **Verify**: `GET /readyz` returns 503 when PG is down, 200 when up. Error responses include `details[].reason`. CI is green.
3. **Rollback**: standard revert; no schema changes.

## Summary

Establish the shared backend runtime, contract governance, and CI baseline that all subsequent feature issues depend on. This change hardens the existing skeleton (main.go wiring, chi middleware, ogen server) into a production-ready runtime with unified error handling, structured logging, real health checks, config validation, graceful shutdown, and an automated CI pipeline that enforces contract consistency.

## Motivation

The 1.01 change (infra-platform-meta-db-foundation) delivered platform PostgreSQL persistence, migrations, seeds, and first-admin bootstrap. However the runtime plumbing remains minimal: errors are raw JSON, /readyz is a stub, there is no CI pipeline, and OpenAPI contract drift can go undetected. Every subsequent feature (auth, IAM, query, export) needs these shared foundations in place first.

## Scope

- Config loading with fast-fail validation (no secret leakage).
- Unified `Error{code, message, details[{reason, domain, metadata}]}` response builder with `ErrorInfo.reason` mapping (12-api-contract §4).
- Pagination helper (keyset cursor, page_size defaults, opaque page_token).
- `X-Request-Id` propagation and idempotent request dedup skeleton.
- Structured logging (slog) with request-scoped fields.
- `/readyz` wired to real platform PG ping; `/healthz` stays pure liveness.
- Graceful shutdown under SIGTERM with timeout.
- OpenAPI lint (redocly), ogen stale check (`internal/oas` diff), CI workflow.
- Backend lint/test, frontend lint/typecheck/test/build CI jobs.
- Docker Compose and `.env.example` polish for one-command local startup.

## Non-Goals

- No business domain implementation (no new API endpoints, no new DB tables).
- No auth/ACL/audit middleware (those are later issues).
- No `internal/oas` hand-edits (generated code stays off-limits).
- No v2 capabilities from `18-roadmap.md`.

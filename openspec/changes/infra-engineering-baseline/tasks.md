## 1. Config loading & validation

- [x] 1.1 Create `backend/internal/infra/config/config.go` with a `Config` struct holding all 16-ops §2 env vars (Port, DB_DSN, JWT_SECRET, MASTER_KEY, LOG_LEVEL, LOG_FORMAT, etc.). Provide `func Load() (*Config, error)` that reads `os.Getenv`, validates required fields (DB_DSN, JWT_SECRET, MASTER_KEY must be non-empty; PORT must parse as int; LOG_LEVEL/LOG_FORMAT must be in allowed sets), and returns an error that describes what is missing without including secret values in the message.
- [x] 1.2 Add unit tests for `config.Load()` — happy path with all vars set; each required var missing produces a clear error; secret values never appear in error messages; invalid PORT/LOG_LEVEL produces clear error.
- [x] 1.3 Wire `config.Load()` as the first call in `main.go` `run()`, before `db.Open`. Replace the current ad-hoc `os.Getenv` calls in main.go with the struct fields.

## 2. Unified error response builder

- [x] 2.1 Create `backend/internal/service/errors.go` with:
  - `type Error struct` matching 12-api-contract §4 (`Code int`, `Message string`, `Details []ErrorInfo`).
  - `ErrorInfo` struct (`Type, Reason, Domain string`, `Metadata map[string]string`).
  - `func NewError(code int, reason, msg string) *Error` — creates a single-detail error.
  - `func (e *Error) WithMetadata(key, value string) *Error` — fluent builder.
  - `func (e *Error) Error() string` — implements `error`.
  - `func WriteError(w http.ResponseWriter, err error)` — writes JSON response with appropriate HTTP status code.
  - A mapping helper `func ReasonForCode(code int) string` or domain-specific constructors (e.g. `ErrResourceNotFound(name)`, `ErrPermissionDenied()`, `ErrValidationError(field, desc)`).
- [x] 2.2 Add unit tests covering: error JSON output matches 12-api-contract §4 format; `WithMetadata` appends correctly; `WriteError` sets correct HTTP status; domain constructors produce correct `reason` and `code`.

## 3. Pagination helpers

- [x] 3.1 Create `backend/internal/service/pagination.go` with:
  - `func ParsePageSize(r *http.Request, defaultVal, maxVal int) int` — reads `page_size` query param, clamps to [1, maxVal].
  - `func EncodeCursor(key string) string` — base64-encodes a JSON cursor struct.
  - `func DecodeCursor(token string) (string, error)` — decodes and validates; returns error for malformed tokens.
  - `func HasNextPage(items []any, limit int) bool` — `len(items) > limit` check (limit+1 pattern).
- [x] 3.2 Add unit tests: page_size parsing with default/missing/invalid/over-max values; cursor encode-decode roundtrip; malformed cursor returns error; HasNextPage with exactly-limit and limit+1 items.

## 4. Structured logging & request context

- [x] 4.1 Replace the current `slog.NewJSONHandler(os.Stdout, nil)` with a handler that includes `request_id` from chi context when available. Add a chi middleware or handler wrapper that extracts `chi.RequestID(r)` and adds it to the slog context for the request lifetime.
- [x] 4.2 Add unit test or integration test verifying that log output contains `request_id` when a request is made through the middleware.

## 5. Readyz with real DB check

- [x] 5.1 Modify the existing ogen-generated readyz/healthz handlers (or override them in `api/handler.go`) so `/readyz` calls `infra/db.Ping(ctx)` against the platform DB and returns 503 with `{"code":503,"message":"not ready","details":[{"reason":"DB_NOT_READY"}]}` when ping fails. `/healthz` remains liveness-only (always 200).
- [x] 5.2 Add test: mock/override the ping to simulate failure → verify 503 response.

## 6. Graceful shutdown verification

- [x] 6.1 Verify the existing SIGTERM handler in `main.go` (10s timeout + `srv.Shutdown`) works correctly. Ensure the platform DB pool (`platformDB.Close()`) is closed in the deferred function after shutdown completes. Add a log line "server stopped" after successful shutdown.
- [x] 6.2 No new test strictly required (shutdown is integration-level), but a manual verification note in the PR description is acceptable.

## 7. CI workflow

- [x] 7.1 Create `.github/workflows/ci.yml` with three jobs triggered on push/PR to `main`:
  - `backend`: checkout → setup Go → `make lint` → `make test-race`
  - `frontend`: checkout → setup Node (pnpm) → `pnpm lint` → `pnpm typecheck` → `pnpm test` → `pnpm build`
  - `contract-check`: checkout → setup Go → `cd backend && make gen && git diff --exit-code internal/oas/`
- [x] 7.2 Verify CI passes on the branch after push.

## 8. Docker Compose & .env.example polish

- [x] 8.1 Review `docker-compose.yml` — ensure PG 17 starts with the correct port, DB name, user, and password matching `backend/.env.example`. Add comments documenting the full local dev flow (from `docker compose up db -d` through `make dev`).
- [x] 8.2 Review `backend/.env.example` — ensure it covers all 16-ops §2 vars with comments. Add `SYNC_INTERVAL`, `DEFAULT_LOCALE` if missing.

## 9. PR self-check (per IMPLEMENTATION_AGENT.md §1 step 5)

- [x] 9.1 `cd backend && go fmt ./... && go vet ./... && go test -race ./... && golangci-lint run ./...` all pass.
- [x] 9.2 `internal/oas/*` shows no diff (no contract change in this issue).
- [x] 9.3 CI workflow is syntactically valid (passes `actionlint` if available, or at minimum `python -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`).
- [x] 9.4 No `internal/oas/*` files were hand-edited.

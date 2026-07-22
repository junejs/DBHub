package api

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-faster/jx"
	"github.com/junepy/dbhub/backend/internal/oas"
	"github.com/junepy/dbhub/backend/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// failingPingDriver is a sql.Driver whose Conn always returns a sentinel
// error from PingContext. Registered once per package via init().
type failingPingDriver struct{}

func (failingPingDriver) Open(_ string) (driver.Conn, error) {
	return failingPingConn{}, nil
}

type failingPingConn struct{}

func (failingPingConn) Prepare(_ string) (driver.Stmt, error) { return nil, errFailing }
func (failingPingConn) Close() error                          { return nil }
func (failingPingConn) Begin() (driver.Tx, error)             { return nil, errFailing }

// Ping always fails — that's the whole point.
func (failingPingConn) Ping(_ context.Context) error { return errFailing }

var errFailing = errors.New("simulated platform DB unreachable")

// init registers the failing driver once for all tests in this package.
func init() {
	sql.Register("failing_ping", failingPingDriver{})
}

// newFailingPlatformDB returns a bun.DB whose PingContext always errors.
// Used to exercise the /readyz not-ready path without spinning up Postgres.
func newFailingPlatformDB(t *testing.T) *bun.DB {
	t.Helper()
	sqlDB, err := sql.Open("failing_ping", "unused")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return bun.NewDB(sqlDB, pgdialect.New())
}

// TestErrorHandler_ServiceError_RendersContractShape verifies the custom
// ogen ErrorHandler renders a *service.Error using the contract's Error
// schema with the correct HTTP status code and reason.
func TestErrorHandler_ServiceError_RendersContractShape(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	ErrorHandler(context.Background(), rec, req, service.ErrResourceNotFound("projects/foo"))

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

	got := mustDecodeOasError(t, rec.Body.Bytes())
	require.Equal(t, http.StatusNotFound, got.Code)
	require.Len(t, got.Details, 1)
	assert.Equal(t, service.ReasonResourceNotFound, got.Details[0].Reason.Value)
	assert.Equal(t, service.ErrorDomain, got.Details[0].Domain.Value)
}

// TestErrorHandler_UnknownError_FallsBackToInternal verifies that errors
// that are NOT a *service.Error are mapped to a generic 500 INTERNAL,
// without leaking the underlying message to the client.
func TestErrorHandler_UnknownError_FallsBackToInternal(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	ErrorHandler(context.Background(), rec, req, errors.New("oh no an internal detail"))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	got := mustDecodeOasError(t, rec.Body.Bytes())
	require.Equal(t, http.StatusInternalServerError, got.Code)
	require.Len(t, got.Details, 1)
	assert.Equal(t, service.ReasonInternal, got.Details[0].Reason.Value)
	// The internal error message must not leak into the response body.
	assert.NotContains(t, rec.Body.String(), "oh no an internal detail")
}

// mustDecodeOasError decodes a JSON body into oas.Error via the generated
// decoder, so the test asserts the same shape a real ogen client sees.
func mustDecodeOasError(t *testing.T, body []byte) oas.Error {
	t.Helper()
	var got oas.Error
	require.NoError(t, got.Decode(jx.DecodeBytes(body)))
	return got
}

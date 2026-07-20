package httplog

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestLogger builds a JSON logger that writes to the returned buffer.
func newTestLogger(t *testing.T, level string) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	return NewLogger(buf, "json", level), buf
}

func TestNewLogger_JSONLevelFiltering(t *testing.T) {
	logger, buf := newTestLogger(t, "warn")
	logger.Info("ignored")
	logger.Warn("emitted")
	out := buf.String()
	assert.NotContains(t, out, "ignored")
	assert.Contains(t, out, "emitted")
}

func TestNewLogger_TextFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(buf, "text", "info")
	logger.Info("hi")
	out := buf.String()
	// Text format emits "level=INFO msg=hi".
	assert.Contains(t, out, "level=INFO")
	assert.Contains(t, out, "msg=hi")
}

func TestFromContext_EmptyWhenMissing(t *testing.T) {
	assert.Empty(t, FromContext(context.Background()))
}

func TestWithRequestID_RoundTrip(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-123")
	assert.Equal(t, "req-123", FromContext(ctx))
}

func TestWithRequestID_EmptyIDNoOp(t *testing.T) {
	ctx := WithRequestID(context.Background(), "")
	// Returns same context shape; FromContext yields "".
	assert.Empty(t, FromContext(ctx))
}

// TestRequestIDMiddleware_Flow runs a request through chi + RequestID +
// httplog.RequestIDMiddleware and asserts the slog output contains the
// request_id emitted by the handler.
func TestRequestIDMiddleware_Flow(t *testing.T) {
	logger, buf := newTestLogger(t, "info")

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(RequestIDMiddleware)
	r.Get("/v1/echo", func(w http.ResponseWriter, r *http.Request) {
		// Use the request context so request_id is attached.
		logger.InfoContext(r.Context(), "handling")
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/echo", nil)
	req.Header.Set("X-Request-Id", "req-abc-123")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	// Inspect the emitted JSON log line.
	var found bool
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry), "line: %s", line)
		if rid, ok := entry["request_id"].(string); ok && rid == "req-abc-123" {
			found = true
			break
		}
	}
	assert.True(t, found, "request_id should be in the log output; got: %s", buf.String())
}

// TestRequestIDMiddleware_NoRequestID is the negative path: middleware is
// a no-op when chi's RequestID middleware is not in front of it.
func TestRequestIDMiddleware_NoRequestID(t *testing.T) {
	logger, buf := newTestLogger(t, "info")

	r := chi.NewRouter()
	r.Use(RequestIDMiddleware)
	r.Get("/v1/echo", func(w http.ResponseWriter, r *http.Request) {
		logger.InfoContext(r.Context(), "handling")
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/echo", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// No request_id field at all.
	assert.NotContains(t, buf.String(), "request_id")
}

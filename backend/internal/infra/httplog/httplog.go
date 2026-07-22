// Package httplog provides request-scoped structured logging helpers.
//
// slog is the project's logger (16-ops §5, NFR §1.5). To carry the request id
// from chi's RequestID middleware into every log line emitted during a
// request, two pieces are wired together:
//
//  1. The slog default handler is built once at startup via NewLogger. It
//     reads attributes off the logging context using a Handle hook, so any
//     code that calls slog.InfoContext(ctx, ...) within a request handler
//     picks up the request_id automatically.
//
//  2. The chi RequestIDMiddleware injects chi's request id into the context
//     used by slog. Handlers must call slog.WithContext(ctx) (or use the
//     Info/Error helpers that take ctx) to opt in.
//
// Code that does not have an *http.Request in scope (background goroutines,
// startup, shutdown) just uses the default logger and gets no request_id —
// which is the correct behaviour.
package httplog

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// CtxKey is the context key under which the request id string is stored for
// slog consumption. It is unexported so callers cannot fudge it; use the
// helpers in this package.
type ctxKey struct{}

// String returns the request id stored in ctx, or "" when none is set.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}

// WithRequestID returns ctx augmented with the given request id, so any
// slog call that receives the resulting context will emit request_id.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, id)
}

// NewLogger returns a *slog.Logger whose handler emits the request_id field
// when present in the logging context. format selects between JSON ("json")
// and development-friendly text ("text"); level selects the minimum severity.
//
// The returned logger DOES NOT register itself as the default — callers
// should slog.SetDefault(...) once at startup. Tests can use this to build
// isolated loggers writing to a buffer.
func NewLogger(w io.Writer, format, level string) *slog.Logger {
	lvl := parseLevel(level)
	opts := &slog.HandlerOptions{
		Level: lvl,
	}
	var h slog.Handler
	if format == "text" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	// Wrap so we can splice in request_id from the context.
	wrapped := slog.New(contextHandler{Handler: h})
	return wrapped
}

// contextHandler delegates to the underlying Handler but pre-pends any
// request_id stored in the logging context as an attribute.
type contextHandler struct {
	slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := FromContext(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

// RequestIDMiddleware is a chi-style middleware that takes the request id
// produced by chi/middleware.RequestID and pushes it into the slog context
// via WithRequestID. Downstream handlers must use slog.InfoContext(ctx, ...)
// or pass ctx to the logger; code that uses the package-level slog.Default()
// without ctx will NOT get the request id (this is by design — there is no
// goroutine-local way to find it).
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := middleware.GetReqID(r.Context())
		if id == "" {
			next.ServeHTTP(w, r)
			return
		}
		ctx := WithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// parseLevel converts the LOG_LEVEL env value (debug|info|warn|error) to a
// slog.Level. Unknown values default to LevelInfo.
func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

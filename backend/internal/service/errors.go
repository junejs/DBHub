package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ErrorInfoType is the @type discriminator for an ErrorInfo detail block.
// Matches the contract example in 12-api-contract.md §4.1.
const ErrorInfoType = "dbh.v1.ErrorInfo"

// ErrorDomain is the canonical domain string written into ErrorInfo.domain.
const ErrorDomain = "dbh"

// Reason constants for the most commonly used business error codes
// (12-api-contract.md §4.3). Domain code can refer to these instead of typing
// the string literal everywhere; new reasons are added here as features land.
const (
	ReasonPermissionDenied       = "PERMISSION_DENIED"
	ReasonProjectIsolation       = "PROJECT_ISOLATION"
	ReasonResourceNotFound       = "RESOURCE_NOT_FOUND"
	ReasonResourceAlreadyExists  = "RESOURCE_ALREADY_EXISTS"
	ReasonConcurrentModification = "CONCURRENT_MODIFICATION"
	ReasonRateLimited            = "RATE_LIMITED"
	ReasonDBConnectionFailed     = "DB_CONNECTION_FAILED"
	ReasonDBNotReady             = "DB_NOT_READY" // readiness/internal
	ReasonInternal               = "INTERNAL"
	ReasonValidationFailed       = "VALIDATION_FAILED"
)

// ErrorInfo mirrors a single entry of Error.details[] from 12-api-contract §4.1.
// One ErrorInfo carries the stable program-readable reason and free-form
// metadata; field-level violations live on FieldViolations when present.
type ErrorInfo struct {
	Type            string            `json:"@type"`
	Reason          string            `json:"reason"`
	Domain          string            `json:"domain"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	FieldViolations []FieldViolation  `json:"field_violations,omitempty"`
}

// FieldViolation describes a single field-level validation failure
// (BadRequest detail body, 12-api-contract §4.1).
type FieldViolation struct {
	Field       string `json:"field"`
	Description string `json:"description"`
}

// Error is the unified service-layer error type. It implements the `error`
// interface so it can be returned from any service method, and carries the
// structured details needed to render the contract's Error response shape.
//
// The api layer (internal/api) maps *Error to the ogen-generated
// oas.ErrorStatusCode; chi-level middleware and any non-ogen routes use
// WriteError to emit the JSON directly. Either way the wire shape is the
// same: {"code":int,"message":string,"details":[...]}.
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Details []ErrorInfo `json:"details"`
	// cause is wrapped but never serialised; allows errors.Is/As to work
	// end-to-end without leaking the underlying error to clients.
	cause error `json:"-"`
}

// Error implements the error interface. The string form intentionally omits
// secret-bearing metadata values — only reason and HTTP code are surfaced.
func (e *Error) Error() string {
	if e == nil {
		return "<nil service.Error>"
	}
	if len(e.Details) > 0 {
		return fmt.Sprintf("service.Error: code=%d reason=%s msg=%q", e.Code, e.Details[0].Reason, e.Message)
	}
	return fmt.Sprintf("service.Error: code=%d msg=%q", e.Code, e.Message)
}

// Unwrap allows errors.Is / errors.As to traverse to the wrapped cause.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// WithCause attaches a wrapped underlying error (for logging / errors.Is) that
// is NOT serialised to the wire response.
func (e *Error) WithCause(err error) *Error {
	e.cause = err
	return e
}

// WithMetadata appends key/value pairs to the FIRST ErrorInfo's metadata map.
// If the Error has no details yet (rare), one is created with the package's
// default reason. Metadata values are stored as strings per the contract.
//
// This is a fluent builder: returns the same *Error pointer for chaining.
func (e *Error) WithMetadata(key, value string) *Error {
	if len(e.Details) == 0 {
		e.Details = []ErrorInfo{{Type: ErrorInfoType, Reason: ReasonInternal, Domain: ErrorDomain}}
	}
	if e.Details[0].Metadata == nil {
		e.Details[0].Metadata = map[string]string{}
	}
	e.Details[0].Metadata[key] = value
	return e
}

// WithDetail appends an additional ErrorInfo block to Details.
func (e *Error) WithDetail(d ErrorInfo) *Error {
	if d.Type == "" {
		d.Type = ErrorInfoType
	}
	if d.Domain == "" {
		d.Domain = ErrorDomain
	}
	e.Details = append(e.Details, d)
	return e
}

// NewError constructs a single-detail Error. The first ErrorInfo is the
// canonical reason block; further details can be added with WithDetail.
func NewError(code int, reason, msg string) *Error {
	return &Error{
		Code:    code,
		Message: msg,
		Details: []ErrorInfo{{
			Type:   ErrorInfoType,
			Reason: reason,
			Domain: ErrorDomain,
		}},
	}
}

// ReasonForCode returns a default reason string for a given HTTP status code,
// when no more specific reason is available. Used by the recovery middleware
// when mapping an unexpected panic / generic error to a 5xx.
func ReasonForCode(code int) string {
	switch {
	case code == http.StatusServiceUnavailable:
		return ReasonDBConnectionFailed
	case code == http.StatusNotFound:
		return ReasonResourceNotFound
	case code == http.StatusForbidden:
		return ReasonPermissionDenied
	case code == http.StatusUnauthorized:
		return ReasonPermissionDenied
	case code == http.StatusConflict:
		return ReasonResourceAlreadyExists
	case code == http.StatusTooManyRequests:
		return ReasonRateLimited
	case code == http.StatusGatewayTimeout:
		return ReasonDBConnectionFailed
	case code >= 500:
		return ReasonInternal
	case code >= 400:
		return ReasonValidationFailed
	default:
		return ReasonInternal
	}
}

// ---------------------------------------------------------------------------
// Domain constructors — preferred way for service code to produce a *Error.
// ---------------------------------------------------------------------------

// ErrResourceNotFound builds a 404 with the resource name in metadata.
func ErrResourceNotFound(resourceName string) *Error {
	return NewError(http.StatusNotFound, ReasonResourceNotFound,
		"resource not found or invisible to caller").
		WithMetadata("resource", resourceName)
}

// ErrPermissionDenied builds a 403 with PERMISSION_DENIED reason.
func ErrPermissionDenied() *Error {
	return NewError(http.StatusForbidden, ReasonPermissionDenied,
		"caller lacks the required permission")
}

// ErrProjectIsolation builds a 403 for cross-project access attempts.
func ErrProjectIsolation(projectKey string) *Error {
	return NewError(http.StatusForbidden, ReasonProjectIsolation,
		"resource is outside the caller's project").
		WithMetadata("project", projectKey)
}

// ErrResourceAlreadyExists builds a 409 with the conflicting identifier.
func ErrResourceAlreadyExists(resourceName string) *Error {
	return NewError(http.StatusConflict, ReasonResourceAlreadyExists,
		"resource already exists").
		WithMetadata("resource", resourceName)
}

// ErrConcurrentModification builds a 409 for ETag mismatch.
func ErrConcurrentModification(resource, got string) *Error {
	return NewError(http.StatusConflict, ReasonConcurrentModification,
		"etag mismatch: resource was modified concurrently").
		WithMetadata("resource", resource).
		WithMetadata("if_match", got)
}

// ErrValidationFailed builds a 400 carrying one or more field violations.
func ErrValidationFailed(violations ...FieldViolation) *Error {
	e := NewError(http.StatusBadRequest, ReasonValidationFailed,
		"request validation failed")
	e.Details[0].FieldViolations = violations
	return e
}

// ErrRateLimited builds a 429 carrying the limit metadata.
func ErrRateLimited(limit string) *Error {
	return NewError(http.StatusTooManyRequests, ReasonRateLimited,
		"rate limit exceeded").
		WithMetadata("limit", limit)
}

// ErrDBConnectionFailed builds a 503 for unreachable business DB.
func ErrDBConnectionFailed(instance string) *Error {
	return NewError(http.StatusServiceUnavailable, ReasonDBConnectionFailed,
		"business database unreachable").
		WithMetadata("instance", instance)
}

// ErrDBNotReady builds a 503 used by /readyz when the platform DB is down.
// Distinct from DB_CONNECTION_FAILED (which is about business DBs the user
// queried): DB_NOT_READY means the control plane itself cannot answer.
func ErrDBNotReady() *Error {
	return NewError(http.StatusServiceUnavailable, ReasonDBNotReady,
		"platform DB unreachable")
}

// ErrInternal wraps an unexpected error as a 500 INTERNAL. The cause is
// attached for logging via WithCause, but not serialised to clients.
func ErrInternal(err error) *Error {
	return NewError(http.StatusInternalServerError, ReasonInternal,
		"internal server error").WithCause(err)
}

// AsError extracts a *Error from any error chain; returns nil if not present.
// Convenience for callers that want to inspect reason / code without a type
// assertion dance.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

// ---------------------------------------------------------------------------
// HTTP rendering
// ---------------------------------------------------------------------------

// WriteError writes the contract Error JSON to w. It accepts either a *Error
// (rendered verbatim with its own Code) or any other error (rendered as a
// 500 INTERNAL with no detail body). The Content-Type and status code are set
// before the body so callers do not need to manage them.
//
// This is intended for routes outside ogen (e.g. chi middleware like
// recoverer / not-found handler) and for tests; ogen routes return an
// *oas.ErrorStatusCode which ogen's encoder renders with the same shape.
func WriteError(w http.ResponseWriter, err error) {
	if err == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	var se *Error
	if errors.As(err, &se) {
		writeServiceError(w, se)
		return
	}
	// Unknown error — do not leak its message; emit a generic 500.
	se = ErrInternal(err)
	writeServiceError(w, se)
}

func writeServiceError(w http.ResponseWriter, e *Error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(e.Code)
	body, err := json.Marshal(e)
	if err != nil {
		// Marshal of a struct with simple fields cannot fail in practice;
		// fall back to a minimal body rather than spinning forever.
		_, _ = w.Write([]byte(`{"code":500,"message":"failed to render error"}`))
		return
	}
	_, _ = w.Write(body)
}

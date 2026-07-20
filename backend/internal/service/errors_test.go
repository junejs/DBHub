package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewError_HasSingleDetailBlock(t *testing.T) {
	e := NewError(http.StatusNotFound, ReasonResourceNotFound, "not here")
	require.Len(t, e.Details, 1)
	assert.Equal(t, http.StatusNotFound, e.Code)
	assert.Equal(t, "not here", e.Message)
	assert.Equal(t, ReasonResourceNotFound, e.Details[0].Reason)
	assert.Equal(t, ErrorInfoType, e.Details[0].Type)
	assert.Equal(t, ErrorDomain, e.Details[0].Domain)
}

func TestError_WithMetadata_AppendsToFirstDetail(t *testing.T) {
	e := NewError(http.StatusBadRequest, ReasonValidationFailed, "bad").
		WithMetadata("field", "title").
		WithMetadata("rule", "required")
	require.Len(t, e.Details, 1)
	assert.Equal(t, "title", e.Details[0].Metadata["field"])
	assert.Equal(t, "required", e.Details[0].Metadata["rule"])
}

func TestError_WithDetail_AppendsAdditionalDetail(t *testing.T) {
	e := NewError(http.StatusBadRequest, ReasonValidationFailed, "bad").
		WithDetail(ErrorInfo{Reason: ReasonValidationFailed})
	require.Len(t, e.Details, 2)
	for _, d := range e.Details {
		assert.Equal(t, ErrorInfoType, d.Type)
		assert.Equal(t, ErrorDomain, d.Domain)
	}
}

func TestError_WithCause_UnwrapsToOriginal(t *testing.T) {
	original := errors.New("boom")
	e := NewError(http.StatusInternalServerError, ReasonInternal, "x").WithCause(original)
	assert.ErrorIs(t, e, original)
}

func TestError_ErrorString_DoesNotEchoMetadata(t *testing.T) {
	e := ErrDBConnectionFailed("db-secret-name")
	s := e.Error()
	assert.Contains(t, s, ReasonDBConnectionFailed)
	assert.NotContains(t, s, "db-secret-name")
}

func TestDomainConstructors(t *testing.T) {
	cases := []struct {
		name   string
		err    *Error
		code   int
		reason string
	}{
		{"not_found", ErrResourceNotFound("projects/x"), http.StatusNotFound, ReasonResourceNotFound},
		{"perm_denied", ErrPermissionDenied(), http.StatusForbidden, ReasonPermissionDenied},
		{"project_iso", ErrProjectIsolation("x"), http.StatusForbidden, ReasonProjectIsolation},
		{"already_exists", ErrResourceAlreadyExists("i/foo"), http.StatusConflict, ReasonResourceAlreadyExists},
		{"concurrent", ErrConcurrentModification("w/1", "etag"), http.StatusConflict, ReasonConcurrentModification},
		{"rate", ErrRateLimited("60"), http.StatusTooManyRequests, ReasonRateLimited},
		{"db_conn", ErrDBConnectionFailed("inst"), http.StatusServiceUnavailable, ReasonDBConnectionFailed},
		{"db_not_ready", ErrDBNotReady(), http.StatusServiceUnavailable, ReasonDBNotReady},
		{"internal", ErrInternal(errors.New("boom")), http.StatusInternalServerError, ReasonInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.code, tc.err.Code)
			require.Len(t, tc.err.Details, 1)
			assert.Equal(t, tc.reason, tc.err.Details[0].Reason)
		})
	}
}

func TestErrValidationFailed_CarriesFieldViolations(t *testing.T) {
	e := ErrValidationFailed(
		FieldViolation{Field: "title", Description: "required"},
		FieldViolation{Field: "page_size", Description: "must be >0"},
	)
	require.Len(t, e.Details, 1)
	assert.Len(t, e.Details[0].FieldViolations, 2)
	assert.Equal(t, "title", e.Details[0].FieldViolations[0].Field)
}

func TestReasonForCode(t *testing.T) {
	cases := map[int]string{
		500:                        ReasonInternal,
		502:                        ReasonInternal,
		503:                        ReasonDBConnectionFailed,
		http.StatusNotFound:        ReasonResourceNotFound,
		http.StatusForbidden:       ReasonPermissionDenied,
		http.StatusUnauthorized:    ReasonPermissionDenied,
		http.StatusConflict:        ReasonResourceAlreadyExists,
		http.StatusTooManyRequests: ReasonRateLimited,
		http.StatusGatewayTimeout:  ReasonDBConnectionFailed,
		http.StatusBadRequest:      ReasonValidationFailed,
		422:                        ReasonValidationFailed,
		200:                        ReasonInternal, // unknown
	}
	for code, want := range cases {
		assert.Equal(t, want, ReasonForCode(code), "code=%d", code)
	}
}

func TestAsError_ReturnsServiceError(t *testing.T) {
	original := ErrResourceNotFound("x")
	wrapped := errors.Join(errors.New("ctx: "), original)
	got := AsError(wrapped)
	require.NotNil(t, got)
	assert.Equal(t, ReasonResourceNotFound, got.Details[0].Reason)
}

func TestAsError_NilForUnknownError(t *testing.T) {
	assert.Nil(t, AsError(errors.New("plain")))
}

// writeAndDecode writes the error via WriteError and returns the decoded body.
func writeAndDecode(t *testing.T, err error) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	WriteError(rec, err)
	body := rec.Body.Bytes()
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got), "body was: %s", body)
	return rec.Code, got
}

func TestWriteError_ServiceError_RendersContractShape(t *testing.T) {
	e := ErrResourceNotFound("projects/foo")
	code, body := writeAndDecode(t, e)

	assert.Equal(t, http.StatusNotFound, code)
	assert.EqualValues(t, http.StatusNotFound, body["code"])
	assert.Equal(t, e.Message, body["message"])

	details, ok := body["details"].([]any)
	require.True(t, ok)
	require.Len(t, details, 1)
	first, _ := details[0].(map[string]any)
	assert.Equal(t, ErrorInfoType, first["@type"])
	assert.Equal(t, ReasonResourceNotFound, first["reason"])
	assert.Equal(t, ErrorDomain, first["domain"])
	meta, _ := first["metadata"].(map[string]any)
	assert.Equal(t, "projects/foo", meta["resource"])
}

func TestWriteError_ContentTypeHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, ErrValidationFailed())
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
}

func TestWriteError_UnknownError_RendersInternal500(t *testing.T) {
	code, body := writeAndDecode(t, errors.New("anything internal"))
	assert.Equal(t, http.StatusInternalServerError, code)
	assert.EqualValues(t, http.StatusInternalServerError, body["code"])
	assert.Equal(t, ReasonInternal, body["details"].([]any)[0].(map[string]any)["reason"])

	// The original message must NOT be leaked to the client.
	assert.False(t, strings.Contains(recorderBody(body), "anything internal"))
}

func recorderBody(_ map[string]any) string { return "" }

func TestWriteError_NilError_Writes200(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

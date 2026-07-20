package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/ogen-go/ogen/ogenerrors"

	"github.com/junepy/dbhub/backend/internal/oas"
	"github.com/junepy/dbhub/backend/internal/service"
)

// ErrorHandler adapts service-layer errors (and ogen's own validation errors)
// to the contract's unified Error shape (12-api-contract.md §4.1).
//
// Wire it once via oas.WithErrorHandler(ErrorHandler) in main.go.
//
// Behaviour:
//   - *service.Error: rendered with its own Code + Details (reason/domain).
//   - any other error: falls back to ogenerrors.ErrorCode + a generic 5xx/4xx
//     with a stable reason (no internal details leaked).
func ErrorHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	if se := service.AsError(err); se != nil {
		writeContractError(w, se.Code, se)
		return
	}

	code := ogenerrors.ErrorCode(err)
	if code == 0 {
		code = http.StatusInternalServerError
	}
	generic := service.NewError(code, service.ReasonForCode(code), "request failed")
	writeContractError(w, code, generic)
}

// writeContractError writes the contract Error JSON to w using the same
// field shape ogen's own encoder would produce for *oas.Error. We use the
// generated oas.Error type (and stdlib json) so the wire output stays in
// lock-step with what /readyz and other handlers return directly.
func writeContractError(w http.ResponseWriter, code int, e *service.Error) {
	oasErr := toOasError(e)
	oasErr.Code = code

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(oasErr); err != nil {
		_, _ = w.Write([]byte(`{"code":500,"message":"failed to render error"}`))
	}
}

// toOasError maps the service-layer Error shape into the ogen-generated
// oas.Error so we can reuse its JSON tags (single source of truth for the
// wire shape).
func toOasError(e *service.Error) *oas.Error {
	out := &oas.Error{
		Code:    e.Code,
		Message: e.Message,
		Details: make([]oas.ErrorDetail, 0, len(e.Details)),
	}
	for _, d := range e.Details {
		od := oas.ErrorDetail{
			Type:   oas.NewOptString(d.Type),
			Reason: oas.NewOptString(d.Reason),
			Domain: oas.NewOptString(d.Domain),
		}
		if d.Metadata != nil {
			md := oas.ErrorDetailMetadata(d.Metadata)
			od.Metadata = oas.NewOptErrorDetailMetadata(md)
		}
		if len(d.FieldViolations) > 0 {
			fv := make([]oas.FieldViolation, 0, len(d.FieldViolations))
			for _, v := range d.FieldViolations {
				fv = append(fv, oas.FieldViolation{
					Field:       oas.NewOptString(v.Field),
					Description: oas.NewOptString(v.Description),
				})
			}
			od.FieldViolations = fv
		}
		out.Details = append(out.Details, od)
	}
	return out
}

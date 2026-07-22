package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// PageCursor is the internal shape of an opaque page_token. Clients must
// treat the encoded token as opaque; the schema below is private and may
// change without notice.
type PageCursor struct {
	// Key is the keyset value the caller should resume after (e.g. last PK
	// or (created_at, id) tuple rendered as string).
	Key string `json:"k"`
}

// ParsePageSize reads the page_size query parameter from r, applies default
// (when missing or empty) and clamps to [1, maxVal]. Invalid (non-integer)
// values fall back to defaultVal rather than producing an error — List
// endpoints treat a malformed page_size as "use default", which matches the
// contract's "default 50" wording (12-api-contract.md §3.2).
func ParsePageSize(r *http.Request, defaultVal, maxVal int) int {
	raw := r.URL.Query().Get("page_size")
	if raw == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return defaultVal
	}
	if n > maxVal {
		return maxVal
	}
	return n
}

// EncodeCursor base64-encodes a JSON-serialised PageCursor holding key.
// The result is URL-safe and opaque to clients. Empty key returns "" so
// callers can cheaply signal "no next page".
func EncodeCursor(key string) string {
	if key == "" {
		return ""
	}
	body, err := json.Marshal(PageCursor{Key: key})
	if err != nil {
		// Marshalling a struct with one string field cannot fail.
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(body)
}

// DecodeCursor reverses EncodeCursor. Returns the inner key and a non-nil
// error when the token is malformed / not a valid PageCursor JSON. Callers
// should map the error to a 400 VALIDATION_FAILED (12-api-contract §4.3).
func DecodeCursor(token string) (string, error) {
	if token == "" {
		return "", nil
	}
	body, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", fmt.Errorf("decode page_token: base64: %w", err)
	}
	var cur PageCursor
	if err := json.Unmarshal(body, &cur); err != nil {
		return "", fmt.Errorf("decode page_token: json: %w", err)
	}
	return cur.Key, nil
}

// HasNextPage implements the LIMIT+1 pattern: the caller fetches limit+1
// rows and passes the full slice; if more than `limit` came back, there is
// another page. The caller is expected to trim items[:limit] before returning.
//
// `items` is typed []any to keep the helper free of generics for now; if the
// codebase standardises on a generic List[T] shape later, this can be
// rewritten as HasNextPage[T](items []T, limit int) bool without breaking
// callers (12-api-contract.md §3.2 "limit+1"判定).
func HasNextPage(items []any, limit int) bool {
	return len(items) > limit
}

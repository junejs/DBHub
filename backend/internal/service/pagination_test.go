package service

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// encodeBase64 wraps the same encoding used by EncodeCursor for test fixtures.
func encodeBase64(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func TestParsePageSize_DefaultWhenAbsent(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
	assert.Equal(t, 50, ParsePageSize(r, 50, 5000))
}

func TestParsePageSize_DefaultWhenEmpty(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/projects?page_size=", nil)
	assert.Equal(t, 50, ParsePageSize(r, 50, 5000))
}

func TestParsePageSize_RespectsExplicitValue(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/projects?page_size=25", nil)
	assert.Equal(t, 25, ParsePageSize(r, 50, 5000))
}

func TestParsePageSize_ClampsToMax(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/projects?page_size=99999", nil)
	assert.Equal(t, 5000, ParsePageSize(r, 50, 5000))
}

func TestParsePageSize_ClampsToMinOne(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/projects?page_size=0", nil)
	assert.Equal(t, 50, ParsePageSize(r, 50, 5000))
}

func TestParsePageSize_InvalidFallsBackToDefault(t *testing.T) {
	for _, raw := range []string{"abc", "-3", "1.5", "null"} {
		r := httptest.NewRequest(http.MethodGet, "/v1/projects?page_size="+raw, nil)
		assert.Equal(t, 50, ParsePageSize(r, 50, 5000), "raw=%s", raw)
	}
}

func TestCursor_RoundTrip(t *testing.T) {
	for _, key := range []string{"abc", "2024-01-01T00:00:00Z|12345", "x"} {
		tok := EncodeCursor(key)
		require.NotEmpty(t, tok)
		got, err := DecodeCursor(tok)
		require.NoError(t, err)
		assert.Equal(t, key, got)
	}
}

func TestEncodeCursor_EmptyKeyYieldsEmptyToken(t *testing.T) {
	assert.Empty(t, EncodeCursor(""))
}

func TestDecodeCursor_EmptyTokenYieldsEmptyKey(t *testing.T) {
	got, err := DecodeCursor("")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestDecodeCursor_MalformedBase64(t *testing.T) {
	_, err := DecodeCursor("not!!base64@@")
	require.Error(t, err)
}

func TestDecodeCursor_ValidBase64InvalidJSON(t *testing.T) {
	// Valid base64 of "not json".
	tok := encodeBase64("not json")
	_, err := DecodeCursor(tok)
	require.Error(t, err)
}

func TestHasNextPage_AtLimit(t *testing.T) {
	items := make([]any, 50)
	assert.False(t, HasNextPage(items, 50))
}

func TestHasNextPage_OverLimit(t *testing.T) {
	items := make([]any, 51)
	assert.True(t, HasNextPage(items, 50))
}

func TestHasNextPage_UnderLimit(t *testing.T) {
	items := make([]any, 10)
	assert.False(t, HasNextPage(items, 50))
}

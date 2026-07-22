package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setEnv sets the given env vars and registers a cleanup that restores the
// prior values (or unsets them) when the test ends.
func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		prev, ok := os.LookupEnv(k)
		_ = os.Setenv(k, v)
		if ok {
			t.Cleanup(func() { _ = os.Setenv(k, prev) })
		} else {
			t.Cleanup(func() { _ = os.Unsetenv(k) })
		}
	}
}

// minRequiredVars is the smallest valid env set required for Load() to succeed.
var minRequiredVars = map[string]string{
	"DB_DSN":     "postgres://user:pass@localhost:5432/db?sslmode=disable",
	"JWT_SECRET": "super-secret-jwt-value-for-tests",
	"MASTER_KEY": "AAAA-BBBB-CCCC-DDDD-EEEE-FFFF",
}

// clearAllConfigEnv unsets every env var Load() reads so each test starts clean.
func clearAllConfigEnv(t *testing.T) {
	t.Helper()
	for _, k := range allConfigEnvVars {
		_ = os.Unsetenv(k)
	}
}

var allConfigEnvVars = []string{
	"DB_DSN", "JWT_SECRET", "MASTER_KEY", "SECRET_MANAGER",
	"PORT", "LOG_LEVEL", "LOG_FORMAT", "DEFAULT_LOCALE",
	"SYNC_INTERVAL", "SCHEMA_CACHE_TTL", "QUERY_TIMEOUT",
	"EXPORT_SYNC_ROW_LIMIT", "EXPORT_RETENTION",
	"RATE_QUERY_PER_USER_PER_MIN", "RATE_LOGIN_PER_IP_PER_MIN",
	"AUDIT_STDOUT_MIRROR",
	"BOOTSTRAP_ADMIN_EMAIL", "BOOTSTRAP_ADMIN_PASSWORD",
}

func TestLoad_HappyPath_AllDefaults(t *testing.T) {
	clearAllConfigEnv(t)
	setEnv(t, minRequiredVars)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "json", cfg.LogFormat)
	assert.Equal(t, "zh", cfg.DefaultLocale)
	assert.Equal(t, 15*time.Minute, cfg.SyncInterval)
	assert.Equal(t, 15*time.Minute, cfg.SchemaCacheTTL)
	assert.Equal(t, 30*time.Second, cfg.QueryTimeout)
	assert.Equal(t, 10000, cfg.ExportSyncRowLimit)
	assert.Equal(t, 24*time.Hour, cfg.ExportRetention)
	assert.Equal(t, 60, cfg.RateQueryPerUserPerMin)
	assert.Equal(t, 10, cfg.RateLoginPerIPPerMin)
	assert.False(t, cfg.AuditStdoutMirror)
	assert.Equal(t, minRequiredVars["DB_DSN"], cfg.DBDSN)
	assert.Equal(t, minRequiredVars["JWT_SECRET"], cfg.JWTSecret)
	assert.Equal(t, minRequiredVars["MASTER_KEY"], cfg.MasterKey)
}

func TestLoad_MissingEachRequiredVar(t *testing.T) {
	for _, key := range []string{"DB_DSN", "JWT_SECRET", "MASTER_KEY"} {
		t.Run(key, func(t *testing.T) {
			clearAllConfigEnv(t)
			vars := make(map[string]string, len(minRequiredVars))
			for k, v := range minRequiredVars {
				vars[k] = v
			}
			delete(vars, key)
			setEnv(t, vars)

			_, err := Load()
			require.Error(t, err)
			assert.Contains(t, err.Error(), key)
		})
	}
}

func TestLoad_DoesNotLeakSecrets(t *testing.T) {
	clearAllConfigEnv(t)
	setEnv(t, minRequiredVars)

	// Mark the secret with a recognisable token so we can assert it never
	// appears in the error string.
	const secretMarker = "MARKER-SECRET-VALUE-X"
	t.Setenv("JWT_SECRET", secretMarker)
	t.Setenv("MASTER_KEY", secretMarker)
	t.Setenv("DB_DSN", "postgres://"+secretMarker+":pwd@h/db")

	// Remove a different required-ish field to force an error.
	t.Setenv("PORT", "not-an-int")

	_, err := Load()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secretMarker,
		"error message must not echo back secret env values")
}

func TestLoad_InvalidPort(t *testing.T) {
	clearAllConfigEnv(t)
	setEnv(t, minRequiredVars)
	t.Setenv("PORT", "not-an-int")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PORT")
}

func TestLoad_PortOutOfRange(t *testing.T) {
	for _, p := range []string{"0", "-1", "65536", "99999"} {
		t.Run(p, func(t *testing.T) {
			clearAllConfigEnv(t)
			setEnv(t, minRequiredVars)
			t.Setenv("PORT", p)

			_, err := Load()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "PORT")
		})
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	clearAllConfigEnv(t)
	setEnv(t, minRequiredVars)
	t.Setenv("LOG_LEVEL", "trace")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LOG_LEVEL")
}

func TestLoad_InvalidLogFormat(t *testing.T) {
	clearAllConfigEnv(t)
	setEnv(t, minRequiredVars)
	t.Setenv("LOG_FORMAT", "yaml")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LOG_FORMAT")
}

func TestLoad_InvalidDefaultLocale(t *testing.T) {
	clearAllConfigEnv(t)
	setEnv(t, minRequiredVars)
	t.Setenv("DEFAULT_LOCALE", "ja")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DEFAULT_LOCALE")
}

func TestLoad_InvalidDuration(t *testing.T) {
	clearAllConfigEnv(t)
	setEnv(t, minRequiredVars)
	t.Setenv("QUERY_TIMEOUT", "10 xyz")

	_, err := Load()
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "QUERY_TIMEOUT"),
		"error should name the failing var: %v", err)
}

func TestLoad_InvalidInt(t *testing.T) {
	clearAllConfigEnv(t)
	setEnv(t, minRequiredVars)
	t.Setenv("RATE_QUERY_PER_USER_PER_MIN", "not-an-int")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RATE_QUERY_PER_USER_PER_MIN")
}

func TestLoad_InvalidBool(t *testing.T) {
	clearAllConfigEnv(t)
	setEnv(t, minRequiredVars)
	t.Setenv("AUDIT_STDOUT_MIRROR", "yes-but-not-bool")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AUDIT_STDOUT_MIRROR")
}

func TestLoad_ParsesProvidedValues(t *testing.T) {
	clearAllConfigEnv(t)
	setEnv(t, map[string]string{
		"DB_DSN":                      "postgres://u:p@h:5432/d",
		"JWT_SECRET":                  "jwt",
		"MASTER_KEY":                  "mk",
		"PORT":                        "9000",
		"LOG_LEVEL":                   "DEBUG", // case-insensitive
		"LOG_FORMAT":                  "TEXT",
		"DEFAULT_LOCALE":              "EN",
		"SYNC_INTERVAL":               "30m",
		"SCHEMA_CACHE_TTL":            "5m",
		"QUERY_TIMEOUT":               "10s",
		"EXPORT_SYNC_ROW_LIMIT":       "5000",
		"EXPORT_RETENTION":            "48h",
		"RATE_QUERY_PER_USER_PER_MIN": "120",
		"RATE_LOGIN_PER_IP_PER_MIN":   "20",
		"AUDIT_STDOUT_MIRROR":         "true",
	})

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 9000, cfg.Port)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "text", cfg.LogFormat)
	assert.Equal(t, "en", cfg.DefaultLocale)
	assert.Equal(t, 30*time.Minute, cfg.SyncInterval)
	assert.Equal(t, 5*time.Minute, cfg.SchemaCacheTTL)
	assert.Equal(t, 10*time.Second, cfg.QueryTimeout)
	assert.Equal(t, 5000, cfg.ExportSyncRowLimit)
	assert.Equal(t, 48*time.Hour, cfg.ExportRetention)
	assert.Equal(t, 120, cfg.RateQueryPerUserPerMin)
	assert.Equal(t, 20, cfg.RateLoginPerIPPerMin)
	assert.True(t, cfg.AuditStdoutMirror)
}

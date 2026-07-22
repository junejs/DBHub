// Package config loads and validates backend environment variables.
//
// All configuration is environment-driven (16-ops §2); there is no config file
// in v1. Load() is called once at startup (main.go) and the resulting *Config
// is passed via constructor injection. Validation follows the fail-fast
// principle: required fields are checked before any external connection is
// opened, and error messages deliberately omit secret-bearing values so a
// misconfiguration cannot leak through logs (acceptance: ZZZ-85 §"验收标准").
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Allowed log levels (slog). Lower-cased before comparison.
var allowedLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// Allowed log formats.
var allowedLogFormats = map[string]bool{
	"json": true,
	"text": true,
}

// Default values for non-required fields (16-ops §2).
const (
	defaultPort         = "8080"
	defaultLogLevel     = "info"
	defaultLogFormat    = "json"
	defaultSyncInterval = 15 * time.Minute
	defaultSchemaTTL    = 15 * time.Minute
	defaultQueryTimeout = 30 * time.Second

	defaultExportSyncRowLimit = 10000
	defaultExportRetention    = 24 * time.Hour

	defaultRateQueryPerMin = 60
	defaultRateLoginPerMin = 10

	defaultAuditStdoutMirror = false
	defaultDefaultLocale     = "zh"
)

// Config is the validated runtime configuration. Field names are PascalCase
// (Go convention); env-var names match 16-ops §2 exactly (snake_case upper).
//
// Secret-bearing fields (DSN, JWTSecret, MasterKey, BootstrapAdminPassword)
// are never included in any String()/error output produced by this package.
type Config struct {
	// HTTP listen port.
	Port int

	// Platform PostgreSQL DSN (16-ops §2).
	DBDSN string

	// JWT signing secret (HS256). Secret-bearing.
	JWTSecret string

	// AES-256-GCM master key (base64, 32B). Secret-bearing.
	MasterKey string

	// External secret manager ("vault"|"aws"|"gcp"|"aliyun"); empty = app-layer crypto.
	SecretManager string

	// First-admin bootstrap (16-ops §2). Email/Password are optional.
	BootstrapAdminEmail    string
	BootstrapAdminPassword string // Secret-bearing.

	// Sync / cache / timeouts.
	SyncInterval       time.Duration
	SchemaCacheTTL     time.Duration
	QueryTimeout       time.Duration
	ExportSyncRowLimit int
	ExportRetention    time.Duration

	// Rate limits.
	RateQueryPerUserPerMin int
	RateLoginPerIPPerMin   int

	// Logging.
	LogLevel  string
	LogFormat string

	// Audit stdout mirror (D30).
	AuditStdoutMirror bool

	// Default locale (D23: zh|en).
	DefaultLocale string
}

// Load reads environment variables, validates required fields and value
// formats, and returns a fully populated *Config or a descriptive error.
//
// Validation order: required-field presence first (DB_DSN, JWT_SECRET,
// MASTER_KEY), then format checks (PORT int, LOG_LEVEL/LOG_FORMAT in allowed
// sets, durations parse). On any failure the returned error lists what is
// wrong WITHOUT including the secret values themselves.
func Load() (*Config, error) {
	// 1) Required secret-bearing fields: presence-only; values not echoed.
	var missing []string
	if strings.TrimSpace(os.Getenv("DB_DSN")) == "" {
		missing = append(missing, "DB_DSN")
	}
	if strings.TrimSpace(os.Getenv("JWT_SECRET")) == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if strings.TrimSpace(os.Getenv("MASTER_KEY")) == "" {
		missing = append(missing, "MASTER_KEY")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("config: missing required environment variable(s): %s", strings.Join(missing, ", "))
	}

	// 2) PORT (default 8080; must parse as int if provided).
	portStr := os.Getenv("PORT")
	if portStr == "" {
		portStr = defaultPort
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("config: PORT %q is not a valid integer", portStr)
	}
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("config: PORT %d out of valid range (1-65535)", port)
	}

	// 3) LOG_LEVEL (default info).
	logLevel := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	if logLevel == "" {
		logLevel = defaultLogLevel
	}
	if !allowedLogLevels[logLevel] {
		return nil, fmt.Errorf("config: LOG_LEVEL %q is not one of debug|info|warn|error", logLevel)
	}

	// 4) LOG_FORMAT (default json).
	logFormat := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT")))
	if logFormat == "" {
		logFormat = defaultLogFormat
	}
	if !allowedLogFormats[logFormat] {
		return nil, fmt.Errorf("config: LOG_FORMAT %q is not one of json|text", logFormat)
	}

	// 5) Default locale (default zh).
	defaultLocale := strings.ToLower(strings.TrimSpace(os.Getenv("DEFAULT_LOCALE")))
	if defaultLocale == "" {
		defaultLocale = defaultDefaultLocale
	}
	if defaultLocale != "zh" && defaultLocale != "en" {
		return nil, fmt.Errorf("config: DEFAULT_LOCALE %q must be zh or en", defaultLocale)
	}

	// 6) Optional durations / ints with defaults (parse errors are config errors).
	syncInterval, err := parseDurationEnv("SYNC_INTERVAL", defaultSyncInterval)
	if err != nil {
		return nil, err
	}
	schemaTTL, err := parseDurationEnv("SCHEMA_CACHE_TTL", defaultSchemaTTL)
	if err != nil {
		return nil, err
	}
	queryTimeout, err := parseDurationEnv("QUERY_TIMEOUT", defaultQueryTimeout)
	if err != nil {
		return nil, err
	}
	exportRetention, err := parseDurationEnv("EXPORT_RETENTION", defaultExportRetention)
	if err != nil {
		return nil, err
	}

	exportSyncRowLimit, err := parseIntEnv("EXPORT_SYNC_ROW_LIMIT", defaultExportSyncRowLimit)
	if err != nil {
		return nil, err
	}
	rateQuery, err := parseIntEnv("RATE_QUERY_PER_USER_PER_MIN", defaultRateQueryPerMin)
	if err != nil {
		return nil, err
	}
	rateLogin, err := parseIntEnv("RATE_LOGIN_PER_IP_PER_MIN", defaultRateLoginPerMin)
	if err != nil {
		return nil, err
	}

	auditMirror, err := parseBoolEnv("AUDIT_STDOUT_MIRROR", defaultAuditStdoutMirror)
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:                   port,
		DBDSN:                  os.Getenv("DB_DSN"),
		JWTSecret:              os.Getenv("JWT_SECRET"),
		MasterKey:              os.Getenv("MASTER_KEY"),
		SecretManager:          os.Getenv("SECRET_MANAGER"),
		BootstrapAdminEmail:    os.Getenv("BOOTSTRAP_ADMIN_EMAIL"),
		BootstrapAdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
		SyncInterval:           syncInterval,
		SchemaCacheTTL:         schemaTTL,
		QueryTimeout:           queryTimeout,
		ExportSyncRowLimit:     exportSyncRowLimit,
		ExportRetention:        exportRetention,
		RateQueryPerUserPerMin: rateQuery,
		RateLoginPerIPPerMin:   rateLogin,
		LogLevel:               logLevel,
		LogFormat:              logFormat,
		AuditStdoutMirror:      auditMirror,
		DefaultLocale:          defaultLocale,
	}, nil
}

// parseDurationEnv reads a single env var as time.Duration. Empty falls back
// to the provided default; malformed values return a config-named error.
func parseDurationEnv(name string, def time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s %q is not a valid duration", name, v)
	}
	return d, nil
}

// parseIntEnv reads a single env var as int. Empty falls back to the provided
// default; malformed values return a config-named error.
func parseIntEnv(name string, def int) (int, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s %q is not a valid integer", name, v)
	}
	return n, nil
}

// parseBoolEnv reads a single env var as bool. Empty falls back to the default.
// Accepts the standard strconv.ParseBool inputs (true/false/1/0/...).
func parseBoolEnv(name string, def bool) (bool, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("config: %s %q is not a valid boolean", name, v)
	}
	return b, nil
}

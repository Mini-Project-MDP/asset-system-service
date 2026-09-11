package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	defaultAppEnvironment      = "development"
	defaultAppPort             = 8080
	defaultDatabasePingTimeout = 5 * time.Second
	defaultJWTSecret           = "mayora-super-secret-jwt-key-2026"
	defaultJWTExpiry           = 24 * time.Hour
)

// Config contains all runtime configuration required by the API.
type Config struct {
	AppEnvironment        string
	AppPort               int
	DatabaseURL           string
	DatabaseAuthToken     string
	DatabasePingTimeout   time.Duration
	JWTSecret             string
	JWTExpiryDuration     time.Duration
	AllowedOrigins        []string
	ApprovalEngineBaseURL string
	ApprovalEngineAPIKey  string
}

// Address returns the HTTP server address derived from APP_PORT.
func (c Config) Address() string {
	return fmt.Sprintf(":%d", c.AppPort)
}

// Load reads an optional local .env file, then loads configuration from the
// process environment. Existing process environment variables take priority.
func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	return loadFromEnvironment(os.LookupEnv)
}

type environmentLookup func(string) (string, bool)

func loadFromEnvironment(lookup environmentLookup) (Config, error) {
	appEnvironment := valueOrDefault(lookup, "APP_ENV", defaultAppEnvironment)

	portStr := valueOrDefault(lookup, "PORT", valueOrDefault(lookup, "APP_PORT", strconv.Itoa(defaultAppPort)))
	appPort, err := parsePort(portStr)
	if err != nil {
		return Config{}, fmt.Errorf("PORT/APP_PORT: %w", err)
	}

	databaseURL, err := resolveDatabaseURL(lookup)
	if err != nil {
		return Config{}, err
	}
	if err := validateDatabaseURL(databaseURL); err != nil {
		return Config{}, err
	}

	databaseAuthToken := valueOrDefault(lookup, "TURSO_AUTH_TOKEN", valueOrDefault(lookup, "DATABASE_AUTH_TOKEN", ""))
	if strings.HasPrefix(databaseURL, "libsql://") && databaseAuthToken == "" {
		return Config{}, fmt.Errorf("TURSO_AUTH_TOKEN is required for libsql databases")
	}

	databasePingTimeout, err := time.ParseDuration(valueOrDefault(
		lookup,
		"DATABASE_PING_TIMEOUT",
		defaultDatabasePingTimeout.String(),
	))
	if err != nil || databasePingTimeout <= 0 {
		return Config{}, fmt.Errorf("DATABASE_PING_TIMEOUT must be a positive duration such as 5s")
	}

	jwtSecret := valueOrDefault(lookup, "JWT_SECRET", defaultJWTSecret)
	jwtExpiryStr := valueOrDefault(lookup, "JWT_EXPIRY", defaultJWTExpiry.String())
	jwtExpiryDuration, err := time.ParseDuration(jwtExpiryStr)
	if err != nil || jwtExpiryDuration <= 0 {
		jwtExpiryDuration = defaultJWTExpiry
	}

	// Optional: approval engine integration. Service starts without it;
	// approval actions will fail gracefully if not configured.
	approvalEngineBaseURL := valueOrDefault(lookup, "APPROVAL_ENGINE_BASE_URL", "")
	approvalEngineAPIKey := valueOrDefault(lookup, "APPROVAL_ENGINE_API_KEY", "")

	allowedOriginsStr := valueOrDefault(lookup, "ALLOWED_ORIGINS", "")
	var allowedOrigins []string
	if allowedOriginsStr != "" {
		for _, o := range strings.Split(allowedOriginsStr, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				allowedOrigins = append(allowedOrigins, trimmed)
			}
		}
	}

	return Config{
		AppEnvironment:        appEnvironment,
		AppPort:               appPort,
		DatabaseURL:           databaseURL,
		DatabaseAuthToken:     databaseAuthToken,
		DatabasePingTimeout:   databasePingTimeout,
		JWTSecret:             jwtSecret,
		JWTExpiryDuration:     jwtExpiryDuration,
		AllowedOrigins:        allowedOrigins,
		ApprovalEngineBaseURL: approvalEngineBaseURL,
		ApprovalEngineAPIKey:  approvalEngineAPIKey,
	}, nil
}

func resolveDatabaseURL(lookup environmentLookup) (string, error) {
	for _, key := range []string{"DATABASE_URL", "SUPABASE_DATABASE_URL", "POSTGRES_URL", "TURSO_DATABASE_URL"} {
		if val, ok := lookup(key); ok && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val), nil
		}
	}

	// If individual DB host is provided, construct PostgreSQL connection URL
	if host, ok := lookup("DB_HOST"); ok && strings.TrimSpace(host) != "" {
		host = strings.TrimSpace(host)
		port := valueOrDefault(lookup, "DB_PORT", "5432")
		user := valueOrDefault(lookup, "DB_USER", "postgres")
		pass := valueOrDefault(lookup, "DB_PASSWORD", "")
		dbName := valueOrDefault(lookup, "DB_NAME", "postgres")
		sslMode := valueOrDefault(lookup, "DB_SSLMODE", "require")

		var userInfo *url.Userinfo
		if pass != "" {
			userInfo = url.UserPassword(user, pass)
		} else if user != "" {
			userInfo = url.User(user)
		}

		u := &url.URL{
			Scheme: "postgres",
			User:   userInfo,
			Host:   fmt.Sprintf("%s:%s", host, port),
			Path:   "/" + strings.TrimPrefix(dbName, "/"),
		}
		if sslMode != "" {
			q := u.Query()
			q.Set("sslmode", sslMode)
			u.RawQuery = q.Encode()
		}
		return u.String(), nil
	}

	return "", fmt.Errorf("DATABASE_URL is required")
}

func valueOrDefault(lookup environmentLookup, key, fallback string) string {
	if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func requiredValue(lookup environmentLookup, key string) (string, error) {
	value, ok := lookup(key)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("APP_PORT must be a number between 1 and 65535")
	}
	return port, nil
}

func validateDatabaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("DATABASE_URL must be a valid database URL")
	}

	switch parsed.Scheme {
	case "postgres", "postgresql":
		return nil
	case "libsql", "https", "http", "wss", "ws":
		if parsed.User != nil || parsed.RawQuery != "" {
			return fmt.Errorf("TURSO_DATABASE_URL must not contain credentials or query parameters")
		}
		return nil
	default:
		return fmt.Errorf("DATABASE_URL uses an unsupported URL scheme %q", parsed.Scheme)
	}
}

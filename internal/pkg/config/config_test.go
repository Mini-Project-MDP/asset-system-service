package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadFromEnvironmentPostgres(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL":             "postgres://user:pass@localhost:5432/testdb?sslmode=disable",
		"APPROVAL_ENGINE_BASE_URL": "http://localhost:8000",
		"APPROVAL_ENGINE_API_KEY":  "test-engine-key",
	}

	config, err := loadFromEnvironment(mapLookup(values))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if config.AppEnvironment != "development" {
		t.Fatalf("expected development environment, got %s", config.AppEnvironment)
	}
	if config.AppPort != 8080 {
		t.Fatalf("expected port 8080, got %d", config.AppPort)
	}
	if config.DatabasePingTimeout != 5*time.Second {
		t.Fatalf("expected 5s timeout, got %s", config.DatabasePingTimeout)
	}
	if config.Address() != ":8080" {
		t.Fatalf("expected address :8080, got %s", config.Address())
	}
	if config.DatabaseURL != "postgres://user:pass@localhost:5432/testdb?sslmode=disable" {
		t.Fatalf("unexpected database URL: %s", config.DatabaseURL)
	}
}

func TestLoadFromEnvironmentIndividualDBVars(t *testing.T) {
	values := map[string]string{
		"DB_HOST":     "example.supabase.co",
		"DB_PORT":     "5432",
		"DB_USER":     "myuser",
		"DB_PASSWORD": "mypassword",
		"DB_NAME":     "mydb",
		"DB_SSLMODE":  "require",
	}

	config, err := loadFromEnvironment(mapLookup(values))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	expectedPrefix := "postgres://myuser:mypassword@example.supabase.co:5432/mydb"
	if !strings.HasPrefix(config.DatabaseURL, expectedPrefix) {
		t.Fatalf("expected %q prefix, got %q", expectedPrefix, config.DatabaseURL)
	}
}

func TestLoadFromEnvironmentWithPort(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL":             "postgres://user:pass@localhost:5432/testdb",
		"APPROVAL_ENGINE_BASE_URL": "http://localhost:8000",
		"APPROVAL_ENGINE_API_KEY":  "test-engine-key",
		"PORT":                     "40399",
	}

	config, err := loadFromEnvironment(mapLookup(values))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if config.AppPort != 40399 {
		t.Fatalf("expected port 40399, got %d", config.AppPort)
	}
	if config.Address() != ":40399" {
		t.Fatalf("expected address :40399, got %s", config.Address())
	}
}

func TestLoadFromEnvironmentRequiresDatabaseConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		values   map[string]string
		expected string
	}{
		{
			name:     "missing URL",
			values:   map[string]string{"APP_PORT": "8080"},
			expected: "DATABASE_URL is required",
		},
		{
			name:     "missing token for libsql",
			values:   map[string]string{"TURSO_DATABASE_URL": "libsql://example.turso.io"},
			expected: "TURSO_AUTH_TOKEN is required for libsql databases",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := loadFromEnvironment(mapLookup(test.values))
			if err == nil || err.Error() != test.expected {
				t.Fatalf("expected %q, got %v", test.expected, err)
			}
		})
	}
}

func TestLoadFromEnvironmentApprovalEngineOptional(t *testing.T) {
	cfg, err := loadFromEnvironment(mapLookup(map[string]string{
		"DATABASE_URL": "postgres://user:pass@localhost:5432/testdb",
	}))
	if err != nil {
		t.Fatalf("expected no error when approval engine env vars are omitted, got %v", err)
	}
	if cfg.ApprovalEngineBaseURL != "" {
		t.Fatalf("expected empty ApprovalEngineBaseURL, got %q", cfg.ApprovalEngineBaseURL)
	}
	if cfg.ApprovalEngineAPIKey != "" {
		t.Fatalf("expected empty ApprovalEngineAPIKey, got %q", cfg.ApprovalEngineAPIKey)
	}
}

func TestLoadFromEnvironmentValidatesValues(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{
			name:     "invalid port",
			key:      "APP_PORT",
			value:    "70000",
			expected: "APP_PORT",
		},
		{
			name:     "invalid database URL scheme",
			key:      "DATABASE_URL",
			value:    "ftp://example.com/database",
			expected: "unsupported URL scheme",
		},
		{
			name:     "invalid ping timeout",
			key:      "DATABASE_PING_TIMEOUT",
			value:    "0s",
			expected: "DATABASE_PING_TIMEOUT",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{
				"DATABASE_URL":             "postgres://user:pass@localhost:5432/testdb",
				"DATABASE_PING_TIMEOUT":    "5s",
				"APPROVAL_ENGINE_BASE_URL": "http://localhost:8000",
				"APPROVAL_ENGINE_API_KEY":  "test-engine-key",
			}
			values[test.key] = test.value

			_, err := loadFromEnvironment(mapLookup(values))
			if err == nil || !strings.Contains(err.Error(), test.expected) {
				t.Fatalf("expected error containing %q, got %v", test.expected, err)
			}
		})
	}
}

func mapLookup(values map[string]string) environmentLookup {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

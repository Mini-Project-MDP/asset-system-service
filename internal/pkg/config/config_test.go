package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadFromEnvironment(t *testing.T) {
	values := map[string]string{
		"TURSO_DATABASE_URL":       "libsql://example.turso.io",
		"TURSO_AUTH_TOKEN":         "test-token",
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
}

func TestLoadFromEnvironmentWithPort(t *testing.T) {
	values := map[string]string{
		"TURSO_DATABASE_URL":       "libsql://example.turso.io",
		"TURSO_AUTH_TOKEN":         "test-token",
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
			values:   map[string]string{"TURSO_AUTH_TOKEN": "test-token"},
			expected: "TURSO_DATABASE_URL is required",
		},
		{
			name:     "missing token",
			values:   map[string]string{"TURSO_DATABASE_URL": "libsql://example.turso.io"},
			expected: "TURSO_AUTH_TOKEN is required",
		},
		{
			name: "missing approval engine base url",
			values: map[string]string{
				"TURSO_DATABASE_URL": "libsql://example.turso.io",
				"TURSO_AUTH_TOKEN":   "test-token",
			},
			expected: "APPROVAL_ENGINE_BASE_URL is required",
		},
		{
			name: "missing approval engine api key",
			values: map[string]string{
				"TURSO_DATABASE_URL":       "libsql://example.turso.io",
				"TURSO_AUTH_TOKEN":         "test-token",
				"APPROVAL_ENGINE_BASE_URL": "http://localhost:8000",
			},
			expected: "APPROVAL_ENGINE_API_KEY is required",
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
			name:     "invalid database URL",
			key:      "TURSO_DATABASE_URL",
			value:    "postgres://example.com/database",
			expected: "TURSO_DATABASE_URL",
		},
		{
			name:     "token embedded in URL",
			key:      "TURSO_DATABASE_URL",
			value:    "libsql://example.turso.io?authToken=secret",
			expected: "must not contain credentials",
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
				"TURSO_DATABASE_URL":       "libsql://example.turso.io",
				"TURSO_AUTH_TOKEN":         "test-token",
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

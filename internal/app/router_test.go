package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type stubDatabase struct {
	pingError error
}

func (database stubDatabase) PingContext(context.Context) error {
	return database.pingError
}

func TestHealth(t *testing.T) {
	response := performRequest(t, stubDatabase{}, "/health")

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	body := decodeResponse(t, response)
	if body["status"] != "ok" || body["service"] != serviceName {
		t.Fatalf("unexpected response body: %#v", body)
	}
}

func TestReadyWhenDatabaseIsConnected(t *testing.T) {
	response := performRequest(t, stubDatabase{}, "/ready")

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	body := decodeResponse(t, response)
	if body["status"] != "ready" || body["database"] != "connected" {
		t.Fatalf("unexpected response body: %#v", body)
	}
}

func TestReadyWhenDatabaseIsUnavailable(t *testing.T) {
	response := performRequest(t, stubDatabase{pingError: errors.New("connection failed")}, "/ready")

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, response.Code)
	}

	body := decodeResponse(t, response)
	if body["status"] != "unavailable" || body["database"] != "unavailable" {
		t.Fatalf("unexpected response body: %#v", body)
	}
	if _, exists := body["error"]; exists {
		t.Fatal("readiness response exposes an internal error")
	}
}

func performRequest(t *testing.T, database DatabasePinger, path string) *httptest.ResponseRecorder {
	t.Helper()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	NewRouter(Dependencies{
		Database:            database,
		DatabasePingTimeout: time.Second,
	}).ServeHTTP(response, request)

	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder) map[string]string {
	t.Helper()

	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.StatusCode)
	}

	body := decodeResponse(t, response)
	if body["status"] != "ok" || body["service"] != serviceName {
		t.Fatalf("unexpected response body: %#v", body)
	}
}

func TestReadyWhenDatabaseIsConnected(t *testing.T) {
	response := performRequest(t, stubDatabase{}, "/ready")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.StatusCode)
	}

	body := decodeResponse(t, response)
	if body["status"] != "ready" || body["database"] != "connected" {
		t.Fatalf("unexpected response body: %#v", body)
	}
}

func TestReadyWhenDatabaseIsUnavailable(t *testing.T) {
	response := performRequest(t, stubDatabase{pingError: errors.New("connection failed")}, "/ready")

	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, response.StatusCode)
	}

	body := decodeResponse(t, response)
	if body["status"] != "unavailable" || body["database"] != "unavailable" {
		t.Fatalf("unexpected response body: %#v", body)
	}
	if _, exists := body["error"]; exists {
		t.Fatal("readiness response exposes an internal error")
	}
}

func performRequest(t *testing.T, database DatabasePinger, path string) *http.Response {
	t.Helper()

	app := NewRouter(Dependencies{
		Database:            database,
		DatabasePingTimeout: time.Second,
	})

	request := httptest.NewRequest(http.MethodGet, path, nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}

	return response
}

func decodeResponse(t *testing.T, response *http.Response) map[string]string {
	t.Helper()

	defer response.Body.Close()
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	var body map[string]string
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}


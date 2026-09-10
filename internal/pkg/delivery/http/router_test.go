package http

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
	app := NewRouter(Dependencies{})
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.StatusCode)
	}
}

func TestReadyWhenDatabaseIsConnected(t *testing.T) {
	app := NewRouter(Dependencies{
		Database:            stubDatabase{},
		DatabasePingTimeout: time.Second,
	})

	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.StatusCode)
	}
}

func TestReadyWhenDatabaseIsUnavailable(t *testing.T) {
	app := NewRouter(Dependencies{
		Database:            stubDatabase{pingError: errors.New("connection failed")},
		DatabasePingTimeout: time.Second,
	})

	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}

	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, response.StatusCode)
	}
}

func decodeResponse(t *testing.T, response *http.Response) map[string]interface{} {
	t.Helper()

	defer response.Body.Close()
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

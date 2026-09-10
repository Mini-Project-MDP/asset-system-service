package approvalengine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

func TestClient_CreateRequest_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/requests" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-API-Key"); got != "test-key" {
			t.Fatalf("expected X-API-Key header, got %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["doc_type"] != "asset_request_barcode" {
			t.Fatalf("unexpected doc_type: %v", body["doc_type"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "request created",
			"data": map[string]any{
				"id":                 "eng-req-1",
				"doc_type":           "asset_request_barcode",
				"resource_id":        "REQ-0001",
				"requester_id":       "EMP101",
				"status":             "pending",
				"current_step_order": 1,
			},
		})
	}))
	defer server.Close()

	client := New(server.URL, "test-key", nil)
	got, err := client.CreateRequest(context.Background(), domain.EngineCreateRequestInput{
		DocType:     "asset_request_barcode",
		ResourceID:  "REQ-0001",
		RequesterID: "EMP101",
		Payload:     map[string]any{"requesterApprovalRank": 10},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "eng-req-1" || got.Status != "pending" || got.CurrentStepOrder != 1 {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestClient_Decide_BusinessRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "not the assigned approver",
		})
	}))
	defer server.Close()

	client := New(server.URL, "test-key", nil)
	_, err := client.Decide(context.Background(), "eng-req-1", domain.EngineDecisionInput{
		UserID:   "wrong-user",
		Decision: domain.EngineDecisionApproved,
	})
	if err == nil {
		t.Fatal("expected an error")
	}

	var apiErr *domain.EngineAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *domain.EngineAPIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "not the assigned approver" {
		t.Fatalf("unexpected message: %q", apiErr.Message)
	}
}

func TestClient_GetRequest_ConnectionFailure(t *testing.T) {
	// A server that's already closed guarantees connection refused, without
	// depending on any real network being unreachable in CI.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	client := New(server.URL, "test-key", nil)
	_, err := client.GetRequest(context.Background(), "eng-req-1")
	if err == nil {
		t.Fatal("expected an error")
	}

	var apiErr *domain.EngineAPIError
	if errors.As(err, &apiErr) {
		t.Fatalf("expected a network error, not *domain.EngineAPIError: %v", apiErr)
	}
}

func TestClient_RequestTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(server.URL, "test-key", &http.Client{Timeout: 5 * time.Millisecond})
	_, err := client.CreateRequest(context.Background(), domain.EngineCreateRequestInput{
		DocType: "asset_request_barcode", ResourceID: "REQ-0001", RequesterID: "EMP101",
	})
	if err == nil {
		t.Fatal("expected a timeout error")
	}

	var apiErr *domain.EngineAPIError
	if errors.As(err, &apiErr) {
		t.Fatalf("expected a network/timeout error, not *domain.EngineAPIError: %v", apiErr)
	}
}

// Compile-time check that *Client satisfies domain.ApprovalEngineClient.
var _ domain.ApprovalEngineClient = (*Client)(nil)

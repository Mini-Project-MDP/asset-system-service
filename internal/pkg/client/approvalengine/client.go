// Package approvalengine is a thin HTTP client for Approval-Engine-Service,
// the external workflow engine that owns approval logic (see
// docs/approval-engine-integration-plan.md). It talks the engine's own JSON
// envelope ({"success","message","data","error"}) and exposes just the 3
// operations request_service needs: CreateRequest, Decide, GetRequest.
//
// *Client satisfies domain.ApprovalEngineClient structurally — this package
// must never be imported by a handler directly, only by service (see
// docs/backend-best-practices.md §1 on layer direction).
package approvalengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

const defaultTimeout = 5 * time.Second

// Client calls Approval-Engine-Service over HTTP.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New creates a Client. If httpClient is nil, one with defaultTimeout is
// used — callers must not pass http.DefaultClient (no timeout) directly.
func New(baseURL, apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{baseURL: baseURL, apiKey: apiKey, http: httpClient}
}

// CreateRequest starts a new approval request. Idempotent on the engine's
// side ((app, doc_type, resource_id)) — safe to retry on timeout.
func (c *Client) CreateRequest(ctx context.Context, in domain.EngineCreateRequestInput) (*domain.EngineApprovalRequest, error) {
	body := struct {
		DocType     string         `json:"doc_type"`
		ResourceID  string         `json:"resource_id"`
		RequesterID string         `json:"requester_id"`
		Payload     map[string]any `json:"payload"`
	}{in.DocType, in.ResourceID, in.RequesterID, in.Payload}

	var out domain.EngineApprovalRequest
	if err := c.do(ctx, http.MethodPost, "/api/v1/requests", body, true, &out); err != nil {
		return nil, fmt.Errorf("approvalengine: create request: %w", err)
	}
	return &out, nil
}

// Decide records one approver's decision on the currently active step of
// approvalRequestID (the id Approval-Engine-Service assigned — not the
// caller's local resource_id).
func (c *Client) Decide(ctx context.Context, approvalRequestID string, in domain.EngineDecisionInput) (*domain.EngineApprovalRequest, error) {
	body := struct {
		UserID   string  `json:"user_id"`
		Decision string  `json:"decision"`
		Comment  *string `json:"comment,omitempty"`
	}{in.UserID, in.Decision, in.Comment}

	var out domain.EngineApprovalRequest
	path := "/api/v1/requests/" + approvalRequestID + "/decision"
	if err := c.do(ctx, http.MethodPost, path, body, false, &out); err != nil {
		return nil, fmt.Errorf("approvalengine: decide %s: %w", approvalRequestID, err)
	}
	return &out, nil
}

// GetRequest fetches one request with its steps and assignments.
func (c *Client) GetRequest(ctx context.Context, approvalRequestID string) (*domain.EngineApprovalRequest, error) {
	var out domain.EngineApprovalRequest
	path := "/api/v1/requests/" + approvalRequestID
	if err := c.do(ctx, http.MethodGet, path, nil, false, &out); err != nil {
		return nil, fmt.Errorf("approvalengine: get request %s: %w", approvalRequestID, err)
	}
	return &out, nil
}

// envelope mirrors Approval-Engine-Service's pkg/response.Envelope.
type envelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// do sends one request and decodes its envelope into out (if non-nil).
// withAPIKey should be true only for the one endpoint that requires it
// (see Approval-Engine-Service's router.go: only POST /requests is gated by
// X-API-Key).
func (c *Client) do(ctx context.Context, method, path string, body any, withAPIKey bool, out any) error {
	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if withAPIKey {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call engine: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	var env envelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		return fmt.Errorf("decode response (status %d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode >= 300 || !env.Success {
		message := env.Error
		if message == "" {
			message = env.Message
		}
		return &domain.EngineAPIError{StatusCode: resp.StatusCode, Message: message}
	}

	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decode response data: %w", err)
		}
	}
	return nil
}

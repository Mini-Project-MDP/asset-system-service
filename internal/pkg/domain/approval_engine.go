package domain

import (
	"context"
	"fmt"
)

// Decision values Approval-Engine-Service accepts. The engine has only these
// two states — approve/reject — deliberately; FE's "revision" action is
// mapped to EngineDecisionRejected by request_service, not by the engine or
// its client. See docs/approval-engine-integration-plan.md Fase 0.
const (
	EngineDecisionApproved = "approved"
	EngineDecisionRejected = "rejected"
)

// EngineApprovalRequest mirrors Approval-Engine-Service's
// internal/domain.ApprovalRequest JSON shape. Duplicated here (not imported)
// because the two services are separate modules/repos with no shared
// package — see Approval-Engine-Service/internal/domain/request.go for the
// source of truth.
type EngineApprovalRequest struct {
	ID               string               `json:"id"`
	AppID            string               `json:"app_id"`
	DefinitionID     string               `json:"definition_id"`
	DocType          string               `json:"doc_type"`
	ResourceID       string               `json:"resource_id"`
	RequesterID      string               `json:"requester_id"`
	Payload          map[string]any       `json:"payload"`
	Status           string               `json:"status"`
	CurrentStepOrder int                  `json:"current_step_order"`
	CreatedAt        string               `json:"created_at,omitempty"`
	CompletedAt      *string              `json:"completed_at,omitempty"`
	Steps            []EngineApprovalStep `json:"steps,omitempty"`
}

// EngineApprovalStep mirrors Approval-Engine-Service's ApprovalStep.
type EngineApprovalStep struct {
	ID           string                     `json:"id"`
	RequestID    string                     `json:"request_id"`
	StepOrder    int                        `json:"step_order"`
	Name         string                     `json:"name"`
	ApprovalMode string                     `json:"approval_mode"`
	Status       string                     `json:"status"`
	ActivatedAt  *string                    `json:"activated_at,omitempty"`
	CompletedAt  *string                    `json:"completed_at,omitempty"`
	Assignments  []EngineApprovalAssignment `json:"assignments,omitempty"`
}

// EngineApprovalAssignment mirrors Approval-Engine-Service's ApprovalAssignment.
type EngineApprovalAssignment struct {
	ID           string  `json:"id"`
	StepID       string  `json:"step_id"`
	RequestID    string  `json:"request_id"`
	UserID       string  `json:"user_id"`
	UserName     string  `json:"user_name,omitempty"`
	UserPosition string  `json:"user_position,omitempty"`
	Status       string  `json:"status"`
	Comment      *string `json:"comment,omitempty"`
	ActedAt      *string `json:"acted_at,omitempty"`
}

// CurrentStepName finds the name of the step matching CurrentStepOrder, or
// "" if the request has completed (no step at that order) or carries no
// step data at all.
func (r EngineApprovalRequest) CurrentStepName() string {
	for _, s := range r.Steps {
		if s.StepOrder == r.CurrentStepOrder {
			return s.Name
		}
	}
	return ""
}

// EngineCreateRequestInput is the payload for ApprovalEngineClient.CreateRequest.
type EngineCreateRequestInput struct {
	DocType     string
	ResourceID  string
	RequesterID string
	Payload     map[string]any
}

// EngineDecisionInput is the payload for ApprovalEngineClient.Decide.
type EngineDecisionInput struct {
	UserID   string
	Decision string // EngineDecisionApproved or EngineDecisionRejected
	Comment  *string
}

// ApprovalEngineClient defines the calls request_service needs against
// Approval-Engine-Service. Implemented by internal/pkg/client/approvalengine.
type ApprovalEngineClient interface {
	CreateRequest(ctx context.Context, in EngineCreateRequestInput) (*EngineApprovalRequest, error)
	Decide(ctx context.Context, approvalRequestID string, in EngineDecisionInput) (*EngineApprovalRequest, error)
	GetRequest(ctx context.Context, approvalRequestID string) (*EngineApprovalRequest, error)
}

// EngineWebhookEvent mirrors Approval-Engine-Service's
// internal/service.WebhookEvent JSON shape — the payload it POSTs to our
// callback_url whenever a request/step changes state (see docs/
// asset-system-integration-checklist.md Langkah 6 in Approval-Engine-Service).
// Duplicated here for the same reason as EngineApprovalRequest above: the two
// services share no Go package.
//
// It carries no authoritative state of its own — RequestID only tells us
// *something* changed on that engine request. The handler treats it purely
// as a cue to re-fetch GetRequest and mirror the current truth locally,
// exactly like the existing on-demand refresh in requestService.Detail.
type EngineWebhookEvent struct {
	Event      string         `json:"event"`
	AppID      string         `json:"app_id"`
	RequestID  string         `json:"request_id"`
	StepID     *string        `json:"step_id,omitempty"`
	ActorID    *string        `json:"actor_id,omitempty"`
	Detail     map[string]any `json:"detail,omitempty"`
	OccurredAt string         `json:"occurred_at"`
}

// EngineAPIError is a business-level rejection from Approval-Engine-Service
// itself (HTTP 4xx with a JSON error body) — as opposed to a network/timeout
// failure. Declared in domain (not the client package) so request_service
// can detect it via errors.As without importing the client package
// directly, keeping the dependency direction handler→service→(repo|domain
// interface) intact. Implementations of ApprovalEngineClient must return
// this type for business rejections.
type EngineAPIError struct {
	StatusCode int
	Message    string
}

func (e *EngineAPIError) Error() string {
	return fmt.Sprintf("engine returned %d: %s", e.StatusCode, e.Message)
}

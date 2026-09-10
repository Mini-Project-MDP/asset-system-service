package domain

import "context"

// Asset request status values, as stored in asset_requests.status.
const (
	RequestStatusWaitingApproval = "WAITING_APPROVAL"
	RequestStatusApproved        = "APPROVED"
	RequestStatusRevision        = "REVISION"
	RequestStatusRejected        = "REJECTED"
	RequestStatusFulfillment     = "FULFILLMENT"
	RequestStatusCompleted       = "COMPLETED"
)

// ApprovalSyncPending marks asset_requests.approval_status when creating the
// request in Approval-Engine-Service failed (network/engine down) — the
// local row is kept as-is, not rolled back, so a retry can pick it up later.
// See docs/approval-engine-integration-plan.md Milestone 5.
const ApprovalSyncPending = "PENDING_ENGINE_SYNC"

// Approval actions accepted by POST /api/v1/approvals/{id}/action.
const (
	ApprovalActionApprove  = "approve"
	ApprovalActionRevision = "revision"
	ApprovalActionReject   = "reject"
)

// AssetRequest is one row of asset_requests, joined with the human-readable
// names of its foreign keys (asset type, outlet, distributor, requester).
type AssetRequest struct {
	ID              string
	Category        string // asset_types.name, empty if unset
	Outlet          string // outlets.name, empty if unset
	Distributor     string // distributors.name, empty if unset
	SalesDivision   string
	RequestType     string
	Quantity        int
	Priority        string
	RequesterName   string
	CreatedAt       string
	CurrentStep     int
	FulfillmentStep *int
	FulfillmentData *string
	Status          string

	// Approval Engine integration (see docs/approval-engine-integration-plan.md):
	// pointer + cached mirror of this request's state in
	// Approval-Engine-Service, kept in sync by request_service.
	ApprovalRequestID *string // engine's own id; nil until CreateRequest succeeds
	ApprovalStatus    string  // mirrors the engine's status, or ApprovalSyncPending
	CurrentStepName   *string // mirrors the engine's active step name
	RevisedFromID     *string // set when this request is a resubmission after a "revision" decision
}

// RequesterInfo is what request_service needs about a requester beyond their
// internal id: the identifier shared with Approval-Engine-Service
// (employee_no — see Fase 0 in approval-engine-integration-plan.md) and
// their approval rank (roles.approval_rank), used to build the
// requesterApprovalRank payload field the engine's workflow conditions key
// off of (see Milestone 3's workflow definitions).
type RequesterInfo struct {
	UserID       string
	EmployeeNo   string
	ApprovalRank int
}

// RequestFilter narrows RequestRepository.List results.
type RequestFilter struct {
	Query string // matched against id, outlet, requester name (case-insensitive substring)
	Type  string // asset category; "" or "All types" means no filter
}

// CreateRequestInput is the payload for creating a new asset request.
type CreateRequestInput struct {
	Category      string
	Outlet        string
	Distributor   string
	SalesDivision string
	ReqType       string
	RequesterRole string
	RequesterName string
	Qty           int
	Priority      string
	// RevisedFromID links this request to the one it resubmits after a
	// "revision" decision (see Fase 0 in approval-engine-integration-plan.md:
	// revision = rejected + brand new request, not an in-place edit). Nil for
	// an ordinary first-time submission.
	RevisedFromID *string
}

// ApprovalActionInput is the payload for acting on a pending approval.
type ApprovalActionInput struct {
	Action string
	// ActorEmployeeNo is the acting approver's employee_no — the identity
	// sent to Approval-Engine-Service, which checks it against the currently
	// assigned approver itself (see Milestone 6).
	ActorEmployeeNo string
	// Comment is forwarded to the engine's decision record. Required when
	// Action is ApprovalActionRevision (see ErrRevisionCommentRequired).
	Comment *string
}

// RequestRepository defines data access methods for asset requests.
type RequestRepository interface {
	ListAll(ctx context.Context) ([]AssetRequest, error)
	GetByID(ctx context.Context, id string) (*AssetRequest, error)
	// ResolveRequester finds a requester by name or email, used to attribute a
	// new request to its requester and to build the Approval Engine payload.
	// Returns nil if no match is found.
	ResolveRequester(ctx context.Context, nameOrEmail string) (*RequesterInfo, error)
	Create(ctx context.Context, requesterID string, input CreateRequestInput) (id string, err error)
	SaveFulfillmentData(ctx context.Context, id, fulfillData string) error
	AdvanceFulfillment(ctx context.Context, id string) error
	// SetApprovalEngineRef persists a successful Approval Engine sync at
	// creation time: engine's own request id, its current status, and its
	// active step name.
	SetApprovalEngineRef(ctx context.Context, id, approvalRequestID, approvalStatus string, currentStepName *string) error
	// SetApprovalSyncPending marks a request as not-yet-synced to the engine
	// (create call failed) without touching any other field.
	SetApprovalSyncPending(ctx context.Context, id string) error
	// SetApprovalDecisionResult mirrors the engine's response to a decision
	// (Milestone 6): localStatus is our own asset_requests.status vocabulary
	// (derived from the engine's overall status), engineStatus/currentStepOrder/
	// currentStepName mirror the engine's fields as-is.
	SetApprovalDecisionResult(ctx context.Context, id, localStatus, engineStatus string, currentStepOrder int, currentStepName *string) error
	// ListPendingApprovalSync returns requests stuck at ApprovalSyncPending
	// (engine create failed at submission time) — feeds the retry job in
	// Milestone 7 (see RequestService.RetryPendingApprovalSync).
	ListPendingApprovalSync(ctx context.Context) ([]AssetRequest, error)
}

// RequestService defines business logic for asset requests, approvals, and
// fulfillment. Approvals/Fulfillment reuse the same underlying data as
// List/Detail (see request_handler.go's original comments) — they expose
// separate methods to keep the handler's route-to-method mapping obvious.
type RequestService interface {
	List(ctx context.Context, filter RequestFilter) ([]AssetRequest, error)
	Detail(ctx context.Context, id string) (*AssetRequest, error)
	Create(ctx context.Context, input CreateRequestInput) (*AssetRequest, error)
	ApprovalAction(ctx context.Context, id string, input ApprovalActionInput) (*AssetRequest, error)
	SaveFulfillmentData(ctx context.Context, id, fulfillData string) (*AssetRequest, error)
	AdvanceFulfillment(ctx context.Context, id string) (*AssetRequest, error)
	// RetryPendingApprovalSync re-attempts Approval Engine registration for
	// every request stuck at ApprovalSyncPending (see Milestone 5/7). Meant
	// to be invoked periodically by an external scheduler (e.g. cron calling
	// cmd/retrypendingsync) — not run automatically by the HTTP server.
	RetryPendingApprovalSync(ctx context.Context) (retried, failed int, err error)
}

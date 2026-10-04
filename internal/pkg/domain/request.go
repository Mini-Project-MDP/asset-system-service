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

// ApprovalStepItem represents a single step in the approval chain for display in the FE.
type ApprovalStepItem struct {
	Role      string `json:"role"`
	RoleLabel string `json:"roleLabel"`
	Status    string `json:"status"` // approved, current, pending, rejected, revision
}

// ApprovalHistoryItem represents one event in the approval history timeline.
type ApprovalHistoryItem struct {
	Role    string  `json:"role"`
	Action  string  `json:"action"`
	Date    string  `json:"date"`
	Type    string  `json:"type"` // go, warn, stop
	Comment *string `json:"comment,omitempty"`
}

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

	// RequesterID is the user the request is attributed to; CreatedBy is the
	// user who actually submitted it (empty for requests made before that was
	// recorded). A user may see a request they submitted or are the requester of.
	RequesterID string
	CreatedBy   string

	Chain []ApprovalStepItem
	Hist  []ApprovalHistoryItem
}

// Viewer is who is asking for requests, for visibility decisions.
type Viewer struct {
	// UserID is the caller's id in this system's users table; empty when the
	// token matches no active user.
	UserID string
	// CanReadAll: may see every request (Admin and Asset Team); everyone else
	// sees only the requests they submitted or are the requester of.
	CanReadAll bool
}

// Request status filters accepted by RequestFilter.Status, as the UI labels them.
const (
	RequestStatusFilterWaiting    = "Waiting"
	RequestStatusFilterInProgress = "In progress"
	RequestStatusFilterCompleted  = "Completed"
	RequestStatusFilterRejected   = "Rejected"
	RequestStatusFilterRevision   = "Revision"
)

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
	// Status is a UI status filter (see the RequestStatusFilter* constants);
	// "" or "All status" means no filter.
	Status string
	// VisibleToUser, when set, restricts the result to requests this user
	// submitted or is the requester of.
	VisibleToUser string
}

// RequestListQuery is RequestFilter after validation, in the form the
// repository applies: statuses are stored status values.
type RequestListQuery struct {
	Search        string
	Type          string
	Statuses      []string
	VisibleToUser string
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
	// CreatedBy is the id of the user submitting the request (empty if unknown).
	CreatedBy string
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
	// List returns the requests matching query, newest first, with their
	// approval chain and history.
	List(ctx context.Context, query RequestListQuery) ([]AssetRequest, error)
	GetByID(ctx context.Context, id string) (*AssetRequest, error)
	// GetByApprovalRequestID finds the local request whose approval_request_id
	// matches the engine's own id — the only identifier a webhook from
	// Approval-Engine-Service carries. Returns nil (no error) if no local
	// request has synced to that engine id.
	GetByApprovalRequestID(ctx context.Context, approvalRequestID string) (*AssetRequest, error)
	// ResolveRequester finds a requester by name or email, used to attribute a
	// new request to its requester and to build the Approval Engine payload.
	// Returns nil if no match is found.
	ResolveRequester(ctx context.Context, nameOrEmail string) (*RequesterInfo, error)
	Create(ctx context.Context, requesterID string, input CreateRequestInput) (id string, err error)
	// SaveFulfillmentData stores the (already validated, normalized JSON)
	// asset data and moves the request from Processing to Shipped. It only
	// applies to a request that is ready for processing (approved, or in
	// fulfillment at step 0); ok is false when it was not, so a double click
	// or a concurrent caller can never skip a stage.
	SaveFulfillmentData(ctx context.Context, id, fulfillData string) (ok bool, err error)
	// AdvanceFulfillment moves a request from Shipped to Delivered, and from
	// Delivered to Completed. ok is false when the request is not in one of
	// those two stages; newStep is the stage it reached (3 = completed).
	AdvanceFulfillment(ctx context.Context, id string) (newStep int, ok bool, err error)
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
	// SaveApprovalSteps replaces the cached steps for a request in request_approval_steps.
	SaveApprovalSteps(ctx context.Context, requestID string, steps []ApprovalStepItem) error
	// GetApprovalSteps retrieves the cached steps for a request.
	GetApprovalSteps(ctx context.Context, requestID string) ([]ApprovalStepItem, error)
	// AddHistory inserts an event into request_history.
	AddHistory(ctx context.Context, requestID string, item ApprovalHistoryItem) error
	// GetHistory retrieves the history items for a request.
	GetHistory(ctx context.Context, requestID string) ([]ApprovalHistoryItem, error)
}

// RequestService defines business logic for asset requests, approvals, and
// fulfillment. Approvals/Fulfillment reuse the same underlying data as
// List/Detail (see request_handler.go's original comments) — they expose
// separate methods to keep the handler's route-to-method mapping obvious.
type RequestService interface {
	List(ctx context.Context, filter RequestFilter) ([]AssetRequest, error)
	Detail(ctx context.Context, id string) (*AssetRequest, error)
	// DetailFor is Detail limited to what the viewer may see: a request that
	// is not theirs is reported as not found, so its existence is not revealed.
	DetailFor(ctx context.Context, id string, viewer Viewer) (*AssetRequest, error)
	Create(ctx context.Context, input CreateRequestInput) (*AssetRequest, error)
	ApprovalAction(ctx context.Context, id string, input ApprovalActionInput) (*AssetRequest, error)
	// SaveFulfillmentData validates fulfillData (JSON) for the request's
	// category, stores it and moves the request to Shipped. actor is the
	// acting user's employee_no, recorded in the request history.
	SaveFulfillmentData(ctx context.Context, id, fulfillData, actor string) (*AssetRequest, error)
	// AdvanceFulfillment moves the request to its next stage (Delivered, then
	// Completed). actor is recorded in the request history.
	AdvanceFulfillment(ctx context.Context, id, actor string) (*AssetRequest, error)
	// RetryPendingApprovalSync re-attempts Approval Engine registration for
	// every request stuck at ApprovalSyncPending (see Milestone 5/7). Meant
	// to be invoked periodically by an external scheduler (e.g. cron calling
	// cmd/retrypendingsync) — not run automatically by the HTTP server.
	RetryPendingApprovalSync(ctx context.Context) (retried, failed int, err error)
	// HandleEngineWebhook processes a (signature-verified) notification from
	// Approval-Engine-Service: it resolves the local request by the engine's
	// RequestID and re-fetches/mirrors its current state. Unknown request ids
	// (e.g. a webhook for another consuming app, or one that arrives before
	// our own CreateRequest response is saved) are ignored, not errored —
	// see Langkah 6 in Approval-Engine-Service's asset-system-integration-checklist.md.
	HandleEngineWebhook(ctx context.Context, event EngineWebhookEvent) error
}

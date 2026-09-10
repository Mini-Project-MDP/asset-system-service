package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

type requestService struct {
	repo   domain.RequestRepository
	engine domain.ApprovalEngineClient
}

// NewRequestService creates a new instance of domain.RequestService. engine
// may be nil only in tests that don't exercise Create (e.g. List-only
// fakes) — production wiring (cmd/api/main.go, api/index.go) always passes
// a real client.
func NewRequestService(repo domain.RequestRepository, engine domain.ApprovalEngineClient) domain.RequestService {
	return &requestService{repo: repo, engine: engine}
}

// List returns asset requests matching filter. Filtering is applied in
// application code (not SQL) to preserve the original handler's behavior:
// case-insensitive substring match on id+outlet+requester name, and an exact
// category match unless the type filter is empty or "All types".
func (s *requestService) List(ctx context.Context, filter domain.RequestFilter) ([]domain.AssetRequest, error) {
	all, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	q := strings.ToLower(strings.TrimSpace(filter.Query))
	items := make([]domain.AssetRequest, 0, len(all))
	for _, r := range all {
		if q != "" && !strings.Contains(strings.ToLower(r.ID+r.Outlet+r.RequesterName), q) {
			continue
		}
		if filter.Type != "" && filter.Type != "All types" && r.Category != filter.Type {
			continue
		}
		items = append(items, r)
	}
	return items, nil
}

func (s *requestService) Detail(ctx context.Context, id string) (*domain.AssetRequest, error) {
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrRequestNotFound
	}
	return item, nil
}

func (s *requestService) Create(ctx context.Context, input domain.CreateRequestInput) (*domain.AssetRequest, error) {
	if input.Qty < 1 || input.Category == "" {
		return nil, ErrInvalidRequestPayload
	}

	requester, err := s.repo.ResolveRequester(ctx, input.RequesterName)
	if err != nil {
		return nil, err
	}
	if requester == nil {
		return nil, ErrRequesterNotFound
	}

	id, err := s.repo.Create(ctx, requester.UserID, input)
	if err != nil {
		return nil, err
	}

	s.syncToApprovalEngine(ctx, id, requester, input)

	return s.Detail(ctx, id)
}

// approvalEngineDocType maps an asset category to the Approval-Engine-Service
// workflow doc_type that governs it. Two doc_types, not one per category,
// because a single WorkflowStep only supports one condition — see
// docs/approval-engine-integration-plan.md Fase 0 for why Android and Server
// (identical hierarchy) share a doc_type while Barcode gets its own.
func approvalEngineDocType(category string) string {
	switch category {
	case "Barcode":
		return "asset_request_barcode"
	case "Android", "Server":
		return "asset_request_field_device"
	default:
		return ""
	}
}

// syncToApprovalEngine registers the newly created request with the engine
// and mirrors the result back onto the local row. It never fails Create: an
// engine outage (or an unmapped category) leaves the request saved locally
// with approval_status=PENDING_ENGINE_SYNC for a later retry, instead of
// rolling back a request the user already submitted. See Milestone 5 in
// docs/backend-milestones.md.
func (s *requestService) syncToApprovalEngine(ctx context.Context, id string, requester *domain.RequesterInfo, input domain.CreateRequestInput) {
	docType := approvalEngineDocType(input.Category)
	if docType == "" {
		log.Printf("request %s: no Approval Engine doc_type mapped for category %q, marking sync pending", id, input.Category)
		s.markSyncPending(ctx, id)
		return
	}

	resp, err := s.engine.CreateRequest(ctx, domain.EngineCreateRequestInput{
		DocType:     docType,
		ResourceID:  id,
		RequesterID: requester.EmployeeNo,
		Payload: map[string]any{
			"requesterApprovalRank": requester.ApprovalRank,
			"category":              input.Category,
			"outlet":                input.Outlet,
			"distributor":           input.Distributor,
			"salesDivision":         input.SalesDivision,
			"reqType":               input.ReqType,
			"qty":                   input.Qty,
			"priority":              input.Priority,
		},
	})
	if err != nil {
		log.Printf("request %s: create in Approval Engine failed, marking sync pending: %v", id, err)
		s.markSyncPending(ctx, id)
		return
	}

	stepName := resp.CurrentStepName()
	var stepNamePtr *string
	if stepName != "" {
		stepNamePtr = &stepName
	}
	if err := s.repo.SetApprovalEngineRef(ctx, id, resp.ID, resp.Status, stepNamePtr); err != nil {
		log.Printf("request %s: save approval engine ref failed: %v", id, err)
	}
}

// RetryPendingApprovalSync re-attempts CreateRequest for every request stuck
// at ApprovalSyncPending. Meant to be invoked periodically by an external
// scheduler (cmd/retrypendingsync), not automatically by the HTTP server —
// see Milestone 7 in docs/backend-milestones.md.
func (s *requestService) RetryPendingApprovalSync(ctx context.Context) (retried, failed int, err error) {
	items, err := s.repo.ListPendingApprovalSync(ctx)
	if err != nil {
		return 0, 0, err
	}

	for _, item := range items {
		requester, rErr := s.repo.ResolveRequester(ctx, item.RequesterName)
		if rErr != nil || requester == nil {
			log.Printf("retry sync %s: cannot resolve requester %q: %v", item.ID, item.RequesterName, rErr)
			failed++
			continue
		}

		input := domain.CreateRequestInput{
			Category:      item.Category,
			Outlet:        item.Outlet,
			Distributor:   item.Distributor,
			SalesDivision: item.SalesDivision,
			ReqType:       item.RequestType,
			RequesterName: item.RequesterName,
			Qty:           item.Quantity,
			Priority:      item.Priority,
		}
		s.syncToApprovalEngine(ctx, item.ID, requester, input)

		after, aErr := s.repo.GetByID(ctx, item.ID)
		if aErr == nil && after != nil && after.ApprovalStatus != domain.ApprovalSyncPending {
			retried++
		} else {
			failed++
		}
	}
	return retried, failed, nil
}

func (s *requestService) markSyncPending(ctx context.Context, id string) {
	if err := s.repo.SetApprovalSyncPending(ctx, id); err != nil {
		log.Printf("request %s: mark approval sync pending failed: %v", id, err)
	}
}

// ApprovalAction records one approver's decision by relaying it to
// Approval-Engine-Service (Milestone 6) — this service no longer computes
// status/step locally; the engine is the source of truth for both, and its
// response is mirrored back onto the local row.
func (s *requestService) ApprovalAction(ctx context.Context, id string, input domain.ApprovalActionInput) (*domain.AssetRequest, error) {
	decision, err := approvalEngineDecision(input.Action)
	if err != nil {
		return nil, err
	}
	if decision == domain.EngineDecisionRejected && input.Action == domain.ApprovalActionRevision &&
		(input.Comment == nil || strings.TrimSpace(*input.Comment) == "") {
		return nil, ErrRevisionCommentRequired
	}

	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrRequestNotFound
	}
	if current.ApprovalRequestID == nil {
		return nil, ErrApprovalNotSynced
	}

	resp, err := s.engine.Decide(ctx, *current.ApprovalRequestID, domain.EngineDecisionInput{
		UserID:   input.ActorEmployeeNo,
		Decision: decision,
		Comment:  input.Comment,
	})
	if err != nil {
		var apiErr *domain.EngineAPIError
		if errors.As(err, &apiErr) {
			return nil, apiErr
		}
		return nil, fmt.Errorf("approval engine decide: %w", err)
	}

	localStatus := localStatusForEngineStatus(resp.Status)
	stepName := resp.CurrentStepName()
	var stepNamePtr *string
	if stepName != "" {
		stepNamePtr = &stepName
	}
	if err := s.repo.SetApprovalDecisionResult(ctx, id, localStatus, resp.Status, resp.CurrentStepOrder, stepNamePtr); err != nil {
		return nil, err
	}
	return s.Detail(ctx, id)
}

// approvalEngineDecision maps a FE approval action to the decision value
// sent to Approval-Engine-Service. "revision" is deliberately mapped to
// EngineDecisionRejected — the engine has no third state; see Fase 0 in
// docs/approval-engine-integration-plan.md.
func approvalEngineDecision(action string) (string, error) {
	switch action {
	case domain.ApprovalActionApprove:
		return domain.EngineDecisionApproved, nil
	case domain.ApprovalActionReject, domain.ApprovalActionRevision:
		return domain.EngineDecisionRejected, nil
	default:
		return "", ErrUnsupportedApprovalAction
	}
}

// localStatusForEngineStatus maps the engine's overall request status
// ("pending"/"approved"/"rejected") to this service's own status
// vocabulary. "pending" keeps the request at RequestStatusWaitingApproval —
// a step advanced, but the request as a whole is still awaiting approval.
func localStatusForEngineStatus(engineStatus string) string {
	switch engineStatus {
	case "approved":
		return domain.RequestStatusApproved
	case "rejected":
		return domain.RequestStatusRejected
	default:
		return domain.RequestStatusWaitingApproval
	}
}

func (s *requestService) SaveFulfillmentData(ctx context.Context, id, fulfillData string) (*domain.AssetRequest, error) {
	if err := s.repo.SaveFulfillmentData(ctx, id, fulfillData); err != nil {
		return nil, err
	}
	return s.Detail(ctx, id)
}

func (s *requestService) AdvanceFulfillment(ctx context.Context, id string) (*domain.AssetRequest, error) {
	if err := s.repo.AdvanceFulfillment(ctx, id); err != nil {
		return nil, err
	}
	return s.Detail(ctx, id)
}

// Sentinel business errors. The handler maps these to specific HTTP status
// codes instead of collapsing everything to 500.
var (
	ErrRequestNotFound           = errors.New("request not found")
	ErrInvalidRequestPayload     = errors.New("invalid request payload")
	ErrRequesterNotFound         = errors.New("requester not found")
	ErrUnsupportedApprovalAction = errors.New("unsupported approval action")
	// ErrRevisionCommentRequired: a "revision" decision must carry a comment
	// so the requester knows what to fix (see Milestone 6).
	ErrRevisionCommentRequired = errors.New("comment is required when requesting revision")
	// ErrApprovalNotSynced: the request hasn't been registered with
	// Approval-Engine-Service yet (approval_request_id is still nil) —
	// happens while approval_status=PENDING_ENGINE_SYNC. There is nothing to
	// decide on yet.
	ErrApprovalNotSynced = errors.New("request has not synced with the approval engine yet")
)

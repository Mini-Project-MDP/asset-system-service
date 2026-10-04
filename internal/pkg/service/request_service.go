package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

type requestService struct {
	repo   domain.RequestRepository
	engine domain.ApprovalEngineClient
	forms  domain.RequestFormSource // master data behind the New Request form; see WithFormSource
}

// NewRequestService creates a new instance of domain.RequestService. engine
// may be nil only in tests that don't exercise Create (e.g. List-only
// fakes) — production wiring (cmd/api/main.go, api/index.go) always passes
// a real client.
func NewRequestService(repo domain.RequestRepository, engine domain.ApprovalEngineClient, options ...RequestServiceOption) domain.RequestService {
	s := &requestService{repo: repo, engine: engine}
	for _, option := range options {
		option(s)
	}
	return s
}

// ErrInvalidRequestFilter: a list filter value is not one the API knows.
var ErrInvalidRequestFilter = errors.New("invalid request filter")

// statusesForFilter maps the status filter the UI offers to stored statuses:
// Waiting is waiting for approval, In progress is everything past approval and
// before completion (approved and waiting to be processed, or in fulfillment).
// "" and "All status" mean no filter.
func statusesForFilter(filter string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "", "all status":
		return []string{}, nil
	case strings.ToLower(domain.RequestStatusFilterWaiting):
		return []string{domain.RequestStatusWaitingApproval}, nil
	case strings.ToLower(domain.RequestStatusFilterInProgress):
		return []string{domain.RequestStatusApproved, domain.RequestStatusFulfillment}, nil
	case strings.ToLower(domain.RequestStatusFilterCompleted):
		return []string{domain.RequestStatusCompleted}, nil
	case strings.ToLower(domain.RequestStatusFilterRejected):
		return []string{domain.RequestStatusRejected}, nil
	case strings.ToLower(domain.RequestStatusFilterRevision):
		return []string{domain.RequestStatusRevision}, nil
	}
	return nil, fmt.Errorf("%w: unknown status %q", ErrInvalidRequestFilter, filter)
}

// List returns the asset requests matching filter, newest first. Searching and
// filtering happen in the database; this validates the filter and translates
// the UI's labels ("In progress", "All types") into what the repository applies.
func (s *requestService) List(ctx context.Context, filter domain.RequestFilter) ([]domain.AssetRequest, error) {
	statuses, err := statusesForFilter(filter.Status)
	if err != nil {
		return nil, err
	}
	category := strings.TrimSpace(filter.Type)
	if strings.EqualFold(category, "all types") {
		category = ""
	}
	return s.repo.List(ctx, domain.RequestListQuery{
		Search:        strings.TrimSpace(filter.Query),
		Type:          category,
		Statuses:      statuses,
		VisibleToUser: filter.VisibleToUser,
	})
}

func (s *requestService) Detail(ctx context.Context, id string) (*domain.AssetRequest, error) {
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrRequestNotFound
	}
	s.refreshFromEngine(ctx, item)
	return item, nil
}

// canSee reports whether the viewer may see the request.
func canSee(viewer domain.Viewer, r *domain.AssetRequest) bool {
	if viewer.CanReadAll {
		return true
	}
	return viewer.UserID != "" && (r.CreatedBy == viewer.UserID || r.RequesterID == viewer.UserID)
}

func (s *requestService) DetailFor(ctx context.Context, id string, viewer domain.Viewer) (*domain.AssetRequest, error) {
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// Someone else's request is reported exactly like a missing one, and the
	// check comes before the engine round trip below.
	if item == nil || !canSee(viewer, item) {
		return nil, ErrRequestNotFound
	}
	s.refreshFromEngine(ctx, item)
	return item, nil
}

// refreshFromEngine updates the cached approval steps of a request synced with
// the Approval Engine, when the engine is reachable.
func (s *requestService) refreshFromEngine(ctx context.Context, item *domain.AssetRequest) {
	if s.engine == nil || item.ApprovalRequestID == nil || *item.ApprovalRequestID == "" {
		return
	}
	engReq, engErr := s.engine.GetRequest(ctx, *item.ApprovalRequestID)
	if engErr != nil || engReq == nil || len(engReq.Steps) == 0 {
		return
	}
	hasRevision := false
	for _, h := range item.Hist {
		if h.Action == "Requested revision" {
			hasRevision = true
			break
		}
	}
	steps := mapEngineStepsToItems(engReq, hasRevision)
	if len(steps) > 0 {
		_ = s.repo.SaveApprovalSteps(ctx, item.ID, steps)
		item.Chain = steps
	}
}

func (s *requestService) Create(ctx context.Context, input domain.CreateRequestInput) (*domain.AssetRequest, error) {
	var data *domain.FormMasterData
	if s.forms != nil {
		loaded, err := s.forms.FormMasterData(ctx)
		if err != nil {
			return nil, err
		}
		data = &loaded
	}
	input, err := validateCreate(input, data)
	if err != nil {
		return nil, err
	}

	requester, err := s.resolveRequester(ctx, input)
	if err != nil {
		return nil, err
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
// (identical hierarchy) share a doc_type while Barcode gets its own. Mobile
// Printer joins them too: the business has not fixed its hierarchy yet, and
// the document's interim recommendation is to treat it like Server.
func approvalEngineDocType(category string) string {
	switch category {
	case "Barcode":
		return "asset_request_barcode"
	case "Android", "Server", domain.CategoryMobilePrinter:
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
			"requesterRole":         input.RequesterRole,
			"outlet":                input.Outlet,
			"distributor":           distributorLabel(input),
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

	stepItems := mapEngineStepsToItems(resp, false)
	if len(stepItems) > 0 {
		if err := s.repo.SaveApprovalSteps(ctx, id, stepItems); err != nil {
			log.Printf("request %s: save approval steps failed: %v", id, err)
		}
	}
	submitted := domain.ApprovalHistoryItem{
		Role:   requester.EmployeeNo,
		Action: "Submitted",
		Date:   time.Now().Format("02 Jan 2006"),
		Type:   "go",
	}
	if standsIn(input.RequesterName, requester) {
		remark := "Atas nama: " + input.RequesterName
		submitted.Comment = &remark
	}
	_ = s.repo.AddHistory(ctx, id, submitted)
}

// mapEngineStepsToItems maps Approval-Engine-Service's EngineApprovalStep slice
// into FE-friendly ApprovalStepItem slice.
func mapEngineStepsToItems(req *domain.EngineApprovalRequest, actionWasRevision bool) []domain.ApprovalStepItem {
	if req == nil || len(req.Steps) == 0 {
		return nil
	}
	items := make([]domain.ApprovalStepItem, 0, len(req.Steps))
	for _, s := range req.Steps {
		status := "pending"
		if req.Status == "approved" || s.StepOrder < req.CurrentStepOrder || s.Status == "completed" || s.Status == "approved" {
			status = "approved"
		} else if s.StepOrder == req.CurrentStepOrder {
			if req.Status == "rejected" {
				if actionWasRevision {
					status = "revision"
				} else {
					status = "rejected"
				}
			} else {
				status = "current"
			}
		}
		items = append(items, domain.ApprovalStepItem{
			Role:      s.Name,
			RoleLabel: s.Name,
			Status:    status,
		})
	}
	return items
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
		// The requester is the person stored on the request: the name typed on
		// the form is free text and may belong to nobody.
		requester, rErr := s.repo.ResolveRequesterByUserID(ctx, item.RequesterID)
		if rErr != nil || requester == nil {
			log.Printf("retry sync %s: cannot resolve requester %q: %v", item.ID, item.RequesterID, rErr)
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
			RequesterRole: item.RequesterRole,
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

	isRevision := input.Action == domain.ApprovalActionRevision
	stepItems := mapEngineStepsToItems(resp, isRevision)
	if len(stepItems) > 0 {
		_ = s.repo.SaveApprovalSteps(ctx, id, stepItems)
	}

	actionLabel := "Approved"
	actionType := "go"
	if input.Action == domain.ApprovalActionRevision {
		actionLabel = "Requested revision"
		actionType = "warn"
	} else if input.Action == domain.ApprovalActionReject {
		actionLabel = "Rejected"
		actionType = "stop"
	}

	_ = s.repo.AddHistory(ctx, id, domain.ApprovalHistoryItem{
		Role:    input.ActorEmployeeNo,
		Action:  actionLabel,
		Date:    time.Now().Format("02 Jan 2006"),
		Type:    actionType,
		Comment: input.Comment,
	})

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

// HandleEngineWebhook re-fetches and mirrors a request's current engine
// state after a webhook notification. Per Approval-Engine-Service's own
// contract (Langkah 6, asset-system-integration-checklist.md) delivery order
// is not guaranteed, so this never trusts event.Detail — it always goes back
// to GetRequest for the current truth, exactly like the on-demand refresh in
// Detail.
func (s *requestService) HandleEngineWebhook(ctx context.Context, event domain.EngineWebhookEvent) error {
	requestID := strings.TrimSpace(event.RequestID)
	if requestID == "" {
		return nil
	}

	local, err := s.repo.GetByApprovalRequestID(ctx, requestID)
	if err != nil {
		return err
	}
	if local == nil {
		// Not a request we know about yet (e.g. delivered before our own
		// CreateRequest response finished saving) — nothing to mirror.
		// GET /requests/{id} will pick up the state on the next poll/detail
		// view either way, so this is safe to drop.
		log.Printf("engine webhook: no local request synced to approval_request_id %s (event %q), ignoring", requestID, event.Event)
		return nil
	}

	// Fulfillment/completion is tracked entirely in this service, past what
	// the engine (or this event) knows about — never let a late or
	// out-of-order webhook drag a request that has already moved on back to
	// an approval-stage status.
	if local.Status == domain.RequestStatusFulfillment || local.Status == domain.RequestStatusCompleted {
		return nil
	}

	engReq, err := s.engine.GetRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("refresh request %s after webhook: %w", local.ID, err)
	}
	if engReq == nil {
		return nil
	}

	hasRevision := false
	for _, h := range local.Hist {
		if h.Action == "Requested revision" {
			hasRevision = true
			break
		}
	}

	localStatus := localStatusForEngineStatus(engReq.Status)
	stepName := engReq.CurrentStepName()
	var stepNamePtr *string
	if stepName != "" {
		stepNamePtr = &stepName
	}
	if err := s.repo.SetApprovalDecisionResult(ctx, local.ID, localStatus, engReq.Status, engReq.CurrentStepOrder, stepNamePtr); err != nil {
		return fmt.Errorf("mirror webhook refresh for request %s: %w", local.ID, err)
	}

	stepItems := mapEngineStepsToItems(engReq, hasRevision)
	if len(stepItems) > 0 {
		if err := s.repo.SaveApprovalSteps(ctx, local.ID, stepItems); err != nil {
			log.Printf("request %s: save approval steps after webhook failed: %v", local.ID, err)
		}
	}

	return nil
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

// readyForFulfillmentData reports whether a request is waiting for its asset
// data: approved (not yet picked up), or in fulfillment at Processing (step 0).
func readyForFulfillmentData(r *domain.AssetRequest) bool {
	atProcessing := r.FulfillmentStep == nil || *r.FulfillmentStep == 0
	switch r.Status {
	case domain.RequestStatusApproved:
		return atProcessing
	case domain.RequestStatusFulfillment:
		return r.FulfillmentStep != nil && *r.FulfillmentStep == 0
	}
	return false
}

// recordFulfillmentEvent appends a fulfillment transition to the request
// history. The history is an audit aid: a failure to write it is logged but
// never undoes a transition that already happened.
func (s *requestService) recordFulfillmentEvent(ctx context.Context, id, actor, action string) {
	err := s.repo.AddHistory(ctx, id, domain.ApprovalHistoryItem{
		Role:   actor,
		Action: action,
		Date:   time.Now().Format("02 Jan 2006"),
		Type:   "go",
	})
	if err != nil {
		log.Printf("request %s: record fulfillment event %q failed: %v", id, action, err)
	}
}

func (s *requestService) SaveFulfillmentData(ctx context.Context, id, fulfillData, actor string) (*domain.AssetRequest, error) {
	// GetByID (not Detail): the guard needs only local state, not an engine round trip.
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrRequestNotFound
	}
	if !readyForFulfillmentData(item) {
		return nil, ErrFulfillmentNotReady
	}

	normalized, err := validateFulfillmentData(item.Category, item.Quantity, fulfillData)
	if err != nil {
		return nil, err
	}

	// The repository re-checks the state atomically, so two concurrent saves
	// cannot both succeed.
	ok, err := s.repo.SaveFulfillmentData(ctx, id, normalized)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrFulfillmentNotReady
	}
	s.recordFulfillmentEvent(ctx, id, actor, "Data aset dicatat — Shipped")
	return s.Detail(ctx, id)
}

func (s *requestService) AdvanceFulfillment(ctx context.Context, id, actor string) (*domain.AssetRequest, error) {
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrRequestNotFound
	}

	newStep, ok, err := s.repo.AdvanceFulfillment(ctx, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrFulfillmentNotReady
	}
	if newStep >= 3 {
		s.recordFulfillmentEvent(ctx, id, actor, "Fulfillment selesai — Completed")
	} else {
		s.recordFulfillmentEvent(ctx, id, actor, "Barang diterima — Delivered")
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

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// fakeRequestRepository is an in-memory domain.RequestRepository for testing
// request_service without a database.
type fakeRequestRepository struct {
	items      []domain.AssetRequest
	requesters map[string]domain.RequesterInfo // name/email -> requester info
	createErr  error
	nextID     string
	engineRefs []struct {
		id, approvalRequestID, approvalStatus string
		currentStepName                       *string
	}
	syncPendingCalls []string
	decisionResults  []struct {
		id, localStatus, engineStatus string
		currentStepOrder              int
	}
	steps   map[string][]domain.ApprovalStepItem
	history map[string][]domain.ApprovalHistoryItem
}

func (f *fakeRequestRepository) ListAll(ctx context.Context) ([]domain.AssetRequest, error) {
	return f.items, nil
}

func (f *fakeRequestRepository) GetByID(ctx context.Context, id string) (*domain.AssetRequest, error) {
	for i := range f.items {
		if f.items[i].ID == id {
			item := f.items[i]
			if f.steps != nil {
				item.Chain = f.steps[id]
			}
			if f.history != nil {
				item.Hist = f.history[id]
			}
			return &item, nil
		}
	}
	return nil, nil
}

func (f *fakeRequestRepository) GetByApprovalRequestID(ctx context.Context, approvalRequestID string) (*domain.AssetRequest, error) {
	for i := range f.items {
		if f.items[i].ApprovalRequestID != nil && *f.items[i].ApprovalRequestID == approvalRequestID {
			item := f.items[i]
			if f.steps != nil {
				item.Chain = f.steps[item.ID]
			}
			if f.history != nil {
				item.Hist = f.history[item.ID]
			}
			return &item, nil
		}
	}
	return nil, nil
}

func (f *fakeRequestRepository) ResolveRequester(ctx context.Context, nameOrEmail string) (*domain.RequesterInfo, error) {
	info, ok := f.requesters[nameOrEmail]
	if !ok {
		return nil, nil
	}
	return &info, nil
}

func (f *fakeRequestRepository) Create(ctx context.Context, requesterID string, input domain.CreateRequestInput) (string, error) {
	if f.createErr != nil {
		return "", f.createErr
	}
	id := f.nextID
	f.items = append(f.items, domain.AssetRequest{
		ID:            id,
		Category:      input.Category,
		Outlet:        input.Outlet,
		Distributor:   input.Distributor,
		SalesDivision: input.SalesDivision,
		RequestType:   input.ReqType,
		Quantity:      input.Qty,
		Priority:      input.Priority,
		RequesterName: input.RequesterName,
		Status:        domain.RequestStatusWaitingApproval,
	})
	return id, nil
}

func (f *fakeRequestRepository) SetApprovalDecisionResult(ctx context.Context, id, localStatus, engineStatus string, currentStepOrder int, currentStepName *string) error {
	f.decisionResults = append(f.decisionResults, struct {
		id, localStatus, engineStatus string
		currentStepOrder              int
	}{id, localStatus, engineStatus, currentStepOrder})
	for i := range f.items {
		if f.items[i].ID == id {
			f.items[i].Status = localStatus
			f.items[i].CurrentStep = currentStepOrder
			f.items[i].ApprovalStatus = engineStatus
			f.items[i].CurrentStepName = currentStepName
		}
	}
	return nil
}

func (f *fakeRequestRepository) SaveFulfillmentData(ctx context.Context, id, fulfillData string) error {
	return nil
}

func (f *fakeRequestRepository) AdvanceFulfillment(ctx context.Context, id string) error {
	return nil
}

func (f *fakeRequestRepository) SetApprovalEngineRef(ctx context.Context, id, approvalRequestID, approvalStatus string, currentStepName *string) error {
	f.engineRefs = append(f.engineRefs, struct {
		id, approvalRequestID, approvalStatus string
		currentStepName                       *string
	}{id, approvalRequestID, approvalStatus, currentStepName})
	for i := range f.items {
		if f.items[i].ID == id {
			f.items[i].ApprovalRequestID = &approvalRequestID
			f.items[i].ApprovalStatus = approvalStatus
			f.items[i].CurrentStepName = currentStepName
		}
	}
	return nil
}

func (f *fakeRequestRepository) ListPendingApprovalSync(ctx context.Context) ([]domain.AssetRequest, error) {
	var pending []domain.AssetRequest
	for _, item := range f.items {
		if item.ApprovalStatus == domain.ApprovalSyncPending {
			pending = append(pending, item)
		}
	}
	return pending, nil
}

func (f *fakeRequestRepository) SetApprovalSyncPending(ctx context.Context, id string) error {
	f.syncPendingCalls = append(f.syncPendingCalls, id)
	for i := range f.items {
		if f.items[i].ID == id {
			f.items[i].ApprovalStatus = domain.ApprovalSyncPending
		}
	}
	return nil
}

func (f *fakeRequestRepository) SaveApprovalSteps(ctx context.Context, requestID string, steps []domain.ApprovalStepItem) error {
	if f.steps == nil {
		f.steps = make(map[string][]domain.ApprovalStepItem)
	}
	f.steps[requestID] = steps
	return nil
}

func (f *fakeRequestRepository) GetApprovalSteps(ctx context.Context, requestID string) ([]domain.ApprovalStepItem, error) {
	if f.steps == nil {
		return nil, nil
	}
	return f.steps[requestID], nil
}

func (f *fakeRequestRepository) AddHistory(ctx context.Context, requestID string, item domain.ApprovalHistoryItem) error {
	if f.history == nil {
		f.history = make(map[string][]domain.ApprovalHistoryItem)
	}
	f.history[requestID] = append(f.history[requestID], item)
	return nil
}

func (f *fakeRequestRepository) GetHistory(ctx context.Context, requestID string) ([]domain.ApprovalHistoryItem, error) {
	if f.history == nil {
		return nil, nil
	}
	return f.history[requestID], nil
}

// fakeApprovalEngineClient is an in-memory domain.ApprovalEngineClient for
// testing request_service without a real Approval-Engine-Service.
type fakeApprovalEngineClient struct {
	createErr    error
	createResult *domain.EngineApprovalRequest
	createCalls  []domain.EngineCreateRequestInput

	decideErr    error
	decideResult *domain.EngineApprovalRequest
	decideCalls  []domain.EngineDecisionInput

	getErr    error
	getResult *domain.EngineApprovalRequest
	getCalls  []string
}

func (f *fakeApprovalEngineClient) CreateRequest(ctx context.Context, in domain.EngineCreateRequestInput) (*domain.EngineApprovalRequest, error) {
	f.createCalls = append(f.createCalls, in)
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.createResult != nil {
		return f.createResult, nil
	}
	return &domain.EngineApprovalRequest{ID: "eng-" + in.ResourceID, Status: "pending", CurrentStepOrder: 1}, nil
}

func (f *fakeApprovalEngineClient) Decide(ctx context.Context, approvalRequestID string, in domain.EngineDecisionInput) (*domain.EngineApprovalRequest, error) {
	f.decideCalls = append(f.decideCalls, in)
	if f.decideErr != nil {
		return nil, f.decideErr
	}
	if f.decideResult != nil {
		return f.decideResult, nil
	}
	return &domain.EngineApprovalRequest{Status: "pending", CurrentStepOrder: 2}, nil
}

func (f *fakeApprovalEngineClient) GetRequest(ctx context.Context, approvalRequestID string) (*domain.EngineApprovalRequest, error) {
	f.getCalls = append(f.getCalls, approvalRequestID)
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.getResult != nil {
		return f.getResult, nil
	}
	return nil, errors.New("not implemented in this fake")
}

func TestRequestService_List_FiltersByQueryAndType(t *testing.T) {
	repo := &fakeRequestRepository{items: []domain.AssetRequest{
		{ID: "REQ-0001", Outlet: "Bandung Kota", RequesterName: "Laras P.", Category: "Barcode"},
		{ID: "REQ-0002", Outlet: "Depok Tengah", RequesterName: "Dimas W.", Category: "Android"},
	}}
	svc := NewRequestService(repo, &fakeApprovalEngineClient{})

	t.Run("query matches id/outlet/requester case-insensitively", func(t *testing.T) {
		got, err := svc.List(context.Background(), domain.RequestFilter{Query: "bandung"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].ID != "REQ-0001" {
			t.Fatalf("expected only REQ-0001, got %+v", got)
		}
	})

	t.Run("type filter narrows by category", func(t *testing.T) {
		got, err := svc.List(context.Background(), domain.RequestFilter{Type: "Android"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].ID != "REQ-0002" {
			t.Fatalf("expected only REQ-0002, got %+v", got)
		}
	})

	t.Run("'All types' means no type filter", func(t *testing.T) {
		got, err := svc.List(context.Background(), domain.RequestFilter{Type: "All types"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected both items, got %+v", got)
		}
	})
}

func TestRequestService_Create(t *testing.T) {
	t.Run("succeeds when requester and payload are valid", func(t *testing.T) {
		repo := &fakeRequestRepository{
			requesters: map[string]domain.RequesterInfo{
				"Laras P.": {UserID: "usr_1", EmployeeNo: "EMP101", ApprovalRank: 10},
			},
			nextID: "REQ-NEW1",
		}
		svc := NewRequestService(repo, &fakeApprovalEngineClient{})

		got, err := svc.Create(context.Background(), domain.CreateRequestInput{
			Category:      "Barcode",
			RequesterName: "Laras P.",
			Qty:           2,
			Priority:      "high",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "REQ-NEW1" {
			t.Fatalf("expected id REQ-NEW1, got %s", got.ID)
		}
		if got.Status != domain.RequestStatusWaitingApproval {
			t.Fatalf("expected status WAITING_APPROVAL, got %s", got.Status)
		}
	})

	t.Run("rejects invalid payload before touching the repository", func(t *testing.T) {
		repo := &fakeRequestRepository{}
		svc := NewRequestService(repo, &fakeApprovalEngineClient{})

		_, err := svc.Create(context.Background(), domain.CreateRequestInput{Category: "", Qty: 1})
		if !errors.Is(err, ErrInvalidRequestPayload) {
			t.Fatalf("expected ErrInvalidRequestPayload, got %v", err)
		}

		_, err = svc.Create(context.Background(), domain.CreateRequestInput{Category: "Barcode", Qty: 0})
		if !errors.Is(err, ErrInvalidRequestPayload) {
			t.Fatalf("expected ErrInvalidRequestPayload, got %v", err)
		}
	})

	t.Run("fails when requester cannot be resolved", func(t *testing.T) {
		repo := &fakeRequestRepository{requesters: map[string]domain.RequesterInfo{}}
		svc := NewRequestService(repo, &fakeApprovalEngineClient{})

		_, err := svc.Create(context.Background(), domain.CreateRequestInput{
			Category:      "Barcode",
			RequesterName: "Unknown Person",
			Qty:           1,
		})
		if !errors.Is(err, ErrRequesterNotFound) {
			t.Fatalf("expected ErrRequesterNotFound, got %v", err)
		}
	})

	t.Run("syncs to Approval Engine and stores the returned ref (Milestone 5)", func(t *testing.T) {
		repo := &fakeRequestRepository{
			requesters: map[string]domain.RequesterInfo{
				"Laras P.": {UserID: "usr_1", EmployeeNo: "EMP101", ApprovalRank: 10},
			},
			nextID: "REQ-NEW1",
		}
		engine := &fakeApprovalEngineClient{createResult: &domain.EngineApprovalRequest{
			ID:               "eng-REQ-NEW1",
			Status:           "pending",
			CurrentStepOrder: 1,
			Steps:            []domain.EngineApprovalStep{{StepOrder: 1, Name: "Sales Supervisor"}},
		}}
		svc := NewRequestService(repo, engine)

		got, err := svc.Create(context.Background(), domain.CreateRequestInput{
			Category:      "Barcode",
			RequesterName: "Laras P.",
			Qty:           2,
			Priority:      "high",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(engine.createCalls) != 1 {
			t.Fatalf("expected exactly 1 engine CreateRequest call, got %d", len(engine.createCalls))
		}
		call := engine.createCalls[0]
		if call.DocType != "asset_request_barcode" {
			t.Fatalf("expected doc_type asset_request_barcode, got %s", call.DocType)
		}
		if call.RequesterID != "EMP101" {
			t.Fatalf("expected requester_id EMP101 (employee_no, not internal id), got %s", call.RequesterID)
		}
		if call.Payload["requesterApprovalRank"] != 10 {
			t.Fatalf("expected requesterApprovalRank 10 in payload, got %v", call.Payload["requesterApprovalRank"])
		}

		if got.ApprovalRequestID == nil || *got.ApprovalRequestID != "eng-REQ-NEW1" {
			t.Fatalf("expected approval_request_id eng-REQ-NEW1, got %v", got.ApprovalRequestID)
		}
		if got.ApprovalStatus != "pending" {
			t.Fatalf("expected approval_status pending, got %s", got.ApprovalStatus)
		}
		if got.CurrentStepName == nil || *got.CurrentStepName != "Sales Supervisor" {
			t.Fatalf("expected current_step_name 'Sales Supervisor', got %v", got.CurrentStepName)
		}
	})

	t.Run("engine failure marks sync pending instead of failing Create (Milestone 5)", func(t *testing.T) {
		repo := &fakeRequestRepository{
			requesters: map[string]domain.RequesterInfo{
				"Laras P.": {UserID: "usr_1", EmployeeNo: "EMP101", ApprovalRank: 10},
			},
			nextID: "REQ-NEW1",
		}
		engine := &fakeApprovalEngineClient{createErr: errors.New("connection refused")}
		svc := NewRequestService(repo, engine)

		got, err := svc.Create(context.Background(), domain.CreateRequestInput{
			Category:      "Barcode",
			RequesterName: "Laras P.",
			Qty:           2,
			Priority:      "high",
		})
		if err != nil {
			t.Fatalf("expected Create to succeed despite engine failure, got error: %v", err)
		}
		if got.ApprovalStatus != domain.ApprovalSyncPending {
			t.Fatalf("expected approval_status PENDING_ENGINE_SYNC, got %s", got.ApprovalStatus)
		}
		if got.ApprovalRequestID != nil {
			t.Fatalf("expected no approval_request_id when engine call failed, got %v", *got.ApprovalRequestID)
		}
		if len(repo.syncPendingCalls) != 1 || repo.syncPendingCalls[0] != "REQ-NEW1" {
			t.Fatalf("expected SetApprovalSyncPending called once for REQ-NEW1, got %v", repo.syncPendingCalls)
		}
	})

	t.Run("unmapped category marks sync pending without calling the engine", func(t *testing.T) {
		repo := &fakeRequestRepository{
			requesters: map[string]domain.RequesterInfo{
				"Laras P.": {UserID: "usr_1", EmployeeNo: "EMP101", ApprovalRank: 10},
			},
			nextID: "REQ-NEW1",
		}
		engine := &fakeApprovalEngineClient{}
		svc := NewRequestService(repo, engine)

		got, err := svc.Create(context.Background(), domain.CreateRequestInput{
			Category:      "SomeUnknownCategory",
			RequesterName: "Laras P.",
			Qty:           1,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(engine.createCalls) != 0 {
			t.Fatalf("expected engine not to be called for an unmapped category, got %d calls", len(engine.createCalls))
		}
		if got.ApprovalStatus != domain.ApprovalSyncPending {
			t.Fatalf("expected approval_status PENDING_ENGINE_SYNC, got %s", got.ApprovalStatus)
		}
	})
}

func TestRequestService_RetryPendingApprovalSync(t *testing.T) {
	repo := &fakeRequestRepository{
		requesters: map[string]domain.RequesterInfo{
			"Demo Sales Admin": {UserID: "usr_sa1", EmployeeNo: "EMP101", ApprovalRank: 10},
		},
		items: []domain.AssetRequest{
			{ID: "REQ-P1", Category: "Barcode", RequesterName: "Demo Sales Admin", ApprovalStatus: domain.ApprovalSyncPending},
			{ID: "REQ-P2", Category: "Barcode", RequesterName: "Unknown Requester", ApprovalStatus: domain.ApprovalSyncPending},
			{ID: "REQ-OK", Category: "Barcode", RequesterName: "Demo Sales Admin", ApprovalStatus: "pending"},
		},
	}
	engine := &fakeApprovalEngineClient{createResult: &domain.EngineApprovalRequest{ID: "eng-retry", Status: "pending", CurrentStepOrder: 1}}
	svc := NewRequestService(repo, engine)

	retried, failed, err := svc.RetryPendingApprovalSync(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retried != 1 {
		t.Fatalf("expected 1 successful retry (REQ-P1), got %d", retried)
	}
	if failed != 1 {
		t.Fatalf("expected 1 failed retry (REQ-P2, unresolvable requester), got %d", failed)
	}
	// REQ-OK was never PENDING_ENGINE_SYNC — must not be touched.
	if len(engine.createCalls) != 1 || engine.createCalls[0].ResourceID != "REQ-P1" {
		t.Fatalf("expected the engine to be called exactly once, for REQ-P1 only; got %+v", engine.createCalls)
	}
}

func TestRequestService_ApprovalAction(t *testing.T) {
	syncedID := "eng-req-1"

	newRepo := func() *fakeRequestRepository {
		return &fakeRequestRepository{items: []domain.AssetRequest{
			{ID: "REQ-0001", Status: domain.RequestStatusWaitingApproval, ApprovalRequestID: &syncedID, ApprovalStatus: "pending"},
			{ID: "REQ-NOTSYNCED", Status: domain.RequestStatusWaitingApproval},
		}}
	}

	t.Run("approve relays to the engine and mirrors its response", func(t *testing.T) {
		repo := newRepo()
		engine := &fakeApprovalEngineClient{decideResult: &domain.EngineApprovalRequest{
			Status: "approved", CurrentStepOrder: 6,
		}}
		svc := NewRequestService(repo, engine)

		got, err := svc.ApprovalAction(context.Background(), "REQ-0001", domain.ApprovalActionInput{
			Action: domain.ApprovalActionApprove, ActorEmployeeNo: "EMP102",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(engine.decideCalls) != 1 {
			t.Fatalf("expected 1 engine Decide call, got %d", len(engine.decideCalls))
		}
		call := engine.decideCalls[0]
		if call.UserID != "EMP102" || call.Decision != domain.EngineDecisionApproved {
			t.Fatalf("unexpected decide call: %+v", call)
		}
		if got.Status != domain.RequestStatusApproved {
			t.Fatalf("expected local status APPROVED (engine status 'approved'), got %s", got.Status)
		}
		if got.CurrentStep != 6 {
			t.Fatalf("expected current_step mirrored from engine (6), got %d", got.CurrentStep)
		}
	})

	t.Run("reject relays 'rejected' to the engine", func(t *testing.T) {
		repo := newRepo()
		engine := &fakeApprovalEngineClient{decideResult: &domain.EngineApprovalRequest{Status: "rejected", CurrentStepOrder: 1}}
		svc := NewRequestService(repo, engine)

		got, err := svc.ApprovalAction(context.Background(), "REQ-0001", domain.ApprovalActionInput{
			Action: domain.ApprovalActionReject, ActorEmployeeNo: "EMP102",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if engine.decideCalls[0].Decision != domain.EngineDecisionRejected {
			t.Fatalf("expected decision 'rejected', got %s", engine.decideCalls[0].Decision)
		}
		if got.Status != domain.RequestStatusRejected {
			t.Fatalf("expected local status REJECTED, got %s", got.Status)
		}
	})

	t.Run("revision also relays 'rejected' but requires a comment", func(t *testing.T) {
		repo := newRepo()
		engine := &fakeApprovalEngineClient{decideResult: &domain.EngineApprovalRequest{Status: "rejected", CurrentStepOrder: 1}}
		svc := NewRequestService(repo, engine)

		_, err := svc.ApprovalAction(context.Background(), "REQ-0001", domain.ApprovalActionInput{
			Action: domain.ApprovalActionRevision, ActorEmployeeNo: "EMP102",
		})
		if !errors.Is(err, ErrRevisionCommentRequired) {
			t.Fatalf("expected ErrRevisionCommentRequired without a comment, got %v", err)
		}

		comment := "please attach the missing PO number"
		got, err := svc.ApprovalAction(context.Background(), "REQ-0001", domain.ApprovalActionInput{
			Action: domain.ApprovalActionRevision, ActorEmployeeNo: "EMP102", Comment: &comment,
		})
		if err != nil {
			t.Fatalf("unexpected error with a comment: %v", err)
		}
		if engine.decideCalls[0].Decision != domain.EngineDecisionRejected {
			t.Fatalf("expected 'revision' to map to decision 'rejected', got %s", engine.decideCalls[0].Decision)
		}
		if got.Status != domain.RequestStatusRejected {
			t.Fatalf("expected local status REJECTED for a revision decision, got %s", got.Status)
		}
	})

	t.Run("business rejection from the engine is returned as *domain.EngineAPIError", func(t *testing.T) {
		repo := newRepo()
		engine := &fakeApprovalEngineClient{decideErr: &domain.EngineAPIError{StatusCode: 400, Message: "not the assigned approver"}}
		svc := NewRequestService(repo, engine)

		_, err := svc.ApprovalAction(context.Background(), "REQ-0001", domain.ApprovalActionInput{
			Action: domain.ApprovalActionApprove, ActorEmployeeNo: "someone-else",
		})
		var apiErr *domain.EngineAPIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *domain.EngineAPIError, got %v", err)
		}
		if apiErr.Message != "not the assigned approver" {
			t.Fatalf("unexpected message: %s", apiErr.Message)
		}
	})

	t.Run("acting on a request that hasn't synced to the engine returns ErrApprovalNotSynced", func(t *testing.T) {
		repo := newRepo()
		svc := NewRequestService(repo, &fakeApprovalEngineClient{})

		_, err := svc.ApprovalAction(context.Background(), "REQ-NOTSYNCED", domain.ApprovalActionInput{
			Action: domain.ApprovalActionApprove, ActorEmployeeNo: "EMP102",
		})
		if !errors.Is(err, ErrApprovalNotSynced) {
			t.Fatalf("expected ErrApprovalNotSynced, got %v", err)
		}
	})

	t.Run("unsupported action is rejected before touching the repository or engine", func(t *testing.T) {
		repo := newRepo()
		engine := &fakeApprovalEngineClient{}
		svc := NewRequestService(repo, engine)

		_, err := svc.ApprovalAction(context.Background(), "REQ-0001", domain.ApprovalActionInput{Action: "bogus"})
		if !errors.Is(err, ErrUnsupportedApprovalAction) {
			t.Fatalf("expected ErrUnsupportedApprovalAction, got %v", err)
		}
		if len(engine.decideCalls) != 0 {
			t.Fatalf("expected engine not to be called for an unsupported action")
		}
	})

	t.Run("acting on a missing request returns ErrRequestNotFound", func(t *testing.T) {
		repo := newRepo()
		svc := NewRequestService(repo, &fakeApprovalEngineClient{})

		_, err := svc.ApprovalAction(context.Background(), "REQ-MISSING", domain.ApprovalActionInput{Action: domain.ApprovalActionApprove})
		if !errors.Is(err, ErrRequestNotFound) {
			t.Fatalf("expected ErrRequestNotFound, got %v", err)
		}
	})
}

func TestRequestService_HandleEngineWebhook(t *testing.T) {
	engineID := "eng-REQ-0001"

	t.Run("refreshes and mirrors the local request found by approval_request_id", func(t *testing.T) {
		repo := &fakeRequestRepository{items: []domain.AssetRequest{
			{ID: "REQ-0001", Status: domain.RequestStatusWaitingApproval, ApprovalRequestID: &engineID},
		}}
		engine := &fakeApprovalEngineClient{getResult: &domain.EngineApprovalRequest{
			ID: engineID, Status: "approved", CurrentStepOrder: 2,
			Steps: []domain.EngineApprovalStep{{StepOrder: 1, Name: "Supervisor"}, {StepOrder: 2, Name: "Manager"}},
		}}
		svc := NewRequestService(repo, engine)

		err := svc.HandleEngineWebhook(context.Background(), domain.EngineWebhookEvent{
			Event: "request.approved", RequestID: engineID,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(engine.getCalls) != 1 || engine.getCalls[0] != engineID {
			t.Fatalf("expected engine.GetRequest to be called with %s, got %v", engineID, engine.getCalls)
		}
		if len(repo.decisionResults) != 1 || repo.decisionResults[0].id != "REQ-0001" || repo.decisionResults[0].localStatus != domain.RequestStatusApproved {
			t.Fatalf("expected REQ-0001 mirrored to APPROVED, got %+v", repo.decisionResults)
		}
	})

	t.Run("unknown approval_request_id is ignored, not an error", func(t *testing.T) {
		repo := &fakeRequestRepository{}
		engine := &fakeApprovalEngineClient{}
		svc := NewRequestService(repo, engine)

		err := svc.HandleEngineWebhook(context.Background(), domain.EngineWebhookEvent{
			Event: "request.approved", RequestID: "eng-does-not-exist",
		})
		if err != nil {
			t.Fatalf("expected nil error for an unknown request id, got %v", err)
		}
		if len(engine.getCalls) != 0 {
			t.Fatalf("expected GetRequest not to be called for an unknown request id")
		}
	})

	t.Run("a request already in fulfillment is not regressed by a late webhook", func(t *testing.T) {
		repo := &fakeRequestRepository{items: []domain.AssetRequest{
			{ID: "REQ-0001", Status: domain.RequestStatusFulfillment, ApprovalRequestID: &engineID},
		}}
		engine := &fakeApprovalEngineClient{getResult: &domain.EngineApprovalRequest{ID: engineID, Status: "approved"}}
		svc := NewRequestService(repo, engine)

		err := svc.HandleEngineWebhook(context.Background(), domain.EngineWebhookEvent{
			Event: "request.approved", RequestID: engineID,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(engine.getCalls) != 0 {
			t.Fatalf("expected GetRequest not to be called once fulfillment has started")
		}
		if len(repo.decisionResults) != 0 {
			t.Fatalf("expected no status mirroring once fulfillment has started")
		}
	})

	t.Run("empty request_id is a no-op", func(t *testing.T) {
		repo := &fakeRequestRepository{}
		svc := NewRequestService(repo, &fakeApprovalEngineClient{})

		if err := svc.HandleEngineWebhook(context.Background(), domain.EngineWebhookEvent{Event: "request.approved"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

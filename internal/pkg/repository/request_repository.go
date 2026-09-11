package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/google/uuid"
)

type requestRepository struct {
	db *sql.DB
}

// NewRequestRepository creates a new instance of domain.RequestRepository.
func NewRequestRepository(db *sql.DB) domain.RequestRepository {
	return &requestRepository{db: db}
}

// at.code (not at.name) is selected as the request's category: it's the
// terse value ("Barcode"/"Android"/"Server") the frontend's
// CATEGORY_HIERARCHY and this service's approvalEngineDocType() both key
// off of — at.name is a human-readable label ("Barcode Scanner") meant for
// display, not comparison. See Milestone 7 in docs/backend-milestones.md
// for how the mismatch surfaced (cmd/retrypendingsync round-tripping
// through the DB, unlike the synchronous create path).
const requestSelectColumns = `
	ar.id, at.code, o.name, ar.quantity, ar.priority, d.name, ar.sales_division,
	ar.request_type, u.name, ar.created_at, ar.current_step, ar.fulfillment_step,
	ar.fulfillment_data, ar.status, ar.approval_request_id, ar.approval_status,
	ar.current_step_name, ar.revised_from_id
`

const requestSelectFrom = `
	FROM asset_requests ar
	JOIN users u ON u.id = ar.requester_id
	LEFT JOIN outlets o ON o.id = ar.outlet_id
	LEFT JOIN distributors d ON d.id = ar.distributor_id
	LEFT JOIN asset_types at ON at.id = ar.asset_type_id
`

func scanAssetRequest(scanner interface{ Scan(...any) error }) (domain.AssetRequest, error) {
	var r domain.AssetRequest
	var categoryNS, outletNS, distributorNS, reqTypeNS, fulfillDataNS sql.NullString
	var approvalRequestIDNS, approvalStatusNS, currentStepNameNS, revisedFromIDNS sql.NullString
	var fulfill sql.NullInt64

	if err := scanner.Scan(
		&r.ID, &categoryNS, &outletNS, &r.Quantity, &r.Priority, &distributorNS,
		&r.SalesDivision, &reqTypeNS, &r.RequesterName, &r.CreatedAt, &r.CurrentStep,
		&fulfill, &fulfillDataNS, &r.Status,
		&approvalRequestIDNS, &approvalStatusNS, &currentStepNameNS, &revisedFromIDNS,
	); err != nil {
		return domain.AssetRequest{}, err
	}

	r.Category = categoryNS.String
	r.Outlet = outletNS.String
	r.Distributor = distributorNS.String
	r.RequestType = reqTypeNS.String
	r.ApprovalStatus = approvalStatusNS.String
	if fulfill.Valid {
		step := int(fulfill.Int64)
		r.FulfillmentStep = &step
	}
	if fulfillDataNS.Valid {
		r.FulfillmentData = &fulfillDataNS.String
	}
	if approvalRequestIDNS.Valid {
		r.ApprovalRequestID = &approvalRequestIDNS.String
	}
	if currentStepNameNS.Valid {
		r.CurrentStepName = &currentStepNameNS.String
	}
	if revisedFromIDNS.Valid {
		r.RevisedFromID = &revisedFromIDNS.String
	}
	return r, nil
}

func (repo *requestRepository) ListAll(ctx context.Context) ([]domain.AssetRequest, error) {
	rows, err := repo.db.QueryContext(ctx, `SELECT`+requestSelectColumns+requestSelectFrom+`ORDER BY ar.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list asset requests: %w", err)
	}
	defer rows.Close()

	items := make([]domain.AssetRequest, 0)
	for rows.Next() {
		item, err := scanAssetRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("scan asset request row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate asset request rows: %w", err)
	}

	if len(items) == 0 {
		return items, nil
	}

	// Batch load approval steps for all requests
	stepRows, sErr := repo.db.QueryContext(ctx, `
		SELECT request_id, role_code, role_label, status
		FROM request_approval_steps
		ORDER BY step_order ASC
	`)
	if sErr == nil {
		defer stepRows.Close()
		stepsMap := make(map[string][]domain.ApprovalStepItem)
		for stepRows.Next() {
			var reqID, role, roleLabel, status string
			if err := stepRows.Scan(&reqID, &role, &roleLabel, &status); err == nil {
				stepsMap[reqID] = append(stepsMap[reqID], domain.ApprovalStepItem{
					Role:      role,
					RoleLabel: roleLabel,
					Status:    status,
				})
			}
		}
		for i := range items {
			if s, ok := stepsMap[items[i].ID]; ok {
				items[i].Chain = s
			} else {
				items[i].Chain = []domain.ApprovalStepItem{}
			}
		}
	}

	// Batch load history for all requests
	histRows, hErr := repo.db.QueryContext(ctx, `
		SELECT request_id, role, action, event_type, comment, created_at
		FROM request_history
		ORDER BY created_at ASC
	`)
	if hErr == nil {
		defer histRows.Close()
		histMap := make(map[string][]domain.ApprovalHistoryItem)
		for histRows.Next() {
			var reqID string
			var roleNS, commentNS sql.NullString
			var h domain.ApprovalHistoryItem
			if err := histRows.Scan(&reqID, &roleNS, &h.Action, &h.Type, &commentNS, &h.Date); err == nil {
				h.Role = roleNS.String
				if commentNS.Valid {
					h.Comment = &commentNS.String
				}
				histMap[reqID] = append(histMap[reqID], h)
			}
		}
		for i := range items {
			if h, ok := histMap[items[i].ID]; ok {
				items[i].Hist = h
			} else {
				items[i].Hist = []domain.ApprovalHistoryItem{}
			}
		}
	}

	return items, nil
}

func (repo *requestRepository) GetByID(ctx context.Context, id string) (*domain.AssetRequest, error) {
	row := repo.db.QueryRowContext(ctx, `SELECT`+requestSelectColumns+requestSelectFrom+`WHERE ar.id = $1`, id)
	item, err := scanAssetRequest(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get asset request %s: %w", id, err)
	}

	steps, err := repo.GetApprovalSteps(ctx, id)
	if err == nil && steps != nil {
		item.Chain = steps
	} else {
		item.Chain = []domain.ApprovalStepItem{}
	}

	hist, err := repo.GetHistory(ctx, id)
	if err == nil && hist != nil {
		item.Hist = hist
	} else {
		item.Hist = []domain.ApprovalHistoryItem{}
	}

	return &item, nil
}

func (repo *requestRepository) GetByApprovalRequestID(ctx context.Context, approvalRequestID string) (*domain.AssetRequest, error) {
	row := repo.db.QueryRowContext(ctx, `SELECT`+requestSelectColumns+requestSelectFrom+`WHERE ar.approval_request_id = ?`, approvalRequestID)
	item, err := scanAssetRequest(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get asset request by approval_request_id %s: %w", approvalRequestID, err)
	}

	steps, err := repo.GetApprovalSteps(ctx, item.ID)
	if err == nil && steps != nil {
		item.Chain = steps
	} else {
		item.Chain = []domain.ApprovalStepItem{}
	}

	hist, err := repo.GetHistory(ctx, item.ID)
	if err == nil && hist != nil {
		item.Hist = hist
	} else {
		item.Hist = []domain.ApprovalHistoryItem{}
	}

	return &item, nil
}

func (repo *requestRepository) ResolveRequester(ctx context.Context, nameOrEmail string) (*domain.RequesterInfo, error) {
	var info domain.RequesterInfo
	var approvalRank sql.NullInt64
	err := repo.db.QueryRowContext(ctx, `
		SELECT u.id, u.employee_no, COALESCE(r.approval_rank, 0)
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id AND ur.is_primary = 1
		LEFT JOIN roles r ON r.id = ur.role_id
		WHERE u.name = $1 OR u.email = $2
		LIMIT 1
	`, nameOrEmail, nameOrEmail).Scan(&info.UserID, &info.EmployeeNo, &approvalRank)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve requester %q: %w", nameOrEmail, err)
	}
	info.ApprovalRank = int(approvalRank.Int64)
	return &info, nil
}

func (repo *requestRepository) Create(ctx context.Context, requesterID string, input domain.CreateRequestInput) (string, error) {
	id := "REQ-" + strings.ToUpper(uuid.NewString()[:8])

	// Resolve category to its asset_type_id so it survives a read-back
	var assetTypeID sql.NullString
	_ = repo.db.QueryRowContext(ctx, `SELECT id FROM asset_types WHERE code = $1 OR name = $2 LIMIT 1`, input.Category, input.Category).Scan(&assetTypeID)

	var outletID sql.NullString
	if input.Outlet != "" {
		_ = repo.db.QueryRowContext(ctx, `SELECT id FROM outlets WHERE name = $1 OR code = $2 LIMIT 1`, input.Outlet, input.Outlet).Scan(&outletID)
	}

	var distributorID sql.NullString
	if input.Distributor != "" {
		_ = repo.db.QueryRowContext(ctx, `SELECT id FROM distributors WHERE name = $1 OR code = $2 LIMIT 1`, input.Distributor, input.Distributor).Scan(&distributorID)
	}

	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO asset_requests
			(id, requester_id, asset_type_id, outlet_id, distributor_id, sales_division, request_type, quantity, priority, status, current_step, fulfillment_step, revised_from_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 0, NULL, $11)
	`, id, requesterID, assetTypeID, outletID, distributorID, input.SalesDivision, input.ReqType, input.Qty, input.Priority, domain.RequestStatusWaitingApproval, input.RevisedFromID)
	if err != nil {
		return "", fmt.Errorf("insert asset request: %w", err)
	}
	return id, nil
}

func (repo *requestRepository) ListPendingApprovalSync(ctx context.Context) ([]domain.AssetRequest, error) {
	rows, err := repo.db.QueryContext(ctx,
		`SELECT`+requestSelectColumns+requestSelectFrom+`WHERE ar.approval_status = $1 ORDER BY ar.created_at ASC`,
		domain.ApprovalSyncPending,
	)
	if err != nil {
		return nil, fmt.Errorf("list pending approval sync: %w", err)
	}
	defer rows.Close()

	items := make([]domain.AssetRequest, 0)
	for rows.Next() {
		item, err := scanAssetRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("scan pending approval sync row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending approval sync rows: %w", err)
	}
	return items, nil
}

func (repo *requestRepository) SetApprovalDecisionResult(ctx context.Context, id, localStatus, engineStatus string, currentStepOrder int, currentStepName *string) error {
	if _, err := repo.db.ExecContext(ctx,
		`UPDATE asset_requests SET status = $1, current_step = $2, approval_status = $3, current_step_name = $4, updated_at = CURRENT_TIMESTAMP WHERE id = $5`,
		localStatus, currentStepOrder, engineStatus, currentStepName, id,
	); err != nil {
		return fmt.Errorf("set approval decision result for %s: %w", id, err)
	}
	return nil
}

func (repo *requestRepository) SaveFulfillmentData(ctx context.Context, id, fulfillData string) error {
	if _, err := repo.db.ExecContext(ctx,
		`UPDATE asset_requests SET fulfillment_data = $1, fulfillment_step = 1, status = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3`,
		fulfillData, domain.RequestStatusFulfillment, id,
	); err != nil {
		return fmt.Errorf("save fulfillment data for %s: %w", id, err)
	}
	return nil
}

func (repo *requestRepository) SetApprovalEngineRef(ctx context.Context, id, approvalRequestID, approvalStatus string, currentStepName *string) error {
	if _, err := repo.db.ExecContext(ctx,
		`UPDATE asset_requests SET approval_request_id = $1, approval_status = $2, current_step_name = $3, updated_at = CURRENT_TIMESTAMP WHERE id = $4`,
		approvalRequestID, approvalStatus, currentStepName, id,
	); err != nil {
		return fmt.Errorf("set approval engine ref for %s: %w", id, err)
	}
	return nil
}

func (repo *requestRepository) SetApprovalSyncPending(ctx context.Context, id string) error {
	if _, err := repo.db.ExecContext(ctx,
		`UPDATE asset_requests SET approval_status = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`,
		domain.ApprovalSyncPending, id,
	); err != nil {
		return fmt.Errorf("set approval sync pending for %s: %w", id, err)
	}
	return nil
}

func (repo *requestRepository) AdvanceFulfillment(ctx context.Context, id string) error {
	if _, err := repo.db.ExecContext(ctx, `
		UPDATE asset_requests
		SET fulfillment_step = COALESCE(fulfillment_step, 0) + 1,
			status = CASE WHEN COALESCE(fulfillment_step, 0) + 1 >= 4 THEN $1 ELSE $2 END,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
	`, domain.RequestStatusCompleted, domain.RequestStatusFulfillment, id); err != nil {
		return fmt.Errorf("advance fulfillment for %s: %w", id, err)
	}
	return nil
}

func (repo *requestRepository) SaveApprovalSteps(ctx context.Context, requestID string, steps []domain.ApprovalStepItem) error {
	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx save approval steps: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM request_approval_steps WHERE request_id = $1`, requestID); err != nil {
		return fmt.Errorf("delete old approval steps: %w", err)
	}

	for i, step := range steps {
		stepID := uuid.NewString()
		roleCode := step.Role
		roleLabel := step.RoleLabel
		if roleLabel == "" {
			roleLabel = roleCode
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO request_approval_steps (id, request_id, step_order, role_code, role_label, status)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, stepID, requestID, i+1, roleCode, roleLabel, step.Status); err != nil {
			return fmt.Errorf("insert approval step: %w", err)
		}
	}

	return tx.Commit()
}

func (repo *requestRepository) GetApprovalSteps(ctx context.Context, requestID string) ([]domain.ApprovalStepItem, error) {
	rows, err := repo.db.QueryContext(ctx, `
		SELECT role_code, role_label, status
		FROM request_approval_steps
		WHERE request_id = $1
		ORDER BY step_order ASC
	`, requestID)
	if err != nil {
		return nil, fmt.Errorf("query approval steps: %w", err)
	}
	defer rows.Close()

	var steps []domain.ApprovalStepItem
	for rows.Next() {
		var s domain.ApprovalStepItem
		if err := rows.Scan(&s.Role, &s.RoleLabel, &s.Status); err != nil {
			return nil, fmt.Errorf("scan approval step: %w", err)
		}
		steps = append(steps, s)
	}
	return steps, rows.Err()
}

func (repo *requestRepository) AddHistory(ctx context.Context, requestID string, item domain.ApprovalHistoryItem) error {
	histID := uuid.NewString()
	eventType := item.Type
	if eventType == "" {
		eventType = "go"
	}
	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO request_history (id, request_id, role, action, event_type, comment)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, histID, requestID, item.Role, item.Action, eventType, item.Comment)
	if err != nil {
		return fmt.Errorf("insert request history: %w", err)
	}
	return nil
}

func (repo *requestRepository) GetHistory(ctx context.Context, requestID string) ([]domain.ApprovalHistoryItem, error) {
	rows, err := repo.db.QueryContext(ctx, `
		SELECT role, action, event_type, comment, created_at
		FROM request_history
		WHERE request_id = $1
		ORDER BY created_at ASC
	`, requestID)
	if err != nil {
		return nil, fmt.Errorf("query request history: %w", err)
	}
	defer rows.Close()

	var history []domain.ApprovalHistoryItem
	for rows.Next() {
		var h domain.ApprovalHistoryItem
		var roleNS, commentNS sql.NullString
		if err := rows.Scan(&roleNS, &h.Action, &h.Type, &commentNS, &h.Date); err != nil {
			return nil, fmt.Errorf("scan request history: %w", err)
		}
		h.Role = roleNS.String
		if commentNS.Valid {
			h.Comment = &commentNS.String
		}
		history = append(history, h)
	}
	return history, rows.Err()
}


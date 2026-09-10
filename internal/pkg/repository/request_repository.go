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
	return items, nil
}

func (repo *requestRepository) GetByID(ctx context.Context, id string) (*domain.AssetRequest, error) {
	row := repo.db.QueryRowContext(ctx, `SELECT`+requestSelectColumns+requestSelectFrom+`WHERE ar.id = ?`, id)
	item, err := scanAssetRequest(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get asset request %s: %w", id, err)
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
		WHERE u.name = ? OR u.email = ?
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
	// (previously lost — see the asset_types seeding note in migration.go).
	// A miss (unknown category) degrades gracefully to NULL, same as
	// outlet/distributor already do, rather than failing the whole create.
	var assetTypeID sql.NullString
	_ = repo.db.QueryRowContext(ctx, `SELECT id FROM asset_types WHERE code = ? OR name = ? LIMIT 1`, input.Category, input.Category).Scan(&assetTypeID)

	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO asset_requests
			(id, requester_id, asset_type_id, sales_division, request_type, quantity, priority, status, current_step, fulfillment_step, revised_from_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, NULL, ?)
	`, id, requesterID, assetTypeID, input.SalesDivision, input.ReqType, input.Qty, input.Priority, domain.RequestStatusWaitingApproval, input.RevisedFromID)
	if err != nil {
		return "", fmt.Errorf("insert asset request: %w", err)
	}
	return id, nil
}

func (repo *requestRepository) ListPendingApprovalSync(ctx context.Context) ([]domain.AssetRequest, error) {
	rows, err := repo.db.QueryContext(ctx,
		`SELECT`+requestSelectColumns+requestSelectFrom+`WHERE ar.approval_status = ? ORDER BY ar.created_at ASC`,
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
		`UPDATE asset_requests SET status = ?, current_step = ?, approval_status = ?, current_step_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		localStatus, currentStepOrder, engineStatus, currentStepName, id,
	); err != nil {
		return fmt.Errorf("set approval decision result for %s: %w", id, err)
	}
	return nil
}

func (repo *requestRepository) SaveFulfillmentData(ctx context.Context, id, fulfillData string) error {
	if _, err := repo.db.ExecContext(ctx,
		`UPDATE asset_requests SET fulfillment_data = ?, fulfillment_step = 1, status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		fulfillData, domain.RequestStatusFulfillment, id,
	); err != nil {
		return fmt.Errorf("save fulfillment data for %s: %w", id, err)
	}
	return nil
}

func (repo *requestRepository) SetApprovalEngineRef(ctx context.Context, id, approvalRequestID, approvalStatus string, currentStepName *string) error {
	if _, err := repo.db.ExecContext(ctx,
		`UPDATE asset_requests SET approval_request_id = ?, approval_status = ?, current_step_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		approvalRequestID, approvalStatus, currentStepName, id,
	); err != nil {
		return fmt.Errorf("set approval engine ref for %s: %w", id, err)
	}
	return nil
}

func (repo *requestRepository) SetApprovalSyncPending(ctx context.Context, id string) error {
	if _, err := repo.db.ExecContext(ctx,
		`UPDATE asset_requests SET approval_status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
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
			status = CASE WHEN COALESCE(fulfillment_step, 0) + 1 >= 4 THEN ? ELSE ? END,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, domain.RequestStatusCompleted, domain.RequestStatusFulfillment, id); err != nil {
		return fmt.Errorf("advance fulfillment for %s: %w", id, err)
	}
	return nil
}

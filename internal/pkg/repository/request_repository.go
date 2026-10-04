package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/google/uuid"
	"github.com/lib/pq"
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
// The distributor is the master data one, or the one typed in by hand; the
// requester is the name typed on the form, or else the user the request is
// attributed to (requests made before the name was stored).
const requestSelectColumns = `
	ar.id, at.code, o.name, ar.quantity, ar.priority, COALESCE(d.name, ar.distributor_manual), ar.sales_division,
	ar.request_type, COALESCE(NULLIF(ar.requester_name, ''), u.name), ar.created_at, ar.current_step, ar.fulfillment_step,
	ar.fulfillment_data, ar.status, ar.approval_request_id, ar.approval_status,
	ar.current_step_name, ar.revised_from_id, ar.requester_id, ar.created_by, ar.requester_role
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
	var approvalRequestIDNS, approvalStatusNS, currentStepNameNS, revisedFromIDNS, createdByNS, requesterRoleNS sql.NullString
	var fulfill sql.NullInt64

	if err := scanner.Scan(
		&r.ID, &categoryNS, &outletNS, &r.Quantity, &r.Priority, &distributorNS,
		&r.SalesDivision, &reqTypeNS, &r.RequesterName, &r.CreatedAt, &r.CurrentStep,
		&fulfill, &fulfillDataNS, &r.Status,
		&approvalRequestIDNS, &approvalStatusNS, &currentStepNameNS, &revisedFromIDNS,
		&r.RequesterID, &createdByNS, &requesterRoleNS,
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
	r.CreatedBy = createdByNS.String
	r.RequesterRole = requesterRoleNS.String
	return r, nil
}

// escapeLike makes user input match literally inside a LIKE pattern, where
// backslash, % and _ are special.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// List returns the requests matching the query, newest first, each with its
// approval chain and history. All filtering happens in SQL, and the chain and
// history are read only for the rows returned.
func (repo *requestRepository) List(ctx context.Context, q domain.RequestListQuery) ([]domain.AssetRequest, error) {
	statuses := q.Statuses
	if statuses == nil {
		statuses = []string{} // an empty array, not NULL: cardinality(NULL) would filter everything out
	}
	rows, err := repo.db.QueryContext(ctx, `SELECT`+requestSelectColumns+requestSelectFrom+`
		WHERE ($1 = '' OR ar.id ILIKE $2 OR o.name ILIKE $2 OR u.name ILIKE $2 OR ar.requester_name ILIKE $2)
		  AND ($3 = '' OR at.code = $3)
		  AND (cardinality($4::text[]) = 0 OR ar.status = ANY($4::text[]))
		  AND ($5 = '' OR ar.created_by = $5 OR ar.requester_id = $5)
		ORDER BY ar.created_at DESC, ar.id`,
		q.Search, "%"+escapeLike(q.Search)+"%", q.Type, pq.Array(statuses), q.VisibleToUser)
	if err != nil {
		return nil, fmt.Errorf("list asset requests: %w", err)
	}
	defer rows.Close()

	items := make([]domain.AssetRequest, 0)
	ids := make([]string, 0)
	for rows.Next() {
		item, err := scanAssetRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("scan asset request row: %w", err)
		}
		items = append(items, item)
		ids = append(ids, item.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate asset request rows: %w", err)
	}
	if len(items) == 0 {
		return items, nil
	}
	if err := repo.attachChainAndHistory(ctx, items, ids); err != nil {
		return nil, err
	}
	return items, nil
}

// attachChainAndHistory fills Chain and Hist of items (whose ids are ids) with
// two queries, however many items there are.
func (repo *requestRepository) attachChainAndHistory(ctx context.Context, items []domain.AssetRequest, ids []string) error {
	stepRows, err := repo.db.QueryContext(ctx, `
		SELECT request_id, role_code, role_label, status
		FROM request_approval_steps
		WHERE request_id = ANY($1::text[])
		ORDER BY request_id, step_order ASC`, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("list approval steps: %w", err)
	}
	defer stepRows.Close()
	steps := make(map[string][]domain.ApprovalStepItem, len(items))
	for stepRows.Next() {
		var requestID string
		var step domain.ApprovalStepItem
		if err := stepRows.Scan(&requestID, &step.Role, &step.RoleLabel, &step.Status); err != nil {
			return fmt.Errorf("scan approval step: %w", err)
		}
		steps[requestID] = append(steps[requestID], step)
	}
	if err := stepRows.Err(); err != nil {
		return err
	}

	histRows, err := repo.db.QueryContext(ctx, `
		SELECT request_id, role, action, event_type, comment, created_at
		FROM request_history
		WHERE request_id = ANY($1::text[])
		ORDER BY created_at ASC`, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("list request history: %w", err)
	}
	defer histRows.Close()
	history := make(map[string][]domain.ApprovalHistoryItem, len(items))
	for histRows.Next() {
		var requestID string
		var roleNS, commentNS sql.NullString
		var h domain.ApprovalHistoryItem
		if err := histRows.Scan(&requestID, &roleNS, &h.Action, &h.Type, &commentNS, &h.Date); err != nil {
			return fmt.Errorf("scan request history: %w", err)
		}
		h.Role = roleNS.String
		if commentNS.Valid {
			h.Comment = &commentNS.String
		}
		history[requestID] = append(history[requestID], h)
	}
	if err := histRows.Err(); err != nil {
		return err
	}

	for i := range items {
		items[i].Chain = steps[items[i].ID]
		if items[i].Chain == nil {
			items[i].Chain = []domain.ApprovalStepItem{}
		}
		items[i].Hist = history[items[i].ID]
		if items[i].Hist == nil {
			items[i].Hist = []domain.ApprovalHistoryItem{}
		}
	}
	return nil
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
	row := repo.db.QueryRowContext(ctx, `SELECT`+requestSelectColumns+requestSelectFrom+`WHERE ar.approval_request_id = $1`, approvalRequestID)
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

// requesterSelect finds a user together with the rank of their primary role.
const requesterSelect = `
	SELECT u.id, u.employee_no, COALESCE(r.approval_rank, 0), u.name, u.email
	FROM users u
	LEFT JOIN user_roles ur ON ur.user_id = u.id AND ur.is_primary = 1
	LEFT JOIN roles r ON r.id = ur.role_id`

func (repo *requestRepository) scanRequester(row *sql.Row, what string) (*domain.RequesterInfo, error) {
	var info domain.RequesterInfo
	var approvalRank sql.NullInt64
	if err := row.Scan(&info.UserID, &info.EmployeeNo, &approvalRank, &info.Name, &info.Email); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve requester %q: %w", what, err)
	}
	info.ApprovalRank = int(approvalRank.Int64)
	return &info, nil
}

func (repo *requestRepository) ResolveRequester(ctx context.Context, nameOrEmail string) (*domain.RequesterInfo, error) {
	return repo.scanRequester(repo.db.QueryRowContext(ctx,
		requesterSelect+` WHERE u.name = $1 OR u.email = $2 ORDER BY u.id LIMIT 1`, nameOrEmail, nameOrEmail), nameOrEmail)
}

func (repo *requestRepository) ResolveRequesterByUserID(ctx context.Context, userID string) (*domain.RequesterInfo, error) {
	return repo.scanRequester(repo.db.QueryRowContext(ctx, requesterSelect+` WHERE u.id = $1`, userID), userID)
}

func (repo *requestRepository) Create(ctx context.Context, requesterID string, input domain.CreateRequestInput) (string, error) {
	// Ids are REQ-0001, REQ-0002, ...: the number comes from a sequence, so two
	// requests created at the same moment can never get the same one.
	var number int64
	if err := repo.db.QueryRowContext(ctx, `SELECT nextval('request_number_seq')`).Scan(&number); err != nil {
		return "", fmt.Errorf("next request number: %w", err)
	}
	id := fmt.Sprintf("REQ-%04d", number)

	// The category must exist; an unknown one is an error, not a request without a type.
	var assetTypeID string
	if err := repo.db.QueryRowContext(ctx, `SELECT id FROM asset_types WHERE code = $1`, input.Category).Scan(&assetTypeID); err != nil {
		return "", fmt.Errorf("find asset type %q: %w", input.Category, err)
	}

	// The service has already matched the distributor and outlet against master
	// data and passes their ids; empty means "none" (a distributor typed by hand).
	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO asset_requests
			(id, requester_id, asset_type_id, outlet_id, distributor_id, distributor_manual, sales_division, request_type, quantity, priority, status, current_step, fulfillment_step, revised_from_id, created_by, requester_name, requester_role)
		VALUES ($1, $2, $3, NULLIF($4::text, ''), NULLIF($5::text, ''), NULLIF($6::text, ''), $7, NULLIF($8::text, ''), $9, $10, $11, 0, NULL, $12, NULLIF($13::text, ''), NULLIF($14::text, ''), NULLIF($15::text, ''))
	`, id, requesterID, assetTypeID, input.OutletID, input.DistributorID, input.DistributorManual, input.SalesDivision, input.ReqType,
		input.Qty, input.Priority, domain.RequestStatusWaitingApproval, input.RevisedFromID, input.CreatedBy, input.RequesterName, input.RequesterRole)
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
		// An approved request is ready for the Asset Team: starting its
		// fulfillment step at 0 (Processing) is what puts it in their queue.
		// The status is passed twice ($1 to store, $6 to compare) because one
		// parameter cannot be both a varchar column value and a text comparison.
		`UPDATE asset_requests SET status = $1, current_step = $2, approval_status = $3, current_step_name = $4,
			fulfillment_step = CASE WHEN $6::text = 'APPROVED' THEN COALESCE(fulfillment_step, 0) ELSE fulfillment_step END,
			updated_at = CURRENT_TIMESTAMP WHERE id = $5`,
		localStatus, currentStepOrder, engineStatus, currentStepName, id, localStatus,
	); err != nil {
		return fmt.Errorf("set approval decision result for %s: %w", id, err)
	}
	return nil
}

// saveFulfillmentDataSQL records the asset data and moves Processing to
// Shipped. The WHERE clause is the state guard: only an approved request that
// has not been picked up yet, or one in fulfillment at step 0, matches. Status
// values are trusted constants, inlined to keep the statement free of
// parameter-type inference.
var saveFulfillmentDataSQL = fmt.Sprintf(`
	UPDATE asset_requests
	SET fulfillment_data = $1, fulfillment_step = 1, status = '%[1]s', updated_at = CURRENT_TIMESTAMP
	WHERE id = $2 AND (
		(status = '%[2]s' AND COALESCE(fulfillment_step, 0) = 0)
		OR (status = '%[1]s' AND fulfillment_step = 0)
	)`, domain.RequestStatusFulfillment, domain.RequestStatusApproved)

func (repo *requestRepository) SaveFulfillmentData(ctx context.Context, id, fulfillData string) (bool, error) {
	res, err := repo.db.ExecContext(ctx, saveFulfillmentDataSQL, fulfillData, id)
	if err != nil {
		return false, fmt.Errorf("save fulfillment data for %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("save fulfillment data for %s: %w", id, err)
	}
	return n > 0, nil
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

// advanceFulfillmentSQL moves Shipped (1) to Delivered (2) and Delivered to
// Completed (3). Stages are Processing=0, Shipped=1, Delivered=2, and step 3
// means the request is complete. Only a request in fulfillment at step 1 or 2
// matches, so a request that is unapproved, not yet shipped, or already
// completed is left alone.
var advanceFulfillmentSQL = fmt.Sprintf(`
	UPDATE asset_requests
	SET fulfillment_step = fulfillment_step + 1,
		status = CASE WHEN fulfillment_step + 1 >= 3 THEN '%[1]s' ELSE '%[2]s' END,
		updated_at = CURRENT_TIMESTAMP
	WHERE id = $1 AND status = '%[2]s' AND fulfillment_step IN (1, 2)
	RETURNING fulfillment_step`, domain.RequestStatusCompleted, domain.RequestStatusFulfillment)

func (repo *requestRepository) AdvanceFulfillment(ctx context.Context, id string) (int, bool, error) {
	var newStep int
	err := repo.db.QueryRowContext(ctx, advanceFulfillmentSQL, id).Scan(&newStep)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("advance fulfillment for %s: %w", id, err)
	}
	return newStep, true, nil
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


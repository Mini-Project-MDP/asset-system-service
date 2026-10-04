package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/lib/pq"
)

type dashboardRepository struct {
	db *sql.DB
}

// NewDashboardRepository creates a PostgreSQL-backed domain.DashboardRepository.
func NewDashboardRepository(db *sql.DB) domain.DashboardRepository {
	return &dashboardRepository{db: db}
}

// dashboardStatsSQL counts everything in one pass. Status values are trusted
// constants, inlined to keep the statement free of parameter-type inference.
var dashboardStatsSQL = fmt.Sprintf(`
	SELECT
		COUNT(*) FILTER (WHERE created_at >= $1 AND created_at < $2),
		COUNT(*) FILTER (WHERE status = '%[1]s'),
		COUNT(*) FILTER (WHERE status IN ('%[2]s', '%[3]s')),
		COALESCE(SUM(quantity) FILTER (
			WHERE status = '%[4]s' OR (status = '%[3]s' AND COALESCE(fulfillment_step, 0) >= 1)
		), 0)
	FROM asset_requests`,
	domain.RequestStatusWaitingApproval, domain.RequestStatusApproved,
	domain.RequestStatusFulfillment, domain.RequestStatusCompleted)

func (r *dashboardRepository) Stats(ctx context.Context, monthStart, monthEnd time.Time) (domain.DashboardStats, error) {
	var s domain.DashboardStats
	err := r.db.QueryRowContext(ctx, dashboardStatsSQL, monthStart, monthEnd).
		Scan(&s.RequestsThisMonth, &s.PendingApproval, &s.InProgress, &s.AssetsRegistered)
	if err != nil {
		return domain.DashboardStats{}, fmt.Errorf("dashboard stats: %w", err)
	}
	return s, nil
}

func (r *dashboardRepository) CategoryMonthCounts(ctx context.Context, yearStart, yearEnd time.Time) ([]domain.CategoryMonthCount, error) {
	// Months are counted in WIB, the same zone the service builds the bounds in.
	rows, err := r.db.QueryContext(ctx, `
		SELECT EXTRACT(MONTH FROM ar.created_at AT TIME ZONE 'Asia/Jakarta')::int, t.code, COUNT(*)
		FROM asset_requests ar
		JOIN asset_types t ON t.id = ar.asset_type_id
		WHERE ar.created_at >= $1 AND ar.created_at < $2
		GROUP BY 1, 2`, yearStart, yearEnd)
	if err != nil {
		return nil, fmt.Errorf("dashboard monthly counts: %w", err)
	}
	defer rows.Close()

	var counts []domain.CategoryMonthCount
	for rows.Next() {
		var c domain.CategoryMonthCount
		if err := rows.Scan(&c.Month, &c.Category, &c.Count); err != nil {
			return nil, fmt.Errorf("scan dashboard monthly count: %w", err)
		}
		counts = append(counts, c)
	}
	return counts, rows.Err()
}

func (r *dashboardRepository) RecentActivity(ctx context.Context, limit int) ([]domain.ActivityRecord, error) {
	// request_history.role holds the actor's employee number, so joining users
	// on it yields a name; unknown actors fall back to the stored value.
	rows, err := r.db.QueryContext(ctx, `
		SELECT h.id, h.request_id, COALESCE(NULLIF(u.name, ''), COALESCE(h.role, '')), h.action, h.created_at
		FROM request_history h
		LEFT JOIN users u ON u.employee_no = h.role
		ORDER BY h.created_at DESC, h.id
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("dashboard recent activity: %w", err)
	}
	defer rows.Close()

	var records []domain.ActivityRecord
	for rows.Next() {
		var a domain.ActivityRecord
		if err := rows.Scan(&a.ID, &a.RequestID, &a.Actor, &a.Action, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan dashboard activity: %w", err)
		}
		records = append(records, a)
	}
	return records, rows.Err()
}

func (r *dashboardRepository) AttentionRequests(ctx context.Context, limit int) ([]domain.AssetRequest, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT`+requestSelectColumns+requestSelectFrom+`
		WHERE ar.status = $1
		ORDER BY CASE LOWER(ar.priority) WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 ELSE 2 END,
			ar.created_at ASC, ar.id
		LIMIT $2`, domain.RequestStatusWaitingApproval, limit)
	if err != nil {
		return nil, fmt.Errorf("dashboard attention requests: %w", err)
	}
	defer rows.Close()

	items := make([]domain.AssetRequest, 0, limit)
	ids := make([]string, 0, limit)
	for rows.Next() {
		item, err := scanAssetRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("scan dashboard attention request: %w", err)
		}
		items = append(items, item)
		ids = append(ids, item.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return items, nil
	}

	// One query for the approval chains of all the rows above.
	stepRows, err := r.db.QueryContext(ctx, `
		SELECT request_id, role_code, role_label, status
		FROM request_approval_steps
		WHERE request_id = ANY($1::text[])
		ORDER BY request_id, step_order`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("dashboard attention steps: %w", err)
	}
	defer stepRows.Close()

	chains := make(map[string][]domain.ApprovalStepItem, len(items))
	for stepRows.Next() {
		var requestID string
		var step domain.ApprovalStepItem
		if err := stepRows.Scan(&requestID, &step.Role, &step.RoleLabel, &step.Status); err != nil {
			return nil, fmt.Errorf("scan dashboard attention step: %w", err)
		}
		chains[requestID] = append(chains[requestID], step)
	}
	if err := stepRows.Err(); err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Chain = chains[items[i].ID]
	}
	return items, nil
}

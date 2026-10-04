package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/lib/pq"
)

type navigationRepository struct {
	db *sql.DB
}

// NewNavigationRepository creates a PostgreSQL-backed domain.NavigationRepository.
func NewNavigationRepository(db *sql.DB) domain.NavigationRepository {
	return &navigationRepository{db: db}
}

func (r *navigationRepository) count(ctx context.Context, what, query string, args ...any) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count %s: %w", what, err)
	}
	return n, nil
}

func (r *navigationRepository) CountInProgress(ctx context.Context) (int, error) {
	return r.count(ctx, "in-progress requests",
		`SELECT COUNT(*) FROM asset_requests WHERE status IN ($1, $2)`,
		domain.RequestStatusApproved, domain.RequestStatusFulfillment)
}

func (r *navigationRepository) CountWaitingApproval(ctx context.Context) (int, error) {
	return r.count(ctx, "requests waiting for approval",
		`SELECT COUNT(*) FROM asset_requests WHERE status = $1`, domain.RequestStatusWaitingApproval)
}

// CountWaitingForRoles counts requests whose current approval step belongs to
// one of the roles. A step stores the engine's step name, which may be a role
// code or a role name, so both are matched.
func (r *navigationRepository) CountWaitingForRoles(ctx context.Context, roleCodes, roleNames []string) (int, error) {
	return r.count(ctx, "requests waiting for the caller's roles", `
		SELECT COUNT(DISTINCT ar.id)
		FROM asset_requests ar
		JOIN request_approval_steps s ON s.request_id = ar.id AND s.status = 'current'
		WHERE ar.status = $1
		  AND (s.role_code = ANY($2::text[]) OR s.role_label = ANY($2::text[])
		       OR s.role_code = ANY($3::text[]) OR s.role_label = ANY($3::text[]))`,
		domain.RequestStatusWaitingApproval, pq.Array(roleCodes), pq.Array(roleNames))
}

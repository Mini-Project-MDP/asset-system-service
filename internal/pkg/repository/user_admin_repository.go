package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/google/uuid"
)

type userAdminRepository struct {
	db *sql.DB
}

// NewUserAdminRepository creates a PostgreSQL-backed domain.UserAdminRepository.
func NewUserAdminRepository(db *sql.DB) domain.UserAdminRepository {
	return &userAdminRepository{db: db}
}

func (r *userAdminRepository) GetUser(ctx context.Context, id string) (*domain.UserProfile, error) {
	var p domain.UserProfile
	var isMaster int
	err := r.db.QueryRowContext(ctx,
		`SELECT id, employee_no, name, email, status, is_master FROM users WHERE id = $1`, id).
		Scan(&p.ID, &p.EmployeeNo, &p.Name, &p.Email, &p.Status, &isMaster)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.IsMaster = isMaster == 1

	rows, err := r.db.QueryContext(ctx, `
		SELECT r.id, r.code, r.name, r.role_type, r.approval_rank, r.is_active
		FROM roles r JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1 ORDER BY ur.is_primary DESC, r.name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	p.Roles = []domain.RoleDto{}
	for rows.Next() {
		var role domain.RoleDto
		var active int
		if err := rows.Scan(&role.ID, &role.Code, &role.Name, &role.RoleType, &role.ApprovalRank, &active); err != nil {
			return nil, err
		}
		role.IsActive = active == 1
		p.Roles = append(p.Roles, role)
	}
	return &p, rows.Err()
}

func (r *userAdminRepository) GetRole(ctx context.Context, id string) (*domain.RoleDto, error) {
	var role domain.RoleDto
	var active int
	err := r.db.QueryRowContext(ctx,
		`SELECT id, code, name, role_type, approval_rank, is_active FROM roles WHERE id = $1`, id).
		Scan(&role.ID, &role.Code, &role.Name, &role.RoleType, &role.ApprovalRank, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	role.IsActive = active == 1
	return &role, nil
}

func (r *userAdminRepository) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func setSingleRole(ctx context.Context, tx *sql.Tx, userID, roleID string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO user_roles (id, user_id, role_id, is_primary) VALUES ($1, $2, $3, 1)`,
		uuid.NewString(), userID, roleID)
	return err
}

func (r *userAdminRepository) CreateUser(ctx context.Context, in domain.SaveUserInput, passwordHash, status string) (string, error) {
	id := uuid.NewString()
	err := r.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO users (id, employee_no, name, email, password_hash, status) VALUES ($1, $2, $3, $4, $5, $6)`,
			id, in.EmployeeNo, in.Name, in.Email, passwordHash, status); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_settings (user_id, theme, email_notifications) VALUES ($1, 'light', 1)`, id); err != nil {
			return err
		}
		return setSingleRole(ctx, tx, id, in.RoleID)
	})
	if err != nil {
		return "", mapWriteError(err)
	}
	return id, nil
}

func (r *userAdminRepository) UpdateUser(ctx context.Context, id string, in domain.SaveUserInput, status string) error {
	err := r.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET employee_no = $1, name = $2, email = $3, status = $4,
			 version = version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $5`,
			in.EmployeeNo, in.Name, in.Email, status, id); err != nil {
			return err
		}
		return setSingleRole(ctx, tx, id, in.RoleID)
	})
	return mapWriteError(err)
}

func (r *userAdminRepository) CountOtherActiveAdmins(ctx context.Context, excludeUserID string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT u.id) FROM users u
		JOIN user_roles ur ON ur.user_id = u.id
		JOIN roles r ON r.id = ur.role_id
		WHERE r.code = 'MASTER_ADMIN' AND u.status = 'ACTIVE' AND u.id <> $1`, excludeUserID).Scan(&n)
	return n, err
}

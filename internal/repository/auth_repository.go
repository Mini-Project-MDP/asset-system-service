package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/domain"
	"github.com/google/uuid"
)

type authRepository struct {
	db *sql.DB
}

// NewAuthRepository creates a new instance of domain.AuthRepository.
func NewAuthRepository(db *sql.DB) domain.AuthRepository {
	return &authRepository{db: db}
}

func (r *authRepository) GetUserByEmailOrUsername(ctx context.Context, identifier string) (*domain.User, error) {
	query := `
		SELECT id, employee_no, name, email, password_hash, status, is_master, version, created_at, updated_at
		FROM users
		WHERE email = ? OR employee_no = ?
		LIMIT 1
	`
	var u domain.User
	var isMasterInt int
	err := r.db.QueryRowContext(ctx, query, identifier, identifier).Scan(
		&u.ID, &u.EmployeeNo, &u.Name, &u.Email, &u.PasswordHash, &u.Status, &isMasterInt, &u.Version, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query user by identifier: %w", err)
	}
	u.IsMaster = isMasterInt == 1
	return &u, nil
}

func (r *authRepository) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	query := `
		SELECT id, employee_no, name, email, password_hash, status, is_master, version, created_at, updated_at
		FROM users
		WHERE id = ?
		LIMIT 1
	`
	var u domain.User
	var isMasterInt int
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&u.ID, &u.EmployeeNo, &u.Name, &u.Email, &u.PasswordHash, &u.Status, &isMasterInt, &u.Version, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query user by id: %w", err)
	}
	u.IsMaster = isMasterInt == 1
	return &u, nil
}

func (r *authRepository) GetUserRoles(ctx context.Context, userID string) ([]domain.RoleDto, error) {
	query := `
		SELECT r.id, r.code, r.name, r.role_type, r.approval_rank, r.is_active
		FROM roles r
		INNER JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = ? AND r.is_active = 1
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query user roles: %w", err)
	}
	defer rows.Close()

	var roles []domain.RoleDto
	for rows.Next() {
		var role domain.RoleDto
		var isActiveInt int
		if err := rows.Scan(&role.ID, &role.Code, &role.Name, &role.RoleType, &role.ApprovalRank, &isActiveInt); err != nil {
			return nil, err
		}
		role.IsActive = isActiveInt == 1
		roles = append(roles, role)
	}
	return roles, nil
}

func (r *authRepository) GetUserPermissions(ctx context.Context, userID string) ([]string, error) {
	query := `
		SELECT DISTINCT p.code
		FROM permissions p
		INNER JOIN role_permissions rp ON rp.permission_id = p.id
		INNER JOIN user_roles ur ON ur.role_id = rp.role_id
		WHERE ur.user_id = ? AND p.is_active = 1
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query user permissions: %w", err)
	}
	defer rows.Close()

	var permissions []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		permissions = append(permissions, code)
	}
	return permissions, nil
}

func (r *authRepository) GetUserSettings(ctx context.Context, userID string) (*domain.UserSettingDto, error) {
	query := `SELECT theme, email_notifications FROM user_settings WHERE user_id = ? LIMIT 1`
	var settings domain.UserSettingDto
	var emailNotifInt int
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&settings.Theme, &emailNotifInt)
	if err != nil {
		if err == sql.ErrNoRows {
			return &domain.UserSettingDto{Theme: "light", EmailNotifications: true}, nil
		}
		return nil, fmt.Errorf("query user settings: %w", err)
	}
	settings.EmailNotifications = emailNotifInt == 1
	return &settings, nil
}

func (r *authRepository) UpdateUserSettings(ctx context.Context, userID string, req domain.UpdateSettingsRequest) error {
	emailNotifInt := 0
	if req.EmailNotifications {
		emailNotifInt = 1
	}

	query := `
		INSERT INTO user_settings (user_id, theme, email_notifications, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			theme = excluded.theme,
			email_notifications = excluded.email_notifications,
			updated_at = excluded.updated_at
	`
	_, err := r.db.ExecContext(ctx, query, userID, req.Theme, emailNotifInt, time.Now())
	if err != nil {
		return fmt.Errorf("update user settings: %w", err)
	}
	return nil
}

func (r *authRepository) UpdateMasterUserStatus(ctx context.Context, userID string, isMaster bool) error {
	isMasterInt := 0
	if isMaster {
		isMasterInt = 1
	}
	query := `UPDATE users SET is_master = ?, updated_at = ? WHERE id = ?`
	res, err := r.db.ExecContext(ctx, query, isMasterInt, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("update master status: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil || rows == 0 {
		return fmt.Errorf("user not found or no change")
	}
	return nil
}

func (r *authRepository) GetRoles(ctx context.Context) ([]domain.RoleDto, error) {
	query := `SELECT id, code, name, role_type, approval_rank, is_active FROM roles ORDER BY approval_rank DESC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query roles: %w", err)
	}
	defer rows.Close()

	var roles []domain.RoleDto
	for rows.Next() {
		var role domain.RoleDto
		var isActiveInt int
		if err := rows.Scan(&role.ID, &role.Code, &role.Name, &role.RoleType, &role.ApprovalRank, &isActiveInt); err != nil {
			return nil, err
		}
		role.IsActive = isActiveInt == 1

		// Fetch permissions for each role
		perms, err := r.getRolePermissions(ctx, role.ID)
		if err == nil {
			role.Permissions = perms
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func (r *authRepository) getRolePermissions(ctx context.Context, roleID string) ([]domain.PermissionDto, error) {
	query := `
		SELECT p.id, p.code, p.name, COALESCE(p.description, '')
		FROM permissions p
		INNER JOIN role_permissions rp ON rp.permission_id = p.id
		WHERE rp.role_id = ?
	`
	rows, err := r.db.QueryContext(ctx, query, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []domain.PermissionDto
	for rows.Next() {
		var p domain.PermissionDto
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.Description); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, nil
}

func (r *authRepository) CreateRole(ctx context.Context, req domain.CreateRoleRequest) (*domain.RoleDto, error) {
	roleID := "role_" + uuid.New().String()[:8]
	if req.RoleType == "" {
		req.RoleType = "SYSTEM"
	}
	query := `
		INSERT INTO roles (id, code, name, role_type, approval_rank, is_active, version)
		VALUES (?, ?, ?, ?, ?, 1, 1)
	`
	_, err := r.db.ExecContext(ctx, query, roleID, req.Code, req.Name, req.RoleType, req.ApprovalRank)
	if err != nil {
		return nil, fmt.Errorf("insert role: %w", err)
	}

	return &domain.RoleDto{
		ID:           roleID,
		Code:         req.Code,
		Name:         req.Name,
		RoleType:     req.RoleType,
		ApprovalRank: req.ApprovalRank,
		IsActive:     true,
	}, nil
}

func (r *authRepository) GetPermissions(ctx context.Context) ([]domain.PermissionDto, error) {
	query := `SELECT id, code, name, COALESCE(description, '') FROM permissions ORDER BY code ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query permissions: %w", err)
	}
	defer rows.Close()

	var permissions []domain.PermissionDto
	for rows.Next() {
		var p domain.PermissionDto
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.Description); err != nil {
			return nil, err
		}
		permissions = append(permissions, p)
	}
	return permissions, nil
}

func (r *authRepository) AssignPermissionsToRole(ctx context.Context, roleID string, permissionIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = ?`, roleID); err != nil {
		return err
	}

	for _, pid := range permissionIDs {
		if pid == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)`, roleID, pid); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *authRepository) AssignRolesToUser(ctx context.Context, userID string, roleIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id = ?`, userID); err != nil {
		return err
	}

	for idx, rid := range roleIDs {
		if rid == "" {
			continue
		}
		urID := fmt.Sprintf("ur_%s_%s", userID, rid)
		isPrimary := 0
		if idx == 0 {
			isPrimary = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_roles (id, user_id, role_id, is_primary) VALUES (?, ?, ?, ?)`, urID, userID, rid, isPrimary); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *authRepository) GetUsers(ctx context.Context) ([]domain.UserProfile, error) {
	query := `SELECT id, employee_no, name, email, status, is_master FROM users ORDER BY name ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	var profiles []domain.UserProfile
	for rows.Next() {
		var p domain.UserProfile
		var isMasterInt int
		if err := rows.Scan(&p.ID, &p.EmployeeNo, &p.Name, &p.Email, &p.Status, &isMasterInt); err != nil {
			return nil, err
		}
		p.IsMaster = isMasterInt == 1

		roles, err := r.GetUserRoles(ctx, p.ID)
		if err == nil {
			p.Roles = roles
		}

		perms, err := r.GetUserPermissions(ctx, p.ID)
		if err == nil {
			p.Permissions = perms
		}

		profiles = append(profiles, p)
	}
	return profiles, nil
}

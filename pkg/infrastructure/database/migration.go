package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"golang.org/x/crypto/bcrypt"
)

// SeedUserDef defines a user to be seeded.
type SeedUserDef struct {
	ID         string
	EmployeeNo string
	Name       string
	Email      string
	Password   string
	IsMaster   int
	RoleID     string
}

// MigrateAndSeed initializes database schema and seeds default data.
func MigrateAndSeed(ctx context.Context, db *sql.DB) error {
	log.Println("Executing database schema migrations...")

	schemaSQL := `
	CREATE TABLE IF NOT EXISTS users (
		id VARCHAR(36) PRIMARY KEY,
		employee_no VARCHAR(50) NOT NULL UNIQUE,
		name VARCHAR(150) NOT NULL,
		email VARCHAR(150) NOT NULL UNIQUE,
		password_hash VARCHAR(255) NOT NULL,
		status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
		is_master INTEGER NOT NULL DEFAULT 0,
		version INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS user_settings (
		user_id VARCHAR(36) PRIMARY KEY,
		theme VARCHAR(20) NOT NULL DEFAULT 'light',
		email_notifications INTEGER NOT NULL DEFAULT 1,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS roles (
		id VARCHAR(36) PRIMARY KEY,
		code VARCHAR(50) NOT NULL UNIQUE,
		name VARCHAR(100) NOT NULL,
		role_type VARCHAR(50) NOT NULL DEFAULT 'SYSTEM',
		approval_rank INTEGER NOT NULL DEFAULT 0,
		is_active INTEGER NOT NULL DEFAULT 1,
		version INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS permissions (
		id VARCHAR(36) PRIMARY KEY,
		code VARCHAR(100) NOT NULL UNIQUE,
		name VARCHAR(100) NOT NULL,
		description TEXT,
		is_active INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS role_permissions (
		role_id VARCHAR(36) NOT NULL,
		permission_id VARCHAR(36) NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (role_id, permission_id),
		FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE,
		FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS user_roles (
		id VARCHAR(36) PRIMARY KEY,
		user_id VARCHAR(36) NOT NULL,
		role_id VARCHAR(36) NOT NULL,
		distributor_id VARCHAR(36),
		outlet_id VARCHAR(36),
		is_primary INTEGER NOT NULL DEFAULT 1,
		valid_from DATETIME,
		valid_until DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS regions (
		id VARCHAR(36) PRIMARY KEY,
		code VARCHAR(50) NOT NULL UNIQUE,
		name VARCHAR(100) NOT NULL,
		is_active INTEGER NOT NULL DEFAULT 1,
		version INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS outlets (
		id VARCHAR(36) PRIMARY KEY,
		code VARCHAR(50) NOT NULL UNIQUE,
		name VARCHAR(150) NOT NULL,
		region_id VARCHAR(36) REFERENCES regions(id),
		is_active INTEGER NOT NULL DEFAULT 1,
		version INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS distributors (
		id VARCHAR(36) PRIMARY KEY,
		code VARCHAR(50) NOT NULL UNIQUE,
		name VARCHAR(150) NOT NULL,
		is_active INTEGER NOT NULL DEFAULT 1,
		version INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS asset_types (
		id VARCHAR(36) PRIMARY KEY,
		code VARCHAR(50) NOT NULL UNIQUE,
		name VARCHAR(100) NOT NULL,
		identifier_type VARCHAR(50) NOT NULL DEFAULT 'SERIAL_NUMBER',
		identifier_required INTEGER NOT NULL DEFAULT 1,
		is_active INTEGER NOT NULL DEFAULT 1,
		version INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`

	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("execute schema migration: %w", err)
	}

	log.Println("Seeding permissions, roles, and user data...")
	seedSQL := `
	INSERT OR IGNORE INTO permissions (id, code, name, description) VALUES
	('perm_u_read', 'user:read', 'Read Users', 'View user profiles and list users'),
	('perm_u_write', 'user:write', 'Write Users', 'Create and edit user accounts'),
	('perm_u_delete', 'user:delete', 'Delete Users', 'Deactivate or delete user accounts'),
	('perm_u_master', 'user:master', 'Set Master User', 'Designate master user privileges'),
	('perm_r_manage', 'role:manage', 'Manage Roles', 'Create, update roles and assign permissions'),
	('perm_p_read', 'permission:read', 'Read Permissions', 'View available permissions'),
	('perm_a_read', 'asset:read', 'Read Assets', 'View assets and inventory'),
	('perm_a_write', 'asset:write', 'Write Assets', 'Register and edit assets'),
	('perm_req_create', 'request:create', 'Create Request', 'Submit new asset requests'),
	('perm_req_read', 'request:read', 'Read Requests', 'View asset requests'),
	('perm_req_approve', 'request:approve', 'Approve Request', 'Approve or reject asset requests'),
	('perm_ful_process', 'fulfillment:process', 'Process Fulfillment', 'Manage asset dispatch and fulfillment'),
	('perm_ful_read', 'fulfillment:read', 'Read Fulfillment', 'View fulfillment statuses'),
	('perm_set_manage', 'settings:manage', 'Manage Settings', 'Update system and application settings');

	INSERT OR IGNORE INTO roles (id, code, name, role_type, approval_rank) VALUES
	('role_master', 'MASTER_ADMIN', 'Master Admin', 'SYSTEM', 100),
	('role_mgr', 'ASSET_MANAGER', 'Asset Manager', 'OPERATIONAL', 80),
	('role_appr', 'DEPARTMENT_APPROVER', 'Department Approver', 'APPROVAL', 50),
	('role_staff', 'FIELD_STAFF', 'Field Staff', 'OPERATIONAL', 30),
	('role_user', 'REGULAR_USER', 'Regular User', 'GENERAL', 10);

	INSERT OR IGNORE INTO role_permissions (role_id, permission_id) VALUES
	('role_master', 'perm_u_read'), ('role_master', 'perm_u_write'), ('role_master', 'perm_u_delete'), ('role_master', 'perm_u_master'),
	('role_master', 'perm_r_manage'), ('role_master', 'perm_p_read'),
	('role_master', 'perm_a_read'), ('role_master', 'perm_a_write'),
	('role_master', 'perm_req_create'), ('role_master', 'perm_req_read'), ('role_master', 'perm_req_approve'),
	('role_master', 'perm_ful_process'), ('role_master', 'perm_ful_read'),
	('role_master', 'perm_set_manage'),
	('role_mgr', 'perm_u_read'), ('role_mgr', 'perm_a_read'), ('role_mgr', 'perm_a_write'),
	('role_mgr', 'perm_req_create'), ('role_mgr', 'perm_req_read'), ('role_mgr', 'perm_req_approve'),
	('role_mgr', 'perm_ful_process'), ('role_mgr', 'perm_ful_read'),
	('role_appr', 'perm_u_read'), ('role_appr', 'perm_a_read'),
	('role_appr', 'perm_req_create'), ('role_appr', 'perm_req_read'), ('role_appr', 'perm_req_approve'),
	('role_staff', 'perm_u_read'), ('role_staff', 'perm_a_read'),
	('role_staff', 'perm_ful_process'), ('role_staff', 'perm_ful_read'),
	('role_user', 'perm_u_read'), ('role_user', 'perm_a_read'),
	('role_user', 'perm_req_create'), ('role_user', 'perm_req_read');
	`

	if _, err := db.ExecContext(ctx, seedSQL); err != nil {
		return fmt.Errorf("execute static seed SQL: %w", err)
	}

	// Seed users with bcrypt hashed password ("Password123!")
	defaultPassword := "Password123!"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash default password: %w", err)
	}

	usersToSeed := []SeedUserDef{
		// Master Admin Users (is_master = 1)
		{ID: "usr_master1", EmployeeNo: "EMP001", Name: "Master Admin One", Email: "master1@mayora.com", Password: string(hashedPassword), IsMaster: 1, RoleID: "role_master"},
		{ID: "usr_master2", EmployeeNo: "EMP002", Name: "Master Admin Two", Email: "master2@mayora.com", Password: string(hashedPassword), IsMaster: 1, RoleID: "role_master"},
		// Asset Managers
		{ID: "usr_mgr1", EmployeeNo: "EMP003", Name: "Manager Asset One", Email: "manager1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_mgr"},
		{ID: "usr_mgr2", EmployeeNo: "EMP004", Name: "Manager Asset Two", Email: "manager2@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_mgr"},
		// Department Approvers
		{ID: "usr_appr1", EmployeeNo: "EMP005", Name: "Approver One", Email: "approver1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_appr"},
		{ID: "usr_appr2", EmployeeNo: "EMP006", Name: "Approver Two", Email: "approver2@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_appr"},
		// Field Staff
		{ID: "usr_staff1", EmployeeNo: "EMP007", Name: "Field Staff One", Email: "staff1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_staff"},
		{ID: "usr_staff2", EmployeeNo: "EMP008", Name: "Field Staff Two", Email: "staff2@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_staff"},
		// Regular Users
		{ID: "usr_user1", EmployeeNo: "EMP009", Name: "Regular User One", Email: "user1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_user"},
		{ID: "usr_user2", EmployeeNo: "EMP010", Name: "Regular User Two", Email: "user2@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_user"},
	}

	for _, u := range usersToSeed {
		_, err := db.ExecContext(ctx, `
			INSERT INTO users (id, employee_no, name, email, password_hash, status, is_master)
			VALUES (?, ?, ?, ?, ?, 'ACTIVE', ?)
			ON CONFLICT(id) DO UPDATE SET
				password_hash = excluded.password_hash,
				is_master = excluded.is_master
		`, u.ID, u.EmployeeNo, u.Name, u.Email, u.Password, u.IsMaster)
		if err != nil {
			return fmt.Errorf("seed user %s: %w", u.Email, err)
		}

		// Seed user_settings
		_, _ = db.ExecContext(ctx, `
			INSERT OR IGNORE INTO user_settings (user_id, theme, email_notifications)
			VALUES (?, 'light', 1)
		`, u.ID)

		// Seed user_roles
		userRoleID := fmt.Sprintf("ur_%s_%s", u.ID, u.RoleID)
		_, _ = db.ExecContext(ctx, `
			INSERT OR IGNORE INTO user_roles (id, user_id, role_id, is_primary)
			VALUES (?, ?, ?, 1)
		`, userRoleID, u.ID, u.RoleID)
	}

	log.Println("Database migration and seeding completed successfully.")
	return nil
}

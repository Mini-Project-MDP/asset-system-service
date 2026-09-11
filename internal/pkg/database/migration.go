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

// ensureApprovalEngineColumns adds any approvalEngineColumns missing from an
// existing asset_requests or request_history table.
func ensureApprovalEngineColumns(ctx context.Context, db *sql.DB) error {
	queries := []string{
		`ALTER TABLE asset_requests ADD COLUMN IF NOT EXISTS approval_request_id VARCHAR(36)`,
		`ALTER TABLE asset_requests ADD COLUMN IF NOT EXISTS approval_status VARCHAR(30)`,
		`ALTER TABLE asset_requests ADD COLUMN IF NOT EXISTS current_step_name VARCHAR(100)`,
		`ALTER TABLE asset_requests ADD COLUMN IF NOT EXISTS revised_from_id VARCHAR(36)`,
		`ALTER TABLE request_history ADD COLUMN IF NOT EXISTS role VARCHAR(100)`,
		`ALTER TABLE request_history ADD COLUMN IF NOT EXISTS event_type VARCHAR(20) DEFAULT 'go'`,
	}
	for _, q := range queries {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("ensure column: %w", err)
		}
	}
	return nil
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
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS user_settings (
		user_id VARCHAR(36) PRIMARY KEY,
		theme VARCHAR(20) NOT NULL DEFAULT 'light',
		email_notifications INTEGER NOT NULL DEFAULT 1,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
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
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS permissions (
		id VARCHAR(36) PRIMARY KEY,
		code VARCHAR(100) NOT NULL UNIQUE,
		name VARCHAR(100) NOT NULL,
		description TEXT,
		is_active INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS role_permissions (
		role_id VARCHAR(36) NOT NULL,
		permission_id VARCHAR(36) NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
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
		valid_from TIMESTAMPTZ,
		valid_until TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS regions (
		id VARCHAR(36) PRIMARY KEY,
		code VARCHAR(50) NOT NULL UNIQUE,
		name VARCHAR(100) NOT NULL,
		is_active INTEGER NOT NULL DEFAULT 1,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS outlets (
		id VARCHAR(36) PRIMARY KEY,
		code VARCHAR(50) NOT NULL UNIQUE,
		name VARCHAR(150) NOT NULL,
		region_id VARCHAR(36) REFERENCES regions(id),
		is_active INTEGER NOT NULL DEFAULT 1,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS distributors (
		id VARCHAR(36) PRIMARY KEY,
		code VARCHAR(50) NOT NULL UNIQUE,
		name VARCHAR(150) NOT NULL,
		is_active INTEGER NOT NULL DEFAULT 1,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS asset_types (
		id VARCHAR(36) PRIMARY KEY,
		code VARCHAR(50) NOT NULL UNIQUE,
		name VARCHAR(100) NOT NULL,
		identifier_type VARCHAR(50) NOT NULL DEFAULT 'SERIAL_NUMBER',
		identifier_required INTEGER NOT NULL DEFAULT 1,
		is_active INTEGER NOT NULL DEFAULT 1,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS asset_requests (
		id VARCHAR(36) PRIMARY KEY,
		requester_id VARCHAR(36) NOT NULL,
		outlet_id VARCHAR(36),
		distributor_id VARCHAR(36),
		asset_type_id VARCHAR(36),
		sales_division VARCHAR(100) NOT NULL,
		request_type VARCHAR(100),
		quantity INTEGER NOT NULL,
		priority VARCHAR(20) NOT NULL DEFAULT 'normal',
		status VARCHAR(30) NOT NULL DEFAULT 'WAITING_APPROVAL',
		current_step INTEGER NOT NULL DEFAULT 0,
		fulfillment_step INTEGER,
		fulfillment_data TEXT,
		approval_request_id VARCHAR(36),
		approval_status VARCHAR(30),
		current_step_name VARCHAR(100),
		revised_from_id VARCHAR(36),
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (requester_id) REFERENCES users(id),
		FOREIGN KEY (outlet_id) REFERENCES outlets(id),
		FOREIGN KEY (distributor_id) REFERENCES distributors(id),
		FOREIGN KEY (asset_type_id) REFERENCES asset_types(id),
		FOREIGN KEY (revised_from_id) REFERENCES asset_requests(id)
	);

	CREATE TABLE IF NOT EXISTS request_approval_steps (
		id VARCHAR(36) PRIMARY KEY,
		request_id VARCHAR(36) NOT NULL,
		step_order INTEGER NOT NULL,
		role_code VARCHAR(50) NOT NULL,
		role_label VARCHAR(100) NOT NULL,
		status VARCHAR(20) NOT NULL DEFAULT 'pending',
		acted_by VARCHAR(36),
		acted_at TIMESTAMPTZ,
		comment TEXT,
		UNIQUE(request_id, step_order),
		FOREIGN KEY (request_id) REFERENCES asset_requests(id) ON DELETE CASCADE,
		FOREIGN KEY (acted_by) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS request_history (
		id VARCHAR(36) PRIMARY KEY,
		request_id VARCHAR(36) NOT NULL,
		actor_id VARCHAR(36),
		role VARCHAR(100),
		event_type VARCHAR(20) DEFAULT 'go',
		action VARCHAR(50) NOT NULL,
		comment TEXT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (request_id) REFERENCES asset_requests(id) ON DELETE CASCADE,
		FOREIGN KEY (actor_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS m_employee (
		nik VARCHAR(50) PRIMARY KEY,
		emp_id VARCHAR(50),
		emp_nm VARCHAR(150),
		user_id VARCHAR(36),
		full_name VARCHAR(150),
		is_vacant VARCHAR(5) DEFAULT 'N',
		superior_id VARCHAR(50),
		is_terminate VARCHAR(5) DEFAULT 'N'
	);
	`

	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("execute schema migration: %w", err)
	}

	if err := ensureApprovalEngineColumns(ctx, db); err != nil {
		return fmt.Errorf("backfill approval engine columns: %w", err)
	}

	log.Println("Seeding permissions, roles, and user data...")
	seedSQL := `
	INSERT INTO permissions (id, code, name, description) VALUES
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
	('perm_set_manage', 'settings:manage', 'Manage Settings', 'Update system and application settings')
	ON CONFLICT (id) DO NOTHING;

	INSERT INTO asset_types (id, code, name, identifier_type, identifier_required) VALUES
	('atype_barcode', 'Barcode', 'Barcode Scanner', 'SERIAL_NUMBER', 1),
	('atype_android', 'Android', 'Android Device', 'SERIAL_NUMBER', 1),
	('atype_server', 'Server', 'Server', 'SERIAL_NUMBER', 1)
	ON CONFLICT (id) DO NOTHING;

	INSERT INTO roles (id, code, name, role_type, approval_rank) VALUES
	('role_master', 'MASTER_ADMIN', 'Master Admin', 'SYSTEM', 100),
	('role_mgr', 'ASSET_MANAGER', 'Asset Manager', 'OPERATIONAL', 80),
	('role_appr', 'DEPARTMENT_APPROVER', 'Department Approver', 'APPROVAL', 50),
	('role_staff', 'FIELD_STAFF', 'Field Staff', 'OPERATIONAL', 30),
	('role_user', 'REGULAR_USER', 'Regular User', 'GENERAL', 10),
	('role_sa', 'SA', 'Sales Admin', 'APPROVAL', 10),
	('role_ss', 'SS', 'Sales Supervisor', 'APPROVAL', 20),
	('role_rsm', 'RSM', 'Regional Sales Manager', 'APPROVAL', 30),
	('role_grsm', 'GRSM', 'Group Regional Sales Manager', 'APPROVAL', 40),
	('role_nsm', 'NSM', 'National Sales Manager', 'APPROVAL', 50),
	('role_sd', 'SD', 'Sales Director', 'APPROVAL', 60),
	('role_cabang', 'Cabang', 'Cabang / Distributor', 'APPROVAL', 25)
	ON CONFLICT (id) DO NOTHING;

	INSERT INTO role_permissions (role_id, permission_id) VALUES
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
	('role_user', 'perm_req_create'), ('role_user', 'perm_req_read'),
	('role_sa', 'perm_req_create'), ('role_sa', 'perm_req_read'),
	('role_ss', 'perm_req_create'), ('role_ss', 'perm_req_read'), ('role_ss', 'perm_req_approve'),
	('role_rsm', 'perm_req_create'), ('role_rsm', 'perm_req_read'), ('role_rsm', 'perm_req_approve'),
	('role_grsm', 'perm_req_create'), ('role_grsm', 'perm_req_read'), ('role_grsm', 'perm_req_approve'),
	('role_nsm', 'perm_req_create'), ('role_nsm', 'perm_req_read'), ('role_nsm', 'perm_req_approve'),
	('role_sd', 'perm_req_create'), ('role_sd', 'perm_req_read'), ('role_sd', 'perm_req_approve'),
	('role_cabang', 'perm_req_create'), ('role_cabang', 'perm_req_read')
	ON CONFLICT (role_id, permission_id) DO NOTHING;
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
		{ID: "usr_master1", EmployeeNo: "EMP001", Name: "Master Admin One", Email: "master1@mayora.com", Password: string(hashedPassword), IsMaster: 1, RoleID: "role_master"},
		{ID: "usr_master2", EmployeeNo: "EMP002", Name: "Master Admin Two", Email: "master2@mayora.com", Password: string(hashedPassword), IsMaster: 1, RoleID: "role_master"},
		{ID: "usr_mgr1", EmployeeNo: "EMP003", Name: "Manager Asset One", Email: "manager1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_mgr"},
		{ID: "usr_mgr2", EmployeeNo: "EMP004", Name: "Manager Asset Two", Email: "manager2@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_mgr"},
		{ID: "usr_appr1", EmployeeNo: "EMP005", Name: "Approver One", Email: "approver1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_appr"},
		{ID: "usr_appr2", EmployeeNo: "EMP006", Name: "Approver Two", Email: "approver2@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_appr"},
		{ID: "usr_staff1", EmployeeNo: "EMP007", Name: "Field Staff One", Email: "staff1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_staff"},
		{ID: "usr_staff2", EmployeeNo: "EMP008", Name: "Field Staff Two", Email: "staff2@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_staff"},
		{ID: "usr_user1", EmployeeNo: "EMP009", Name: "Regular User One", Email: "user1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_user"},
		{ID: "usr_user2", EmployeeNo: "EMP010", Name: "Regular User Two", Email: "user2@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_user"},
		{ID: "usr_sa1", EmployeeNo: "EMP101", Name: "Demo Sales Admin", Email: "demo.sa1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_sa"},
		{ID: "usr_ss1", EmployeeNo: "EMP102", Name: "Demo Sales Supervisor", Email: "demo.ss1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_ss"},
		{ID: "usr_rsm1", EmployeeNo: "EMP103", Name: "Demo Regional Sales Manager", Email: "demo.rsm1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_rsm"},
		{ID: "usr_grsm1", EmployeeNo: "EMP104", Name: "Demo Group RSM", Email: "demo.grsm1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_grsm"},
		{ID: "usr_nsm1", EmployeeNo: "EMP105", Name: "Demo National Sales Manager", Email: "demo.nsm1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_nsm"},
		{ID: "usr_sd1", EmployeeNo: "EMP106", Name: "Demo Sales Director", Email: "demo.sd1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_sd"},
		{ID: "usr_cabang1", EmployeeNo: "EMP107", Name: "Demo Cabang", Email: "demo.cabang1@mayora.com", Password: string(hashedPassword), IsMaster: 0, RoleID: "role_cabang"},
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin seed tx: %w", err)
	}
	defer tx.Rollback()

	for _, u := range usersToSeed {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO users (id, employee_no, name, email, password_hash, status, is_master)
			VALUES ($1, $2, $3, $4, $5, 'ACTIVE', $6)
			ON CONFLICT (id) DO UPDATE SET
				password_hash = EXCLUDED.password_hash,
				is_master = EXCLUDED.is_master
		`, u.ID, u.EmployeeNo, u.Name, u.Email, u.Password, u.IsMaster)
		if err != nil {
			return fmt.Errorf("seed user %s: %w", u.Email, err)
		}

		// Seed user_settings
		_, _ = tx.ExecContext(ctx, `
			INSERT INTO user_settings (user_id, theme, email_notifications)
			VALUES ($1, 'light', 1)
			ON CONFLICT (user_id) DO NOTHING
		`, u.ID)

		// Seed user_roles
		userRoleID := fmt.Sprintf("ur_%s_%s", u.ID, u.RoleID)
		_, _ = tx.ExecContext(ctx, `
			INSERT INTO user_roles (id, user_id, role_id, is_primary)
			VALUES ($1, $2, $3, 1)
			ON CONFLICT (id) DO NOTHING
		`, userRoleID, u.ID, u.RoleID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed tx: %w", err)
	}

	if err := seedDemoApprovalHierarchy(ctx, db); err != nil {
		return fmt.Errorf("seed demo approval hierarchy: %w", err)
	}

	log.Println("Database migration and seeding completed successfully.")
	return nil
}

type demoEmployee struct {
	NIK        string
	UserID     string
	FullName   string
	SuperiorID string
}

func seedDemoApprovalHierarchy(ctx context.Context, db *sql.DB) error {
	employees := []demoEmployee{
		{NIK: "EMP106", UserID: "usr_sd1", FullName: "Demo Sales Director", SuperiorID: ""},
		{NIK: "EMP105", UserID: "usr_nsm1", FullName: "Demo National Sales Manager", SuperiorID: "EMP106"},
		{NIK: "EMP104", UserID: "usr_grsm1", FullName: "Demo Group RSM", SuperiorID: "EMP105"},
		{NIK: "EMP103", UserID: "usr_rsm1", FullName: "Demo Regional Sales Manager", SuperiorID: "EMP104"},
		{NIK: "EMP102", UserID: "usr_ss1", FullName: "Demo Sales Supervisor", SuperiorID: "EMP103"},
		{NIK: "EMP101", UserID: "usr_sa1", FullName: "Demo Sales Admin", SuperiorID: "EMP102"},
		{NIK: "EMP107", UserID: "usr_cabang1", FullName: "Demo Cabang", SuperiorID: "EMP104"},
	}

	htx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin hierarchy tx: %w", err)
	}
	defer htx.Rollback()

	for _, e := range employees {
		var exists int
		if err := htx.QueryRowContext(ctx, `SELECT COUNT(*) FROM m_employee WHERE nik = $1`, e.NIK).Scan(&exists); err != nil {
			return fmt.Errorf("check m_employee %s: %w", e.NIK, err)
		}
		if exists > 0 {
			continue
		}

		var superiorID sql.NullString
		if e.SuperiorID != "" {
			superiorID = sql.NullString{String: e.SuperiorID, Valid: true}
		}
		if _, err := htx.ExecContext(ctx, `
			INSERT INTO m_employee (nik, emp_id, emp_nm, user_id, full_name, is_vacant, superior_id, is_terminate)
			VALUES ($1, $2, $3, $4, $5, 'N', $6, 'N')
			ON CONFLICT (nik) DO NOTHING
		`, e.NIK, e.NIK, e.FullName, e.UserID, e.FullName, superiorID); err != nil {
			return fmt.Errorf("insert m_employee %s: %w", e.NIK, err)
		}
	}
	return htx.Commit()
}

-- Seed: 000001_seed_auth.sql
-- Description: Seed initial permissions, roles, role_permissions, users, user_roles, and user_settings.

-- Permissions
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

-- Roles
INSERT OR IGNORE INTO roles (id, code, name, role_type, approval_rank) VALUES
('role_master', 'MASTER_ADMIN', 'Master Admin', 'SYSTEM', 100),
('role_mgr', 'ASSET_MANAGER', 'Asset Manager', 'OPERATIONAL', 80),
('role_appr', 'DEPARTMENT_APPROVER', 'Department Approver', 'APPROVAL', 50),
('role_staff', 'FIELD_STAFF', 'Field Staff', 'OPERATIONAL', 30),
('role_user', 'REGULAR_USER', 'Regular User', 'GENERAL', 10);

-- Role Permissions Mapping
-- MASTER_ADMIN: All permissions
INSERT OR IGNORE INTO role_permissions (role_id, permission_id) VALUES
('role_master', 'perm_u_read'), ('role_master', 'perm_u_write'), ('role_master', 'perm_u_delete'), ('role_master', 'perm_u_master'),
('role_master', 'perm_r_manage'), ('role_master', 'perm_p_read'),
('role_master', 'perm_a_read'), ('role_master', 'perm_a_write'),
('role_master', 'perm_req_create'), ('role_master', 'perm_req_read'), ('role_master', 'perm_req_approve'),
('role_master', 'perm_ful_process'), ('role_master', 'perm_ful_read'),
('role_master', 'perm_set_manage');

-- ASSET_MANAGER: Asset, Fulfillment, Request Approval
INSERT OR IGNORE INTO role_permissions (role_id, permission_id) VALUES
('role_mgr', 'perm_u_read'),
('role_mgr', 'perm_a_read'), ('role_mgr', 'perm_a_write'),
('role_mgr', 'perm_req_create'), ('role_mgr', 'perm_req_read'), ('role_mgr', 'perm_req_approve'),
('role_mgr', 'perm_ful_process'), ('role_mgr', 'perm_ful_read');

-- DEPARTMENT_APPROVER: Request Approval, Request Read, Asset Read
INSERT OR IGNORE INTO role_permissions (role_id, permission_id) VALUES
('role_appr', 'perm_u_read'),
('role_appr', 'perm_a_read'),
('role_appr', 'perm_req_create'), ('role_appr', 'perm_req_read'), ('role_appr', 'perm_req_approve');

-- FIELD_STAFF: Fulfillment Process, Asset Read
INSERT OR IGNORE INTO role_permissions (role_id, permission_id) VALUES
('role_staff', 'perm_u_read'),
('role_staff', 'perm_a_read'),
('role_staff', 'perm_ful_process'), ('role_staff', 'perm_ful_read');

-- REGULAR_USER: Request Create & Read, Asset Read
INSERT OR IGNORE INTO role_permissions (role_id, permission_id) VALUES
('role_user', 'perm_u_read'),
('role_user', 'perm_a_read'),
('role_user', 'perm_req_create'), ('role_user', 'perm_req_read');

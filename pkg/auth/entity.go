package auth

import "time"

// LoginRequest contains payload for user login.
type LoginRequest struct {
	UsernameOrEmail string `json:"username_or_email" validate:"required"`
	Password        string `json:"password" validate:"required"`
}

// LoginResponse contains authentication token and user details.
type LoginResponse struct {
	AccessToken string      `json:"access_token"`
	TokenType   string      `json:"token_type"`
	ExpiresAt   time.Time   `json:"expires_at"`
	User        UserProfile `json:"user"`
}

// UserProfile represents user information returned on login and /me endpoint.
type UserProfile struct {
	ID          string          `json:"id"`
	EmployeeNo  string          `json:"employee_no"`
	Name        string          `json:"name"`
	Email       string          `json:"email"`
	Status      string          `json:"status"`
	IsMaster    bool            `json:"is_master"`
	Roles       []RoleDto       `json:"roles"`
	Permissions []string        `json:"permissions"`
	Settings    *UserSettingDto `json:"settings,omitempty"`
}

// User represents internal database user entity.
type User struct {
	ID           string    `json:"id"`
	EmployeeNo   string    `json:"employee_no"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Status       string    `json:"status"`
	IsMaster     bool      `json:"is_master"`
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UserSettingDto represents user display and notification preferences.
type UserSettingDto struct {
	Theme              string `json:"theme"`
	EmailNotifications bool   `json:"email_notifications"`
}

// RoleDto represents a role entity in API responses.
type RoleDto struct {
	ID           string          `json:"id"`
	Code         string          `json:"code"`
	Name         string          `json:"name"`
	RoleType     string          `json:"role_type"`
	ApprovalRank int             `json:"approval_rank"`
	IsActive     bool            `json:"is_active"`
	Permissions  []PermissionDto `json:"permissions,omitempty"`
}

// PermissionDto represents a permission entity in API responses.
type PermissionDto struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CreateRoleRequest payload for creating new roles.
type CreateRoleRequest struct {
	Code         string `json:"code" validate:"required"`
	Name         string `json:"name" validate:"required"`
	RoleType     string `json:"role_type"`
	ApprovalRank int    `json:"approval_rank"`
}

// UpdateMasterUserRequest payload for toggling master user status.
type UpdateMasterUserRequest struct {
	IsMaster bool `json:"is_master"`
}

// AssignRoleRequest payload for assigning roles to a user.
type AssignRoleRequest struct {
	RoleIDs []string `json:"role_ids"`
}

// AssignPermissionRequest payload for assigning permissions to a role.
type AssignPermissionRequest struct {
	PermissionIDs []string `json:"permission_ids"`
}

// UpdateSettingsRequest payload for updating user settings.
type UpdateSettingsRequest struct {
	Theme              string `json:"theme"`
	EmailNotifications bool   `json:"email_notifications"`
}

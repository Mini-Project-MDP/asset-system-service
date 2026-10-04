package domain

import "context"

// User account status values, as stored in users.status.
const (
	UserStatusActive   = "ACTIVE"
	UserStatusInactive = "INACTIVE"
)

// SaveUserInput is the create/update payload for a user account. IsActive is
// nil when the caller does not want to change it. Password is only honoured
// on create.
type SaveUserInput struct {
	Name       string
	Email      string
	EmployeeNo string
	Password   string
	RoleID     string
	IsActive   *bool
}

// UserAdminRepository persists user account maintenance. GetUser and GetRole
// return (nil, nil) when the row does not exist; writes return ErrDuplicate
// on a uniqueness violation (email or employee number).
type UserAdminRepository interface {
	GetUser(ctx context.Context, id string) (*UserProfile, error)
	GetRole(ctx context.Context, id string) (*RoleDto, error)
	CreateUser(ctx context.Context, in SaveUserInput, passwordHash, status string) (string, error)
	// UpdateUser rewrites the account fields and replaces the user's single role.
	UpdateUser(ctx context.Context, id string, in SaveUserInput, status string) error
	// CountOtherActiveAdmins counts active users holding the MASTER_ADMIN role,
	// excluding excludeUserID.
	CountOtherActiveAdmins(ctx context.Context, excludeUserID string) (int, error)
}

// UserService holds the business rules for user account maintenance.
type UserService interface {
	CreateUser(ctx context.Context, in SaveUserInput) (*UserProfile, error)
	UpdateUser(ctx context.Context, id string, in SaveUserInput) (*UserProfile, error)
	// AssignRoles sets the user's single role; roleIDs must contain exactly one id.
	AssignRoles(ctx context.Context, userID string, roleIDs []string) (*UserProfile, error)
}

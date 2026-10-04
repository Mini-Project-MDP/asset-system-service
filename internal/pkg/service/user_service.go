package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/jwt"
)

// Errors returned by userService. Handlers map them to HTTP statuses.
var (
	ErrUserValidation = errors.New("invalid user")
	ErrUserConflict   = errors.New("user conflict")
	ErrUserNotFound   = errors.New("user not found")
	ErrLastAdmin      = errors.New("cannot remove the last active admin")
)

// minPasswordLength is the shortest password accepted when one is supplied.
const minPasswordLength = 8

// adminRoleCode is the role whose last active holder must always remain.
const adminRoleCode = "MASTER_ADMIN"

// standardRoleCodes are the 8 login roles a user may hold (Admin, Asset Team,
// and the six sales levels). "Cabang" is only a requester label and the older
// seeded roles are not offered for user maintenance.
var standardRoleCodes = map[string]bool{
	"MASTER_ADMIN":  true,
	"ASSET_MANAGER": true,
	"SA":            true,
	"SS":            true,
	"RSM":           true,
	"GRSM":          true,
	"NSM":           true,
	"SD":            true,
}

type userService struct {
	repo domain.UserAdminRepository
}

// NewUserService creates a domain.UserService.
func NewUserService(repo domain.UserAdminRepository) domain.UserService {
	return &userService{repo: repo}
}

func userInvalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUserValidation, fmt.Sprintf(format, args...))
}

// normalizeUser trims fields and checks the ones both create and update need.
func normalizeUser(in domain.SaveUserInput) (domain.SaveUserInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.EmployeeNo = strings.TrimSpace(in.EmployeeNo)
	in.RoleID = strings.TrimSpace(in.RoleID)
	if in.Name == "" {
		return in, userInvalid("name is required")
	}
	if in.Email == "" {
		return in, userInvalid("email is required")
	}
	if addr, err := mail.ParseAddress(in.Email); err != nil || addr.Address != in.Email {
		return in, userInvalid("email is not a valid address")
	}
	if in.RoleID == "" {
		return in, userInvalid("role_id is required")
	}
	return in, nil
}

// checkRole confirms the role exists, is active and is one of the 8 standard roles.
func (s *userService) checkRole(ctx context.Context, roleID string) (*domain.RoleDto, error) {
	role, err := s.repo.GetRole(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, userInvalid("unknown role %q", roleID)
	}
	if !role.IsActive {
		return nil, userInvalid("role %s is not active", role.Code)
	}
	if !standardRoleCodes[role.Code] {
		return nil, userInvalid("role %s cannot be assigned to a user", role.Code)
	}
	return role, nil
}

func mapUserRepoError(err error) error {
	if errors.Is(err, domain.ErrDuplicate) {
		return fmt.Errorf("%w: email or employee_no already in use", ErrUserConflict)
	}
	return err
}

func statusFor(active bool) string {
	if active {
		return domain.UserStatusActive
	}
	return domain.UserStatusInactive
}

// randomPassword returns a password nobody sees: accounts created without one
// sign in through SSO only.
func randomPassword() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *userService) CreateUser(ctx context.Context, in domain.SaveUserInput) (*domain.UserProfile, error) {
	in, err := normalizeUser(in)
	if err != nil {
		return nil, err
	}
	if in.EmployeeNo == "" {
		in.EmployeeNo = in.Email
	}
	if _, err := s.checkRole(ctx, in.RoleID); err != nil {
		return nil, err
	}

	password := in.Password
	if password == "" {
		if password, err = randomPassword(); err != nil {
			return nil, err
		}
	} else if len(password) < minPasswordLength {
		return nil, userInvalid("password must be at least %d characters", minPasswordLength)
	}
	hash, err := jwt.HashPassword(password)
	if err != nil {
		return nil, err
	}

	active := in.IsActive == nil || *in.IsActive
	id, err := s.repo.CreateUser(ctx, in, hash, statusFor(active))
	if err != nil {
		return nil, mapUserRepoError(err)
	}
	return s.repo.GetUser(ctx, id)
}

func (s *userService) UpdateUser(ctx context.Context, id string, in domain.SaveUserInput) (*domain.UserProfile, error) {
	in, err := normalizeUser(in)
	if err != nil {
		return nil, err
	}
	if in.Password != "" {
		return nil, userInvalid("password cannot be changed here")
	}
	current, err := s.repo.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrUserNotFound
	}
	if in.EmployeeNo == "" {
		in.EmployeeNo = current.EmployeeNo
	}
	return s.save(ctx, current, in)
}

func (s *userService) AssignRoles(ctx context.Context, userID string, roleIDs []string) (*domain.UserProfile, error) {
	var ids []string
	for _, id := range roleIDs {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) != 1 {
		return nil, userInvalid("a user holds exactly one role")
	}
	current, err := s.repo.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrUserNotFound
	}
	return s.save(ctx, current, domain.SaveUserInput{
		Name: current.Name, Email: current.Email, EmployeeNo: current.EmployeeNo, RoleID: ids[0],
	})
}

// save validates the role, applies the last-admin guard and writes the update.
func (s *userService) save(ctx context.Context, current *domain.UserProfile, in domain.SaveUserInput) (*domain.UserProfile, error) {
	role, err := s.checkRole(ctx, in.RoleID)
	if err != nil {
		return nil, err
	}
	active := current.Status == domain.UserStatusActive
	if in.IsActive != nil {
		active = *in.IsActive
	}

	wasAdmin := current.Status == domain.UserStatusActive && hasRole(current.Roles, adminRoleCode)
	staysAdmin := active && role.Code == adminRoleCode
	if wasAdmin && !staysAdmin {
		others, err := s.repo.CountOtherActiveAdmins(ctx, current.ID)
		if err != nil {
			return nil, err
		}
		if others == 0 {
			return nil, ErrLastAdmin
		}
	}

	if err := s.repo.UpdateUser(ctx, current.ID, in, statusFor(active)); err != nil {
		return nil, mapUserRepoError(err)
	}
	return s.repo.GetUser(ctx, current.ID)
}

func hasRole(roles []domain.RoleDto, code string) bool {
	for _, r := range roles {
		if r.Code == code {
			return true
		}
	}
	return false
}

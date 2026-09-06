package auth

import (
	"context"
	"errors"
	"fmt"

	infraAuth "github.com/Mini-Project-MDP/asset-system-service/pkg/infrastructure/auth"
)

type Service struct {
	repo         *Repository
	tokenManager *infraAuth.TokenManager
}

func NewService(repo *Repository, tm *infraAuth.TokenManager) *Service {
	return &Service{
		repo:         repo,
		tokenManager: tm,
	}
}

func (s *Service) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	if req.UsernameOrEmail == "" || req.Password == "" {
		return nil, errors.New("username/email and password are required")
	}

	user, err := s.repo.GetUserByEmailOrEmployeeNo(ctx, req.UsernameOrEmail)
	if err != nil {
		return nil, fmt.Errorf("database query error: %w", err)
	}
	if user == nil {
		return nil, errors.New("invalid credentials")
	}

	if user.Status != "ACTIVE" {
		return nil, errors.New("user account is inactive or disabled")
	}

	if !infraAuth.CheckPasswordHash(req.Password, user.PasswordHash) {
		return nil, errors.New("invalid credentials")
	}

	roles, err := s.repo.GetUserRoles(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("fetch user roles: %w", err)
	}

	permissions, err := s.repo.GetUserPermissions(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("fetch user permissions: %w", err)
	}

	settings, err := s.repo.GetUserSettings(ctx, user.ID)
	if err != nil {
		settings = &UserSettingDto{Theme: "light", EmailNotifications: true}
	}

	var roleCodes []string
	for _, r := range roles {
		roleCodes = append(roleCodes, r.Code)
	}

	tokenStr, expiresAt, err := s.tokenManager.GenerateToken(
		user.ID,
		user.Email,
		user.Name,
		user.EmployeeNo,
		user.IsMaster,
		roleCodes,
		permissions,
	)
	if err != nil {
		return nil, fmt.Errorf("generate authentication token: %w", err)
	}

	profile := UserProfile{
		ID:          user.ID,
		EmployeeNo:  user.EmployeeNo,
		Name:        user.Name,
		Email:       user.Email,
		Status:      user.Status,
		IsMaster:    user.IsMaster,
		Roles:       roles,
		Permissions: permissions,
		Settings:    settings,
	}

	return &LoginResponse{
		AccessToken: tokenStr,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
		User:        profile,
	}, nil
}

func (s *Service) GetCurrentUserProfile(ctx context.Context, userID string) (*UserProfile, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	roles, err := s.repo.GetUserRoles(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	permissions, err := s.repo.GetUserPermissions(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	settings, err := s.repo.GetUserSettings(ctx, user.ID)
	if err != nil {
		settings = &UserSettingDto{Theme: "light", EmailNotifications: true}
	}

	return &UserProfile{
		ID:          user.ID,
		EmployeeNo:  user.EmployeeNo,
		Name:        user.Name,
		Email:       user.Email,
		Status:      user.Status,
		IsMaster:    user.IsMaster,
		Roles:       roles,
		Permissions: permissions,
		Settings:    settings,
	}, nil
}

func (s *Service) UpdateMasterUserStatus(ctx context.Context, userID string, isMaster bool) error {
	return s.repo.SetMasterUserStatus(ctx, userID, isMaster)
}

func (s *Service) UpdateUserSettings(ctx context.Context, userID string, req UpdateSettingsRequest) error {
	return s.repo.UpdateUserSettings(ctx, userID, req)
}

func (s *Service) GetRoles(ctx context.Context) ([]RoleDto, error) {
	return s.repo.GetAllRoles(ctx)
}

func (s *Service) CreateRole(ctx context.Context, req CreateRoleRequest) (*RoleDto, error) {
	if req.Code == "" || req.Name == "" {
		return nil, errors.New("role code and name are required")
	}
	return s.repo.CreateRole(ctx, req)
}

func (s *Service) GetPermissions(ctx context.Context) ([]PermissionDto, error) {
	return s.repo.GetAllPermissions(ctx)
}

func (s *Service) AssignPermissionsToRole(ctx context.Context, roleID string, permissionIDs []string) error {
	return s.repo.AssignPermissionsToRole(ctx, roleID, permissionIDs)
}

func (s *Service) AssignRolesToUser(ctx context.Context, userID string, roleIDs []string) error {
	return s.repo.AssignRolesToUser(ctx, userID, roleIDs)
}

func (s *Service) GetUsers(ctx context.Context) ([]UserProfile, error) {
	return s.repo.GetAllUsers(ctx)
}

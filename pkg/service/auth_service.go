package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Mini-Project-MDP/asset-system-service/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/jwt"
)

type authService struct {
	repo         domain.AuthRepository
	tokenManager *jwt.TokenManager
}

// NewAuthService creates a new instance of domain.AuthService.
func NewAuthService(repo domain.AuthRepository, tm *jwt.TokenManager) domain.AuthService {
	return &authService{
		repo:         repo,
		tokenManager: tm,
	}
}

func (s *authService) Login(ctx context.Context, req domain.LoginRequest) (*domain.LoginResponse, error) {
	if req.UsernameOrEmail == "" || req.Password == "" {
		return nil, errors.New("username/email and password are required")
	}

	user, err := s.repo.GetUserByEmailOrUsername(ctx, req.UsernameOrEmail)
	if err != nil {
		return nil, fmt.Errorf("database query error: %w", err)
	}
	if user == nil {
		return nil, errors.New("invalid credentials")
	}

	if user.Status != "ACTIVE" {
		return nil, errors.New("user account is inactive or disabled")
	}

	if !jwt.CheckPasswordHash(req.Password, user.PasswordHash) {
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
		settings = &domain.UserSettingDto{Theme: "light", EmailNotifications: true}
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

	profile := domain.UserProfile{
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

	return &domain.LoginResponse{
		AccessToken: tokenStr,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
		User:        profile,
	}, nil
}

func (s *authService) GetCurrentUserProfile(ctx context.Context, userID string) (*domain.UserProfile, error) {
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
		settings = &domain.UserSettingDto{Theme: "light", EmailNotifications: true}
	}

	return &domain.UserProfile{
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

func (s *authService) UpdateMasterUserStatus(ctx context.Context, userID string, isMaster bool) error {
	return s.repo.UpdateMasterUserStatus(ctx, userID, isMaster)
}

func (s *authService) UpdateUserSettings(ctx context.Context, userID string, req domain.UpdateSettingsRequest) error {
	return s.repo.UpdateUserSettings(ctx, userID, req)
}

func (s *authService) GetRoles(ctx context.Context) ([]domain.RoleDto, error) {
	return s.repo.GetRoles(ctx)
}

func (s *authService) CreateRole(ctx context.Context, req domain.CreateRoleRequest) (*domain.RoleDto, error) {
	if req.Code == "" || req.Name == "" {
		return nil, errors.New("role code and name are required")
	}
	return s.repo.CreateRole(ctx, req)
}

func (s *authService) GetPermissions(ctx context.Context) ([]domain.PermissionDto, error) {
	return s.repo.GetPermissions(ctx)
}

func (s *authService) AssignPermissionsToRole(ctx context.Context, roleID string, permissionIDs []string) error {
	return s.repo.AssignPermissionsToRole(ctx, roleID, permissionIDs)
}

func (s *authService) AssignRolesToUser(ctx context.Context, userID string, roleIDs []string) error {
	return s.repo.AssignRolesToUser(ctx, userID, roleIDs)
}

func (s *authService) GetUsers(ctx context.Context) ([]domain.UserProfile, error) {
	return s.repo.GetUsers(ctx)
}

package handler

import (
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/delivery/http/middleware"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/jwt"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/gofiber/fiber/v3"
)

type AuthHandler struct {
	authService domain.AuthService
}

func NewAuthHandler(authService domain.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Login handles POST /api/v1/auth/login
// @Summary User login
// @Description Authenticates user credentials and returns JWT access token along with user profile
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body domain.LoginRequest true "Login Credentials"
// @Success 200 {object} response.Response{data=domain.LoginResponse}
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Router /api/v1/auth/login [post]
func (h *AuthHandler) Login(c fiber.Ctx) error {
	var req domain.LoginRequest
	if err := c.Bind().Body(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}

	resp, err := h.authService.Login(c.Context(), req)
	if err != nil {
		return response.Error(c, fiber.StatusUnauthorized, err.Error())
	}

	return response.Success(c, resp)
}

// GetMe handles GET /api/v1/me
// @Summary Get current user profile
// @Description Retrieves profile and permissions of the currently authenticated user
// @Tags User Profile
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response{data=domain.UserProfile}
// @Failure 401 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/me [get]
func (h *AuthHandler) GetMe(c fiber.Ctx) error {
	val := c.Locals(middleware.UserContextKey)
	claims, ok := val.(*jwt.UserClaims)
	if !ok || claims == nil {
		return response.Error(c, fiber.StatusUnauthorized, "Unauthorized context")
	}

	profile, err := h.authService.GetCurrentUserProfile(c.Context(), claims.UserID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, err.Error())
	}

	return response.Success(c, profile)
}

// UpdateUserSettings handles PUT /api/v1/me/settings
// @Summary Update user settings
// @Description Updates preferences (theme, email notifications) for the current user
// @Tags User Profile
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body domain.UpdateSettingsRequest true "User Settings Payload"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/me/settings [put]
func (h *AuthHandler) UpdateUserSettings(c fiber.Ctx) error {
	val := c.Locals(middleware.UserContextKey)
	claims, ok := val.(*jwt.UserClaims)
	if !ok || claims == nil {
		return response.Error(c, fiber.StatusUnauthorized, "Unauthorized context")
	}

	var req domain.UpdateSettingsRequest
	if err := c.Bind().Body(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}

	if err := h.authService.UpdateUserSettings(c.Context(), claims.UserID, req); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, err.Error())
	}

	return response.JSON(c, fiber.StatusOK, "User settings updated successfully", nil)
}

// GetRoles handles GET /api/v1/roles
// @Summary Get all roles
// @Description Retrieves all system roles and their assigned permissions
// @Tags Roles & Permissions
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response{data=[]domain.RoleDto}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/roles [get]
func (h *AuthHandler) GetRoles(c fiber.Ctx) error {
	roles, err := h.authService.GetRoles(c.Context())
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	return response.Success(c, roles)
}

// CreateRole handles POST /api/v1/roles
// @Summary Create a new role
// @Description Creates a new custom role in the system
// @Tags Roles & Permissions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body domain.CreateRoleRequest true "Create Role Payload"
// @Success 201 {object} response.Response{data=domain.RoleDto}
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Router /api/v1/roles [post]
func (h *AuthHandler) CreateRole(c fiber.Ctx) error {
	var req domain.CreateRoleRequest
	if err := c.Bind().Body(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}

	role, err := h.authService.CreateRole(c.Context(), req)
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, err.Error())
	}

	return response.Created(c, role)
}

// GetPermissions handles GET /api/v1/permissions
// @Summary Get all permissions
// @Description Retrieves list of all available system permissions
// @Tags Roles & Permissions
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response{data=[]domain.PermissionDto}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/permissions [get]
func (h *AuthHandler) GetPermissions(c fiber.Ctx) error {
	perms, err := h.authService.GetPermissions(c.Context())
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	return response.Success(c, perms)
}

// AssignRolePermissions handles POST /api/v1/roles/:id/permissions
// @Summary Assign permissions to role
// @Description Replaces assigned permissions for a specific role
// @Tags Roles & Permissions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Role ID"
// @Param request body domain.AssignPermissionRequest true "Assign Permissions Payload"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/roles/{id}/permissions [post]
func (h *AuthHandler) AssignRolePermissions(c fiber.Ctx) error {
	roleID := c.Params("id")
	var req domain.AssignPermissionRequest
	if err := c.Bind().Body(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}

	if err := h.authService.AssignPermissionsToRole(c.Context(), roleID, req.PermissionIDs); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, err.Error())
	}

	return response.JSON(c, fiber.StatusOK, "Role permissions assigned successfully", nil)
}

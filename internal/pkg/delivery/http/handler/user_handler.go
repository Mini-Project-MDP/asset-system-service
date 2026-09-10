package handler

import (
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/gofiber/fiber/v3"
)

type UserHandler struct {
	authService domain.AuthService
}

func NewUserHandler(authService domain.AuthService) *UserHandler {
	return &UserHandler{authService: authService}
}

// GetUsers handles GET /api/v1/users
// @Summary Get all users
// @Description Retrieves list of all user profiles in the system
// @Tags Users
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response{data=[]domain.UserProfile}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/users [get]
func (h *UserHandler) GetUsers(c fiber.Ctx) error {
	users, err := h.authService.GetUsers(c.Context())
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	return response.Success(c, users)
}

// SetMasterUser handles POST /api/v1/users/:id/master
// @Summary Toggle Master User status
// @Description Designates or revokes master user status for a target user
// @Tags Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Param request body domain.UpdateMasterUserRequest false "Master User Payload"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Router /api/v1/users/{id}/master [post]
func (h *UserHandler) SetMasterUser(c fiber.Ctx) error {
	userID := c.Params("id")
	if userID == "" {
		return response.Error(c, fiber.StatusBadRequest, "User ID parameter is required")
	}

	var req domain.UpdateMasterUserRequest
	if err := c.Bind().Body(&req); err != nil {
		// default to setting master = true if body not provided
		req.IsMaster = true
	}

	if err := h.authService.UpdateMasterUserStatus(c.Context(), userID, req.IsMaster); err != nil {
		return response.Error(c, fiber.StatusBadRequest, err.Error())
	}

	return response.JSON(c, fiber.StatusOK, "Master user status updated successfully", fiber.Map{
		"user_id":   userID,
		"is_master": req.IsMaster,
	})
}

// AssignUserRoles handles POST /api/v1/users/:id/roles
// @Summary Assign roles to user
// @Description Assigns a list of roles to a target user
// @Tags Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Param request body domain.AssignRoleRequest true "Assign Roles Payload"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/users/{id}/roles [post]
func (h *UserHandler) AssignUserRoles(c fiber.Ctx) error {
	userID := c.Params("id")
	var req domain.AssignRoleRequest
	if err := c.Bind().Body(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}

	if err := h.authService.AssignRolesToUser(c.Context(), userID, req.RoleIDs); err != nil {
		return response.Error(c, fiber.StatusInternalServerError, err.Error())
	}

	return response.JSON(c, fiber.StatusOK, "User roles assigned successfully", nil)
}

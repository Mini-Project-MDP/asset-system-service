package handler

import (
	"errors"
	"log"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/service"
	"github.com/gofiber/fiber/v3"
)

type UserHandler struct {
	authService domain.AuthService
	userService domain.UserService
}

func NewUserHandler(authService domain.AuthService, userService domain.UserService) *UserHandler {
	return &UserHandler{authService: authService, userService: userService}
}

// userBody is the JSON payload for creating/updating a user.
type userBody struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	EmployeeNo string `json:"employee_no"`
	Password   string `json:"password"` // create only; omit for SSO-only accounts
	RoleID     string `json:"role_id"`
	IsActive   *bool  `json:"is_active"`
}

// userError maps user-service errors to HTTP responses.
func userError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrUserValidation):
		return response.Error(c, fiber.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrUserNotFound):
		return response.Error(c, fiber.StatusNotFound, "User not found")
	case errors.Is(err, service.ErrUserConflict), errors.Is(err, service.ErrLastAdmin):
		return response.Error(c, fiber.StatusConflict, err.Error())
	default:
		log.Printf("user management: %v", err)
		return response.Error(c, fiber.StatusInternalServerError, "Internal server error")
	}
}

// CreateUser handles POST /api/v1/users
// @Summary Create a user
// @Description Creates a user with exactly one of the 8 standard roles. Without a password the account signs in through SSO only.
// @Tags Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body userBody true "User"
// @Success 201 {object} response.Response{data=domain.UserProfile}
// @Failure 400 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/users [post]
func (h *UserHandler) CreateUser(c fiber.Ctx) error {
	var body userBody
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}
	u, err := h.userService.CreateUser(c.Context(), domain.SaveUserInput(body))
	if err != nil {
		return userError(c, err)
	}
	return response.Created(c, u)
}

// UpdateUser handles PUT /api/v1/users/:id
// @Summary Update a user
// @Description Changes name, email, role and status. Set is_active=false to deactivate. The last active admin cannot be demoted or deactivated (409).
// @Tags Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Param request body userBody true "User"
// @Success 200 {object} response.Response{data=domain.UserProfile}
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/users/{id} [put]
func (h *UserHandler) UpdateUser(c fiber.Ctx) error {
	var body userBody
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}
	u, err := h.userService.UpdateUser(c.Context(), c.Params("id"), domain.SaveUserInput(body))
	if err != nil {
		return userError(c, err)
	}
	return response.Success(c, u)
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
// @Description Sets the single role of a target user; role_ids must contain exactly one id
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

	if _, err := h.userService.AssignRoles(c.Context(), userID, req.RoleIDs); err != nil {
		return userError(c, err)
	}

	return response.JSON(c, fiber.StatusOK, "User roles assigned successfully", nil)
}

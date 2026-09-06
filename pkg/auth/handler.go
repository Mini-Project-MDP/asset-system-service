package auth

import (
	infraAuth "github.com/Mini-Project-MDP/asset-system-service/pkg/infrastructure/auth"
	"github.com/gofiber/fiber/v3"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Login handles POST /api/v1/auth/login
func (h *Handler) Login(c fiber.Ctx) error {
	var req LoginRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request payload",
		})
	}

	resp, err := h.service.Login(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

// GetMe handles GET /api/v1/me
func (h *Handler) GetMe(c fiber.Ctx) error {
	val := c.Locals(infraAuth.UserContextKey)
	claims, ok := val.(*infraAuth.UserClaims)
	if !ok || claims == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized context",
		})
	}

	profile, err := h.service.GetCurrentUserProfile(c.Context(), claims.UserID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(profile)
}

// UpdateUserSettings handles PUT /api/v1/me/settings
func (h *Handler) UpdateUserSettings(c fiber.Ctx) error {
	val := c.Locals(infraAuth.UserContextKey)
	claims, ok := val.(*infraAuth.UserClaims)
	if !ok || claims == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized context",
		})
	}

	var req UpdateSettingsRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request payload",
		})
	}

	if err := h.service.UpdateUserSettings(c.Context(), claims.UserID, req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "User settings updated successfully",
	})
}

// SetMasterUser handles POST /api/v1/users/:id/master
func (h *Handler) SetMasterUser(c fiber.Ctx) error {
	userID := c.Params("id")
	if userID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "User ID parameter is required",
		})
	}

	var req UpdateMasterUserRequest
	if err := c.Bind().Body(&req); err != nil {
		// default to setting master = true if body not provided
		req.IsMaster = true
	}

	if err := h.service.UpdateMasterUserStatus(c.Context(), userID, req.IsMaster); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Master user status updated successfully",
		"user_id": userID,
		"is_master": req.IsMaster,
	})
}

// GetRoles handles GET /api/v1/roles
func (h *Handler) GetRoles(c fiber.Ctx) error {
	roles, err := h.service.GetRoles(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(roles)
}

// CreateRole handles POST /api/v1/roles
func (h *Handler) CreateRole(c fiber.Ctx) error {
	var req CreateRoleRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request payload",
		})
	}

	role, err := h.service.CreateRole(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(role)
}

// GetPermissions handles GET /api/v1/permissions
func (h *Handler) GetPermissions(c fiber.Ctx) error {
	perms, err := h.service.GetPermissions(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(perms)
}

// AssignRolePermissions handles POST /api/v1/roles/:id/permissions
func (h *Handler) AssignRolePermissions(c fiber.Ctx) error {
	roleID := c.Params("id")
	var req AssignPermissionRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request payload",
		})
	}

	if err := h.service.AssignPermissionsToRole(c.Context(), roleID, req.PermissionIDs); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Role permissions assigned successfully",
	})
}

// AssignUserRoles handles POST /api/v1/users/:id/roles
func (h *Handler) AssignUserRoles(c fiber.Ctx) error {
	userID := c.Params("id")
	var req AssignRoleRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request payload",
		})
	}

	if err := h.service.AssignRolesToUser(c.Context(), userID, req.RoleIDs); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "User roles assigned successfully",
	})
}

// GetUsers handles GET /api/v1/users
func (h *Handler) GetUsers(c fiber.Ctx) error {
	users, err := h.service.GetUsers(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(users)
}

package handler

import (
	"context"
	"log"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/delivery/http/middleware"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/jwt"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/gofiber/fiber/v3"
)

// CallerResolver tells who the authenticated caller is and what they may do.
// See service.PermissionResolver.
type CallerResolver interface {
	PermissionsFor(ctx context.Context, claims *jwt.UserClaims) ([]string, error)
	ViewerFor(ctx context.Context, claims *jwt.UserClaims) (domain.Viewer, error)
}

// NavigationHandler serves the sidebar badge counters.
type NavigationHandler struct {
	svc     domain.NavigationService
	callers CallerResolver
}

func NewNavigationHandler(svc domain.NavigationService, callers CallerResolver) *NavigationHandler {
	return &NavigationHandler{svc: svc, callers: callers}
}

// Badges handles GET /api/v1/navigation/badges.
// @Summary Sidebar badge counters
// @Description approvals: requests waiting for a decision from the caller's role (Admin: every waiting request). fulfillment: requests ready to process or in fulfillment. A counter is omitted for a module the caller cannot open.
// @Tags Navigation
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Router /api/v1/navigation/badges [get]
func (h *NavigationHandler) Badges(c fiber.Ctx) error {
	claims, ok := c.Locals(middleware.UserContextKey).(*jwt.UserClaims)
	if !ok || claims == nil {
		return response.Error(c, fiber.StatusUnauthorized, "Unauthorized context")
	}

	permissions, err := h.callers.PermissionsFor(c.Context(), claims)
	if err != nil {
		log.Printf("navigation badges: permissions: %v", err)
		return response.Error(c, fiber.StatusInternalServerError, "Unable to verify access")
	}
	viewer, err := h.callers.ViewerFor(c.Context(), claims)
	if err != nil {
		log.Printf("navigation badges: viewer: %v", err)
		return response.Error(c, fiber.StatusInternalServerError, "Unable to verify access")
	}

	badges, err := h.svc.Badges(c.Context(), domain.NavigationCaller{
		UserID: viewer.UserID, Permissions: permissions, IsMaster: claims.IsMaster,
	})
	if err != nil {
		log.Printf("navigation badges: %v", err)
		return response.Error(c, fiber.StatusInternalServerError, "Internal server error")
	}

	out := fiber.Map{}
	if badges.Approvals != nil {
		out["approvals"] = *badges.Approvals
	}
	if badges.Fulfillment != nil {
		out["fulfillment"] = *badges.Fulfillment
	}
	return response.Success(c, out)
}

package middleware

import (
	"context"
	"log"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/jwt"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/gofiber/fiber/v3"
)

const UserContextKey = "user"

// JWTAuth returns a Fiber middleware that validates JWT Bearer tokens.
func JWTAuth(tm *jwt.TokenManager) fiber.Handler {
	return func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return response.Error(c, fiber.StatusUnauthorized, "Missing Authorization header")
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return response.Error(c, fiber.StatusUnauthorized, "Invalid Authorization header format")
		}

		claims, err := tm.ValidateToken(parts[1])
		if err != nil {
			return response.Error(c, fiber.StatusUnauthorized, "Unauthorized: "+err.Error())
		}

		c.Locals(UserContextKey, claims)
		return c.Next()
	}
}

// PermissionResolver tells what a caller is allowed to do. It exists because a
// token alone is not a reliable source: see service.PermissionResolver.
type PermissionResolver interface {
	PermissionsFor(ctx context.Context, claims *jwt.UserClaims) ([]string, error)
}

// PermissionGuard builds the per-route permission checks.
type PermissionGuard struct {
	resolver PermissionResolver
}

// NewPermissionGuard creates a guard. With a nil resolver, permissions are
// taken from the token itself (the behaviour before the resolver existed).
func NewPermissionGuard(resolver PermissionResolver) *PermissionGuard {
	return &PermissionGuard{resolver: resolver}
}

// Require enforces that the current user holds permissionCode.
// Master users automatically pass permission checks.
func (g *PermissionGuard) Require(permissionCode string) fiber.Handler {
	return func(c fiber.Ctx) error {
		val := c.Locals(UserContextKey)
		claims, ok := val.(*jwt.UserClaims)
		if !ok || claims == nil {
			return response.Error(c, fiber.StatusUnauthorized, "Unauthorized context")
		}

		if claims.IsMaster {
			return c.Next()
		}

		granted := claims.Permissions
		if g.resolver != nil {
			resolved, err := g.resolver.PermissionsFor(c.Context(), claims)
			if err != nil {
				log.Printf("resolve permissions for %q: %v", claims.UserID, err)
				return response.Error(c, fiber.StatusInternalServerError, "Unable to verify permissions")
			}
			granted = resolved
		}

		for _, p := range granted {
			if p == permissionCode {
				return c.Next()
			}
		}

		return response.Error(c, fiber.StatusForbidden, "Forbidden: permission '"+permissionCode+"' required")
	}
}

// RequirePermission enforces a permission using the token's own permissions
// only. Prefer PermissionGuard.Require, which reads them from the database.
func RequirePermission(permissionCode string) fiber.Handler {
	return NewPermissionGuard(nil).Require(permissionCode)
}

// RequireMasterUser enforces that only designated Master Users can access the endpoint.
func RequireMasterUser() fiber.Handler {
	return func(c fiber.Ctx) error {
		val := c.Locals(UserContextKey)
		claims, ok := val.(*jwt.UserClaims)
		if !ok || claims == nil {
			return response.Error(c, fiber.StatusUnauthorized, "Unauthorized context")
		}

		if !claims.IsMaster {
			return response.Error(c, fiber.StatusForbidden, "Forbidden: Master user status required")
		}

		return c.Next()
	}
}

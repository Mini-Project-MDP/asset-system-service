package middleware

import (
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

// RequirePermission enforces that the current user possesses a specific permission.
// Master users automatically pass permission checks.
func RequirePermission(permissionCode string) fiber.Handler {
	return func(c fiber.Ctx) error {
		val := c.Locals(UserContextKey)
		claims, ok := val.(*jwt.UserClaims)
		if !ok || claims == nil {
			return response.Error(c, fiber.StatusUnauthorized, "Unauthorized context")
		}

		if claims.IsMaster {
			return c.Next()
		}

		for _, p := range claims.Permissions {
			if p == permissionCode {
				return c.Next()
			}
		}

		return response.Error(c, fiber.StatusForbidden, "Forbidden: permission '"+permissionCode+"' required")
	}
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

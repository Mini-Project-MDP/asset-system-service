package auth

import (
	"strings"

	"github.com/gofiber/fiber/v3"
)

const UserContextKey = "user"

// JWTAuth returns a Fiber middleware that validates JWT Bearer tokens.
func JWTAuth(tm *TokenManager) fiber.Handler {
	return func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing Authorization header",
			})
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid Authorization header format",
			})
		}

		claims, err := tm.ValidateToken(parts[1])
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized: " + err.Error(),
			})
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
		claims, ok := val.(*UserClaims)
		if !ok || claims == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized context",
			})
		}

		if claims.IsMaster {
			return c.Next()
		}

		for _, p := range claims.Permissions {
			if p == permissionCode {
				return c.Next()
			}
		}

		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Forbidden: permission '" + permissionCode + "' required",
		})
	}
}

// RequireMasterUser enforces that only designated Master Users can access the endpoint.
func RequireMasterUser() fiber.Handler {
	return func(c fiber.Ctx) error {
		val := c.Locals(UserContextKey)
		claims, ok := val.(*UserClaims)
		if !ok || claims == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized context",
			})
		}

		if !claims.IsMaster {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Forbidden: Master user status required",
			})
		}

		return c.Next()
	}
}

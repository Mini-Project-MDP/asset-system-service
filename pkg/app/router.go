package app

import (
	"context"
	"database/sql"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/docs"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/auth"
	infraAuth "github.com/Mini-Project-MDP/asset-system-service/pkg/infrastructure/auth"
	"github.com/gofiber/contrib/v3/swaggerui"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

const serviceName = "asset-system-service"

// DatabasePinger is the minimum database capability required by readiness.
type DatabasePinger interface {
	PingContext(context.Context) error
}

// Dependencies contains infrastructure used by the HTTP application.
type Dependencies struct {
	Database            DatabasePinger
	SQLDB               *sql.DB
	DatabasePingTimeout time.Duration
	TokenManager        *infraAuth.TokenManager
}

// NewRouter builds the HTTP router for the service.
func NewRouter(dependencies Dependencies) *fiber.App {
	app := fiber.New()

	// CORS middleware to allow cross-origin requests from Frontend
	app.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
	}))

	// Swagger UI middleware
	app.Use(swaggerui.New(swaggerui.Config{
		BasePath:    "/",
		FileContent: docs.SwaggerJSON,
		Path:        "swagger",
		Title:       "Asset System Service API Documentation",
	}))

	// Public system endpoints
	app.Get("/health", health)
	app.Get("/ready", ready(dependencies))

	// Setup Auth Domain if SQLDB and TokenManager are provided
	if dependencies.SQLDB != nil && dependencies.TokenManager != nil {
		authRepo := auth.NewRepository(dependencies.SQLDB)
		authService := auth.NewService(authRepo, dependencies.TokenManager)
		authHandler := auth.NewHandler(authService)

		// Public Auth Endpoints
		authGroup := app.Group("/api/v1/auth")
		authGroup.Post("/login", authHandler.Login)

		// Protected API Routes
		api := app.Group("/api/v1", infraAuth.JWTAuth(dependencies.TokenManager))

		// User Profile & Settings
		api.Get("/me", authHandler.GetMe)
		api.Put("/me/settings", authHandler.UpdateUserSettings)

		// Users Management
		api.Get("/users", infraAuth.RequirePermission("user:read"), authHandler.GetUsers)
		api.Post("/users/:id/master", infraAuth.RequirePermission("user:master"), authHandler.SetMasterUser)
		api.Post("/users/:id/roles", infraAuth.RequirePermission("role:manage"), authHandler.AssignUserRoles)

		// Roles & Permissions Management
		api.Get("/roles", infraAuth.RequirePermission("role:manage"), authHandler.GetRoles)
		api.Post("/roles", infraAuth.RequirePermission("role:manage"), authHandler.CreateRole)
		api.Post("/roles/:id/permissions", infraAuth.RequirePermission("role:manage"), authHandler.AssignRolePermissions)
		api.Get("/permissions", infraAuth.RequirePermission("role:manage"), authHandler.GetPermissions)
	}

	return app
}

// @Summary Service health status
// @Description Returns operational status of the service
// @Tags System
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health [get]
func health(c fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":  "ok",
		"service": serviceName,
	})
}

// @Summary Service readiness status
// @Description Verifies database connectivity and readiness of the service
// @Tags System
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /ready [get]
func ready(dependencies Dependencies) fiber.Handler {
	return func(c fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), dependencies.DatabasePingTimeout)
		defer cancel()

		if err := dependencies.Database.PingContext(ctx); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status":   "unavailable",
				"service":  serviceName,
				"database": "unavailable",
			})
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status":   "ready",
			"service":  serviceName,
			"database": "connected",
		})
	}
}

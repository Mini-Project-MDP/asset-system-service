package http

import (
	"context"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/docs"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/delivery/http/handler"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/delivery/http/middleware"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/jwt"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/response"
	"github.com/gofiber/contrib/v3/swaggerui"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

const serviceName = "asset-system-service"

// DatabasePinger is the minimum database capability required by readiness.
type DatabasePinger interface {
	PingContext(context.Context) error
}

// Handlers encapsulates all HTTP handlers for the application.
type Handlers struct {
	Auth *handler.AuthHandler
	User *handler.UserHandler
}

// Dependencies contains infrastructure used by the HTTP application.
type Dependencies struct {
	Database            DatabasePinger
	DatabasePingTimeout time.Duration
	TokenManager        *jwt.TokenManager
	AllowedOrigins      []string
	Handlers            Handlers
}

// NewRouter builds the HTTP router for the service.
func NewRouter(deps Dependencies) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: middleware.ErrorHandler(),
	})

	origins := deps.AllowedOrigins
	if len(origins) == 0 {
		origins = []string{"*"}
	}

	// CORS middleware
	app.Use(cors.New(cors.Config{
		AllowOrigins: origins,
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
	app.Get("/ready", ready(deps))

	// Register API v1 Routes
	registerAPIRoutes(app, deps)

	return app
}

func registerAPIRoutes(app *fiber.App, deps Dependencies) {
	api := app.Group("/api/v1")

	// Public Auth routes
	if deps.Handlers.Auth != nil {
		authGroup := api.Group("/auth")
		authGroup.Post("/login", deps.Handlers.Auth.Login)
	}

	// Protected routes (require JWT)
	if deps.TokenManager != nil {
		protected := api.Group("", middleware.JWTAuth(deps.TokenManager))

		if deps.Handlers.Auth != nil {
			// Profile & User Settings
			protected.Get("/me", deps.Handlers.Auth.GetMe)
			protected.Put("/me/settings", deps.Handlers.Auth.UpdateUserSettings)

			// Roles & Permissions Management
			protected.Get("/roles", middleware.RequirePermission("role:manage"), deps.Handlers.Auth.GetRoles)
			protected.Post("/roles", middleware.RequirePermission("role:manage"), deps.Handlers.Auth.CreateRole)
			protected.Post("/roles/:id/permissions", middleware.RequirePermission("role:manage"), deps.Handlers.Auth.AssignRolePermissions)
			protected.Get("/permissions", middleware.RequirePermission("role:manage"), deps.Handlers.Auth.GetPermissions)
		}

		if deps.Handlers.User != nil {
			// User Management
			protected.Get("/users", middleware.RequirePermission("user:read"), deps.Handlers.User.GetUsers)
			protected.Post("/users/:id/master", middleware.RequirePermission("user:master"), deps.Handlers.User.SetMasterUser)
			protected.Post("/users/:id/roles", middleware.RequirePermission("role:manage"), deps.Handlers.User.AssignUserRoles)
		}
	}
}

// @Summary Service health status
// @Description Returns operational status of the service
// @Tags System
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health [get]
func health(c fiber.Ctx) error {
	return response.Success(c, fiber.Map{
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
func ready(deps Dependencies) fiber.Handler {
	return func(c fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), deps.DatabasePingTimeout)
		defer cancel()

		if deps.Database == nil || deps.Database.PingContext(ctx) != nil {
			return response.Error(c, fiber.StatusServiceUnavailable, "Service database connection unavailable")
		}

		return response.Success(c, fiber.Map{
			"status":   "ready",
			"service":  serviceName,
			"database": "connected",
		})
	}
}

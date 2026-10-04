package http

import (
	"context"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/docs"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/delivery/http/handler"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/delivery/http/middleware"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/jwt"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
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
	Auth         *handler.AuthHandler
	User         *handler.UserHandler
	Request      *handler.RequestHandler
	Dashboard    *handler.DashboardHandler
	Navigation   *handler.NavigationHandler
	MasterData   *handler.MasterDataHandler
	PhoneCatalog *handler.PhoneCatalogHandler
	Imei         *handler.ImeiHandler
	Webhook      *handler.WebhookHandler
}

// Dependencies contains infrastructure used by the HTTP application.
type Dependencies struct {
	Database             DatabasePinger
	DatabasePingTimeout  time.Duration
	TokenManager         *jwt.TokenManager
	AllowedOrigins       []string
	Handlers             Handlers
	ApprovalEngineAPIKey string // secret used to verify inbound webhook signatures
	// Permissions resolves what a caller may do (from the database). When nil,
	// the permissions inside the token are used instead.
	Permissions middleware.PermissionResolver
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
	guard := middleware.NewPermissionGuard(deps.Permissions)

	// Public Auth routes
	if deps.Handlers.Auth != nil {
		authGroup := api.Group("/auth")
		authGroup.Post("/login", deps.Handlers.Auth.Login)
	}

	// Public webhook routes — the caller is Approval-Engine-Service, not a
	// logged-in user, so this is gated by signature verification instead of
	// JWT (see middleware.VerifyWebhookSignature).
	if deps.Handlers.Webhook != nil {
		webhooks := api.Group("/webhooks", middleware.VerifyWebhookSignature(deps.ApprovalEngineAPIKey))
		webhooks.Post("/approval-engine", deps.Handlers.Webhook.ApprovalEngine)
	}

	// Protected routes (require JWT)
	if deps.TokenManager != nil {
		protected := api.Group("", middleware.JWTAuth(deps.TokenManager))
		// Registered before the /fulfillment/:id routes so "phone-catalog" is not read as an id.
		if deps.Handlers.PhoneCatalog != nil {
			protected.Get("/fulfillment/phone-catalog", guard.Require("fulfillment:read"), deps.Handlers.PhoneCatalog.Catalog)
			// Per-route guards: a group-level guard on "/settings" would apply to every
			// route under it, whichever group declared it.
			manage := guard.Require("settings:manage")
			protected.Post("/settings/phone-brands", manage, deps.Handlers.PhoneCatalog.CreateBrand)
			protected.Put("/settings/phone-brands/:id", manage, deps.Handlers.PhoneCatalog.UpdateBrand)
			protected.Post("/settings/phone-models", manage, deps.Handlers.PhoneCatalog.CreateModel)
			protected.Put("/settings/phone-models/:id", manage, deps.Handlers.PhoneCatalog.UpdateModel)
		}
		if deps.Handlers.Imei != nil {
			protected.Get("/fulfillment/imei-lookup/:imei", guard.Require("fulfillment:read"), deps.Handlers.Imei.Lookup)
		}
		if deps.Handlers.Request != nil {
			// Registered before the /requests group, whose /:id route would otherwise take "form-options" for an id.
			protected.Get("/requests/form-options", guard.Require("request:create"), deps.Handlers.Request.FormOptions)
			requests := protected.Group("/requests", guard.Require("request:read"))
			requests.Get("/", deps.Handlers.Request.List)
			requests.Get("/:id", deps.Handlers.Request.Detail)
			requests.Post("/", guard.Require("request:create"), deps.Handlers.Request.Create)
			approvals := protected.Group("/approvals", guard.Require("approvals:read"))
			approvals.Get("/", deps.Handlers.Request.Approvals)
			approvals.Get("/:id", deps.Handlers.Request.ApprovalDetail)
			approvals.Post("/:id/action", guard.Require("request:approve"), deps.Handlers.Request.ApprovalAction)
			fulfillment := protected.Group("/fulfillment", guard.Require("fulfillment:read"))
			fulfillment.Get("/", deps.Handlers.Request.Fulfillment)
			fulfillment.Get("/:id", deps.Handlers.Request.FulfillmentDetail)
			fulfillment.Post("/:id/data", guard.Require("fulfillment:process"), deps.Handlers.Request.SaveFulfillmentData)
			fulfillment.Post("/:id/advance", guard.Require("fulfillment:process"), deps.Handlers.Request.AdvanceFulfillment)
		}
		if deps.Handlers.Navigation != nil {
			// Any signed-in user may ask; the answer only holds the counters of modules they can open.
			protected.Get("/navigation/badges", deps.Handlers.Navigation.Badges)
		}
		if deps.Handlers.Dashboard != nil {
			protected.Get("/dashboard/overview", guard.Require("dashboard:read"), deps.Handlers.Dashboard.Overview)
		}
		if deps.Handlers.MasterData != nil {
			// Outlets and distributors: Admin and Asset Team. Asset types: Admin only
			// (access matrix of the requirement document).
			masterData := guard.Require("masterdata:manage")
			manage := guard.Require("settings:manage")
			protected.Get("/settings/outlets", masterData, deps.Handlers.MasterData.Outlets)
			protected.Post("/settings/outlets", masterData, deps.Handlers.MasterData.CreateOutlet)
			protected.Put("/settings/outlets/:id", masterData, deps.Handlers.MasterData.UpdateOutlet)
			protected.Get("/settings/distributors", masterData, deps.Handlers.MasterData.Distributors)
			protected.Post("/settings/distributors", masterData, deps.Handlers.MasterData.CreateDistributor)
			protected.Put("/settings/distributors/:id", masterData, deps.Handlers.MasterData.UpdateDistributor)
			protected.Get("/settings/types", manage, deps.Handlers.MasterData.Types)
			protected.Post("/settings/types", manage, deps.Handlers.MasterData.CreateType)
			protected.Put("/settings/types/:id", manage, deps.Handlers.MasterData.UpdateType)
		}

		if deps.Handlers.Auth != nil {
			// Profile & User Settings
			protected.Get("/me", deps.Handlers.Auth.GetMe)
			protected.Put("/me/settings", deps.Handlers.Auth.UpdateUserSettings)

			// Roles & Permissions Management
			protected.Get("/roles", guard.Require("role:manage"), deps.Handlers.Auth.GetRoles)
			protected.Post("/roles", guard.Require("role:manage"), deps.Handlers.Auth.CreateRole)
			protected.Post("/roles/:id/permissions", guard.Require("role:manage"), deps.Handlers.Auth.AssignRolePermissions)
			protected.Get("/permissions", guard.Require("role:manage"), deps.Handlers.Auth.GetPermissions)
		}

		if deps.Handlers.User != nil {
			// User Management
			protected.Get("/users", guard.Require("user:read"), deps.Handlers.User.GetUsers)
			protected.Post("/users", guard.Require("user:write"), deps.Handlers.User.CreateUser)
			protected.Put("/users/:id", guard.Require("user:write"), deps.Handlers.User.UpdateUser)
			protected.Post("/users/:id/master", guard.Require("user:master"), deps.Handlers.User.SetMasterUser)
			protected.Post("/users/:id/roles", guard.Require("role:manage"), deps.Handlers.User.AssignUserRoles)
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

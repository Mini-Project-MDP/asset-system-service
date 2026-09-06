package app

import (
	"context"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/docs"
	"github.com/gofiber/contrib/v3/swaggerui"
	"github.com/gofiber/fiber/v3"
)

const serviceName = "asset-system-service"

// DatabasePinger is the minimum database capability required by readiness.
type DatabasePinger interface {
	PingContext(context.Context) error
}

// Dependencies contains infrastructure used by the HTTP application.
type Dependencies struct {
	Database            DatabasePinger
	DatabasePingTimeout time.Duration
}

// NewRouter builds the HTTP router for the service.
func NewRouter(dependencies Dependencies) *fiber.App {
	app := fiber.New()

	// Official Fiber v3 Swagger UI middleware from github.com/gofiber/contrib/v3/swaggerui
	app.Use(swaggerui.New(swaggerui.Config{
		BasePath:    "/",
		FileContent: docs.SwaggerJSON,
		Path:        "swagger",
		Title:       "Asset System Service API Documentation",
	}))

	app.Get("/health", health)
	app.Get("/ready", ready(dependencies))

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




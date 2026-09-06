package app

import (
	"context"
	"os"
	"time"

	"github.com/gofiber/contrib/swagger"
	"github.com/gofiber/fiber/v2"
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
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// Official maintained Swagger middleware from github.com/gofiber/contrib/swagger
	swaggerPath := "./docs/swagger.json"
	if _, err := os.Stat(swaggerPath); err != nil {
		if _, errParent := os.Stat("../../docs/swagger.json"); errParent == nil {
			swaggerPath = "../../docs/swagger.json"
		}
	}
	if _, err := os.Stat(swaggerPath); err == nil {
		app.Use(swagger.New(swagger.Config{
			BasePath: "/",
			FilePath: swaggerPath,
			Path:     "swagger",
			Title:    "Asset System Service API Documentation",
		}))
	}

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
func health(c *fiber.Ctx) error {
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
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.UserContext(), dependencies.DatabasePingTimeout)
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



package app

import (
	"context"
	"time"

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

	app.Get("/health", health)
	app.Get("/ready", ready(dependencies))

	return app
}

func health(c *fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":  "ok",
		"service": serviceName,
	})
}

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


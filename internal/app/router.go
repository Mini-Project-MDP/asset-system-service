package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
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
func NewRouter(dependencies Dependencies) *gin.Engine {
	router := gin.Default()

	router.GET("/health", health)
	router.GET("/ready", ready(dependencies))

	return router
}

func health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": serviceName,
	})
}

func ready(dependencies Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), dependencies.DatabasePingTimeout)
		defer cancel()

		if err := dependencies.Database.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":   "unavailable",
				"service":  serviceName,
				"database": "unavailable",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":   "ready",
			"service":  serviceName,
			"database": "connected",
		})
	}
}

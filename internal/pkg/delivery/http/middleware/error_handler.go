package middleware

import (
	"errors"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/gofiber/fiber/v3"
)

// ErrorHandler returns a Fiber ErrorHandler to capture unhandled errors gracefully.
func ErrorHandler() fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		var e *fiber.Error
		if errors.As(err, &e) {
			code = e.Code
		}

		return response.Error(c, code, err.Error())
	}
}

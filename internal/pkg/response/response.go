package response

import (
	"github.com/gofiber/fiber/v3"
)

// Response represents a standard JSON API response structure.
type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

// JSON sends a standardized success JSON response.
func JSON(c fiber.Ctx, statusCode int, message string, data interface{}) error {
	return c.Status(statusCode).JSON(Response{
		Success: statusCode >= 200 && statusCode < 300,
		Message: message,
		Data:    data,
	})
}

// Success sends a HTTP 200 OK success response.
func Success(c fiber.Ctx, data interface{}) error {
	return JSON(c, fiber.StatusOK, "Success", data)
}

// Created sends a HTTP 201 Created response.
func Created(c fiber.Ctx, data interface{}) error {
	return JSON(c, fiber.StatusCreated, "Created successfully", data)
}

// Error sends a standardized error JSON response.
func Error(c fiber.Ctx, statusCode int, errMessage string) error {
	return c.Status(statusCode).JSON(Response{
		Success: false,
		Error:   errMessage,
	})
}

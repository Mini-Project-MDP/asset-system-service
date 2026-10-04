package handler

import (
	"errors"
	"log"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/service"
	"github.com/gofiber/fiber/v3"
)

// ImeiHandler serves the IMEI lookup used to pre-fill the Android fulfillment form.
type ImeiHandler struct {
	svc domain.ImeiService
}

func NewImeiHandler(svc domain.ImeiService) *ImeiHandler { return &ImeiHandler{svc: svc} }

// Lookup handles GET /api/v1/fulfillment/imei-lookup/:imei.
// @Summary Identify a device from its IMEI
// @Description Matches the first 8 digits (TAC) against the reference table. An unknown device answers 200 with found=false so the form can fall back to manual input.
// @Tags Fulfillment
// @Produce json
// @Security BearerAuth
// @Param imei path string true "IMEI (14-16 digits)"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Router /api/v1/fulfillment/imei-lookup/{imei} [get]
func (h *ImeiHandler) Lookup(c fiber.Ctx) error {
	info, err := h.svc.Lookup(c.Context(), c.Params("imei"))
	switch {
	case errors.Is(err, service.ErrInvalidIMEI):
		return response.Error(c, fiber.StatusBadRequest, "IMEI must be 14-16 digits")
	case err != nil:
		log.Printf("imei lookup: %v", err)
		return response.Error(c, fiber.StatusInternalServerError, "Internal server error")
	case info == nil:
		return response.Success(c, fiber.Map{"found": false})
	}
	return response.Success(c, fiber.Map{
		"found": true, "brand": info.Brand, "model": info.Model, "releaseYear": info.ReleaseYear,
	})
}

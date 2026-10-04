package handler

import (
	"errors"
	"log"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/service"
	"github.com/gofiber/fiber/v3"
)

// MasterDataHandler serves the /settings master-data endpoints (outlets,
// distributors, asset types).
type MasterDataHandler struct {
	svc domain.MasterDataService
}

func NewMasterDataHandler(svc domain.MasterDataService) *MasterDataHandler {
	return &MasterDataHandler{svc: svc}
}

// outletBody is the JSON payload for creating/updating an outlet.
type outletBody struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Region   string `json:"region"`
	IsActive *bool  `json:"is_active"`
}

// distributorBody is the JSON payload for creating/updating a distributor.
type distributorBody struct {
	Code      string   `json:"code"`
	Name      string   `json:"name"`
	OutletIDs []string `json:"outlet_ids"`
	IsActive  *bool    `json:"is_active"`
}

// assetTypeBody is the JSON payload for creating/updating an asset type.
type assetTypeBody struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Identifier string `json:"identifier"` // NONE, IMEI or SERIAL_NUMBER
	IsActive   *bool  `json:"is_active"`
}

func outletJSON(o domain.Outlet) fiber.Map {
	return fiber.Map{"id": o.ID, "code": o.Code, "name": o.Name, "region": o.Region, "is_active": o.IsActive}
}

func distributorJSON(d domain.Distributor) fiber.Map {
	names := make([]string, 0, len(d.Outlets))
	ids := make([]string, 0, len(d.Outlets))
	for _, o := range d.Outlets {
		names = append(names, o.Name)
		ids = append(ids, o.ID)
	}
	return fiber.Map{"id": d.ID, "code": d.Code, "name": d.Name, "is_active": d.IsActive, "outlets": names, "outlet_ids": ids}
}

func assetTypeJSON(t domain.AssetType) fiber.Map {
	identifier := "No"
	if t.IdentifierRequired {
		identifier = "Yes — " + t.IdentifierType + " required"
	}
	return fiber.Map{
		"id": t.ID, "code": t.Code, "name": t.Name, "identifier": identifier,
		"identifier_type": t.IdentifierType, "identifier_required": t.IdentifierRequired, "is_active": t.IsActive,
	}
}

// masterDataError maps service errors to HTTP responses.
func masterDataError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrMasterDataValidation):
		return response.Error(c, fiber.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrMasterDataNotFound):
		return response.Error(c, fiber.StatusNotFound, "Not found")
	case errors.Is(err, service.ErrMasterDataConflict), errors.Is(err, service.ErrMasterDataInUse):
		return response.Error(c, fiber.StatusConflict, err.Error())
	default:
		log.Printf("master data: %v", err)
		return response.Error(c, fiber.StatusInternalServerError, "Internal server error")
	}
}

func includeInactive(c fiber.Ctx) bool { return c.Query("include_inactive") == "true" }

// Outlets handles GET /api/v1/settings/outlets.
// @Summary List outlets
// @Description Returns active outlets; pass include_inactive=true to include deactivated ones.
// @Tags Master Data
// @Produce json
// @Security BearerAuth
// @Param include_inactive query bool false "Include deactivated outlets"
// @Success 200 {object} response.Response
// @Router /api/v1/settings/outlets [get]
func (h *MasterDataHandler) Outlets(c fiber.Ctx) error {
	items, err := h.svc.ListOutlets(c.Context(), includeInactive(c))
	if err != nil {
		return masterDataError(c, err)
	}
	out := make([]fiber.Map, 0, len(items))
	for _, o := range items {
		out = append(out, outletJSON(o))
	}
	return response.Success(c, out)
}

// CreateOutlet handles POST /api/v1/settings/outlets.
// @Summary Create an outlet
// @Tags Master Data
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body outletBody true "Outlet"
// @Success 201 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/settings/outlets [post]
func (h *MasterDataHandler) CreateOutlet(c fiber.Ctx) error {
	var body outletBody
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}
	o, err := h.svc.CreateOutlet(c.Context(), domain.SaveOutletInput(body))
	if err != nil {
		return masterDataError(c, err)
	}
	return response.Created(c, outletJSON(*o))
}

// UpdateOutlet handles PUT /api/v1/settings/outlets/:id.
// @Summary Update an outlet
// @Description Set is_active=false to deactivate; refused with 409 while an unfinished request uses the outlet.
// @Tags Master Data
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Outlet ID"
// @Param request body outletBody true "Outlet"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/settings/outlets/{id} [put]
func (h *MasterDataHandler) UpdateOutlet(c fiber.Ctx) error {
	var body outletBody
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}
	o, err := h.svc.UpdateOutlet(c.Context(), c.Params("id"), domain.SaveOutletInput(body))
	if err != nil {
		return masterDataError(c, err)
	}
	return response.Success(c, outletJSON(*o))
}

// Distributors handles GET /api/v1/settings/distributors.
// @Summary List distributors with their outlets
// @Tags Master Data
// @Produce json
// @Security BearerAuth
// @Param include_inactive query bool false "Include deactivated distributors"
// @Success 200 {object} response.Response
// @Router /api/v1/settings/distributors [get]
func (h *MasterDataHandler) Distributors(c fiber.Ctx) error {
	items, err := h.svc.ListDistributors(c.Context(), includeInactive(c))
	if err != nil {
		return masterDataError(c, err)
	}
	out := make([]fiber.Map, 0, len(items))
	for _, d := range items {
		out = append(out, distributorJSON(d))
	}
	return response.Success(c, out)
}

// CreateDistributor handles POST /api/v1/settings/distributors.
// @Summary Create a distributor and map its outlets
// @Tags Master Data
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body distributorBody true "Distributor"
// @Success 201 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/settings/distributors [post]
func (h *MasterDataHandler) CreateDistributor(c fiber.Ctx) error {
	var body distributorBody
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}
	d, err := h.svc.CreateDistributor(c.Context(), domain.SaveDistributorInput(body))
	if err != nil {
		return masterDataError(c, err)
	}
	return response.Created(c, distributorJSON(*d))
}

// UpdateDistributor handles PUT /api/v1/settings/distributors/:id.
// @Summary Update a distributor; outlet_ids replaces the whole outlet mapping
// @Tags Master Data
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Distributor ID"
// @Param request body distributorBody true "Distributor"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/settings/distributors/{id} [put]
func (h *MasterDataHandler) UpdateDistributor(c fiber.Ctx) error {
	var body distributorBody
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}
	d, err := h.svc.UpdateDistributor(c.Context(), c.Params("id"), domain.SaveDistributorInput(body))
	if err != nil {
		return masterDataError(c, err)
	}
	return response.Success(c, distributorJSON(*d))
}

// Types handles GET /api/v1/settings/types.
// @Summary List asset types
// @Tags Master Data
// @Produce json
// @Security BearerAuth
// @Param include_inactive query bool false "Include deactivated asset types"
// @Success 200 {object} response.Response
// @Router /api/v1/settings/types [get]
func (h *MasterDataHandler) Types(c fiber.Ctx) error {
	items, err := h.svc.ListAssetTypes(c.Context(), includeInactive(c))
	if err != nil {
		return masterDataError(c, err)
	}
	out := make([]fiber.Map, 0, len(items))
	for _, t := range items {
		out = append(out, assetTypeJSON(t))
	}
	return response.Success(c, out)
}

// CreateType handles POST /api/v1/settings/types.
// @Summary Create an asset type
// @Tags Master Data
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body assetTypeBody true "Asset type"
// @Success 201 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/settings/types [post]
func (h *MasterDataHandler) CreateType(c fiber.Ctx) error {
	var body assetTypeBody
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}
	t, err := h.svc.CreateAssetType(c.Context(), domain.SaveAssetTypeInput(body))
	if err != nil {
		return masterDataError(c, err)
	}
	return response.Created(c, assetTypeJSON(*t))
}

// UpdateType handles PUT /api/v1/settings/types/:id.
// @Summary Update an asset type
// @Tags Master Data
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Asset type ID"
// @Param request body assetTypeBody true "Asset type"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/settings/types/{id} [put]
func (h *MasterDataHandler) UpdateType(c fiber.Ctx) error {
	var body assetTypeBody
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}
	t, err := h.svc.UpdateAssetType(c.Context(), c.Params("id"), domain.SaveAssetTypeInput(body))
	if err != nil {
		return masterDataError(c, err)
	}
	return response.Success(c, assetTypeJSON(*t))
}

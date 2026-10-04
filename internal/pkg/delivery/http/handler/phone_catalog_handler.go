package handler

import (
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/gofiber/fiber/v3"
)

// PhoneCatalogHandler serves the phone brand/model catalog used by the
// Android fulfillment form. Reads need fulfillment:read, writes settings:manage.
type PhoneCatalogHandler struct {
	svc domain.PhoneCatalogService
}

func NewPhoneCatalogHandler(svc domain.PhoneCatalogService) *PhoneCatalogHandler {
	return &PhoneCatalogHandler{svc: svc}
}

// phoneBrandBody is the JSON payload for creating/updating a brand.
type phoneBrandBody struct {
	Name     string `json:"name"`
	IsActive *bool  `json:"is_active"`
}

// phoneModelBody is the JSON payload for creating/updating a model.
type phoneModelBody struct {
	BrandID  string `json:"brand_id"`
	Name     string `json:"name"`
	IsActive *bool  `json:"is_active"`
}

func phoneModelJSON(m domain.PhoneModel) fiber.Map {
	return fiber.Map{"id": m.ID, "brand_id": m.BrandID, "name": m.Name, "is_active": m.IsActive}
}

func phoneBrandJSON(b domain.PhoneBrand) fiber.Map {
	models := make([]fiber.Map, 0, len(b.Models))
	for _, m := range b.Models {
		models = append(models, phoneModelJSON(m))
	}
	return fiber.Map{"id": b.ID, "name": b.Name, "is_active": b.IsActive, "models": models}
}

// Catalog handles GET /api/v1/fulfillment/phone-catalog.
// @Summary List phone brands with their models
// @Description Active brands and active models; pass include_inactive=true to include deactivated ones. Models are nested so the form can filter them by the chosen brand.
// @Tags Phone Catalog
// @Produce json
// @Security BearerAuth
// @Param include_inactive query bool false "Include deactivated brands and models"
// @Success 200 {object} response.Response
// @Router /api/v1/fulfillment/phone-catalog [get]
func (h *PhoneCatalogHandler) Catalog(c fiber.Ctx) error {
	brands, err := h.svc.ListCatalog(c.Context(), includeInactive(c))
	if err != nil {
		return masterDataError(c, err)
	}
	out := make([]fiber.Map, 0, len(brands))
	for _, b := range brands {
		out = append(out, phoneBrandJSON(b))
	}
	return response.Success(c, out)
}

// CreateBrand handles POST /api/v1/settings/phone-brands.
// @Summary Create a phone brand
// @Tags Phone Catalog
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body phoneBrandBody true "Brand"
// @Success 201 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/settings/phone-brands [post]
func (h *PhoneCatalogHandler) CreateBrand(c fiber.Ctx) error {
	var body phoneBrandBody
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}
	b, err := h.svc.CreateBrand(c.Context(), domain.SavePhoneBrandInput(body))
	if err != nil {
		return masterDataError(c, err)
	}
	return response.Created(c, phoneBrandJSON(*b))
}

// UpdateBrand handles PUT /api/v1/settings/phone-brands/:id.
// @Summary Update a phone brand
// @Description Set is_active=false to hide the brand and its models from the catalog. Existing fulfillment records keep the name they stored.
// @Tags Phone Catalog
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Brand ID"
// @Param request body phoneBrandBody true "Brand"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/settings/phone-brands/{id} [put]
func (h *PhoneCatalogHandler) UpdateBrand(c fiber.Ctx) error {
	var body phoneBrandBody
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}
	b, err := h.svc.UpdateBrand(c.Context(), c.Params("id"), domain.SavePhoneBrandInput(body))
	if err != nil {
		return masterDataError(c, err)
	}
	return response.Success(c, phoneBrandJSON(*b))
}

// CreateModel handles POST /api/v1/settings/phone-models.
// @Summary Create a phone model under a brand
// @Tags Phone Catalog
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body phoneModelBody true "Model"
// @Success 201 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/settings/phone-models [post]
func (h *PhoneCatalogHandler) CreateModel(c fiber.Ctx) error {
	var body phoneModelBody
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}
	m, err := h.svc.CreateModel(c.Context(), domain.SavePhoneModelInput(body))
	if err != nil {
		return masterDataError(c, err)
	}
	return response.Created(c, phoneModelJSON(*m))
}

// UpdateModel handles PUT /api/v1/settings/phone-models/:id.
// @Summary Update a phone model (rename, move to another brand, deactivate)
// @Tags Phone Catalog
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Model ID"
// @Param request body phoneModelBody true "Model"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/settings/phone-models/{id} [put]
func (h *PhoneCatalogHandler) UpdateModel(c fiber.Ctx) error {
	var body phoneModelBody
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}
	m, err := h.svc.UpdateModel(c.Context(), c.Params("id"), domain.SavePhoneModelInput(body))
	if err != nil {
		return masterDataError(c, err)
	}
	return response.Success(c, phoneModelJSON(*m))
}

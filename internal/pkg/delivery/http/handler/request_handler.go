package handler

import (
	"errors"
	"fmt"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/delivery/http/middleware"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/jwt"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/service"
	"github.com/gofiber/fiber/v3"
)

type RequestHandler struct {
	service domain.RequestService
}

func NewRequestHandler(requestService domain.RequestService) *RequestHandler {
	return &RequestHandler{service: requestService}
}

type createRequestBody struct {
	Category      string `json:"category"`
	Outlet        string `json:"outlet"`
	Distributor   string `json:"distributor"`
	SalesDivision string `json:"salesDivision"`
	ReqType       string `json:"reqType"`
	RequesterRole string `json:"requesterRole"`
	RequesterName string `json:"requesterName"`
	Qty           int    `json:"qty"`
	Priority      string `json:"priority"`
	// RevisedFromID: set when this request resubmits one that was sent back
	// for revision (see docs/approval-engine-integration-plan.md Fase 0).
	RevisedFromID *string `json:"revisedFromId,omitempty"`
}

// List handles GET /api/v1/requests.
// @Summary List asset requests
// @Description Returns asset requests with optional text and category filters.
// @Tags Requests
// @Produce json
// @Security BearerAuth
// @Param q query string false "Search request ID, outlet, or requester"
// @Param type query string false "Asset category filter"
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/requests [get]
func (h *RequestHandler) List(c fiber.Ctx) error {
	items, err := h.service.List(c.Context(), domain.RequestFilter{
		Query: c.Query("q"),
		Type:  c.Query("type"),
	})
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	return response.Success(c, mapRequestList(items))
}

// Detail handles GET /api/v1/requests/{id}.
// @Summary Get asset request detail
// @Description Returns a single asset request and its current workflow status.
// @Tags Requests
// @Produce json
// @Security BearerAuth
// @Param id path string true "Request ID"
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 404 {object} response.Response
// @Router /api/v1/requests/{id} [get]
func (h *RequestHandler) Detail(c fiber.Ctx) error {
	item, err := h.service.Detail(c.Context(), c.Params("id"))
	if err != nil {
		return requestErrorResponse(c, err)
	}
	return response.Success(c, mapRequest(*item))
}

// Create handles POST /api/v1/requests.
// @Summary Create asset request
// @Description Creates a new asset request.
// @Tags Requests
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body createRequestBody true "Asset request payload"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/requests [post]
func (h *RequestHandler) Create(c fiber.Ctx) error {
	var body createRequestBody
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	}

	item, err := h.service.Create(c.Context(), domain.CreateRequestInput{
		Category:      body.Category,
		Outlet:        body.Outlet,
		Distributor:   body.Distributor,
		SalesDivision: body.SalesDivision,
		ReqType:       body.ReqType,
		RequesterRole: body.RequesterRole,
		RequesterName: body.RequesterName,
		Qty:           body.Qty,
		Priority:      body.Priority,
		RevisedFromID: body.RevisedFromID,
	})
	if err != nil {
		return requestErrorResponse(c, err)
	}
	return response.Success(c, mapRequest(*item))
}

// Approvals handles GET /api/v1/approvals.
// @Summary List approval requests
// @Tags Requests
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Router /api/v1/approvals [get]
func (h *RequestHandler) Approvals(c fiber.Ctx) error { return h.List(c) }

// ApprovalDetail handles GET /api/v1/approvals/{id}.
// @Summary Get approval request detail
// @Tags Requests
// @Produce json
// @Security BearerAuth
// @Param id path string true "Request ID"
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 404 {object} response.Response
// @Router /api/v1/approvals/{id} [get]
func (h *RequestHandler) ApprovalDetail(c fiber.Ctx) error { return h.Detail(c) }

// ApprovalAction handles POST /api/v1/approvals/{id}/action.
// @Summary Act on an approval request
// @Tags Requests
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Request ID"
// @Param request body map[string]string true "Approval action"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Router /api/v1/approvals/{id}/action [post]
func (h *RequestHandler) ApprovalAction(c fiber.Ctx) error {
	var body struct {
		Action  string  `json:"action"`
		Comment *string `json:"comment,omitempty"`
	}
	if err := c.Bind().Body(&body); err != nil || body.Action == "" {
		return response.Error(c, fiber.StatusBadRequest, "Invalid approval action")
	}

	claims, ok := c.Locals(middleware.UserContextKey).(*jwt.UserClaims)
	if !ok || claims == nil {
		return response.Error(c, fiber.StatusUnauthorized, "Unauthorized context")
	}

	item, err := h.service.ApprovalAction(c.Context(), c.Params("id"), domain.ApprovalActionInput{
		Action:          body.Action,
		ActorEmployeeNo: claims.EmployeeNo,
		Comment:         body.Comment,
	})
	if err != nil {
		return requestErrorResponse(c, err)
	}
	return response.Success(c, mapRequest(*item))
}

// Fulfillment handles GET /api/v1/fulfillment.
// @Summary List fulfillment requests
// @Tags Requests
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Router /api/v1/fulfillment [get]
func (h *RequestHandler) Fulfillment(c fiber.Ctx) error { return h.List(c) }

// FulfillmentDetail handles GET /api/v1/fulfillment/{id}.
// @Summary Get fulfillment request detail
// @Tags Requests
// @Produce json
// @Security BearerAuth
// @Param id path string true "Request ID"
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 404 {object} response.Response
// @Router /api/v1/fulfillment/{id} [get]
func (h *RequestHandler) FulfillmentDetail(c fiber.Ctx) error { return h.Detail(c) }

// SaveFulfillmentData handles POST /api/v1/fulfillment/{id}/data.
// @Summary Save fulfillment data
// @Tags Requests
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Request ID"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Router /api/v1/fulfillment/{id}/data [post]
func (h *RequestHandler) SaveFulfillmentData(c fiber.Ctx) error {
	var body struct {
		FulfillData interface{} `json:"fulfillData"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid fulfillment payload")
	}

	item, err := h.service.SaveFulfillmentData(c.Context(), c.Params("id"), fmt.Sprint(body.FulfillData))
	if err != nil {
		return requestErrorResponse(c, err)
	}
	return response.Success(c, mapRequest(*item))
}

// AdvanceFulfillment handles POST /api/v1/fulfillment/{id}/advance.
// @Summary Advance fulfillment to the next stage
// @Tags Requests
// @Produce json
// @Security BearerAuth
// @Param id path string true "Request ID"
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Router /api/v1/fulfillment/{id}/advance [post]
func (h *RequestHandler) AdvanceFulfillment(c fiber.Ctx) error {
	item, err := h.service.AdvanceFulfillment(c.Context(), c.Params("id"))
	if err != nil {
		return requestErrorResponse(c, err)
	}
	return response.Success(c, mapRequest(*item))
}

// requestErrorResponse maps request_service sentinel errors to their HTTP
// status code, instead of collapsing every failure to 500.
func requestErrorResponse(c fiber.Ctx, err error) error {
	var engineErr *domain.EngineAPIError
	switch {
	case errors.Is(err, service.ErrRequestNotFound):
		return response.Error(c, fiber.StatusNotFound, "Request not found")
	case errors.Is(err, service.ErrRequesterNotFound):
		return response.Error(c, fiber.StatusBadRequest, "Requester not found")
	case errors.Is(err, service.ErrInvalidRequestPayload):
		return response.Error(c, fiber.StatusBadRequest, "Invalid request payload")
	case errors.Is(err, service.ErrUnsupportedApprovalAction):
		return response.Error(c, fiber.StatusBadRequest, "Unsupported approval action")
	case errors.Is(err, service.ErrRevisionCommentRequired):
		return response.Error(c, fiber.StatusBadRequest, "Comment is required when requesting revision")
	case errors.Is(err, service.ErrApprovalNotSynced):
		return response.Error(c, fiber.StatusConflict, "Request has not synced with the approval engine yet, try again shortly")
	case errors.As(err, &engineErr):
		// The engine's own business rejections (e.g. "not the assigned
		// approver", "already decided") are already framed as 4xx — pass
		// them through as-is instead of collapsing to 500. An unexpected
		// non-4xx status from the engine is surfaced as a gateway failure.
		status := engineErr.StatusCode
		if status < 400 || status >= 500 {
			status = fiber.StatusBadGateway
		}
		return response.Error(c, status, engineErr.Message)
	default:
		return response.Error(c, fiber.StatusInternalServerError, err.Error())
	}
}

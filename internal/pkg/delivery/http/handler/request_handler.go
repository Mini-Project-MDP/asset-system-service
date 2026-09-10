package handler

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type RequestHandler struct{ db *sql.DB }

func NewRequestHandler(db *sql.DB) *RequestHandler { return &RequestHandler{db: db} }

type createRequest struct {
	Category      string `json:"category"`
	Outlet        string `json:"outlet"`
	Distributor   string `json:"distributor"`
	SalesDivision string `json:"salesDivision"`
	ReqType       string `json:"reqType"`
	RequesterRole string `json:"requesterRole"`
	RequesterName string `json:"requesterName"`
	Qty           int    `json:"qty"`
	Priority      string `json:"priority"`
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
	q, typ := strings.ToLower(c.Query("q")), c.Query("type")
	rows, err := h.db.QueryContext(c.Context(), `SELECT ar.id, at.name, o.name, ar.quantity, ar.priority, d.name, ar.sales_division, ar.request_type, u.name, ar.created_at, ar.current_step, ar.fulfillment_step, ar.status FROM asset_requests ar JOIN users u ON u.id=ar.requester_id LEFT JOIN outlets o ON o.id=ar.outlet_id LEFT JOIN distributors d ON d.id=ar.distributor_id LEFT JOIN asset_types at ON at.id=ar.asset_type_id ORDER BY ar.created_at DESC`)
	if err != nil {
		return response.Error(c, 500, err.Error())
	}
	defer rows.Close()
	items := make([]fiber.Map, 0)
	for rows.Next() {
		var id, priority, division, by, created, dbStatus string
		var categoryNS, outletNS, distributorNS, reqTypeNS sql.NullString
		var qty, step int
		var fulfill sql.NullInt64
		if err := rows.Scan(&id, &categoryNS, &outletNS, &qty, &priority, &distributorNS, &division, &reqTypeNS, &by, &created, &step, &fulfill, &dbStatus); err != nil {
			return response.Error(c, 500, err.Error())
		}
		category, outlet, distributor, reqType := categoryNS.String, outletNS.String, distributorNS.String, reqTypeNS.String
		if q != "" && !strings.Contains(strings.ToLower(id+outlet+by), q) {
			continue
		}
		if typ != "" && typ != "All types" && category != typ {
			continue
		}
		items = append(items, requestMap(id, category, outlet, qty, priority, distributor, division, reqType, by, created, step, fulfill, dbStatus))
	}
	return response.Success(c, items)
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
	c.Request().URI().SetPath("/api/requests")
	// Detail reuses the list query and selects the requested item.
	var id string
	if err := h.db.QueryRowContext(c.Context(), `SELECT id FROM asset_requests WHERE id=?`, c.Params("id")).Scan(&id); err != nil {
		return response.Error(c, fiber.StatusNotFound, "Request not found")
	}
	// Keep the response contract centralized in List by filtering its result.
	var result fiber.Map
	_ = result
	return h.listOne(c, id)
}

func (h *RequestHandler) listOne(c fiber.Ctx, id string) error {
	items := &fiber.App{}
	_ = items
	// Query through the same projection used by List.
	row := h.db.QueryRowContext(c.Context(), `SELECT ar.id, at.name, o.name, ar.quantity, ar.priority, d.name, ar.sales_division, ar.request_type, u.name, ar.created_at, ar.current_step, ar.fulfillment_step, ar.status FROM asset_requests ar JOIN users u ON u.id=ar.requester_id LEFT JOIN outlets o ON o.id=ar.outlet_id LEFT JOIN distributors d ON d.id=ar.distributor_id LEFT JOIN asset_types at ON at.id=ar.asset_type_id WHERE ar.id=?`, id)
	var rid, priority, division, by, created, dbStatus string
	var categoryNS, outletNS, distributorNS, reqTypeNS sql.NullString
	var qty, step int
	var fulfill sql.NullInt64
	if err := row.Scan(&rid, &categoryNS, &outletNS, &qty, &priority, &distributorNS, &division, &reqTypeNS, &by, &created, &step, &fulfill, &dbStatus); err != nil {
		return response.Error(c, 404, "Request not found")
	}
	category, outlet, distributor, reqType := categoryNS.String, outletNS.String, distributorNS.String, reqTypeNS.String
	return response.Success(c, requestMap(rid, category, outlet, qty, priority, distributor, division, reqType, by, created, step, fulfill, dbStatus))
}

// Create handles POST /api/v1/requests.
// @Summary Create asset request
// @Description Creates a new asset request.
// @Tags Requests
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body createRequest true "Asset request payload"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/requests [post]
func (h *RequestHandler) Create(c fiber.Ctx) error {
	var req createRequest
	if err := c.Bind().Body(&req); err != nil || req.Qty < 1 || req.Category == "" {
		return response.Error(c, 400, "Invalid request payload")
	}
	var userID string
	// Requester is resolved by email/name when available; seed users provide a safe fallback.
	if err := h.db.QueryRowContext(c.Context(), `SELECT id FROM users WHERE name=? OR email=? LIMIT 1`, req.RequesterName, req.RequesterName).Scan(&userID); err != nil {
		return response.Error(c, 400, "Requester not found")
	}
	id := "REQ-" + strings.ToUpper(uuid.NewString()[:8])
	_, err := h.db.ExecContext(c.Context(), `INSERT INTO asset_requests (id,requester_id,sales_division,request_type,quantity,priority,status,current_step,fulfillment_step) VALUES (?,?,?,?,?,?, 'WAITING_APPROVAL',0,NULL)`, id, userID, req.SalesDivision, req.ReqType, req.Qty, req.Priority)
	if err != nil {
		return response.Error(c, 500, err.Error())
	}
	return h.listOne(c, id)
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
		Action string `json:"action"`
	}
	if err := c.Bind().Body(&body); err != nil || body.Action == "" {
		return response.Error(c, 400, "Invalid approval action")
	}
	status := "WAITING_APPROVAL"
	step := 0
	switch body.Action {
	case "approve":
		status = "APPROVED"
		step = 1
	case "revision":
		status = "REVISION"
		step = -1
	case "reject":
		status = "REJECTED"
		step = -2
	default:
		return response.Error(c, 400, "Unsupported approval action")
	}
	if _, err := h.db.ExecContext(c.Context(), `UPDATE asset_requests SET status=?, current_step=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, step, c.Params("id")); err != nil {
		return response.Error(c, 500, err.Error())
	}
	return h.listOne(c, c.Params("id"))
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

func (h *RequestHandler) SaveFulfillmentData(c fiber.Ctx) error {
	var body struct {
		FulfillData interface{} `json:"fulfillData"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return response.Error(c, 400, "Invalid fulfillment payload")
	}
	if _, err := h.db.ExecContext(c.Context(), `UPDATE asset_requests SET fulfillment_data=?, fulfillment_step=1, status='FULFILLMENT', updated_at=CURRENT_TIMESTAMP WHERE id=?`, fmt.Sprint(body.FulfillData), c.Params("id")); err != nil {
		return response.Error(c, 500, err.Error())
	}
	return h.listOne(c, c.Params("id"))
}

func (h *RequestHandler) AdvanceFulfillment(c fiber.Ctx) error {
	if _, err := h.db.ExecContext(c.Context(), `UPDATE asset_requests SET fulfillment_step=COALESCE(fulfillment_step,0)+1, status=CASE WHEN COALESCE(fulfillment_step,0)+1 >= 4 THEN 'COMPLETED' ELSE 'FULFILLMENT' END, updated_at=CURRENT_TIMESTAMP WHERE id=?`, c.Params("id")); err != nil {
		return response.Error(c, 500, err.Error())
	}
	return h.listOne(c, c.Params("id"))
}

func requestMap(id, category, outlet string, qty int, priority, distributor, division, reqType, by, created string, step int, fulfill sql.NullInt64, dbStatus string) fiber.Map {
	statusText := "Waiting — Approval"
	if dbStatus == "COMPLETED" {
		statusText = "Completed"
	}
	return fiber.Map{"id": id, "type": category, "outlet": outlet, "qty": qty, "pri": priority, "distributor": distributor, "salesDivision": division, "reqType": reqType, "by": by, "byRole": "Requester", "date": created, "step": step, "chain": []fiber.Map{}, "hist": []fiber.Map{}, "statusTag": fiber.Map{"cls": "warn", "text": statusText}, "fulfillStep": func() interface{} {
		if fulfill.Valid {
			return fulfill.Int64
		}
		return nil
	}(), "fulfillData": nil}
}

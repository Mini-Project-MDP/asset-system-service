package handler

import (
	"database/sql"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/gofiber/fiber/v3"
)

type CatalogHandler struct{ db *sql.DB }

func NewCatalogHandler(db *sql.DB) *CatalogHandler { return &CatalogHandler{db: db} }

// Dashboard handles GET /api/v1/dashboard/overview.
// @Summary Get dashboard overview
// @Description Returns request statistics and dashboard activity data.
// @Tags Catalog
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Router /api/v1/dashboard/overview [get]
func (h *CatalogHandler) Dashboard(c fiber.Ctx) error {
	var total, pending, progress, completed int
	_ = h.db.QueryRowContext(c.Context(), `SELECT COUNT(*) FROM asset_requests`).Scan(&total)
	_ = h.db.QueryRowContext(c.Context(), `SELECT COUNT(*) FROM asset_requests WHERE status='WAITING_APPROVAL'`).Scan(&pending)
	_ = h.db.QueryRowContext(c.Context(), `SELECT COUNT(*) FROM asset_requests WHERE status='FULFILLMENT'`).Scan(&progress)
	_ = h.db.QueryRowContext(c.Context(), `SELECT COUNT(*) FROM asset_requests WHERE status='COMPLETED'`).Scan(&completed)
	return response.Success(c, fiber.Map{"stats": []fiber.Map{{"id": "total", "title": "Total Requests", "value": total, "subtitle": "All requests", "category": "total"}, {"id": "pending", "title": "Pending Approval", "value": pending, "subtitle": "Waiting approval", "category": "pending"}, {"id": "in_progress", "title": "In Progress", "value": progress, "subtitle": "Fulfillment", "category": "in_progress"}, {"id": "assets", "title": "Completed", "value": completed, "subtitle": "Completed requests", "category": "assets"}}, "chartData": []fiber.Map{}, "activities": []fiber.Map{}, "attentionRequests": []fiber.Map{}})
}

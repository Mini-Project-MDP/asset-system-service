package handler

import (
	"database/sql"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/gofiber/fiber/v3"
)

type CatalogHandler struct{ db *sql.DB }

func NewCatalogHandler(db *sql.DB) *CatalogHandler { return &CatalogHandler{db: db} }

func (h *CatalogHandler) Outlets(c fiber.Ctx) error {
	rows, err := h.db.QueryContext(c.Context(), `SELECT o.id,o.code,o.name,COALESCE(r.name,'') FROM outlets o LEFT JOIN regions r ON r.id=o.region_id WHERE o.is_active=1 ORDER BY o.name`)
	if err != nil {
		return response.Error(c, 500, err.Error())
	}
	defer rows.Close()
	items := make([]fiber.Map, 0)
	for rows.Next() {
		var id, code, name, region string
		if err := rows.Scan(&id, &code, &name, &region); err != nil {
			return response.Error(c, 500, err.Error())
		}
		items = append(items, fiber.Map{"id": id, "code": code, "name": name, "region": region})
	}
	return response.Success(c, items)
}
func (h *CatalogHandler) Distributors(c fiber.Ctx) error {
	rows, err := h.db.QueryContext(c.Context(), `SELECT id,name FROM distributors WHERE is_active=1 ORDER BY name`)
	if err != nil {
		return response.Error(c, 500, err.Error())
	}
	defer rows.Close()
	items := make([]fiber.Map, 0)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return response.Error(c, 500, err.Error())
		}
		items = append(items, fiber.Map{"id": id, "name": name, "outlets": []string{}})
	}
	return response.Success(c, items)
}
func (h *CatalogHandler) Types(c fiber.Ctx) error {
	rows, err := h.db.QueryContext(c.Context(), `SELECT id,name,code,CASE WHEN identifier_required=1 THEN 'Yes — '||identifier_type||' required' ELSE 'No' END FROM asset_types WHERE is_active=1 ORDER BY name`)
	if err != nil {
		return response.Error(c, 500, err.Error())
	}
	defer rows.Close()
	items := make([]fiber.Map, 0)
	for rows.Next() {
		var id, name, code, identifier string
		if err := rows.Scan(&id, &name, &code, &identifier); err != nil {
			return response.Error(c, 500, err.Error())
		}
		items = append(items, fiber.Map{"id": id, "name": name, "code": code, "identifier": identifier})
	}
	return response.Success(c, items)
}
func (h *CatalogHandler) Dashboard(c fiber.Ctx) error {
	var total, pending, progress, completed int
	_ = h.db.QueryRowContext(c.Context(), `SELECT COUNT(*) FROM asset_requests`).Scan(&total)
	_ = h.db.QueryRowContext(c.Context(), `SELECT COUNT(*) FROM asset_requests WHERE status='WAITING_APPROVAL'`).Scan(&pending)
	_ = h.db.QueryRowContext(c.Context(), `SELECT COUNT(*) FROM asset_requests WHERE status='FULFILLMENT'`).Scan(&progress)
	_ = h.db.QueryRowContext(c.Context(), `SELECT COUNT(*) FROM asset_requests WHERE status='COMPLETED'`).Scan(&completed)
	return response.Success(c, fiber.Map{"stats": []fiber.Map{{"id": "total", "title": "Total Requests", "value": total, "subtitle": "All requests", "category": "total"}, {"id": "pending", "title": "Pending Approval", "value": pending, "subtitle": "Waiting approval", "category": "pending"}, {"id": "in_progress", "title": "In Progress", "value": progress, "subtitle": "Fulfillment", "category": "in_progress"}, {"id": "assets", "title": "Completed", "value": completed, "subtitle": "Completed requests", "category": "assets"}}, "chartData": []fiber.Map{}, "activities": []fiber.Map{}, "attentionRequests": []fiber.Map{}})
}

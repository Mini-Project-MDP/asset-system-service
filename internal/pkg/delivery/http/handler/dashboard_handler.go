package handler

import (
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/service"
	"github.com/gofiber/fiber/v3"
)

// DashboardHandler serves GET /api/v1/dashboard/overview.
type DashboardHandler struct {
	svc domain.DashboardService
}

func NewDashboardHandler(svc domain.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

// indonesianMonths names the months for the first KPI card's subtitle.
var indonesianMonths = [...]string{
	"Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

// Overview handles GET /api/v1/dashboard/overview.
// @Summary Get dashboard overview
// @Description KPI cards, requests per month per category for the year, the latest activity, and the requests waiting longest for approval. Needs dashboard:read (Admin and Asset Team).
// @Tags Dashboard
// @Produce json
// @Security BearerAuth
// @Param year query int false "Chart year (2000-2100), default the current year"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Router /api/v1/dashboard/overview [get]
func (h *DashboardHandler) Overview(c fiber.Ctx) error {
	year := 0
	if raw := strings.TrimSpace(c.Query("year")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return response.Error(c, fiber.StatusBadRequest, "year must be a number")
		}
		year = parsed
	}

	overview, err := h.svc.Overview(c.Context(), year)
	switch {
	case errors.Is(err, service.ErrInvalidDashboardYear):
		return response.Error(c, fiber.StatusBadRequest, err.Error())
	case err != nil:
		log.Printf("dashboard overview: %v", err)
		return response.Error(c, fiber.StatusInternalServerError, "Internal server error")
	}
	return response.Success(c, mapDashboard(*overview))
}

func mapDashboard(o domain.DashboardOverview) fiber.Map {
	stats := []fiber.Map{
		{"id": "total", "title": "Total Requests", "value": o.Stats.RequestsThisMonth,
			"subtitle": "periode — " + indonesianMonths[o.Month-1], "category": "total"},
		{"id": "pending", "title": "Pending Approval", "value": o.Stats.PendingApproval,
			"subtitle": "menunggu approval", "category": "pending"},
		{"id": "in_progress", "title": "In Progress", "value": o.Stats.InProgress,
			"subtitle": "proses & fulfillment", "category": "in_progress"},
		{"id": "assets", "title": "Assets Registered", "value": o.Stats.AssetsRegistered,
			"subtitle": "unit tercatat", "category": "assets"},
	}

	chart := make([]fiber.Map, 0, len(o.Months))
	for _, m := range o.Months {
		chart = append(chart, fiber.Map{
			"month":   time.Month(m.Month).String()[:3],
			"barcode": m.Barcode, "android": m.Android, "server": m.Server, "mobilePrinter": m.MobilePrinter,
		})
	}

	activities := make([]fiber.Map, 0, len(o.Activities))
	for _, a := range o.Activities {
		activities = append(activities, fiber.Map{"id": a.ID, "title": a.Title, "timeAgo": a.TimeAgo, "type": a.Type})
	}

	attention := make([]fiber.Map, 0, len(o.Attention))
	for _, r := range o.Attention {
		item := mapRequest(r)
		item["priority"] = normalizePriority(r.Priority)
		attention = append(attention, item)
	}

	return fiber.Map{
		"year": o.Year, "stats": stats, "chartData": chart,
		"activities": activities, "attentionRequests": attention,
	}
}

// normalizePriority maps a stored priority to the three values the table styles.
func normalizePriority(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "urgent":
		return "urgent"
	case "high":
		return "high"
	default:
		return "normal"
	}
}

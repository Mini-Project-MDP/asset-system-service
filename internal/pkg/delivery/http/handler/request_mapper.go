package handler

import (
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/gofiber/fiber/v3"
)

// mapRequestList applies mapRequest to every item, preserving order.
func mapRequestList(items []domain.AssetRequest) []fiber.Map {
	mapped := make([]fiber.Map, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, mapRequest(item))
	}
	return mapped
}

// mapRequest builds the JSON shape the frontend expects for an asset
// request. `chain`/`hist` are left empty here on purpose — the FE currently
// computes the approval chain client-side from a mock role hierarchy; once
// Approval-Engine-Service integration lands (Milestone 5/6), this mapper
// will populate them from the engine's own step/assignment data instead.
func mapRequest(r domain.AssetRequest) fiber.Map {
	statusText := "Waiting — Approval"
	if r.Status == domain.RequestStatusCompleted {
		statusText = "Completed"
	}

	var fulfillStep interface{}
	if r.FulfillmentStep != nil {
		fulfillStep = *r.FulfillmentStep
	}

	return fiber.Map{
		"id":            r.ID,
		"type":          r.Category,
		"outlet":        r.Outlet,
		"qty":           r.Quantity,
		"pri":           r.Priority,
		"distributor":   r.Distributor,
		"salesDivision": r.SalesDivision,
		"reqType":       r.RequestType,
		"by":            r.RequesterName,
		"byRole":        "Requester",
		"date":          r.CreatedAt,
		"step":          r.CurrentStep,
		"chain":         []fiber.Map{},
		"hist":          []fiber.Map{},
		"statusTag":     fiber.Map{"cls": "warn", "text": statusText},
		"fulfillStep":   fulfillStep,
		"fulfillData":   nil,
	}
}

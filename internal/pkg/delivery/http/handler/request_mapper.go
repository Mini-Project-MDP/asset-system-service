package handler

import (
	"encoding/json"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/gofiber/fiber/v3"
)

var fulfillmentStages = []string{"Processing", "Shipped", "Delivered"}

// revisionTag is the "Revision — <Role>" pill, naming the role that sent the
// request back. If no step records that, it is just "Revision".
func revisionTag(r domain.AssetRequest) fiber.Map {
	for _, step := range r.Chain {
		if step.Status == "revision" {
			label := step.RoleLabel
			if label == "" {
				label = step.Role
			}
			return fiber.Map{"cls": "warn", "text": "Revision — " + label}
		}
	}
	return fiber.Map{"cls": "warn", "text": "Revision"}
}

func computeStatusTag(r domain.AssetRequest) fiber.Map {
	switch r.Status {
	case domain.RequestStatusCompleted:
		return fiber.Map{"cls": "go", "text": "Completed"}
	case domain.RequestStatusRejected:
		for _, h := range r.Hist {
			if strings.Contains(strings.ToLower(h.Action), "revision") {
				return revisionTag(r)
			}
		}
		return fiber.Map{"cls": "stop", "text": "Rejected"}
	case domain.RequestStatusRevision:
		return revisionTag(r)
	case domain.RequestStatusFulfillment:
		stage := "Processing"
		if r.FulfillmentStep != nil && *r.FulfillmentStep >= 0 && *r.FulfillmentStep < len(fulfillmentStages) {
			stage = fulfillmentStages[*r.FulfillmentStep]
		}
		return fiber.Map{"cls": "brand", "text": "Fulfillment — " + stage}
	case domain.RequestStatusApproved:
		if r.FulfillmentStep != nil {
			stage := "Processing"
			if *r.FulfillmentStep >= 0 && *r.FulfillmentStep < len(fulfillmentStages) {
				stage = fulfillmentStages[*r.FulfillmentStep]
			}
			return fiber.Map{"cls": "brand", "text": "Fulfillment — " + stage}
		}
		return fiber.Map{"cls": "go", "text": "Approved"}
	default:
		if r.ApprovalStatus == domain.ApprovalSyncPending {
			return fiber.Map{"cls": "warn", "text": "Syncing with Approval Engine"}
		}
		if r.CurrentStepName != nil && *r.CurrentStepName != "" {
			return fiber.Map{"cls": "warn", "text": "Waiting — " + *r.CurrentStepName}
		}
		return fiber.Map{"cls": "warn", "text": "Waiting — Approval"}
	}
}

// mapRequestList applies mapRequest to every item, preserving order.
func mapRequestList(items []domain.AssetRequest) []fiber.Map {
	mapped := make([]fiber.Map, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, mapRequest(item))
	}
	return mapped
}

// parseFulfillmentData returns the stored asset data as a JSON object. Data
// that is not valid JSON (rows written before the data was stored as JSON used
// Go's value formatting, e.g. "map[codes:[A B]]") cannot be recovered, so it
// is reported as absent instead of being passed on as an unusable string.
func parseFulfillmentData(stored *string) any {
	if stored == nil || *stored == "" {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(*stored), &parsed); err != nil {
		return nil
	}
	return parsed
}

// mapRequest builds the JSON shape the frontend expects for an asset request.
func mapRequest(r domain.AssetRequest) fiber.Map {
	var fulfillStep interface{}
	if r.FulfillmentStep != nil {
		fulfillStep = *r.FulfillmentStep
	}

	chain := r.Chain
	if chain == nil {
		chain = []domain.ApprovalStepItem{}
	}

	hist := r.Hist
	if hist == nil {
		hist = []domain.ApprovalHistoryItem{}
	}

	return fiber.Map{
		"id":              r.ID,
		"type":            r.Category,
		"outlet":          r.Outlet,
		"qty":             r.Quantity,
		"pri":             r.Priority,
		"distributor":     r.Distributor,
		"salesDivision":   r.SalesDivision,
		"reqType":         r.RequestType,
		"by":              r.RequesterName,
		"byRole":          requesterRoleOrDefault(r.RequesterRole),
		"date":            r.CreatedAt,
		"step":            r.CurrentStep,
		"chain":           chain,
		"hist":            hist,
		"statusTag":       computeStatusTag(r),
		"fulfillStep":     fulfillStep,
		"fulfillData":     parseFulfillmentData(r.FulfillmentData),
		"revisedFromId":   r.RevisedFromID,
		"approvalStatus":  r.ApprovalStatus,
		"currentStepName": r.CurrentStepName,
	}
}

// mapFormOptions builds the JSON the New Request form reads.
func mapFormOptions(o domain.RequestFormOptions) fiber.Map {
	refs := func(list []domain.OutletRef) []fiber.Map {
		out := make([]fiber.Map, 0, len(list))
		for _, r := range list {
			out = append(out, fiber.Map{"id": r.ID, "name": r.Name})
		}
		return out
	}
	distributors := make([]fiber.Map, 0, len(o.Distributors))
	for _, d := range o.Distributors {
		distributors = append(distributors, fiber.Map{"id": d.ID, "name": d.Name, "outlets": refs(d.Outlets)})
	}
	roles := make(fiber.Map, len(o.RequesterRoles))
	for category, options := range o.RequesterRoles {
		list := make([]fiber.Map, 0, len(options))
		for _, r := range options {
			list = append(list, fiber.Map{"code": r.Code, "label": r.Label})
		}
		roles[category] = list
	}
	return fiber.Map{
		"categories":     o.Categories,
		"distributors":   distributors,
		"outlets":        refs(o.Outlets),
		"salesDivisions": o.SalesDivisions,
		"requestTypes":   o.RequestTypes,
		"priorities":     o.Priorities,
		"requesterRoles": roles,
	}
}

// requesterRoleOrDefault is the role stored with the request; requests made
// before roles were stored show the generic "Requester".
func requesterRoleOrDefault(role string) string {
	if role == "" {
		return "Requester"
	}
	return role
}

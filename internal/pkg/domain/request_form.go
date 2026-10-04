package domain

import "context"

// Request categories offered by the New Request form. Further categories
// (Mobile Printer) arrive with their own module.
const (
	CategoryBarcode = "Barcode"
	CategoryAndroid = "Android"
	CategoryServer  = "Server"
)

// Request types, asked for Android requests only.
const (
	RequestTypeNew     = "Baru"
	RequestTypeRenewal = "Peremajaan"
)

// Request priorities, as stored in asset_requests.priority.
const (
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"
)

// RequesterRoleOption is one entry of the Requester Role select.
type RequesterRoleOption struct {
	Code  string
	Label string
}

// FormMasterData is the master data the New Request form chooses from.
type FormMasterData struct {
	// Distributors are the active ones, each with the outlets it covers.
	Distributors []Distributor
	// Outlets are all active outlets (offered when the distributor is typed in by hand).
	Outlets        []Outlet
	SalesDivisions []string
}

// RequestFormSource reads the master data behind the form.
type RequestFormSource interface {
	FormMasterData(ctx context.Context) (FormMasterData, error)
}

// RequestFormOptions is everything the New Request form needs to render.
type RequestFormOptions struct {
	Categories     []string
	Distributors   []Distributor
	Outlets        []OutletRef
	SalesDivisions []string
	RequestTypes   []string
	Priorities     []string
	// RequesterRoles lists the roles allowed per category.
	RequesterRoles map[string][]RequesterRoleOption
}

package domain

import "context"

// Request categories offered by the New Request form. Further categories
// (Mobile Printer) arrive with their own module.
const (
	CategoryBarcode = "Barcode"
	CategoryAndroid = "Android"
	CategoryServer  = "Server"
	// CategoryMobilePrinter: new requests only, never a replacement.
	CategoryMobilePrinter = "Mobile Printer"
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

// Barcode requests say why the units are needed (Tipe Pengajuan), with a count per reason.
const (
	BarcodeKindDamaged     = "Rusak"
	BarcodeKindLost        = "Hilang"
	BarcodeKindNewOutlet   = "NOO" // new outlet
	BarcodeKindBufferStock = "Buffer Stock"
)

// QuantityBreakdownItem is how many units of a request are for one reason.
type QuantityBreakdownItem struct {
	Kind     string
	Quantity int
}

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
	// BarcodeKinds are the Tipe Pengajuan choices of a Barcode request.
	BarcodeKinds []string
	Priorities   []string
	// RequesterRoles lists the roles allowed per category.
	RequesterRoles map[string][]RequesterRoleOption
}

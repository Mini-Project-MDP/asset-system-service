package domain

import (
	"context"
	"errors"
)

// ErrDuplicate is returned by MasterDataRepository when a write violates a
// uniqueness constraint (duplicate code, or an outlet already mapped to a
// distributor).
var ErrDuplicate = errors.New("duplicate value")

// Identifier types accepted for an asset type. IdentifierNone means the type
// needs no per-unit identifier at fulfillment.
const (
	IdentifierNone   = "NONE"
	IdentifierIMEI   = "IMEI"
	IdentifierSerial = "SERIAL_NUMBER"
)

// Outlet is a store/customer covered by a distributor.
type Outlet struct {
	ID       string
	Code     string
	Name     string
	Region   string // regions.name, empty if unset
	IsActive bool
}

// OutletRef is the id/name pair of an outlet mapped to a distributor.
type OutletRef struct {
	ID   string
	Name string
}

// Distributor is a master-data distributor ("Subdist") and the outlets it covers.
type Distributor struct {
	ID       string
	Code     string
	Name     string
	IsActive bool
	Outlets  []OutletRef
}

// AssetType is a request category whose identifier flag drives the
// fulfillment data-capture form.
type AssetType struct {
	ID                 string
	Code               string
	Name               string
	IdentifierType     string // one of the Identifier* constants
	IdentifierRequired bool
	IsActive           bool
}

// SaveOutletInput is the create/update payload for an outlet. IsActive is nil
// when the caller does not want to change it.
type SaveOutletInput struct {
	Code     string
	Name     string
	Region   string
	IsActive *bool
}

// SaveDistributorInput is the create/update payload for a distributor.
type SaveDistributorInput struct {
	Code      string
	Name      string
	OutletIDs []string
	IsActive  *bool
}

// SaveAssetTypeInput is the create/update payload for an asset type.
type SaveAssetTypeInput struct {
	Code       string
	Name       string
	Identifier string // one of the Identifier* constants
	IsActive   *bool
}

// MasterDataRepository persists outlets, distributors and asset types. Get*
// methods return (nil, nil) when the row does not exist; writes return
// ErrDuplicate on a uniqueness violation.
type MasterDataRepository interface {
	ListOutlets(ctx context.Context, includeInactive bool) ([]Outlet, error)
	GetOutlet(ctx context.Context, id string) (*Outlet, error)
	CreateOutlet(ctx context.Context, in SaveOutletInput, isActive bool) (string, error)
	UpdateOutlet(ctx context.Context, id string, in SaveOutletInput, isActive bool) error
	OutletInUse(ctx context.Context, id string) (bool, error)

	ListDistributors(ctx context.Context, includeInactive bool) ([]Distributor, error)
	GetDistributor(ctx context.Context, id string) (*Distributor, error)
	CreateDistributor(ctx context.Context, in SaveDistributorInput, isActive bool) (string, error)
	UpdateDistributor(ctx context.Context, id string, in SaveDistributorInput, isActive bool) error
	DistributorInUse(ctx context.Context, id string) (bool, error)
	// MissingOutlets returns the ids in outletIDs that do not exist.
	MissingOutlets(ctx context.Context, outletIDs []string) ([]string, error)
	// OutletsMappedElsewhere returns the ids in outletIDs already mapped to a
	// distributor other than exceptDistributorID (empty for a new distributor).
	OutletsMappedElsewhere(ctx context.Context, outletIDs []string, exceptDistributorID string) ([]string, error)

	ListAssetTypes(ctx context.Context, includeInactive bool) ([]AssetType, error)
	GetAssetType(ctx context.Context, id string) (*AssetType, error)
	CreateAssetType(ctx context.Context, in SaveAssetTypeInput, isActive bool) (string, error)
	UpdateAssetType(ctx context.Context, id string, in SaveAssetTypeInput, isActive bool) error
	AssetTypeInUse(ctx context.Context, id string) (bool, error)
}

// MasterDataService holds the business rules for master-data maintenance.
type MasterDataService interface {
	ListOutlets(ctx context.Context, includeInactive bool) ([]Outlet, error)
	CreateOutlet(ctx context.Context, in SaveOutletInput) (*Outlet, error)
	UpdateOutlet(ctx context.Context, id string, in SaveOutletInput) (*Outlet, error)

	ListDistributors(ctx context.Context, includeInactive bool) ([]Distributor, error)
	CreateDistributor(ctx context.Context, in SaveDistributorInput) (*Distributor, error)
	UpdateDistributor(ctx context.Context, id string, in SaveDistributorInput) (*Distributor, error)

	ListAssetTypes(ctx context.Context, includeInactive bool) ([]AssetType, error)
	CreateAssetType(ctx context.Context, in SaveAssetTypeInput) (*AssetType, error)
	UpdateAssetType(ctx context.Context, id string, in SaveAssetTypeInput) (*AssetType, error)
}

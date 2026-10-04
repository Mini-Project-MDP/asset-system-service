package domain

import "context"

// PhoneModel is a model (Jenis/Model) of a phone brand, as offered in the
// Android fulfillment form.
type PhoneModel struct {
	ID       string
	BrandID  string
	Name     string
	IsActive bool
}

// PhoneBrand is a phone brand (Merk HP) with its models.
type PhoneBrand struct {
	ID       string
	Name     string
	IsActive bool
	Models   []PhoneModel
}

// SavePhoneBrandInput is the create/update payload for a brand. IsActive is
// nil when the caller does not want to change it.
type SavePhoneBrandInput struct {
	Name     string
	IsActive *bool
}

// SavePhoneModelInput is the create/update payload for a model.
type SavePhoneModelInput struct {
	BrandID  string
	Name     string
	IsActive *bool
}

// PhoneCatalogRepository persists the phone brand/model catalog. Get*
// methods return (nil, nil) when the row does not exist; writes return
// ErrDuplicate when a name is already taken (brand names globally, model
// names within a brand, both ignoring letter case).
type PhoneCatalogRepository interface {
	// ListBrands returns brands with their models. Unless includeInactive is
	// set, only active brands and their active models are returned.
	ListBrands(ctx context.Context, includeInactive bool) ([]PhoneBrand, error)
	GetBrand(ctx context.Context, id string) (*PhoneBrand, error)
	CreateBrand(ctx context.Context, in SavePhoneBrandInput, isActive bool) (string, error)
	UpdateBrand(ctx context.Context, id string, in SavePhoneBrandInput, isActive bool) error

	GetModel(ctx context.Context, id string) (*PhoneModel, error)
	CreateModel(ctx context.Context, in SavePhoneModelInput, isActive bool) (string, error)
	UpdateModel(ctx context.Context, id string, in SavePhoneModelInput, isActive bool) error
}

// PhoneCatalogService holds the business rules for the phone catalog.
type PhoneCatalogService interface {
	ListCatalog(ctx context.Context, includeInactive bool) ([]PhoneBrand, error)
	CreateBrand(ctx context.Context, in SavePhoneBrandInput) (*PhoneBrand, error)
	UpdateBrand(ctx context.Context, id string, in SavePhoneBrandInput) (*PhoneBrand, error)
	CreateModel(ctx context.Context, in SavePhoneModelInput) (*PhoneModel, error)
	UpdateModel(ctx context.Context, id string, in SavePhoneModelInput) (*PhoneModel, error)
}

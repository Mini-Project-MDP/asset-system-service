package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// fakeMasterDataRepository is an in-memory domain.MasterDataRepository.
type fakeMasterDataRepository struct {
	outlets      map[string]*domain.Outlet
	distributors map[string]*domain.Distributor
	assetTypes   map[string]*domain.AssetType
	inUse        map[string]bool   // id -> referenced by an unfinished request
	mappedTo     map[string]string // outlet id -> distributor id
	nextID       int
}

func newFakeMasterDataRepository() *fakeMasterDataRepository {
	return &fakeMasterDataRepository{
		outlets:      map[string]*domain.Outlet{},
		distributors: map[string]*domain.Distributor{},
		assetTypes:   map[string]*domain.AssetType{},
		inUse:        map[string]bool{},
		mappedTo:     map[string]string{},
	}
}

func (f *fakeMasterDataRepository) id(prefix string) string {
	f.nextID++
	return prefix + string(rune('0'+f.nextID))
}

func (f *fakeMasterDataRepository) ListOutlets(ctx context.Context, includeInactive bool) ([]domain.Outlet, error) {
	var out []domain.Outlet
	for _, o := range f.outlets {
		if o.IsActive || includeInactive {
			out = append(out, *o)
		}
	}
	return out, nil
}

func (f *fakeMasterDataRepository) GetOutlet(ctx context.Context, id string) (*domain.Outlet, error) {
	if o, ok := f.outlets[id]; ok {
		c := *o
		return &c, nil
	}
	return nil, nil
}

func (f *fakeMasterDataRepository) CreateOutlet(ctx context.Context, in domain.SaveOutletInput, isActive bool) (string, error) {
	for _, o := range f.outlets {
		if o.Code == in.Code {
			return "", domain.ErrDuplicate
		}
	}
	id := f.id("out")
	f.outlets[id] = &domain.Outlet{ID: id, Code: in.Code, Name: in.Name, Region: in.Region, IsActive: isActive}
	return id, nil
}

func (f *fakeMasterDataRepository) UpdateOutlet(ctx context.Context, id string, in domain.SaveOutletInput, isActive bool) error {
	for oid, o := range f.outlets {
		if oid != id && o.Code == in.Code {
			return domain.ErrDuplicate
		}
	}
	f.outlets[id] = &domain.Outlet{ID: id, Code: in.Code, Name: in.Name, Region: in.Region, IsActive: isActive}
	return nil
}

func (f *fakeMasterDataRepository) OutletInUse(ctx context.Context, id string) (bool, error) {
	return f.inUse[id], nil
}

func (f *fakeMasterDataRepository) ListDistributors(ctx context.Context, includeInactive bool) ([]domain.Distributor, error) {
	var out []domain.Distributor
	for _, d := range f.distributors {
		if d.IsActive || includeInactive {
			out = append(out, *d)
		}
	}
	return out, nil
}

func (f *fakeMasterDataRepository) GetDistributor(ctx context.Context, id string) (*domain.Distributor, error) {
	if d, ok := f.distributors[id]; ok {
		c := *d
		return &c, nil
	}
	return nil, nil
}

func (f *fakeMasterDataRepository) CreateDistributor(ctx context.Context, in domain.SaveDistributorInput, isActive bool) (string, error) {
	for _, d := range f.distributors {
		if d.Code == in.Code {
			return "", domain.ErrDuplicate
		}
	}
	id := f.id("dist")
	d := &domain.Distributor{ID: id, Code: in.Code, Name: in.Name, IsActive: isActive}
	for _, oid := range in.OutletIDs {
		d.Outlets = append(d.Outlets, domain.OutletRef{ID: oid, Name: f.outlets[oid].Name})
		f.mappedTo[oid] = id
	}
	f.distributors[id] = d
	return id, nil
}

func (f *fakeMasterDataRepository) UpdateDistributor(ctx context.Context, id string, in domain.SaveDistributorInput, isActive bool) error {
	for did, d := range f.distributors {
		if did != id && d.Code == in.Code {
			return domain.ErrDuplicate
		}
	}
	for oid, did := range f.mappedTo {
		if did == id {
			delete(f.mappedTo, oid)
		}
	}
	d := &domain.Distributor{ID: id, Code: in.Code, Name: in.Name, IsActive: isActive}
	for _, oid := range in.OutletIDs {
		d.Outlets = append(d.Outlets, domain.OutletRef{ID: oid, Name: f.outlets[oid].Name})
		f.mappedTo[oid] = id
	}
	f.distributors[id] = d
	return nil
}

func (f *fakeMasterDataRepository) DistributorInUse(ctx context.Context, id string) (bool, error) {
	return f.inUse[id], nil
}

func (f *fakeMasterDataRepository) MissingOutlets(ctx context.Context, outletIDs []string) ([]string, error) {
	var missing []string
	for _, id := range outletIDs {
		if _, ok := f.outlets[id]; !ok {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

func (f *fakeMasterDataRepository) OutletsMappedElsewhere(ctx context.Context, outletIDs []string, exceptDistributorID string) ([]string, error) {
	var taken []string
	for _, id := range outletIDs {
		if did, ok := f.mappedTo[id]; ok && did != exceptDistributorID {
			taken = append(taken, id)
		}
	}
	return taken, nil
}

func (f *fakeMasterDataRepository) ListAssetTypes(ctx context.Context, includeInactive bool) ([]domain.AssetType, error) {
	var out []domain.AssetType
	for _, t := range f.assetTypes {
		if t.IsActive || includeInactive {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (f *fakeMasterDataRepository) GetAssetType(ctx context.Context, id string) (*domain.AssetType, error) {
	if t, ok := f.assetTypes[id]; ok {
		c := *t
		return &c, nil
	}
	return nil, nil
}

func (f *fakeMasterDataRepository) CreateAssetType(ctx context.Context, in domain.SaveAssetTypeInput, isActive bool) (string, error) {
	for _, t := range f.assetTypes {
		if t.Code == in.Code {
			return "", domain.ErrDuplicate
		}
	}
	id := f.id("type")
	f.assetTypes[id] = &domain.AssetType{
		ID: id, Code: in.Code, Name: in.Name, IdentifierType: in.Identifier,
		IdentifierRequired: in.Identifier != domain.IdentifierNone, IsActive: isActive,
	}
	return id, nil
}

func (f *fakeMasterDataRepository) UpdateAssetType(ctx context.Context, id string, in domain.SaveAssetTypeInput, isActive bool) error {
	for tid, t := range f.assetTypes {
		if tid != id && t.Code == in.Code {
			return domain.ErrDuplicate
		}
	}
	f.assetTypes[id] = &domain.AssetType{
		ID: id, Code: in.Code, Name: in.Name, IdentifierType: in.Identifier,
		IdentifierRequired: in.Identifier != domain.IdentifierNone, IsActive: isActive,
	}
	return nil
}

func (f *fakeMasterDataRepository) AssetTypeInUse(ctx context.Context, id string) (bool, error) {
	return f.inUse[id], nil
}

func boolPtr(b bool) *bool { return &b }

func TestCreateOutlet(t *testing.T) {
	ctx := context.Background()

	t.Run("creates active outlet with trimmed fields", func(t *testing.T) {
		svc := NewMasterDataService(newFakeMasterDataRepository())
		got, err := svc.CreateOutlet(ctx, domain.SaveOutletInput{Code: " OUT-011 ", Name: " Bandung Kota ", Region: "West Java"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Code != "OUT-011" || got.Name != "Bandung Kota" || !got.IsActive {
			t.Fatalf("got %+v, want trimmed active outlet", got)
		}
	})

	t.Run("rejects missing required fields", func(t *testing.T) {
		svc := NewMasterDataService(newFakeMasterDataRepository())
		for _, in := range []domain.SaveOutletInput{
			{Name: "Bandung", Region: "West Java"},
			{Code: "OUT-1", Region: "West Java"},
			{Code: "OUT-1", Name: "Bandung"},
			{Code: "  ", Name: "Bandung", Region: "West Java"},
		} {
			if _, err := svc.CreateOutlet(ctx, in); !errors.Is(err, ErrMasterDataValidation) {
				t.Errorf("input %+v: err = %v, want ErrMasterDataValidation", in, err)
			}
		}
	})

	t.Run("duplicate code is a conflict", func(t *testing.T) {
		svc := NewMasterDataService(newFakeMasterDataRepository())
		in := domain.SaveOutletInput{Code: "OUT-011", Name: "Bandung", Region: "West Java"}
		if _, err := svc.CreateOutlet(ctx, in); err != nil {
			t.Fatalf("first create: %v", err)
		}
		if _, err := svc.CreateOutlet(ctx, in); !errors.Is(err, ErrMasterDataConflict) {
			t.Fatalf("second create err = %v, want ErrMasterDataConflict", err)
		}
	})
}

func TestUpdateOutlet(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown id is not found", func(t *testing.T) {
		svc := NewMasterDataService(newFakeMasterDataRepository())
		_, err := svc.UpdateOutlet(ctx, "nope", domain.SaveOutletInput{Code: "A", Name: "A", Region: "R"})
		if !errors.Is(err, ErrMasterDataNotFound) {
			t.Fatalf("err = %v, want ErrMasterDataNotFound", err)
		}
	})

	t.Run("nil IsActive keeps current state", func(t *testing.T) {
		repo := newFakeMasterDataRepository()
		svc := NewMasterDataService(repo)
		o, _ := svc.CreateOutlet(ctx, domain.SaveOutletInput{Code: "A", Name: "A", Region: "R", IsActive: boolPtr(false)})
		got, err := svc.UpdateOutlet(ctx, o.ID, domain.SaveOutletInput{Code: "A", Name: "Renamed", Region: "R"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.IsActive || got.Name != "Renamed" {
			t.Fatalf("got %+v, want inactive and renamed", got)
		}
	})

	t.Run("cannot deactivate an outlet used by an unfinished request", func(t *testing.T) {
		repo := newFakeMasterDataRepository()
		svc := NewMasterDataService(repo)
		o, _ := svc.CreateOutlet(ctx, domain.SaveOutletInput{Code: "A", Name: "A", Region: "R"})
		repo.inUse[o.ID] = true
		_, err := svc.UpdateOutlet(ctx, o.ID, domain.SaveOutletInput{Code: "A", Name: "A", Region: "R", IsActive: boolPtr(false)})
		if !errors.Is(err, ErrMasterDataInUse) {
			t.Fatalf("err = %v, want ErrMasterDataInUse", err)
		}
	})

	t.Run("can still rename an in-use outlet", func(t *testing.T) {
		repo := newFakeMasterDataRepository()
		svc := NewMasterDataService(repo)
		o, _ := svc.CreateOutlet(ctx, domain.SaveOutletInput{Code: "A", Name: "A", Region: "R"})
		repo.inUse[o.ID] = true
		if _, err := svc.UpdateOutlet(ctx, o.ID, domain.SaveOutletInput{Code: "A", Name: "B", Region: "R"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("code taken by another outlet is a conflict", func(t *testing.T) {
		svc := NewMasterDataService(newFakeMasterDataRepository())
		svc.CreateOutlet(ctx, domain.SaveOutletInput{Code: "A", Name: "A", Region: "R"})
		b, _ := svc.CreateOutlet(ctx, domain.SaveOutletInput{Code: "B", Name: "B", Region: "R"})
		_, err := svc.UpdateOutlet(ctx, b.ID, domain.SaveOutletInput{Code: "A", Name: "B", Region: "R"})
		if !errors.Is(err, ErrMasterDataConflict) {
			t.Fatalf("err = %v, want ErrMasterDataConflict", err)
		}
	})
}

func TestCreateDistributor(t *testing.T) {
	ctx := context.Background()

	newSvcWithOutlets := func() (domain.MasterDataService, *fakeMasterDataRepository, []string) {
		repo := newFakeMasterDataRepository()
		svc := NewMasterDataService(repo)
		a, _ := svc.CreateOutlet(ctx, domain.SaveOutletInput{Code: "OUT-1", Name: "Bandung", Region: "West Java"})
		b, _ := svc.CreateOutlet(ctx, domain.SaveOutletInput{Code: "OUT-2", Name: "Depok", Region: "West Java"})
		return svc, repo, []string{a.ID, b.ID}
	}

	t.Run("maps outlets to the new distributor", func(t *testing.T) {
		svc, _, outlets := newSvcWithOutlets()
		got, err := svc.CreateDistributor(ctx, domain.SaveDistributorInput{Code: "D1", Name: "PT Utama", OutletIDs: outlets})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.Outlets) != 2 {
			t.Fatalf("got %d outlets, want 2", len(got.Outlets))
		}
	})

	t.Run("rejects missing code or name", func(t *testing.T) {
		svc, _, _ := newSvcWithOutlets()
		for _, in := range []domain.SaveDistributorInput{{Name: "PT"}, {Code: "D1"}} {
			if _, err := svc.CreateDistributor(ctx, in); !errors.Is(err, ErrMasterDataValidation) {
				t.Errorf("input %+v: err = %v, want ErrMasterDataValidation", in, err)
			}
		}
	})

	t.Run("rejects unknown outlet id", func(t *testing.T) {
		svc, _, _ := newSvcWithOutlets()
		_, err := svc.CreateDistributor(ctx, domain.SaveDistributorInput{Code: "D1", Name: "PT", OutletIDs: []string{"ghost"}})
		if !errors.Is(err, ErrMasterDataValidation) {
			t.Fatalf("err = %v, want ErrMasterDataValidation", err)
		}
	})

	t.Run("rejects the same outlet twice in one payload", func(t *testing.T) {
		svc, _, outlets := newSvcWithOutlets()
		_, err := svc.CreateDistributor(ctx, domain.SaveDistributorInput{Code: "D1", Name: "PT", OutletIDs: []string{outlets[0], outlets[0]}})
		if !errors.Is(err, ErrMasterDataValidation) {
			t.Fatalf("err = %v, want ErrMasterDataValidation", err)
		}
	})

	t.Run("outlet already served by another distributor is a conflict", func(t *testing.T) {
		svc, _, outlets := newSvcWithOutlets()
		if _, err := svc.CreateDistributor(ctx, domain.SaveDistributorInput{Code: "D1", Name: "PT A", OutletIDs: outlets[:1]}); err != nil {
			t.Fatalf("first: %v", err)
		}
		_, err := svc.CreateDistributor(ctx, domain.SaveDistributorInput{Code: "D2", Name: "PT B", OutletIDs: outlets[:1]})
		if !errors.Is(err, ErrMasterDataConflict) {
			t.Fatalf("err = %v, want ErrMasterDataConflict", err)
		}
	})

	t.Run("update may keep its own outlets", func(t *testing.T) {
		svc, _, outlets := newSvcWithOutlets()
		d, _ := svc.CreateDistributor(ctx, domain.SaveDistributorInput{Code: "D1", Name: "PT A", OutletIDs: outlets[:1]})
		got, err := svc.UpdateDistributor(ctx, d.ID, domain.SaveDistributorInput{Code: "D1", Name: "PT A2", OutletIDs: outlets})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "PT A2" || len(got.Outlets) != 2 {
			t.Fatalf("got %+v, want renamed with 2 outlets", got)
		}
	})

	t.Run("cannot deactivate a distributor used by an unfinished request", func(t *testing.T) {
		svc, repo, outlets := newSvcWithOutlets()
		d, _ := svc.CreateDistributor(ctx, domain.SaveDistributorInput{Code: "D1", Name: "PT A", OutletIDs: outlets})
		repo.inUse[d.ID] = true
		_, err := svc.UpdateDistributor(ctx, d.ID, domain.SaveDistributorInput{Code: "D1", Name: "PT A", OutletIDs: outlets, IsActive: boolPtr(false)})
		if !errors.Is(err, ErrMasterDataInUse) {
			t.Fatalf("err = %v, want ErrMasterDataInUse", err)
		}
	})
}

func TestCreateAssetType(t *testing.T) {
	ctx := context.Background()

	t.Run("NONE identifier means not required", func(t *testing.T) {
		svc := NewMasterDataService(newFakeMasterDataRepository())
		got, err := svc.CreateAssetType(ctx, domain.SaveAssetTypeInput{Code: "BC", Name: "Barcode", Identifier: domain.IdentifierNone})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.IdentifierRequired {
			t.Fatalf("got %+v, want identifier not required", got)
		}
	})

	t.Run("IMEI identifier is required", func(t *testing.T) {
		svc := NewMasterDataService(newFakeMasterDataRepository())
		got, err := svc.CreateAssetType(ctx, domain.SaveAssetTypeInput{Code: "AN", Name: "Android", Identifier: domain.IdentifierIMEI})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.IdentifierRequired || got.IdentifierType != domain.IdentifierIMEI {
			t.Fatalf("got %+v, want IMEI required", got)
		}
	})

	t.Run("rejects unknown identifier and missing fields", func(t *testing.T) {
		svc := NewMasterDataService(newFakeMasterDataRepository())
		for _, in := range []domain.SaveAssetTypeInput{
			{Code: "X", Name: "X", Identifier: "BARCODE"},
			{Code: "X", Name: "X"},
			{Name: "X", Identifier: domain.IdentifierNone},
			{Code: "X", Identifier: domain.IdentifierNone},
		} {
			if _, err := svc.CreateAssetType(ctx, in); !errors.Is(err, ErrMasterDataValidation) {
				t.Errorf("input %+v: err = %v, want ErrMasterDataValidation", in, err)
			}
		}
	})

	t.Run("cannot deactivate a type used by an unfinished request", func(t *testing.T) {
		repo := newFakeMasterDataRepository()
		svc := NewMasterDataService(repo)
		a, _ := svc.CreateAssetType(ctx, domain.SaveAssetTypeInput{Code: "AN", Name: "Android", Identifier: domain.IdentifierIMEI})
		repo.inUse[a.ID] = true
		_, err := svc.UpdateAssetType(ctx, a.ID, domain.SaveAssetTypeInput{Code: "AN", Name: "Android", Identifier: domain.IdentifierIMEI, IsActive: boolPtr(false)})
		if !errors.Is(err, ErrMasterDataInUse) {
			t.Fatalf("err = %v, want ErrMasterDataInUse", err)
		}
	})
}

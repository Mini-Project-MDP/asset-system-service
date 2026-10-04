package service

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// fakePhoneCatalogRepository is an in-memory domain.PhoneCatalogRepository.
type fakePhoneCatalogRepository struct {
	brands map[string]*domain.PhoneBrand
	models map[string]*domain.PhoneModel
	nextID int
}

func newFakePhoneCatalogRepository() *fakePhoneCatalogRepository {
	return &fakePhoneCatalogRepository{
		brands: map[string]*domain.PhoneBrand{},
		models: map[string]*domain.PhoneModel{},
	}
}

func (f *fakePhoneCatalogRepository) newID(prefix string) string {
	f.nextID++
	return prefix + strconv.Itoa(f.nextID)
}

func (f *fakePhoneCatalogRepository) brandWithModels(b *domain.PhoneBrand, includeInactive bool) domain.PhoneBrand {
	out := *b
	out.Models = nil
	for _, m := range f.models {
		if m.BrandID == b.ID && (m.IsActive || includeInactive) {
			out.Models = append(out.Models, *m)
		}
	}
	sort.Slice(out.Models, func(i, j int) bool { return out.Models[i].Name < out.Models[j].Name })
	return out
}

func (f *fakePhoneCatalogRepository) ListBrands(ctx context.Context, includeInactive bool) ([]domain.PhoneBrand, error) {
	var out []domain.PhoneBrand
	for _, b := range f.brands {
		if b.IsActive || includeInactive {
			out = append(out, f.brandWithModels(b, includeInactive))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *fakePhoneCatalogRepository) GetBrand(ctx context.Context, id string) (*domain.PhoneBrand, error) {
	b, ok := f.brands[id]
	if !ok {
		return nil, nil
	}
	out := f.brandWithModels(b, true)
	return &out, nil
}

func (f *fakePhoneCatalogRepository) brandNameTaken(name, exceptID string) bool {
	for id, b := range f.brands {
		if id != exceptID && strings.EqualFold(b.Name, name) {
			return true
		}
	}
	return false
}

func (f *fakePhoneCatalogRepository) CreateBrand(ctx context.Context, in domain.SavePhoneBrandInput, isActive bool) (string, error) {
	if f.brandNameTaken(in.Name, "") {
		return "", domain.ErrDuplicate
	}
	id := f.newID("brand")
	f.brands[id] = &domain.PhoneBrand{ID: id, Name: in.Name, IsActive: isActive}
	return id, nil
}

func (f *fakePhoneCatalogRepository) UpdateBrand(ctx context.Context, id string, in domain.SavePhoneBrandInput, isActive bool) error {
	if f.brandNameTaken(in.Name, id) {
		return domain.ErrDuplicate
	}
	f.brands[id] = &domain.PhoneBrand{ID: id, Name: in.Name, IsActive: isActive}
	return nil
}

func (f *fakePhoneCatalogRepository) GetModel(ctx context.Context, id string) (*domain.PhoneModel, error) {
	if m, ok := f.models[id]; ok {
		c := *m
		return &c, nil
	}
	return nil, nil
}

func (f *fakePhoneCatalogRepository) modelNameTaken(brandID, name, exceptID string) bool {
	for id, m := range f.models {
		if id != exceptID && m.BrandID == brandID && strings.EqualFold(m.Name, name) {
			return true
		}
	}
	return false
}

func (f *fakePhoneCatalogRepository) CreateModel(ctx context.Context, in domain.SavePhoneModelInput, isActive bool) (string, error) {
	if f.modelNameTaken(in.BrandID, in.Name, "") {
		return "", domain.ErrDuplicate
	}
	id := f.newID("model")
	f.models[id] = &domain.PhoneModel{ID: id, BrandID: in.BrandID, Name: in.Name, IsActive: isActive}
	return id, nil
}

func (f *fakePhoneCatalogRepository) UpdateModel(ctx context.Context, id string, in domain.SavePhoneModelInput, isActive bool) error {
	if f.modelNameTaken(in.BrandID, in.Name, id) {
		return domain.ErrDuplicate
	}
	f.models[id] = &domain.PhoneModel{ID: id, BrandID: in.BrandID, Name: in.Name, IsActive: isActive}
	return nil
}

func TestCreateBrand(t *testing.T) {
	ctx := context.Background()

	t.Run("creates an active brand with a trimmed name", func(t *testing.T) {
		svc := NewPhoneCatalogService(newFakePhoneCatalogRepository())
		got, err := svc.CreateBrand(ctx, domain.SavePhoneBrandInput{Name: "  Samsung "})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "Samsung" || !got.IsActive {
			t.Fatalf("got %+v, want active Samsung", got)
		}
	})

	t.Run("rejects an empty name", func(t *testing.T) {
		svc := NewPhoneCatalogService(newFakePhoneCatalogRepository())
		if _, err := svc.CreateBrand(ctx, domain.SavePhoneBrandInput{Name: "  "}); !errors.Is(err, ErrMasterDataValidation) {
			t.Fatalf("err = %v, want ErrMasterDataValidation", err)
		}
	})

	t.Run("duplicate name is a conflict regardless of letter case", func(t *testing.T) {
		svc := NewPhoneCatalogService(newFakePhoneCatalogRepository())
		svc.CreateBrand(ctx, domain.SavePhoneBrandInput{Name: "Samsung"})
		if _, err := svc.CreateBrand(ctx, domain.SavePhoneBrandInput{Name: "SAMSUNG"}); !errors.Is(err, ErrMasterDataConflict) {
			t.Fatalf("err = %v, want ErrMasterDataConflict", err)
		}
	})
}

func TestUpdateBrand(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown id is not found", func(t *testing.T) {
		svc := NewPhoneCatalogService(newFakePhoneCatalogRepository())
		if _, err := svc.UpdateBrand(ctx, "nope", domain.SavePhoneBrandInput{Name: "X"}); !errors.Is(err, ErrMasterDataNotFound) {
			t.Fatalf("err = %v, want ErrMasterDataNotFound", err)
		}
	})

	t.Run("renames and keeps state when IsActive is nil", func(t *testing.T) {
		svc := NewPhoneCatalogService(newFakePhoneCatalogRepository())
		off := false
		b, _ := svc.CreateBrand(ctx, domain.SavePhoneBrandInput{Name: "Oppo", IsActive: &off})
		got, err := svc.UpdateBrand(ctx, b.ID, domain.SavePhoneBrandInput{Name: "OPPO"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "OPPO" || got.IsActive {
			t.Fatalf("got %+v, want renamed and still inactive", got)
		}
	})

	t.Run("name taken by another brand is a conflict", func(t *testing.T) {
		svc := NewPhoneCatalogService(newFakePhoneCatalogRepository())
		svc.CreateBrand(ctx, domain.SavePhoneBrandInput{Name: "Vivo"})
		b, _ := svc.CreateBrand(ctx, domain.SavePhoneBrandInput{Name: "Realme"})
		if _, err := svc.UpdateBrand(ctx, b.ID, domain.SavePhoneBrandInput{Name: "vivo"}); !errors.Is(err, ErrMasterDataConflict) {
			t.Fatalf("err = %v, want ErrMasterDataConflict", err)
		}
	})
}

func TestCreateModel(t *testing.T) {
	ctx := context.Background()

	setup := func() (domain.PhoneCatalogService, *domain.PhoneBrand, *domain.PhoneBrand) {
		svc := NewPhoneCatalogService(newFakePhoneCatalogRepository())
		a, _ := svc.CreateBrand(ctx, domain.SavePhoneBrandInput{Name: "Samsung"})
		b, _ := svc.CreateBrand(ctx, domain.SavePhoneBrandInput{Name: "Oppo"})
		return svc, a, b
	}

	t.Run("creates an active model under a brand", func(t *testing.T) {
		svc, samsung, _ := setup()
		got, err := svc.CreateModel(ctx, domain.SavePhoneModelInput{BrandID: samsung.ID, Name: " Galaxy Tab "})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "Galaxy Tab" || got.BrandID != samsung.ID || !got.IsActive {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("rejects empty name, missing brand and unknown brand", func(t *testing.T) {
		svc, samsung, _ := setup()
		for _, in := range []domain.SavePhoneModelInput{
			{BrandID: samsung.ID, Name: " "},
			{Name: "Galaxy Tab"},
			{BrandID: "ghost", Name: "Galaxy Tab"},
		} {
			if _, err := svc.CreateModel(ctx, in); !errors.Is(err, ErrMasterDataValidation) {
				t.Errorf("input %+v: err = %v, want ErrMasterDataValidation", in, err)
			}
		}
	})

	t.Run("duplicate within a brand conflicts, same name under another brand is fine", func(t *testing.T) {
		svc, samsung, oppo := setup()
		svc.CreateModel(ctx, domain.SavePhoneModelInput{BrandID: samsung.ID, Name: "A-series"})
		if _, err := svc.CreateModel(ctx, domain.SavePhoneModelInput{BrandID: samsung.ID, Name: "a-SERIES"}); !errors.Is(err, ErrMasterDataConflict) {
			t.Fatalf("same brand err = %v, want ErrMasterDataConflict", err)
		}
		if _, err := svc.CreateModel(ctx, domain.SavePhoneModelInput{BrandID: oppo.ID, Name: "A-series"}); err != nil {
			t.Fatalf("other brand: unexpected error %v", err)
		}
	})
}

func TestUpdateModel(t *testing.T) {
	ctx := context.Background()

	setup := func() (domain.PhoneCatalogService, *domain.PhoneBrand, *domain.PhoneBrand, *domain.PhoneModel) {
		svc := NewPhoneCatalogService(newFakePhoneCatalogRepository())
		samsung, _ := svc.CreateBrand(ctx, domain.SavePhoneBrandInput{Name: "Samsung"})
		oppo, _ := svc.CreateBrand(ctx, domain.SavePhoneBrandInput{Name: "Oppo"})
		m, _ := svc.CreateModel(ctx, domain.SavePhoneModelInput{BrandID: samsung.ID, Name: "A-series"})
		return svc, samsung, oppo, m
	}

	t.Run("unknown id is not found", func(t *testing.T) {
		svc, samsung, _, _ := setup()
		if _, err := svc.UpdateModel(ctx, "nope", domain.SavePhoneModelInput{BrandID: samsung.ID, Name: "X"}); !errors.Is(err, ErrMasterDataNotFound) {
			t.Fatalf("err = %v, want ErrMasterDataNotFound", err)
		}
	})

	t.Run("renames, moves to another brand and deactivates", func(t *testing.T) {
		svc, _, oppo, m := setup()
		off := false
		got, err := svc.UpdateModel(ctx, m.ID, domain.SavePhoneModelInput{BrandID: oppo.ID, Name: "Reno", IsActive: &off})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.BrandID != oppo.ID || got.Name != "Reno" || got.IsActive {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("nil IsActive keeps the current state", func(t *testing.T) {
		svc, samsung, _, m := setup()
		got, err := svc.UpdateModel(ctx, m.ID, domain.SavePhoneModelInput{BrandID: samsung.ID, Name: "A-series 2"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.IsActive {
			t.Fatal("model should have stayed active")
		}
	})

	t.Run("unknown target brand is a validation error", func(t *testing.T) {
		svc, _, _, m := setup()
		if _, err := svc.UpdateModel(ctx, m.ID, domain.SavePhoneModelInput{BrandID: "ghost", Name: "X"}); !errors.Is(err, ErrMasterDataValidation) {
			t.Fatalf("err = %v, want ErrMasterDataValidation", err)
		}
	})

	t.Run("name clash inside the target brand is a conflict", func(t *testing.T) {
		svc, samsung, _, _ := setup()
		other, _ := svc.CreateModel(ctx, domain.SavePhoneModelInput{BrandID: samsung.ID, Name: "Galaxy Tab"})
		if _, err := svc.UpdateModel(ctx, other.ID, domain.SavePhoneModelInput{BrandID: samsung.ID, Name: "a-series"}); !errors.Is(err, ErrMasterDataConflict) {
			t.Fatalf("err = %v, want ErrMasterDataConflict", err)
		}
	})
}

func TestListCatalog(t *testing.T) {
	ctx := context.Background()
	svc := NewPhoneCatalogService(newFakePhoneCatalogRepository())
	off := false
	samsung, _ := svc.CreateBrand(ctx, domain.SavePhoneBrandInput{Name: "Samsung"})
	svc.CreateBrand(ctx, domain.SavePhoneBrandInput{Name: "Nokia", IsActive: &off})
	svc.CreateModel(ctx, domain.SavePhoneModelInput{BrandID: samsung.ID, Name: "Galaxy Tab"})
	svc.CreateModel(ctx, domain.SavePhoneModelInput{BrandID: samsung.ID, Name: "Retired", IsActive: &off})

	active, err := svc.ListCatalog(ctx, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) != 1 || active[0].Name != "Samsung" || len(active[0].Models) != 1 || active[0].Models[0].Name != "Galaxy Tab" {
		t.Fatalf("active catalog = %+v, want only Samsung/Galaxy Tab", active)
	}

	all, _ := svc.ListCatalog(ctx, true)
	if len(all) != 2 {
		t.Fatalf("full catalog has %d brands, want 2", len(all))
	}
}

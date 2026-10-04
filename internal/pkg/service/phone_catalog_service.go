package service

import (
	"context"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

type phoneCatalogService struct {
	repo domain.PhoneCatalogRepository
}

// NewPhoneCatalogService creates a domain.PhoneCatalogService. Errors reuse
// the ErrMasterData* sentinels: the catalog is master data like outlets.
func NewPhoneCatalogService(repo domain.PhoneCatalogRepository) domain.PhoneCatalogService {
	return &phoneCatalogService{repo: repo}
}

func (s *phoneCatalogService) ListCatalog(ctx context.Context, includeInactive bool) ([]domain.PhoneBrand, error) {
	return s.repo.ListBrands(ctx, includeInactive)
}

func (s *phoneCatalogService) CreateBrand(ctx context.Context, in domain.SavePhoneBrandInput) (*domain.PhoneBrand, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, validation("name is required")
	}
	id, err := s.repo.CreateBrand(ctx, in, in.IsActive == nil || *in.IsActive)
	if err != nil {
		return nil, mapRepoError(err, "phone brand name")
	}
	return s.repo.GetBrand(ctx, id)
}

func (s *phoneCatalogService) UpdateBrand(ctx context.Context, id string, in domain.SavePhoneBrandInput) (*domain.PhoneBrand, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, validation("name is required")
	}
	current, err := s.repo.GetBrand(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrMasterDataNotFound
	}
	isActive := current.IsActive
	if in.IsActive != nil {
		isActive = *in.IsActive
	}
	if err := s.repo.UpdateBrand(ctx, id, in, isActive); err != nil {
		return nil, mapRepoError(err, "phone brand name")
	}
	return s.repo.GetBrand(ctx, id)
}

// checkModelInput trims the name and confirms the target brand exists.
func (s *phoneCatalogService) checkModelInput(ctx context.Context, in domain.SavePhoneModelInput) (domain.SavePhoneModelInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.BrandID = strings.TrimSpace(in.BrandID)
	if in.Name == "" {
		return in, validation("name is required")
	}
	if in.BrandID == "" {
		return in, validation("brand_id is required")
	}
	brand, err := s.repo.GetBrand(ctx, in.BrandID)
	if err != nil {
		return in, err
	}
	if brand == nil {
		return in, validation("unknown brand %q", in.BrandID)
	}
	return in, nil
}

func (s *phoneCatalogService) CreateModel(ctx context.Context, in domain.SavePhoneModelInput) (*domain.PhoneModel, error) {
	in, err := s.checkModelInput(ctx, in)
	if err != nil {
		return nil, err
	}
	id, err := s.repo.CreateModel(ctx, in, in.IsActive == nil || *in.IsActive)
	if err != nil {
		return nil, mapRepoError(err, "phone model name in this brand")
	}
	return s.repo.GetModel(ctx, id)
}

func (s *phoneCatalogService) UpdateModel(ctx context.Context, id string, in domain.SavePhoneModelInput) (*domain.PhoneModel, error) {
	current, err := s.repo.GetModel(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrMasterDataNotFound
	}
	in, err = s.checkModelInput(ctx, in)
	if err != nil {
		return nil, err
	}
	isActive := current.IsActive
	if in.IsActive != nil {
		isActive = *in.IsActive
	}
	if err := s.repo.UpdateModel(ctx, id, in, isActive); err != nil {
		return nil, mapRepoError(err, "phone model name in this brand")
	}
	return s.repo.GetModel(ctx, id)
}

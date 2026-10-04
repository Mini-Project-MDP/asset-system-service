package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// Errors returned by masterDataService. Handlers map them to HTTP statuses.
var (
	ErrMasterDataValidation = errors.New("invalid master data")
	ErrMasterDataConflict   = errors.New("master data conflict")
	ErrMasterDataNotFound   = errors.New("master data not found")
	ErrMasterDataInUse      = errors.New("cannot deactivate")
)

type masterDataService struct {
	repo domain.MasterDataRepository
}

// NewMasterDataService creates a domain.MasterDataService.
func NewMasterDataService(repo domain.MasterDataRepository) domain.MasterDataService {
	return &masterDataService{repo: repo}
}

func validation(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrMasterDataValidation, fmt.Sprintf(format, args...))
}

// mapRepoError turns a repository uniqueness violation into ErrMasterDataConflict.
func mapRepoError(err error, what string) error {
	if errors.Is(err, domain.ErrDuplicate) {
		return fmt.Errorf("%w: %s already exists", ErrMasterDataConflict, what)
	}
	return err
}

// resolveActive returns the is_active value to persist: the requested one, or
// the current one when the caller left it unset. Deactivating something that
// an unfinished request still references is refused.
func resolveActive(requested *bool, current bool, inUse func() (bool, error), what string) (bool, error) {
	if requested == nil {
		return current, nil
	}
	if current && !*requested {
		used, err := inUse()
		if err != nil {
			return false, err
		}
		if used {
			return false, fmt.Errorf("%w: %s", ErrMasterDataInUse, what)
		}
	}
	return *requested, nil
}

// --- Outlets ---

func (s *masterDataService) ListOutlets(ctx context.Context, includeInactive bool) ([]domain.Outlet, error) {
	return s.repo.ListOutlets(ctx, includeInactive)
}

func normalizeOutlet(in domain.SaveOutletInput) (domain.SaveOutletInput, error) {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.Region = strings.TrimSpace(in.Region)
	switch {
	case in.Code == "":
		return in, validation("code is required")
	case in.Name == "":
		return in, validation("name is required")
	case in.Region == "":
		return in, validation("region is required")
	}
	return in, nil
}

func (s *masterDataService) CreateOutlet(ctx context.Context, in domain.SaveOutletInput) (*domain.Outlet, error) {
	in, err := normalizeOutlet(in)
	if err != nil {
		return nil, err
	}
	isActive := in.IsActive == nil || *in.IsActive
	id, err := s.repo.CreateOutlet(ctx, in, isActive)
	if err != nil {
		return nil, mapRepoError(err, "outlet code")
	}
	return s.repo.GetOutlet(ctx, id)
}

func (s *masterDataService) UpdateOutlet(ctx context.Context, id string, in domain.SaveOutletInput) (*domain.Outlet, error) {
	in, err := normalizeOutlet(in)
	if err != nil {
		return nil, err
	}
	current, err := s.repo.GetOutlet(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrMasterDataNotFound
	}
	isActive, err := resolveActive(in.IsActive, current.IsActive,
		func() (bool, error) { return s.repo.OutletInUse(ctx, id) }, "outlet is used by an unfinished request")
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateOutlet(ctx, id, in, isActive); err != nil {
		return nil, mapRepoError(err, "outlet code")
	}
	return s.repo.GetOutlet(ctx, id)
}

// --- Distributors ---

func (s *masterDataService) ListDistributors(ctx context.Context, includeInactive bool) ([]domain.Distributor, error) {
	return s.repo.ListDistributors(ctx, includeInactive)
}

// checkOutletMapping verifies the outlets exist, appear once, and are not
// already served by another distributor.
func (s *masterDataService) checkOutletMapping(ctx context.Context, outletIDs []string, distributorID string) error {
	seen := make(map[string]bool, len(outletIDs))
	for _, id := range outletIDs {
		if strings.TrimSpace(id) == "" {
			return validation("outlet_ids must not contain empty ids")
		}
		if seen[id] {
			return validation("outlet %s is listed more than once", id)
		}
		seen[id] = true
	}
	if len(outletIDs) == 0 {
		return nil
	}
	missing, err := s.repo.MissingOutlets(ctx, outletIDs)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return validation("unknown outlet ids: %s", strings.Join(missing, ", "))
	}
	taken, err := s.repo.OutletsMappedElsewhere(ctx, outletIDs, distributorID)
	if err != nil {
		return err
	}
	if len(taken) > 0 {
		return fmt.Errorf("%w: outlets already served by another distributor: %s", ErrMasterDataConflict, strings.Join(taken, ", "))
	}
	return nil
}

func normalizeDistributor(in domain.SaveDistributorInput) (domain.SaveDistributorInput, error) {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Code == "":
		return in, validation("code is required")
	case in.Name == "":
		return in, validation("name is required")
	}
	return in, nil
}

func (s *masterDataService) CreateDistributor(ctx context.Context, in domain.SaveDistributorInput) (*domain.Distributor, error) {
	in, err := normalizeDistributor(in)
	if err != nil {
		return nil, err
	}
	if err := s.checkOutletMapping(ctx, in.OutletIDs, ""); err != nil {
		return nil, err
	}
	isActive := in.IsActive == nil || *in.IsActive
	id, err := s.repo.CreateDistributor(ctx, in, isActive)
	if err != nil {
		return nil, mapRepoError(err, "distributor code or outlet mapping")
	}
	return s.repo.GetDistributor(ctx, id)
}

func (s *masterDataService) UpdateDistributor(ctx context.Context, id string, in domain.SaveDistributorInput) (*domain.Distributor, error) {
	in, err := normalizeDistributor(in)
	if err != nil {
		return nil, err
	}
	current, err := s.repo.GetDistributor(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrMasterDataNotFound
	}
	if err := s.checkOutletMapping(ctx, in.OutletIDs, id); err != nil {
		return nil, err
	}
	isActive, err := resolveActive(in.IsActive, current.IsActive,
		func() (bool, error) { return s.repo.DistributorInUse(ctx, id) }, "distributor is used by an unfinished request")
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateDistributor(ctx, id, in, isActive); err != nil {
		return nil, mapRepoError(err, "distributor code or outlet mapping")
	}
	return s.repo.GetDistributor(ctx, id)
}

// --- Asset types ---

func (s *masterDataService) ListAssetTypes(ctx context.Context, includeInactive bool) ([]domain.AssetType, error) {
	return s.repo.ListAssetTypes(ctx, includeInactive)
}

func normalizeAssetType(in domain.SaveAssetTypeInput) (domain.SaveAssetTypeInput, error) {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.Identifier = strings.ToUpper(strings.TrimSpace(in.Identifier))
	switch {
	case in.Code == "":
		return in, validation("code is required")
	case in.Name == "":
		return in, validation("name is required")
	}
	switch in.Identifier {
	case domain.IdentifierNone, domain.IdentifierIMEI, domain.IdentifierSerial:
	default:
		return in, validation("identifier must be one of %s, %s, %s", domain.IdentifierNone, domain.IdentifierIMEI, domain.IdentifierSerial)
	}
	return in, nil
}

func (s *masterDataService) CreateAssetType(ctx context.Context, in domain.SaveAssetTypeInput) (*domain.AssetType, error) {
	in, err := normalizeAssetType(in)
	if err != nil {
		return nil, err
	}
	isActive := in.IsActive == nil || *in.IsActive
	id, err := s.repo.CreateAssetType(ctx, in, isActive)
	if err != nil {
		return nil, mapRepoError(err, "asset type code")
	}
	return s.repo.GetAssetType(ctx, id)
}

func (s *masterDataService) UpdateAssetType(ctx context.Context, id string, in domain.SaveAssetTypeInput) (*domain.AssetType, error) {
	in, err := normalizeAssetType(in)
	if err != nil {
		return nil, err
	}
	current, err := s.repo.GetAssetType(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrMasterDataNotFound
	}
	isActive, err := resolveActive(in.IsActive, current.IsActive,
		func() (bool, error) { return s.repo.AssetTypeInUse(ctx, id) }, "asset type is used by an unfinished request")
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateAssetType(ctx, id, in, isActive); err != nil {
		return nil, mapRepoError(err, "asset type code")
	}
	return s.repo.GetAssetType(ctx, id)
}

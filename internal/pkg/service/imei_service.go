package service

import (
	"context"
	"errors"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// ErrInvalidIMEI: the value is not a plausible IMEI (14-16 digits).
var ErrInvalidIMEI = errors.New("invalid IMEI")

// tacLength is the number of leading IMEI digits that identify the device model.
const tacLength = 8

type imeiService struct {
	repo domain.ImeiReferenceRepository
}

// NewImeiService creates a domain.ImeiService.
func NewImeiService(repo domain.ImeiReferenceRepository) domain.ImeiService {
	return &imeiService{repo: repo}
}

func (s *imeiService) Lookup(ctx context.Context, imei string) (*domain.ImeiInfo, error) {
	imei = strings.TrimSpace(imei)
	if !isDigits(imei) || len(imei) < imeiMinLen || len(imei) > imeiMaxLen {
		return nil, ErrInvalidIMEI
	}
	return s.repo.LookupTAC(ctx, imei[:tacLength])
}

package service

import (
	"context"
	"sync"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// RoleLister is the part of the auth repository the badges need.
type RoleLister interface {
	GetUserRoles(ctx context.Context, userID string) ([]domain.RoleDto, error)
}

// adminRoleCodeForBadges is the Admin role: its Approvals queue holds every
// request waiting for anyone's approval, not just its own turn.
const adminRoleCodeForBadges = "MASTER_ADMIN"

type navigationService struct {
	repo  domain.NavigationRepository
	roles RoleLister
}

// NewNavigationService creates a domain.NavigationService.
func NewNavigationService(repo domain.NavigationRepository, roles RoleLister) domain.NavigationService {
	return &navigationService{repo: repo, roles: roles}
}

func hasPermission(caller domain.NavigationCaller, code string) bool {
	if caller.IsMaster {
		return true
	}
	for _, p := range caller.Permissions {
		if p == code {
			return true
		}
	}
	return false
}

// Badges counts what the caller's sidebar shows. A module the caller cannot
// open gets no counter (and no query), and the counters are read together, so
// the cost is one round trip to the database, not two.
//
// The Approvals count is read from this service's own copy of each request's
// approval steps: it counts requests whose current step belongs to the
// caller's role. The Approval Engine remains the authority on whose turn it is;
// this is an approximation of it that costs no call to the engine.
func (s *navigationService) Badges(ctx context.Context, caller domain.NavigationCaller) (domain.NavigationBadges, error) {
	var (
		badges      domain.NavigationBadges
		approvalErr error
		fulfillErr  error
		wg          sync.WaitGroup
	)

	if hasPermission(caller, "fulfillment:read") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := s.repo.CountInProgress(ctx)
			if err != nil {
				fulfillErr = err
				return
			}
			badges.Fulfillment = &n
		}()
	}
	if hasPermission(caller, "approvals:read") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := s.countApprovals(ctx, caller)
			if err != nil {
				approvalErr = err
				return
			}
			badges.Approvals = &n
		}()
	}
	wg.Wait()

	if approvalErr != nil {
		return domain.NavigationBadges{}, approvalErr
	}
	if fulfillErr != nil {
		return domain.NavigationBadges{}, fulfillErr
	}
	return badges, nil
}

func (s *navigationService) countApprovals(ctx context.Context, caller domain.NavigationCaller) (int, error) {
	if caller.IsMaster {
		return s.repo.CountWaitingApproval(ctx)
	}
	if caller.UserID == "" {
		return 0, nil // no known identity: no turn can be theirs
	}
	roles, err := s.roles.GetUserRoles(ctx, caller.UserID)
	if err != nil {
		return 0, err
	}
	var codes, names []string
	for _, r := range roles {
		if r.Code == adminRoleCodeForBadges {
			return s.repo.CountWaitingApproval(ctx)
		}
		codes = append(codes, r.Code)
		names = append(names, r.Name)
	}
	if len(codes) == 0 {
		return 0, nil
	}
	return s.repo.CountWaitingForRoles(ctx, codes, names)
}

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

type fakeNavigationRepo struct {
	inProgress   int
	waitingAll   int
	waitingRoles int
	err          error

	gotCodes, gotNames []string
	calls              []string
}

func (f *fakeNavigationRepo) CountInProgress(ctx context.Context) (int, error) {
	f.calls = append(f.calls, "inProgress")
	return f.inProgress, f.err
}

func (f *fakeNavigationRepo) CountWaitingApproval(ctx context.Context) (int, error) {
	f.calls = append(f.calls, "waitingAll")
	return f.waitingAll, f.err
}

func (f *fakeNavigationRepo) CountWaitingForRoles(ctx context.Context, codes, names []string) (int, error) {
	f.calls = append(f.calls, "waitingForRoles")
	f.gotCodes, f.gotNames = codes, names
	return f.waitingRoles, f.err
}

type fakeRoleLister struct {
	roles map[string][]domain.RoleDto
	err   error
}

func (f fakeRoleLister) GetUserRoles(ctx context.Context, userID string) ([]domain.RoleDto, error) {
	return f.roles[userID], f.err
}

func val(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestNavigationBadges(t *testing.T) {
	ctx := context.Background()
	roles := fakeRoleLister{roles: map[string][]domain.RoleDto{
		"u-ss":    {{Code: "SS", Name: "Sales Supervisor"}},
		"u-admin": {{Code: "MASTER_ADMIN", Name: "Admin"}},
		"u-none":  {},
	}}
	newRepo := func() *fakeNavigationRepo {
		return &fakeNavigationRepo{inProgress: 5, waitingAll: 14, waitingRoles: 2}
	}

	t.Run("a master user sees both, counting every waiting request", func(t *testing.T) {
		repo := newRepo()
		got, err := NewNavigationService(repo, roles).Badges(ctx, domain.NavigationCaller{IsMaster: true, UserID: "u-x"})
		if err != nil || val(got.Approvals) != 14 || val(got.Fulfillment) != 5 {
			t.Fatalf("got approvals=%v fulfillment=%v err=%v", val(got.Approvals), val(got.Fulfillment), err)
		}
	})

	t.Run("an Admin role is counted like a master user", func(t *testing.T) {
		repo := newRepo()
		got, _ := NewNavigationService(repo, roles).Badges(ctx, domain.NavigationCaller{
			UserID: "u-admin", Permissions: []string{"approvals:read", "fulfillment:read"},
		})
		if val(got.Approvals) != 14 {
			t.Fatalf("approvals = %v, want all 14 waiting", val(got.Approvals))
		}
	})

	t.Run("Asset Team sees Fulfillment only", func(t *testing.T) {
		repo := newRepo()
		got, _ := NewNavigationService(repo, roles).Badges(ctx, domain.NavigationCaller{
			UserID: "u-at", Permissions: []string{"fulfillment:read", "dashboard:read"},
		})
		if got.Approvals != nil || val(got.Fulfillment) != 5 {
			t.Fatalf("got approvals=%v fulfillment=%v, want nil and 5", val(got.Approvals), val(got.Fulfillment))
		}
		for _, c := range repo.calls {
			if c != "inProgress" {
				t.Errorf("unexpected query %q for a caller without approvals access", c)
			}
		}
	})

	t.Run("an approver counts only what waits for their role", func(t *testing.T) {
		repo := newRepo()
		got, _ := NewNavigationService(repo, roles).Badges(ctx, domain.NavigationCaller{
			UserID: "u-ss", Permissions: []string{"approvals:read"},
		})
		if val(got.Approvals) != 2 || got.Fulfillment != nil {
			t.Fatalf("got approvals=%v fulfillment=%v, want 2 and nil", val(got.Approvals), val(got.Fulfillment))
		}
		if len(repo.gotCodes) != 1 || repo.gotCodes[0] != "SS" || len(repo.gotNames) != 1 || repo.gotNames[0] != "Sales Supervisor" {
			t.Fatalf("asked about roles %v / %v, want [SS] / [Sales Supervisor]", repo.gotCodes, repo.gotNames)
		}
	})

	t.Run("an approver whose identity or roles are unknown has nothing waiting", func(t *testing.T) {
		for _, uid := range []string{"", "u-none", "u-unknown"} {
			repo := newRepo()
			got, err := NewNavigationService(repo, roles).Badges(ctx, domain.NavigationCaller{UserID: uid, Permissions: []string{"approvals:read"}})
			if err != nil || val(got.Approvals) != 0 {
				t.Errorf("user %q: approvals = %v, err %v; want 0", uid, val(got.Approvals), err)
			}
		}
	})

	t.Run("a role with neither module gets no counters and no queries", func(t *testing.T) {
		repo := newRepo()
		got, err := NewNavigationService(repo, roles).Badges(ctx, domain.NavigationCaller{UserID: "u-sa", Permissions: []string{"request:read", "request:create"}})
		if err != nil || got.Approvals != nil || got.Fulfillment != nil || len(repo.calls) != 0 {
			t.Fatalf("got %+v, err %v, queries %v", got, err, repo.calls)
		}
	})

	t.Run("failures are returned", func(t *testing.T) {
		boom := errors.New("database unavailable")
		repo := newRepo()
		repo.err = boom
		if _, err := NewNavigationService(repo, roles).Badges(ctx, domain.NavigationCaller{IsMaster: true, UserID: "u"}); !errors.Is(err, boom) {
			t.Errorf("count failure: err = %v", err)
		}
		if _, err := NewNavigationService(newRepo(), fakeRoleLister{err: boom}).Badges(ctx, domain.NavigationCaller{UserID: "u-ss", Permissions: []string{"approvals:read"}}); !errors.Is(err, boom) {
			t.Errorf("role lookup failure: err = %v", err)
		}
	})
}

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/jwt"
)

// fakePermissionRepo implements only what the resolver calls; the embedded nil
// interface makes any other call panic, which keeps the test honest.
type fakePermissionRepo struct {
	domain.AuthRepository
	mu          sync.Mutex
	users       map[string]*domain.User // lookup key (id, email or employee_no) -> user
	permissions map[string][]string     // user id -> permission codes
	lookups     int
	permCalls   int
	err         error
}

func (f *fakePermissionRepo) GetUserByID(ctx context.Context, key string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups++
	if f.err != nil {
		return nil, f.err
	}
	return f.users[key], nil
}

func (f *fakePermissionRepo) GetUserPermissions(ctx context.Context, userID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.permCalls++
	return f.permissions[userID], f.err
}

func newPermissionRepo() *fakePermissionRepo {
	manager := &domain.User{ID: "usr_mgr1", EmployeeNo: "EMP003", Email: "manager1@mayora.com", Status: "ACTIVE"}
	return &fakePermissionRepo{
		users: map[string]*domain.User{
			"usr_mgr1": manager, "manager1@mayora.com": manager, "EMP003": manager,
			"usr_off":        {ID: "usr_off", Email: "off@mayora.com", Status: "INACTIVE"},
			"usr_all":        {ID: "usr_all", Email: "all@mayora.com", Status: "ACTIVE"},
			"all@mayora.com": {ID: "usr_all", Email: "all@mayora.com", Status: "ACTIVE"},
			"usr_own":        {ID: "usr_own", Email: "own@mayora.com", Status: "ACTIVE"},
		},
		permissions: map[string][]string{
			"usr_mgr1": {"request:read", "fulfillment:read", "dashboard:read"},
			"usr_off":  {"request:read", "request:read_all"},
			"usr_all":  {"request:read", "request:read_all"},
			"usr_own":  {"request:read"},
		},
	}
}

func TestPermissionResolverFindsTheUser(t *testing.T) {
	ctx := context.Background()
	want := []string{"request:read", "fulfillment:read", "dashboard:read"}

	cases := map[string]*jwt.UserClaims{
		"by user id": {UserID: "usr_mgr1"},
		"by email after the SSO user id is unknown":       {UserID: "EXT-EMP003", Email: "manager1@mayora.com"},
		"by employee_no only when the token has no email": {UserID: "EXT-EMP003", EmployeeNo: "EMP003"},
	}
	for name, claims := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := NewPermissionResolver(newPermissionRepo(), time.Minute).PermissionsFor(ctx, claims)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}

func TestPermissionResolverDenies(t *testing.T) {
	ctx := context.Background()

	t.Run("an email that matches nobody is not rescued by the employee number", func(t *testing.T) {
		// Employee numbers are not unique across systems (the SSO's EMP002 can be a
		// different person here). Falling back to one would grant someone else's
		// permissions, so an unmatched email must end in a denial.
		got, err := NewPermissionResolver(newPermissionRepo(), time.Minute).
			PermissionsFor(ctx, &jwt.UserClaims{UserID: "EXT-EMP003", Email: "nobody@mayora.com", EmployeeNo: "EMP003"})
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, %v; want no permissions and no error", got, err)
		}
	})

	t.Run("an unknown user has no permissions", func(t *testing.T) {
		got, err := NewPermissionResolver(newPermissionRepo(), time.Minute).
			PermissionsFor(ctx, &jwt.UserClaims{UserID: "EXT-9", Email: "ghost@mayora.com", EmployeeNo: "EMP999"})
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, %v; want no permissions and no error", got, err)
		}
	})

	t.Run("a deactivated user has no permissions even with a valid token", func(t *testing.T) {
		got, err := NewPermissionResolver(newPermissionRepo(), time.Minute).
			PermissionsFor(ctx, &jwt.UserClaims{UserID: "usr_off"})
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, %v; want no permissions and no error", got, err)
		}
	})

	t.Run("a database error is returned, not turned into a denial", func(t *testing.T) {
		repo := newPermissionRepo()
		repo.err = errors.New("database unavailable")
		if _, err := NewPermissionResolver(repo, time.Minute).PermissionsFor(ctx, &jwt.UserClaims{UserID: "usr_mgr1"}); err == nil {
			t.Fatal("expected the database error")
		}
	})
}

func TestPermissionResolverCaching(t *testing.T) {
	ctx := context.Background()
	claims := &jwt.UserClaims{UserID: "usr_mgr1"}
	clock := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	t.Run("the database is asked once within the TTL, again after it", func(t *testing.T) {
		repo := newPermissionRepo()
		resolver := NewPermissionResolver(repo, time.Minute)
		resolver.now = func() time.Time { return clock }

		resolver.PermissionsFor(ctx, claims)
		resolver.PermissionsFor(ctx, claims)
		if repo.permCalls != 1 {
			t.Fatalf("permissions read %d times within the TTL, want 1", repo.permCalls)
		}

		clock = clock.Add(61 * time.Second)
		resolver.PermissionsFor(ctx, claims)
		if repo.permCalls != 2 {
			t.Fatalf("permissions read %d times after the TTL, want 2", repo.permCalls)
		}
	})

	t.Run("a role change shows up once the cached entry expires", func(t *testing.T) {
		repo := newPermissionRepo()
		resolver := NewPermissionResolver(repo, time.Minute)
		resolver.now = func() time.Time { return clock }
		before, _ := resolver.PermissionsFor(ctx, claims)

		repo.permissions["usr_mgr1"] = []string{"request:read"}
		clock = clock.Add(2 * time.Minute)
		after, _ := resolver.PermissionsFor(ctx, claims)
		if len(before) != 3 || len(after) != 1 {
			t.Fatalf("before %v, after %v; want 3 then 1 permissions", before, after)
		}
	})

	t.Run("errors are not cached", func(t *testing.T) {
		repo := newPermissionRepo()
		resolver := NewPermissionResolver(repo, time.Minute)
		repo.err = errors.New("blip")
		resolver.PermissionsFor(ctx, claims)
		repo.err = nil
		got, err := resolver.PermissionsFor(ctx, claims)
		if err != nil || len(got) != 3 {
			t.Fatalf("after recovery got %v, %v; want the 3 permissions", got, err)
		}
	})

	t.Run("different users do not share entries", func(t *testing.T) {
		repo := newPermissionRepo()
		resolver := NewPermissionResolver(repo, time.Minute)
		a, _ := resolver.PermissionsFor(ctx, claims)
		b, _ := resolver.PermissionsFor(ctx, &jwt.UserClaims{UserID: "usr_off"})
		if len(a) != 3 || len(b) != 0 {
			t.Fatalf("a = %v, b = %v; want 3 and 0", a, b)
		}
	})
}

func TestPermissionResolverViewer(t *testing.T) {
	ctx := context.Background()
	newResolver := func() (*PermissionResolver, *fakePermissionRepo) {
		repo := newPermissionRepo()
		return NewPermissionResolver(repo, time.Minute), repo
	}

	t.Run("a holder of request:read_all can read everything", func(t *testing.T) {
		r, _ := newResolver()
		got, err := r.ViewerFor(ctx, &jwt.UserClaims{UserID: "EXT-9", Email: "all@mayora.com"})
		if err != nil || got != (domain.Viewer{UserID: "usr_all", CanReadAll: true}) {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	t.Run("without it the viewer is limited to their own requests", func(t *testing.T) {
		r, _ := newResolver()
		got, _ := r.ViewerFor(ctx, &jwt.UserClaims{UserID: "usr_own"})
		if got != (domain.Viewer{UserID: "usr_own", CanReadAll: false}) {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("a master user can read everything and still has an identity", func(t *testing.T) {
		r, _ := newResolver()
		got, _ := r.ViewerFor(ctx, &jwt.UserClaims{UserID: "usr_own", IsMaster: true})
		if got != (domain.Viewer{UserID: "usr_own", CanReadAll: true}) {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("an unknown user has no identity and cannot read everything", func(t *testing.T) {
		r, _ := newResolver()
		got, err := r.ViewerFor(ctx, &jwt.UserClaims{UserID: "EXT-9", Email: "ghost@mayora.com"})
		if err != nil || got != (domain.Viewer{}) {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	t.Run("a deactivated user is not trusted, even holding request:read_all", func(t *testing.T) {
		r, _ := newResolver()
		got, _ := r.ViewerFor(ctx, &jwt.UserClaims{UserID: "usr_off"})
		if got != (domain.Viewer{}) {
			t.Fatalf("got %+v, want nobody", got)
		}
	})

	t.Run("permissions and viewer share one lookup", func(t *testing.T) {
		r, repo := newResolver()
		claims := &jwt.UserClaims{UserID: "usr_own"}
		r.PermissionsFor(ctx, claims)
		r.ViewerFor(ctx, claims)
		if repo.permCalls != 1 {
			t.Fatalf("permissions read %d times, want 1", repo.permCalls)
		}
	})

	t.Run("a database error is returned", func(t *testing.T) {
		r, repo := newResolver()
		repo.err = errors.New("database unavailable")
		if _, err := r.ViewerFor(ctx, &jwt.UserClaims{UserID: "usr_own"}); err == nil {
			t.Fatal("expected the database error")
		}
	})
}

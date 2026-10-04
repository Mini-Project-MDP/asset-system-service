package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/jwt"
	"github.com/gofiber/fiber/v3"
)

type stubResolver struct {
	perms []string
	err   error
	calls int
}

func (s *stubResolver) PermissionsFor(ctx context.Context, claims *jwt.UserClaims) ([]string, error) {
	s.calls++
	return s.perms, s.err
}

// guardedStatus runs one request through a route guarded by Require("dashboard:read")
// for a caller holding the given token claims.
func guardedStatus(t *testing.T, resolver PermissionResolver, claims *jwt.UserClaims) int {
	t.Helper()
	app := fiber.New()
	app.Get("/x", func(c fiber.Ctx) error {
		if claims != nil {
			c.Locals(UserContextKey, claims)
		}
		return c.Next()
	}, NewPermissionGuard(resolver).Require("dashboard:read"), func(c fiber.Ctx) error { return c.SendStatus(http.StatusOK) })
	res, err := app.Test(httptest.NewRequest(http.MethodGet, "/x", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return res.StatusCode
}

func TestPermissionGuard(t *testing.T) {
	// A token carrying the SSO vocabulary: it must not decide anything once a resolver exists.
	ssoToken := &jwt.UserClaims{UserID: "EXT-EMP002", Permissions: []string{"asset:read", "asset:approve"}}

	t.Run("allows what the database grants, whatever the token says", func(t *testing.T) {
		if got := guardedStatus(t, &stubResolver{perms: []string{"dashboard:read"}}, ssoToken); got != http.StatusOK {
			t.Fatalf("status = %d, want 200", got)
		}
	})

	t.Run("denies what the database does not grant, even if the token lists it", func(t *testing.T) {
		stale := &jwt.UserClaims{UserID: "u", Permissions: []string{"dashboard:read"}}
		if got := guardedStatus(t, &stubResolver{perms: []string{"request:read"}}, stale); got != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", got)
		}
	})

	t.Run("a master user passes without asking the database", func(t *testing.T) {
		r := &stubResolver{}
		if got := guardedStatus(t, r, &jwt.UserClaims{IsMaster: true}); got != http.StatusOK || r.calls != 0 {
			t.Fatalf("status = %d, resolver calls = %d; want 200 and 0", got, r.calls)
		}
	})

	t.Run("a resolver failure is a server error, never a pass", func(t *testing.T) {
		if got := guardedStatus(t, &stubResolver{err: errors.New("db down")}, ssoToken); got != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", got)
		}
	})

	t.Run("no claims is unauthorized", func(t *testing.T) {
		if got := guardedStatus(t, &stubResolver{perms: []string{"dashboard:read"}}, nil); got != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", got)
		}
	})

	t.Run("without a resolver the token's own permissions decide (previous behaviour)", func(t *testing.T) {
		if got := guardedStatus(t, nil, &jwt.UserClaims{Permissions: []string{"dashboard:read"}}); got != http.StatusOK {
			t.Errorf("token with the permission: status = %d, want 200", got)
		}
		if got := guardedStatus(t, nil, &jwt.UserClaims{Permissions: []string{"asset:read"}}); got != http.StatusForbidden {
			t.Errorf("token without it: status = %d, want 403", got)
		}
	})
}

package service

import (
	"context"
	"sync"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/jwt"
)

// maxCachedPermissionEntries bounds the cache; expired entries are dropped when it is reached.
const maxCachedPermissionEntries = 1000

// readAllRequestsPermission lets a caller see every request, not just their own.
const readAllRequestsPermission = "request:read_all"

// resolvedUser is what is known about a caller after looking them up.
type resolvedUser struct {
	userID      string // this system's user id; empty if the caller matches no active user
	permissions []string
	expires     time.Time
}

// PermissionResolver answers "what may this caller do?" from the database
// (users, roles and permissions), not from the token.
//
// Why: a token only identifies the caller. SSO tokens carry permissions in the
// SSO's own vocabulary (e.g. "asset:read"), which this API does not recognise,
// and a database-issued token freezes the permissions as of login. Reading the
// database makes the role matrix the single source of truth, applies a role
// change within the cache TTL, and denies a deactivated user at once.
//
// Results are cached per caller for ttl to avoid a database round trip on every request.
type PermissionResolver struct {
	repo domain.AuthRepository
	ttl  time.Duration
	now  func() time.Time

	mu    sync.Mutex
	cache map[string]resolvedUser
}

// NewPermissionResolver creates a resolver that caches each caller's permissions for ttl.
func NewPermissionResolver(repo domain.AuthRepository, ttl time.Duration) *PermissionResolver {
	return &PermissionResolver{repo: repo, ttl: ttl, now: time.Now, cache: map[string]resolvedUser{}}
}

// PermissionsFor returns the permission codes of the user the token belongs to.
// The user is found by token user id, then by email (an SSO token's user id is
// not this system's id, its email is). The employee number is used only when
// the token has no email: employee numbers are not unique across systems, so
// falling back to one after an email mismatch could hand someone else's
// permissions to the caller. An unknown or deactivated user has no permissions.
// A database error is returned, so the caller can refuse the request rather
// than guess.
func (r *PermissionResolver) PermissionsFor(ctx context.Context, claims *jwt.UserClaims) ([]string, error) {
	resolved, err := r.resolve(ctx, claims)
	if err != nil {
		return nil, err
	}
	return resolved.permissions, nil
}

// ViewerFor says who the caller is and how much they may see: the user id in
// this system (empty when the token matches no active user), and whether they
// can read every request. Master users can; so can holders of request:read_all.
func (r *PermissionResolver) ViewerFor(ctx context.Context, claims *jwt.UserClaims) (domain.Viewer, error) {
	resolved, err := r.resolve(ctx, claims)
	if err != nil {
		return domain.Viewer{}, err
	}
	viewer := domain.Viewer{UserID: resolved.userID, CanReadAll: claims.IsMaster}
	for _, p := range resolved.permissions {
		if p == readAllRequestsPermission {
			viewer.CanReadAll = true
		}
	}
	return viewer, nil
}

// resolve looks the caller up, or returns the cached result.
func (r *PermissionResolver) resolve(ctx context.Context, claims *jwt.UserClaims) (resolvedUser, error) {
	key := claims.UserID + "|" + claims.Email + "|" + claims.EmployeeNo
	now := r.now()

	r.mu.Lock()
	if hit, ok := r.cache[key]; ok && now.Before(hit.expires) {
		r.mu.Unlock()
		return hit, nil
	}
	r.mu.Unlock()

	resolved, err := r.load(ctx, claims)
	if err != nil {
		return resolvedUser{}, err
	}
	resolved.expires = now.Add(r.ttl)

	r.mu.Lock()
	if len(r.cache) >= maxCachedPermissionEntries {
		for k, v := range r.cache {
			if !now.Before(v.expires) {
				delete(r.cache, k)
			}
		}
	}
	r.cache[key] = resolved
	r.mu.Unlock()
	return resolved, nil
}

func (r *PermissionResolver) load(ctx context.Context, claims *jwt.UserClaims) (resolvedUser, error) {
	var user *domain.User
	identifiers := []string{claims.UserID, claims.Email}
	if claims.Email == "" {
		identifiers = append(identifiers, claims.EmployeeNo)
	}
	for _, identifier := range identifiers {
		if identifier == "" {
			continue
		}
		found, err := r.repo.GetUserByID(ctx, identifier)
		if err != nil {
			return resolvedUser{}, err
		}
		if found != nil {
			user = found
			break
		}
	}
	if user == nil || user.Status != domain.UserStatusActive {
		return resolvedUser{permissions: []string{}}, nil
	}
	permissions, err := r.repo.GetUserPermissions(ctx, user.ID)
	if err != nil {
		return resolvedUser{}, err
	}
	if permissions == nil {
		permissions = []string{}
	}
	return resolvedUser{userID: user.ID, permissions: permissions}, nil
}

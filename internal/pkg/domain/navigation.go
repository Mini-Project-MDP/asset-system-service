package domain

import "context"

// NavigationBadges are the counters on the sidebar. A nil counter means the
// caller has no access to that module, so there is nothing to count.
type NavigationBadges struct {
	// Approvals: requests waiting for a decision from the caller's role.
	Approvals *int
	// Fulfillment: requests ready to be processed or being processed by the Asset Team.
	Fulfillment *int
}

// NavigationCaller is who asks for the badges.
type NavigationCaller struct {
	UserID      string // this system's user id; empty if unknown
	Permissions []string
	IsMaster    bool
}

// NavigationRepository counts what the sidebar badges show.
type NavigationRepository interface {
	// CountInProgress counts approved requests and those in fulfillment.
	CountInProgress(ctx context.Context) (int, error)
	// CountWaitingApproval counts every request waiting for approval.
	CountWaitingApproval(ctx context.Context) (int, error)
	// CountWaitingForRoles counts requests waiting for approval whose current
	// step belongs to one of the given roles (matched by role code or name).
	CountWaitingForRoles(ctx context.Context, roleCodes, roleNames []string) (int, error)
}

// NavigationService builds the sidebar badges for a caller.
type NavigationService interface {
	Badges(ctx context.Context, caller NavigationCaller) (NavigationBadges, error)
}

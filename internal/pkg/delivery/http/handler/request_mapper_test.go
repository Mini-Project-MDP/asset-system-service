package handler

import (
	"testing"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

func TestComputeStatusTagRevisionNamesTheRole(t *testing.T) {
	chain := []domain.ApprovalStepItem{
		{Role: "SS", RoleLabel: "Sales Supervisor", Status: "approved"},
		{Role: "RSM", RoleLabel: "Regional Sales Manager", Status: "revision"},
		{Role: "GRSM", RoleLabel: "Group Regional Sales Manager", Status: "pending"},
	}
	revisionHistory := []domain.ApprovalHistoryItem{{Action: "Requested revision"}}

	cases := []struct {
		name string
		req  domain.AssetRequest
		want string
		cls  string
	}{
		{"revision status names the role that asked", domain.AssetRequest{Status: domain.RequestStatusRevision, Chain: chain}, "Revision — Regional Sales Manager", "warn"},
		{"a rejected request that was sent back for revision names the role too", domain.AssetRequest{Status: domain.RequestStatusRejected, Chain: chain, Hist: revisionHistory}, "Revision — Regional Sales Manager", "warn"},
		{"revision without a revision step stays plain", domain.AssetRequest{Status: domain.RequestStatusRevision}, "Revision", "warn"},
		{"a plain rejection is still Rejected", domain.AssetRequest{Status: domain.RequestStatusRejected, Chain: chain}, "Rejected", "stop"},
		{"waiting shows the current step", domain.AssetRequest{Status: domain.RequestStatusWaitingApproval, CurrentStepName: strPtr("Sales Supervisor")}, "Waiting — Sales Supervisor", "warn"},
		{"completed", domain.AssetRequest{Status: domain.RequestStatusCompleted}, "Completed", "go"},
	}
	for _, c := range cases {
		got := computeStatusTag(c.req)
		if got["text"] != c.want || got["cls"] != c.cls {
			t.Errorf("%s: got %v, want %q (%s)", c.name, got, c.want, c.cls)
		}
	}
}

func strPtr(s string) *string { return &s }

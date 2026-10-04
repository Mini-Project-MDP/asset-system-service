package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

func listFixture() *fakeRequestRepository {
	return &fakeRequestRepository{items: []domain.AssetRequest{
		{ID: "REQ-1", Category: "Barcode", Status: domain.RequestStatusWaitingApproval, CreatedBy: "u-laras", RequesterID: "u-laras"},
		{ID: "REQ-2", Category: "Android", Status: domain.RequestStatusApproved, CreatedBy: "u-dimas", RequesterID: "u-dimas"},
		{ID: "REQ-3", Category: "Server", Status: domain.RequestStatusFulfillment, CreatedBy: "u-laras", RequesterID: "u-laras"},
		{ID: "REQ-4", Category: "Barcode", Status: domain.RequestStatusCompleted, CreatedBy: "u-dimas", RequesterID: "u-dimas"},
		{ID: "REQ-5", Category: "Barcode", Status: domain.RequestStatusRejected, CreatedBy: "u-dimas", RequesterID: "u-dimas"},
		{ID: "REQ-6", Category: "Android", Status: domain.RequestStatusRevision, CreatedBy: "", RequesterID: "u-laras"}, // older row: no submitter recorded
	}}
}

func ids(items []domain.AssetRequest) []string {
	out := make([]string, 0, len(items))
	for _, r := range items {
		out = append(out, r.ID)
	}
	return out
}

func TestListStatusFilter(t *testing.T) {
	ctx := context.Background()
	svc := NewRequestService(listFixture(), &fakeApprovalEngineClient{})

	cases := []struct {
		status string
		want   []string
	}{
		{"", []string{"REQ-1", "REQ-2", "REQ-3", "REQ-4", "REQ-5", "REQ-6"}},
		{"All status", []string{"REQ-1", "REQ-2", "REQ-3", "REQ-4", "REQ-5", "REQ-6"}},
		{"Waiting", []string{"REQ-1"}},
		{"In progress", []string{"REQ-2", "REQ-3"}}, // approved (waiting to be processed) and in fulfillment
		{"Completed", []string{"REQ-4"}},
		{"Rejected", []string{"REQ-5"}},
		{"Revision", []string{"REQ-6"}},
		{"in PROGRESS", []string{"REQ-2", "REQ-3"}}, // case and surrounding space do not matter
		{"  waiting ", []string{"REQ-1"}},
	}
	for _, c := range cases {
		got, err := svc.List(ctx, domain.RequestFilter{Status: c.status})
		if err != nil {
			t.Errorf("status %q: unexpected error %v", c.status, err)
			continue
		}
		if g := ids(got); len(g) != len(c.want) || (len(g) > 0 && g[0] != c.want[0]) || (len(g) > 1 && g[len(g)-1] != c.want[len(c.want)-1]) {
			t.Errorf("status %q: got %v, want %v", c.status, g, c.want)
		}
	}

	t.Run("an unknown status is rejected", func(t *testing.T) {
		if _, err := svc.List(ctx, domain.RequestFilter{Status: "Archived"}); !errors.Is(err, ErrInvalidRequestFilter) {
			t.Fatalf("err = %v, want ErrInvalidRequestFilter", err)
		}
	})

	t.Run("filters combine", func(t *testing.T) {
		got, err := svc.List(ctx, domain.RequestFilter{Type: "Barcode", Status: "Completed"})
		if err != nil || len(got) != 1 || got[0].ID != "REQ-4" {
			t.Fatalf("got %v, %v; want only REQ-4", ids(got), err)
		}
		got, _ = svc.List(ctx, domain.RequestFilter{Type: "Android", Status: "Waiting"})
		if len(got) != 0 {
			t.Fatalf("got %v, want nothing", ids(got))
		}
	})

	t.Run("the repository receives stored status values, not UI labels", func(t *testing.T) {
		repo := listFixture()
		NewRequestService(repo, &fakeApprovalEngineClient{}).List(ctx, domain.RequestFilter{Status: "In progress"})
		got := repo.lastListQuery.Statuses
		if len(got) != 2 || got[0] != domain.RequestStatusApproved || got[1] != domain.RequestStatusFulfillment {
			t.Fatalf("statuses sent to the repository = %v", got)
		}
	})
}

func TestListVisibility(t *testing.T) {
	ctx := context.Background()
	svc := NewRequestService(listFixture(), &fakeApprovalEngineClient{})

	t.Run("a user sees what they submitted or are the requester of", func(t *testing.T) {
		got, _ := svc.List(ctx, domain.RequestFilter{VisibleToUser: "u-laras"})
		want := map[string]bool{"REQ-1": true, "REQ-3": true, "REQ-6": true} // REQ-6 has no submitter but names her as requester
		if len(got) != 3 {
			t.Fatalf("got %v, want REQ-1, REQ-3, REQ-6", ids(got))
		}
		for _, r := range got {
			if !want[r.ID] {
				t.Fatalf("unexpected %s in %v", r.ID, ids(got))
			}
		}
	})

	t.Run("visibility combines with the other filters", func(t *testing.T) {
		got, _ := svc.List(ctx, domain.RequestFilter{VisibleToUser: "u-dimas", Status: "Completed"})
		if len(got) != 1 || got[0].ID != "REQ-4" {
			t.Fatalf("got %v, want only REQ-4", ids(got))
		}
	})

	t.Run("no visibility restriction returns everything", func(t *testing.T) {
		got, _ := svc.List(ctx, domain.RequestFilter{})
		if len(got) != 6 {
			t.Fatalf("got %d, want 6", len(got))
		}
	})
}

func TestDetailFor(t *testing.T) {
	ctx := context.Background()
	svc := NewRequestService(listFixture(), &fakeApprovalEngineClient{})

	t.Run("a viewer who can read everything sees any request", func(t *testing.T) {
		got, err := svc.DetailFor(ctx, "REQ-4", domain.Viewer{UserID: "u-admin", CanReadAll: true})
		if err != nil || got.ID != "REQ-4" {
			t.Fatalf("got %v, %v", got, err)
		}
	})

	t.Run("the submitter and the named requester see it", func(t *testing.T) {
		if _, err := svc.DetailFor(ctx, "REQ-1", domain.Viewer{UserID: "u-laras"}); err != nil {
			t.Errorf("submitter: %v", err)
		}
		if _, err := svc.DetailFor(ctx, "REQ-6", domain.Viewer{UserID: "u-laras"}); err != nil {
			t.Errorf("named requester of an older request: %v", err)
		}
	})

	t.Run("someone else's request looks like it does not exist", func(t *testing.T) {
		if _, err := svc.DetailFor(ctx, "REQ-2", domain.Viewer{UserID: "u-laras"}); !errors.Is(err, ErrRequestNotFound) {
			t.Fatalf("err = %v, want ErrRequestNotFound", err)
		}
	})

	t.Run("a caller with no known identity sees nothing", func(t *testing.T) {
		if _, err := svc.DetailFor(ctx, "REQ-6", domain.Viewer{}); !errors.Is(err, ErrRequestNotFound) {
			t.Fatalf("err = %v, want ErrRequestNotFound", err)
		}
	})

	t.Run("a missing request is not found for everyone", func(t *testing.T) {
		if _, err := svc.DetailFor(ctx, "REQ-404", domain.Viewer{CanReadAll: true}); !errors.Is(err, ErrRequestNotFound) {
			t.Fatalf("err = %v, want ErrRequestNotFound", err)
		}
	})
}

func TestCreateRecordsTheSubmitter(t *testing.T) {
	repo := &fakeRequestRepository{
		nextID:     "REQ-NEW",
		requesters: map[string]domain.RequesterInfo{"Laras P.": {UserID: "u-laras", EmployeeNo: "E1"}},
	}
	svc := NewRequestService(repo, &fakeApprovalEngineClient{})
	in := validInput()
	in.Qty, in.CreatedBy = 1, "u-admin"
	_, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var created domain.AssetRequest
	for _, r := range repo.items {
		if r.ID == "REQ-NEW" {
			created = r
		}
	}
	if created.CreatedBy != "u-admin" || created.RequesterID != "u-laras" {
		t.Fatalf("created = %+v, want submitter u-admin and requester u-laras", created)
	}
}

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// Runs the sidebar badge SQL against a real PostgreSQL; skipped unless
// TEST_DATABASE_URL points at a disposable database (see openTestDB).
func TestNavigationCountsSQL(t *testing.T) {
	db := openTestDB(t)
	repo := NewNavigationRepository(db)
	ctx := context.Background()

	counts := func() (inProgress, waiting, forSS, forRSM int) {
		t.Helper()
		var err error
		if inProgress, err = repo.CountInProgress(ctx); err != nil {
			t.Fatalf("in progress: %v", err)
		}
		if waiting, err = repo.CountWaitingApproval(ctx); err != nil {
			t.Fatalf("waiting: %v", err)
		}
		if forSS, err = repo.CountWaitingForRoles(ctx, []string{"SS"}, []string{"Sales Supervisor"}); err != nil {
			t.Fatalf("for SS: %v", err)
		}
		if forRSM, err = repo.CountWaitingForRoles(ctx, []string{"RSM"}, []string{"Regional Sales Manager"}); err != nil {
			t.Fatalf("for RSM: %v", err)
		}
		return
	}
	beforeProgress, beforeWaiting, beforeSS, beforeRSM := counts()

	now := time.Now()
	insertDashboardRequest(t, db, "ITEST-N1", domain.RequestStatusWaitingApproval, "normal", "atype_barcode", 1, nil, now)
	insertDashboardRequest(t, db, "ITEST-N2", domain.RequestStatusWaitingApproval, "normal", "atype_barcode", 1, nil, now)
	insertDashboardRequest(t, db, "ITEST-N3", domain.RequestStatusWaitingApproval, "normal", "atype_barcode", 1, nil, now)
	insertDashboardRequest(t, db, "ITEST-N4", domain.RequestStatusApproved, "normal", "atype_barcode", 1, 0, now)
	insertDashboardRequest(t, db, "ITEST-N5", domain.RequestStatusFulfillment, "normal", "atype_barcode", 1, 1, now)
	insertDashboardRequest(t, db, "ITEST-N6", domain.RequestStatusCompleted, "normal", "atype_barcode", 1, 3, now)
	insertDashboardRequest(t, db, "ITEST-N7", domain.RequestStatusRejected, "normal", "atype_barcode", 1, nil, now)

	step := func(id, request string, order int, code, label, status string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO request_approval_steps (id, request_id, step_order, role_code, role_label, status) VALUES ($1,$2,$3,$4,$5,$6)`,
			id, request, order, code, label, status); err != nil {
			t.Fatalf("insert step %s: %v", id, err)
		}
	}
	// N1: the Sales Supervisor's turn (the engine stores the step name in both columns).
	step("ITEST-NS1", "ITEST-N1", 0, "Sales Supervisor", "Sales Supervisor", "current")
	step("ITEST-NS2", "ITEST-N1", 1, "Regional Sales Manager", "Regional Sales Manager", "pending")
	// N2: the RSM's turn, stored as a code.
	step("ITEST-NS3", "ITEST-N2", 0, "SS", "Sales Supervisor", "approved")
	step("ITEST-NS4", "ITEST-N2", 1, "RSM", "Regional Sales Manager", "current")
	// N3: the SS already approved, the RSM's turn.
	step("ITEST-NS5", "ITEST-N3", 0, "SS", "Sales Supervisor", "approved")
	step("ITEST-NS6", "ITEST-N3", 1, "RSM", "Regional Sales Manager", "current")
	// N7 is rejected: a leftover "current" step must not make it count.
	step("ITEST-NS7", "ITEST-N7", 0, "SS", "Sales Supervisor", "current")

	progress, waiting, forSS, forRSM := counts()
	if d := progress - beforeProgress; d != 2 {
		t.Errorf("in progress grew by %d, want 2 (N4 approved, N5 in fulfillment)", d)
	}
	if d := waiting - beforeWaiting; d != 3 {
		t.Errorf("waiting grew by %d, want 3 (N1, N2, N3)", d)
	}
	if d := forSS - beforeSS; d != 1 {
		t.Errorf("waiting for SS grew by %d, want 1 (N1 only: N7 is rejected, and N2/N3 are past SS)", d)
	}
	if d := forRSM - beforeRSM; d != 2 {
		t.Errorf("waiting for RSM grew by %d, want 2 (N2, N3)", d)
	}

	if n, err := repo.CountWaitingForRoles(ctx, []string{"SS", "RSM"}, []string{"Sales Supervisor", "Regional Sales Manager"}); err != nil || n-(beforeSS+beforeRSM) != 3 {
		t.Errorf("for both roles = %d (err %v), want a request counted once even if several names match it", n, err)
	}
	if n, err := repo.CountWaitingForRoles(ctx, nil, nil); err != nil || n != 0 {
		t.Errorf("for no roles = %d (err %v), want 0", n, err)
	}
}

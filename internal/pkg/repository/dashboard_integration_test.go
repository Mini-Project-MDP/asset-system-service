package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// Runs the dashboard SQL against a real PostgreSQL; skipped unless
// TEST_DATABASE_URL points at a disposable database (see openTestDB).

var wibZone = time.FixedZone("WIB", 7*3600)

// insertDashboardRequest inserts a request with explicit status, priority,
// category, quantity, step and creation time.
func insertDashboardRequest(t *testing.T, db *sql.DB, id, status, priority, typeID string, qty int, step any, createdAt time.Time) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO asset_requests
		(id, requester_id, asset_type_id, sales_division, quantity, priority, status, fulfillment_step, created_at)
		VALUES ($1, 'usr_master1', $2, 'M1', $3, $4, $5, $6, $7)`,
		id, typeID, qty, priority, status, step, createdAt)
	if err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

func TestDashboardStatsSQL(t *testing.T) {
	db := openTestDB(t)
	repo := NewDashboardRepository(db)
	ctx := context.Background()

	// A month far from any real data, so the "this month" counter can be asserted exactly.
	monthStart := time.Date(2031, 5, 1, 0, 0, 0, 0, wibZone)
	monthEnd := monthStart.AddDate(0, 1, 0)

	before, err := repo.Stats(ctx, monthStart, monthEnd)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}

	inMonth := monthStart.Add(36 * time.Hour)
	insertDashboardRequest(t, db, "ITEST-D1", domain.RequestStatusWaitingApproval, "normal", "atype_barcode", 3, nil, inMonth)
	insertDashboardRequest(t, db, "ITEST-D2", domain.RequestStatusApproved, "normal", "atype_barcode", 5, 0, inMonth)                       // Processing: in progress, data not recorded
	insertDashboardRequest(t, db, "ITEST-D3", domain.RequestStatusFulfillment, "normal", "atype_android", 7, 1, monthStart.Add(-time.Hour)) // Shipped, last month
	insertDashboardRequest(t, db, "ITEST-D4", domain.RequestStatusCompleted, "normal", "atype_server", 11, 3, inMonth)
	insertDashboardRequest(t, db, "ITEST-D5", domain.RequestStatusRejected, "normal", "atype_server", 100, nil, inMonth)
	insertDashboardRequest(t, db, "ITEST-D6", domain.RequestStatusFulfillment, "normal", "atype_android", 13, 0, inMonth) // Processing in FULFILLMENT: in progress, data not recorded

	after, err := repo.Stats(ctx, monthStart, monthEnd)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if d := after.RequestsThisMonth - before.RequestsThisMonth; d != 5 {
		t.Errorf("requests this month grew by %d, want 5 (D3 was created last month)", d)
	}
	if d := after.PendingApproval - before.PendingApproval; d != 1 {
		t.Errorf("pending approval grew by %d, want 1", d)
	}
	if d := after.InProgress - before.InProgress; d != 3 {
		t.Errorf("in progress grew by %d, want 3 (D2, D3, D6)", d)
	}
	// Registered = data recorded: D3 (7, Shipped) + D4 (11, Completed). D2/D6 still at Processing, D5 rejected.
	if d := after.AssetsRegistered - before.AssetsRegistered; d != 18 {
		t.Errorf("assets registered grew by %d, want 18", d)
	}
}

func TestDashboardCategoryMonthCountsSQL(t *testing.T) {
	db := openTestDB(t)
	repo := NewDashboardRepository(db)
	ctx := context.Background()

	// 2031-03-31 18:00 UTC is 2031-04-01 01:00 WIB: April, not March.
	insertDashboardRequest(t, db, "ITEST-C1", domain.RequestStatusWaitingApproval, "normal", "atype_barcode", 1, nil, time.Date(2031, 3, 31, 18, 0, 0, 0, time.UTC))
	insertDashboardRequest(t, db, "ITEST-C2", domain.RequestStatusWaitingApproval, "normal", "atype_barcode", 1, nil, time.Date(2031, 4, 15, 3, 0, 0, 0, wibZone))
	insertDashboardRequest(t, db, "ITEST-C3", domain.RequestStatusWaitingApproval, "normal", "atype_android", 1, nil, time.Date(2031, 4, 20, 3, 0, 0, 0, wibZone))
	insertDashboardRequest(t, db, "ITEST-C4", domain.RequestStatusWaitingApproval, "normal", "atype_server", 1, nil, time.Date(2031, 12, 31, 23, 0, 0, 0, wibZone))
	insertDashboardRequest(t, db, "ITEST-C5", domain.RequestStatusWaitingApproval, "normal", "atype_barcode", 1, nil, time.Date(2032, 1, 1, 0, 30, 0, 0, wibZone)) // next year

	counts, err := repo.CategoryMonthCounts(ctx, time.Date(2031, 1, 1, 0, 0, 0, 0, wibZone), time.Date(2032, 1, 1, 0, 0, 0, 0, wibZone))
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	got := map[[2]any]int{}
	for _, c := range counts {
		got[[2]any{c.Month, c.Category}] = c.Count
	}
	want := map[[2]any]int{
		{4, "Barcode"}: 2, // C1 (by WIB) and C2
		{4, "Android"}: 1,
		{12, "Server"}: 1,
	}
	if len(got) != len(want) {
		t.Fatalf("counts = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%v = %d, want %d (all: %v)", k, got[k], v, got)
		}
	}
}

func TestDashboardRecentActivitySQL(t *testing.T) {
	db := openTestDB(t)
	repo := NewDashboardRepository(db)
	ctx := context.Background()

	insertDashboardRequest(t, db, "ITEST-H1", domain.RequestStatusWaitingApproval, "normal", "atype_barcode", 1, nil, time.Now())
	// EMP001 is the seeded Master Admin One; EMP-NOBODY matches no user.
	for i, h := range []struct{ role, action string }{{"EMP-NOBODY", "Approved"}, {"EMP001", "Submitted"}} {
		_, err := db.Exec(`INSERT INTO request_history (id, request_id, role, action, created_at)
			VALUES ($1, 'ITEST-H1', $2, $3, $4)`,
			"ITEST-HIST-"+string(rune('A'+i)), h.role, h.action, time.Now().Add(time.Duration(i+1)*time.Hour+24*365*time.Hour))
		if err != nil {
			t.Fatalf("insert history: %v", err)
		}
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM request_history WHERE id LIKE 'ITEST-HIST-%'`) })

	got, err := repo.RecentActivity(ctx, 2)
	if err != nil {
		t.Fatalf("activity: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2 (the limit)", len(got))
	}
	if got[0].Action != "Submitted" || got[0].Actor != "Master Admin One" {
		t.Errorf("newest = %+v, want Submitted by the resolved name Master Admin One", got[0])
	}
	if got[1].Action != "Approved" || got[1].Actor != "EMP-NOBODY" {
		t.Errorf("second = %+v, want Approved by the raw value EMP-NOBODY", got[1])
	}
}

func TestDashboardAttentionRequestsSQL(t *testing.T) {
	db := openTestDB(t)
	repo := NewDashboardRepository(db)
	ctx := context.Background()

	base := time.Date(2031, 6, 1, 0, 0, 0, 0, time.UTC)
	insertDashboardRequest(t, db, "ITEST-W1", domain.RequestStatusWaitingApproval, "normal", "atype_barcode", 1, nil, base)
	insertDashboardRequest(t, db, "ITEST-W2", domain.RequestStatusWaitingApproval, "high", "atype_barcode", 1, nil, base.Add(2*time.Hour))
	insertDashboardRequest(t, db, "ITEST-W3", domain.RequestStatusWaitingApproval, "URGENT", "atype_android", 1, nil, base.Add(3*time.Hour))
	insertDashboardRequest(t, db, "ITEST-W4", domain.RequestStatusWaitingApproval, "urgent", "atype_server", 1, nil, base.Add(time.Hour))
	insertDashboardRequest(t, db, "ITEST-W5", domain.RequestStatusApproved, "urgent", "atype_server", 1, 0, base) // not waiting: must not appear
	for i, s := range []struct{ code, label, status string }{{"SS", "Sales Supervisor", "current"}, {"RSM", "Regional Sales Manager", "pending"}} {
		_, err := db.Exec(`INSERT INTO request_approval_steps (id, request_id, step_order, role_code, role_label, status)
			VALUES ($1, 'ITEST-W4', $2, $3, $4, $5)`, "ITEST-STEP-"+string(rune('A'+i)), i, s.code, s.label, s.status)
		if err != nil {
			t.Fatalf("insert step: %v", err)
		}
	}

	items, err := repo.AttentionRequests(ctx, 100)
	if err != nil {
		t.Fatalf("attention: %v", err)
	}
	var order []string
	byID := map[string]domain.AssetRequest{}
	for _, r := range items {
		if len(r.ID) > 6 && r.ID[:6] == "ITEST-" {
			order = append(order, r.ID)
			byID[r.ID] = r
		}
	}
	// urgent oldest first (W4 then W3), then high (W2), then normal (W1). W5 is excluded.
	want := []string{"ITEST-W4", "ITEST-W3", "ITEST-W2", "ITEST-W1"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
	chain := byID["ITEST-W4"].Chain
	if len(chain) != 2 || chain[0].Role != "SS" || chain[0].Status != "current" || chain[1].Role != "RSM" {
		t.Errorf("chain of W4 = %+v, want SS (current) then RSM", chain)
	}
	if len(byID["ITEST-W1"].Chain) != 0 {
		t.Errorf("W1 has no steps, chain = %+v", byID["ITEST-W1"].Chain)
	}

	limited, err := repo.AttentionRequests(ctx, 2)
	if err != nil || len(limited) != 2 {
		t.Fatalf("limit 2: got %d rows, err %v", len(limited), err)
	}
}

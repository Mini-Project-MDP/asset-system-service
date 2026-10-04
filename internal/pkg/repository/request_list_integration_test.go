package repository

import (
	"context"
	"database/sql"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// Runs the request list SQL against a real PostgreSQL; skipped unless
// TEST_DATABASE_URL points at a disposable database (see openTestDB).

// insertListRequest inserts a request with the columns the list filters on.
// createdAt orders the rows; requesterID/createdBy decide who can see it.
func insertListRequest(t *testing.T, db *sql.DB, id, typeID, outletID, status, requesterID string, createdBy any, createdAt time.Time) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO asset_requests
		(id, requester_id, asset_type_id, outlet_id, sales_division, quantity, priority, status, created_by, created_at)
		VALUES ($1, $2, $3, $4, 'M1', 1, 'normal', $5, $6, $7)`,
		id, requesterID, typeID, outletID, status, createdBy, createdAt)
	if err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

func listIDs(items []domain.AssetRequest) []string {
	out := make([]string, 0, len(items))
	for _, r := range items {
		if len(r.ID) >= 6 && r.ID[:6] == "ITEST-" { // ignore any other data in the database
			out = append(out, r.ID)
		}
	}
	return out
}

func TestRequestListSQL(t *testing.T) {
	db := openTestDB(t)
	repo := NewRequestRepository(db)
	ctx := context.Background()

	if _, err := db.Exec(`INSERT INTO outlets (id, code, name) VALUES ('ITEST-OUT1', 'ITEST-O1', 'ITEST Bandung Kota'), ('ITEST-OUT2', 'ITEST-O2', 'ITEST 50% Outlet_X')`); err != nil {
		t.Fatalf("insert outlets: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM outlets WHERE id LIKE 'ITEST-OUT%'`) })

	base := time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)
	// requester usr_mgr1 = "Manager Asset One"; usr_staff1 = "Field Staff One"
	insertListRequest(t, db, "ITEST-L1", "atype_barcode", "ITEST-OUT1", domain.RequestStatusWaitingApproval, "usr_mgr1", "usr_mgr1", base.Add(1*time.Hour))
	insertListRequest(t, db, "ITEST-L2", "atype_android", "ITEST-OUT1", domain.RequestStatusApproved, "usr_staff1", "usr_master1", base.Add(2*time.Hour))
	insertListRequest(t, db, "ITEST-L3", "atype_barcode", "ITEST-OUT2", domain.RequestStatusFulfillment, "usr_staff1", nil, base.Add(3*time.Hour)) // older row: no submitter
	insertListRequest(t, db, "ITEST-L4", "atype_server", "ITEST-OUT2", domain.RequestStatusCompleted, "usr_mgr1", "usr_mgr1", base.Add(4*time.Hour))
	insertListRequest(t, db, "ITEST-L5", "atype_android", "ITEST-OUT1", domain.RequestStatusRejected, "usr_mgr1", "usr_master1", base.Add(5*time.Hour))

	list := func(q domain.RequestListQuery) []string {
		t.Helper()
		items, err := repo.List(ctx, q)
		if err != nil {
			t.Fatalf("List(%+v): %v", q, err)
		}
		return listIDs(items)
	}
	sameSet := func(name string, got, want []string) {
		t.Helper()
		g, w := append([]string{}, got...), append([]string{}, want...)
		sort.Strings(g)
		sort.Strings(w)
		if !reflect.DeepEqual(g, w) {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}

	t.Run("newest first", func(t *testing.T) {
		got := list(domain.RequestListQuery{})
		want := []string{"ITEST-L5", "ITEST-L4", "ITEST-L3", "ITEST-L2", "ITEST-L1"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})

	t.Run("search matches id, outlet and requester name, ignoring case", func(t *testing.T) {
		sameSet("by id", list(domain.RequestListQuery{Search: "itest-l3"}), []string{"ITEST-L3"})
		sameSet("by outlet", list(domain.RequestListQuery{Search: "bandung"}), []string{"ITEST-L1", "ITEST-L2", "ITEST-L5"})
		sameSet("by requester name", list(domain.RequestListQuery{Search: "field staff"}), []string{"ITEST-L2", "ITEST-L3"})
	})

	t.Run("% and _ in the search text are literal", func(t *testing.T) {
		sameSet("percent", list(domain.RequestListQuery{Search: "50%"}), []string{"ITEST-L3", "ITEST-L4"})
		sameSet("underscore", list(domain.RequestListQuery{Search: "Outlet_X"}), []string{"ITEST-L3", "ITEST-L4"})
		sameSet("a lone percent is not a wildcard for everything", list(domain.RequestListQuery{Search: "ITEST-L%"}), []string{})
		sameSet("a lone underscore is not a wildcard", list(domain.RequestListQuery{Search: "ITEST-L_"}), []string{})
	})

	t.Run("type and status filters", func(t *testing.T) {
		sameSet("barcode", list(domain.RequestListQuery{Type: "Barcode"}), []string{"ITEST-L1", "ITEST-L3"})
		sameSet("in progress", list(domain.RequestListQuery{Statuses: []string{domain.RequestStatusApproved, domain.RequestStatusFulfillment}}), []string{"ITEST-L2", "ITEST-L3"})
		sameSet("nil statuses means all", list(domain.RequestListQuery{Statuses: nil}), []string{"ITEST-L1", "ITEST-L2", "ITEST-L3", "ITEST-L4", "ITEST-L5"})
		sameSet("combined", list(domain.RequestListQuery{Type: "Android", Statuses: []string{domain.RequestStatusRejected}}), []string{"ITEST-L5"})
	})

	t.Run("visibility: what a user submitted or is the requester of", func(t *testing.T) {
		// usr_mgr1: submitted L1 and L4; requester of L1, L4, L5 (L5 was submitted by an admin on their behalf)
		sameSet("usr_mgr1", list(domain.RequestListQuery{VisibleToUser: "usr_mgr1"}), []string{"ITEST-L1", "ITEST-L4", "ITEST-L5"})
		// usr_master1: submitted L2 and L5; requester of none
		sameSet("usr_master1", list(domain.RequestListQuery{VisibleToUser: "usr_master1"}), []string{"ITEST-L2", "ITEST-L5"})
		// usr_staff1: requester of L2 and L3 (L3 has no submitter recorded)
		sameSet("usr_staff1", list(domain.RequestListQuery{VisibleToUser: "usr_staff1"}), []string{"ITEST-L2", "ITEST-L3"})
		sameSet("everything combines", list(domain.RequestListQuery{VisibleToUser: "usr_mgr1", Type: "Android"}), []string{"ITEST-L5"})
		sameSet("a stranger sees nothing", list(domain.RequestListQuery{VisibleToUser: "usr_nobody"}), []string{})
	})

	t.Run("each row carries only its own chain and history", func(t *testing.T) {
		for _, step := range []struct{ id, req, code, label, status string }{
			{"ITEST-ST1", "ITEST-L1", "SS", "Sales Supervisor", "current"},
			{"ITEST-ST2", "ITEST-L1", "RSM", "Regional Sales Manager", "pending"},
			{"ITEST-ST3", "ITEST-L2", "SS", "Sales Supervisor", "approved"},
		} {
			order := 0
			if step.code == "RSM" {
				order = 1
			}
			if _, err := db.Exec(`INSERT INTO request_approval_steps (id, request_id, step_order, role_code, role_label, status) VALUES ($1,$2,$3,$4,$5,$6)`,
				step.id, step.req, order, step.code, step.label, step.status); err != nil {
				t.Fatalf("insert step: %v", err)
			}
		}
		if _, err := db.Exec(`INSERT INTO request_history (id, request_id, role, action) VALUES ('ITEST-HI1', 'ITEST-L1', 'EMP003', 'Submitted')`); err != nil {
			t.Fatalf("insert history: %v", err)
		}

		items, err := repo.List(ctx, domain.RequestListQuery{Search: "ITEST-L"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		byID := map[string]domain.AssetRequest{}
		for _, r := range items {
			byID[r.ID] = r
		}
		l1, l2, l4 := byID["ITEST-L1"], byID["ITEST-L2"], byID["ITEST-L4"]
		if len(l1.Chain) != 2 || l1.Chain[0].Role != "SS" || l1.Chain[1].Role != "RSM" {
			t.Errorf("L1 chain = %+v, want SS then RSM in order", l1.Chain)
		}
		if len(l2.Chain) != 1 || l2.Chain[0].Status != "approved" {
			t.Errorf("L2 chain = %+v, want one approved step", l2.Chain)
		}
		if l4.Chain == nil || len(l4.Chain) != 0 || l4.Hist == nil || len(l4.Hist) != 0 {
			t.Errorf("L4 chain/history = %+v / %+v, want empty non-nil slices", l4.Chain, l4.Hist)
		}
		if len(l1.Hist) != 1 || l1.Hist[0].Action != "Submitted" || l1.Hist[0].Role != "EMP003" {
			t.Errorf("L1 history = %+v", l1.Hist)
		}
		if l1.CreatedBy != "usr_mgr1" || l1.RequesterID != "usr_mgr1" || byID["ITEST-L3"].CreatedBy != "" {
			t.Errorf("owner fields: L1 created_by=%q requester=%q, L3 created_by=%q", l1.CreatedBy, l1.RequesterID, byID["ITEST-L3"].CreatedBy)
		}
	})
}

func TestCreateStoresTheSubmitterSQL(t *testing.T) {
	db := openTestDB(t)
	repo := NewRequestRepository(db)
	ctx := context.Background()

	create := func(createdBy string) (id string, stored sql.NullString) {
		t.Helper()
		id, err := repo.Create(ctx, "usr_mgr1", domain.CreateRequestInput{
			Category: "Barcode", SalesDivision: "M1", Qty: 1, Priority: "normal", CreatedBy: createdBy,
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM asset_requests WHERE id = $1`, id) })
		if err := db.QueryRow(`SELECT created_by FROM asset_requests WHERE id = $1`, id).Scan(&stored); err != nil {
			t.Fatalf("read back: %v", err)
		}
		return id, stored
	}

	if _, stored := create("usr_master1"); !stored.Valid || stored.String != "usr_master1" {
		t.Errorf("created_by = %+v, want usr_master1", stored)
	}
	if _, stored := create(""); stored.Valid {
		t.Errorf("created_by = %+v, want NULL when the submitter is unknown", stored)
	}
}

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/database"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// These tests run the fulfillment SQL against a real PostgreSQL. They are
// skipped unless TEST_DATABASE_URL points at a disposable database (never a
// shared one): the schema is created and a few ITEST-* rows are written, then
// removed again.
//
//	TEST_DATABASE_URL='postgres://postgres@[::1]:5439/postgres?sslmode=disable' go test ./internal/pkg/repository/
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, url)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := database.MigrateAndSeed(ctx, db); err != nil {
		t.Fatalf("migrate and seed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM asset_requests WHERE id LIKE 'ITEST-%'`)
		_, _ = db.Exec(`DELETE FROM imei_reference WHERE brand LIKE 'ITEST%'`)
		db.Close()
	})
	return db
}

func insertRequest(t *testing.T, db *sql.DB, id, status string, step any) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO asset_requests (id, requester_id, sales_division, quantity, status, fulfillment_step)
		VALUES ($1, 'usr_master1', 'M1', 1, $2, $3)`, id, status, step)
	if err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

type fulfillmentRow struct {
	status string
	step   sql.NullInt64
	data   sql.NullString
}

func readRow(t *testing.T, db *sql.DB, id string) fulfillmentRow {
	t.Helper()
	var r fulfillmentRow
	if err := db.QueryRow(`SELECT status, fulfillment_step, fulfillment_data FROM asset_requests WHERE id = $1`, id).
		Scan(&r.status, &r.step, &r.data); err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	return r
}

func TestSaveFulfillmentDataSQL(t *testing.T) {
	db := openTestDB(t)
	repo := NewRequestRepository(db)
	ctx := context.Background()
	const data = `{"codes":["A"]}`

	t.Run("a request waiting for approval is left untouched", func(t *testing.T) {
		insertRequest(t, db, "ITEST-S1", domain.RequestStatusWaitingApproval, nil)
		ok, err := repo.SaveFulfillmentData(ctx, "ITEST-S1", data)
		if err != nil || ok {
			t.Fatalf("ok=%v err=%v, want false, nil", ok, err)
		}
		if r := readRow(t, db, "ITEST-S1"); r.status != domain.RequestStatusWaitingApproval || r.step.Valid || r.data.Valid {
			t.Fatalf("row changed: %+v", r)
		}
	})

	t.Run("an approved request moves to Shipped with its data", func(t *testing.T) {
		insertRequest(t, db, "ITEST-S2", domain.RequestStatusApproved, nil)
		ok, err := repo.SaveFulfillmentData(ctx, "ITEST-S2", data)
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v, want true, nil", ok, err)
		}
		r := readRow(t, db, "ITEST-S2")
		if r.status != domain.RequestStatusFulfillment || r.step.Int64 != 1 || r.data.String != data {
			t.Fatalf("row = %+v, want FULFILLMENT, step 1, data stored", r)
		}
		if ok, _ := repo.SaveFulfillmentData(ctx, "ITEST-S2", data); ok {
			t.Fatal("a second save must be refused once the request is Shipped")
		}
	})

	t.Run("a request already at Processing can be saved", func(t *testing.T) {
		insertRequest(t, db, "ITEST-S3", domain.RequestStatusFulfillment, 0)
		if ok, err := repo.SaveFulfillmentData(ctx, "ITEST-S3", data); err != nil || !ok {
			t.Fatalf("ok=%v err=%v, want true, nil", ok, err)
		}
	})

	t.Run("rejected, shipped and completed requests are refused", func(t *testing.T) {
		insertRequest(t, db, "ITEST-S4", domain.RequestStatusRejected, nil)
		insertRequest(t, db, "ITEST-S5", domain.RequestStatusFulfillment, 2)
		insertRequest(t, db, "ITEST-S6", domain.RequestStatusCompleted, 3)
		for _, id := range []string{"ITEST-S4", "ITEST-S5", "ITEST-S6"} {
			if ok, err := repo.SaveFulfillmentData(ctx, id, data); err != nil || ok {
				t.Errorf("%s: ok=%v err=%v, want false, nil", id, ok, err)
			}
		}
	})

	t.Run("concurrent saves: exactly one wins", func(t *testing.T) {
		insertRequest(t, db, "ITEST-S7", domain.RequestStatusApproved, 0)
		const callers = 12
		var wins int
		var mu sync.Mutex
		var wg sync.WaitGroup
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if ok, err := repo.SaveFulfillmentData(ctx, "ITEST-S7", data); err == nil && ok {
					mu.Lock()
					wins++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if wins != 1 {
			t.Fatalf("%d of %d concurrent saves succeeded, want exactly 1", wins, callers)
		}
	})
}

func TestAdvanceFulfillmentSQL(t *testing.T) {
	db := openTestDB(t)
	repo := NewRequestRepository(db)
	ctx := context.Background()

	t.Run("Shipped -> Delivered -> Completed, then refused", func(t *testing.T) {
		insertRequest(t, db, "ITEST-A1", domain.RequestStatusFulfillment, 1)

		step, ok, err := repo.AdvanceFulfillment(ctx, "ITEST-A1")
		if err != nil || !ok || step != 2 {
			t.Fatalf("first advance: step=%d ok=%v err=%v, want 2, true, nil", step, ok, err)
		}
		if r := readRow(t, db, "ITEST-A1"); r.status != domain.RequestStatusFulfillment || r.step.Int64 != 2 {
			t.Fatalf("after Delivered: %+v, want FULFILLMENT at step 2", r)
		}

		step, ok, err = repo.AdvanceFulfillment(ctx, "ITEST-A1")
		if err != nil || !ok || step != 3 {
			t.Fatalf("second advance: step=%d ok=%v err=%v, want 3, true, nil", step, ok, err)
		}
		if r := readRow(t, db, "ITEST-A1"); r.status != domain.RequestStatusCompleted || r.step.Int64 != 3 {
			t.Fatalf("after Completed: %+v, want COMPLETED at step 3", r)
		}

		if _, ok, _ := repo.AdvanceFulfillment(ctx, "ITEST-A1"); ok {
			t.Fatal("a completed request must not advance again")
		}
	})

	t.Run("requests that are not Shipped or Delivered are left alone", func(t *testing.T) {
		insertRequest(t, db, "ITEST-A2", domain.RequestStatusWaitingApproval, nil)
		insertRequest(t, db, "ITEST-A3", domain.RequestStatusApproved, 0)
		insertRequest(t, db, "ITEST-A4", domain.RequestStatusFulfillment, 0)
		for _, id := range []string{"ITEST-A2", "ITEST-A3", "ITEST-A4"} {
			if _, ok, err := repo.AdvanceFulfillment(ctx, id); err != nil || ok {
				t.Errorf("%s: ok=%v err=%v, want false, nil", id, ok, err)
			}
		}
		if r := readRow(t, db, "ITEST-A2"); r.status != domain.RequestStatusWaitingApproval || r.step.Valid {
			t.Errorf("ITEST-A2 changed: %+v", r)
		}
	})

	t.Run("concurrent advances from Delivered: exactly one completes it", func(t *testing.T) {
		insertRequest(t, db, "ITEST-A5", domain.RequestStatusFulfillment, 2)
		const callers = 12
		var wins int
		var mu sync.Mutex
		var wg sync.WaitGroup
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, ok, err := repo.AdvanceFulfillment(ctx, "ITEST-A5"); err == nil && ok {
					mu.Lock()
					wins++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if wins != 1 {
			t.Fatalf("%d of %d concurrent advances succeeded, want exactly 1", wins, callers)
		}
		if r := readRow(t, db, "ITEST-A5"); r.step.Int64 != 3 || r.status != domain.RequestStatusCompleted {
			t.Fatalf("final row %+v, want step 3 COMPLETED", r)
		}
	})
}

func TestApprovalStartsProcessingSQL(t *testing.T) {
	db := openTestDB(t)
	repo := NewRequestRepository(db)
	ctx := context.Background()

	t.Run("an approval sets fulfillment_step to 0", func(t *testing.T) {
		insertRequest(t, db, "ITEST-P1", domain.RequestStatusWaitingApproval, nil)
		if err := repo.SetApprovalDecisionResult(ctx, "ITEST-P1", domain.RequestStatusApproved, "approved", 3, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r := readRow(t, db, "ITEST-P1"); r.status != domain.RequestStatusApproved || !r.step.Valid || r.step.Int64 != 0 {
			t.Fatalf("row = %+v, want APPROVED with step 0", r)
		}
	})

	t.Run("an approval never resets a step that already moved on", func(t *testing.T) {
		insertRequest(t, db, "ITEST-P2", domain.RequestStatusFulfillment, 2)
		if err := repo.SetApprovalDecisionResult(ctx, "ITEST-P2", domain.RequestStatusApproved, "approved", 3, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r := readRow(t, db, "ITEST-P2"); r.step.Int64 != 2 {
			t.Fatalf("step = %d, want it to stay 2", r.step.Int64)
		}
	})

	t.Run("pending and rejected results leave the step empty", func(t *testing.T) {
		for i, status := range []string{domain.RequestStatusWaitingApproval, domain.RequestStatusRejected} {
			id := fmt.Sprintf("ITEST-P%d", 3+i)
			insertRequest(t, db, id, domain.RequestStatusWaitingApproval, nil)
			if err := repo.SetApprovalDecisionResult(ctx, id, status, "pending", 1, nil); err != nil {
				t.Fatalf("%s: unexpected error: %v", id, err)
			}
			if r := readRow(t, db, id); r.step.Valid {
				t.Errorf("%s (%s): step = %d, want NULL", id, status, r.step.Int64)
			}
		}
	})
}

func TestImportImeiReferenceSQL(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// 1200 rows crosses the 500-row chunk boundary twice.
	rows := make([]domain.ImeiReferenceRow, 0, 1200)
	for i := 0; i < 1200; i++ {
		rows = append(rows, domain.ImeiReferenceRow{
			TAC: fmt.Sprintf("9%07d", i), Brand: "ITEST Brand", Model: fmt.Sprintf("M%d", i), ReleaseYear: "2024",
		})
	}
	n, err := ImportImeiReference(ctx, db, rows)
	if err != nil || n != 1200 {
		t.Fatalf("import: n=%d err=%v, want 1200, nil", n, err)
	}

	repo := NewImeiReferenceRepository(db)
	got, err := repo.LookupTAC(ctx, "90000999")
	if err != nil || got == nil || got.Model != "M999" || got.Brand != "ITEST Brand" {
		t.Fatalf("lookup after import: %+v, %v", got, err)
	}

	rows[999].Brand = "ITEST Updated"
	if _, err := ImportImeiReference(ctx, db, rows[900:1100]); err != nil {
		t.Fatalf("re-import: %v", err)
	}
	got, _ = repo.LookupTAC(ctx, "90000999")
	if got == nil || got.Brand != "ITEST Updated" {
		t.Fatalf("re-import should update by TAC, got %+v", got)
	}

	if got, err := repo.LookupTAC(ctx, "00000000"); err != nil || got != nil {
		t.Fatalf("unknown TAC: got %+v, %v; want nil, nil", got, err)
	}
}

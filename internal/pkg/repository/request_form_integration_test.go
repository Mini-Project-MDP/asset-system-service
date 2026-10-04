package repository

import (
	"context"
	"database/sql"
	"regexp"
	"sync"
	"testing"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// Runs the New Request form's SQL against a real PostgreSQL; skipped unless
// TEST_DATABASE_URL points at a disposable database (see openTestDB).

type createdRow struct {
	outletID, distributorID, distributorManual, requestType sql.NullString
	requesterName, requesterRole, createdBy                 sql.NullString
	requesterID, assetTypeID, salesDivision, priority       string
}

func readCreated(t *testing.T, db *sql.DB, id string) createdRow {
	t.Helper()
	var r createdRow
	err := db.QueryRow(`SELECT outlet_id, distributor_id, distributor_manual, request_type, requester_name, requester_role,
		created_by, requester_id, asset_type_id, sales_division, priority FROM asset_requests WHERE id = $1`, id).
		Scan(&r.outletID, &r.distributorID, &r.distributorManual, &r.requestType, &r.requesterName, &r.requesterRole,
			&r.createdBy, &r.requesterID, &r.assetTypeID, &r.salesDivision, &r.priority)
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	return r
}

func TestCreateRequestSQL(t *testing.T) {
	db := openTestDB(t)
	repo := NewRequestRepository(db)
	ctx := context.Background()

	if _, err := db.Exec(`INSERT INTO outlets (id, code, name) VALUES ('ITEST-FO1', 'ITEST-F1', 'ITEST Form Outlet')`); err != nil {
		t.Fatalf("insert outlet: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO distributors (id, code, name) VALUES ('ITEST-FD1', 'ITEST-FD1', 'ITEST Form Distributor')`); err != nil {
		t.Fatalf("insert distributor: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM asset_requests WHERE requester_name LIKE 'ITEST%'`)
		_, _ = db.Exec(`DELETE FROM distributors WHERE id = 'ITEST-FD1'`)
		_, _ = db.Exec(`DELETE FROM outlets WHERE id = 'ITEST-FO1'`)
	})

	create := func(in domain.CreateRequestInput) string {
		t.Helper()
		id, err := repo.Create(ctx, "usr_master1", in)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		return id
	}
	base := domain.CreateRequestInput{
		Category: "Android", SalesDivision: "M1 BIS", ReqType: "Baru", RequesterName: "ITEST Cabang Bandung", RequesterRole: "Cabang",
		Qty: 2, Priority: "high", CreatedBy: "usr_master2", OutletID: "ITEST-FO1", DistributorID: "ITEST-FD1",
	}

	t.Run("a master data distributor and outlet are stored by id, with the typed requester and role", func(t *testing.T) {
		id := create(base)
		if ok, _ := regexp.MatchString(`^REQ-\d{4,}$`, id); !ok {
			t.Fatalf("id %q is not of the form REQ-####", id)
		}
		r := readCreated(t, db, id)
		if r.outletID.String != "ITEST-FO1" || r.distributorID.String != "ITEST-FD1" || r.distributorManual.Valid {
			t.Errorf("outlet %v, distributor %v, manual %v", r.outletID, r.distributorID, r.distributorManual)
		}
		if r.requesterName.String != "ITEST Cabang Bandung" || r.requesterRole.String != "Cabang" || r.createdBy.String != "usr_master2" || r.requesterID != "usr_master1" {
			t.Errorf("requester name %v role %v created_by %v requester_id %q", r.requesterName, r.requesterRole, r.createdBy, r.requesterID)
		}
		if r.assetTypeID != "atype_android" || r.requestType.String != "Baru" || r.salesDivision != "M1 BIS" || r.priority != "high" {
			t.Errorf("type %q request type %v division %q priority %q", r.assetTypeID, r.requestType, r.salesDivision, r.priority)
		}
	})

	t.Run("a distributor typed by hand is stored as text, with no distributor id", func(t *testing.T) {
		in := base
		in.DistributorID, in.DistributorManual = "", "PT Ketik Manual"
		r := readCreated(t, db, create(in))
		if r.distributorID.Valid || r.distributorManual.String != "PT Ketik Manual" {
			t.Errorf("distributor id %v, manual %v", r.distributorID, r.distributorManual)
		}
	})

	t.Run("the list shows the typed distributor and requester, and searches the requester name", func(t *testing.T) {
		in := base
		in.DistributorID, in.DistributorManual, in.RequesterName = "", "PT Ketik Manual", "ITEST Nama Unik Xyz"
		id := create(in)

		items, err := repo.List(ctx, domain.RequestListQuery{Search: "nama unik xyz"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(items) != 1 || items[0].ID != id {
			t.Fatalf("search by the typed requester name found %d rows, want exactly %s", len(items), id)
		}
		got := items[0]
		if got.Distributor != "PT Ketik Manual" || got.RequesterName != "ITEST Nama Unik Xyz" || got.RequesterRole != "Cabang" {
			t.Errorf("distributor %q requester %q role %q", got.Distributor, got.RequesterName, got.RequesterRole)
		}
	})

	t.Run("older requests without the new columns still read back", func(t *testing.T) {
		if _, err := db.Exec(`INSERT INTO asset_requests (id, requester_id, asset_type_id, sales_division, quantity, status)
			VALUES ('ITEST-OLD', 'usr_master1', 'atype_barcode', 'M1', 1, 'WAITING_APPROVAL')`); err != nil {
			t.Fatalf("insert: %v", err)
		}
		got, err := repo.GetByID(ctx, "ITEST-OLD")
		if err != nil || got == nil {
			t.Fatalf("get: %v, %v", got, err)
		}
		if got.RequesterName != "Master Admin One" || got.RequesterRole != "" || got.Distributor != "" {
			t.Errorf("requester %q role %q distributor %q; want the user's name, no role, no distributor", got.RequesterName, got.RequesterRole, got.Distributor)
		}
	})

	t.Run("an unknown category is an error, not a request without a type", func(t *testing.T) {
		in := base
		in.Category = "Hologram"
		if _, err := repo.Create(ctx, "usr_master1", in); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("ids are unique and increasing under concurrent creation", func(t *testing.T) {
		const callers = 20
		ids := make(chan string, callers)
		var wg sync.WaitGroup
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if id, err := repo.Create(ctx, "usr_master1", base); err == nil {
					ids <- id
				}
			}()
		}
		wg.Wait()
		close(ids)
		seen := map[string]bool{}
		for id := range ids {
			if seen[id] {
				t.Fatalf("duplicate id %s", id)
			}
			seen[id] = true
		}
		if len(seen) != callers {
			t.Fatalf("%d of %d concurrent creations succeeded", len(seen), callers)
		}
	})
}

func TestResolveRequesterSQL(t *testing.T) {
	db := openTestDB(t)
	repo := NewRequestRepository(db)
	ctx := context.Background()

	byName, err := repo.ResolveRequester(ctx, "Manager Asset One")
	if err != nil || byName == nil {
		t.Fatalf("by name: %v, %v", byName, err)
	}
	if byName.UserID != "usr_mgr1" || byName.Name != "Manager Asset One" || byName.Email != "manager1@mayora.com" || byName.EmployeeNo != "EMP003" || byName.ApprovalRank != 80 {
		t.Errorf("by name = %+v", byName)
	}
	if byEmail, _ := repo.ResolveRequester(ctx, "manager1@mayora.com"); byEmail == nil || byEmail.UserID != "usr_mgr1" {
		t.Errorf("by email = %+v", byEmail)
	}
	byID, err := repo.ResolveRequesterByUserID(ctx, "usr_mgr1")
	if err != nil || byID == nil || *byID != *byName {
		t.Errorf("by id = %+v (err %v), want the same person as by name", byID, err)
	}
	if none, err := repo.ResolveRequester(ctx, "Nobody At All"); err != nil || none != nil {
		t.Errorf("unknown name = %+v, %v; want nil, nil", none, err)
	}
	if none, err := repo.ResolveRequesterByUserID(ctx, "usr_missing"); err != nil || none != nil {
		t.Errorf("unknown id = %+v, %v; want nil, nil", none, err)
	}
}

func TestRequestFormSourceSQL(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	source := NewRequestFormSource(db, NewMasterDataRepository(db))

	if _, err := db.Exec(`INSERT INTO outlets (id, code, name, is_active) VALUES ('ITEST-FS1', 'ITEST-S1', 'ITEST Active Outlet', 1), ('ITEST-FS2', 'ITEST-S2', 'ITEST Closed Outlet', 0)`); err != nil {
		t.Fatalf("insert outlets: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO distributors (id, code, name, is_active) VALUES ('ITEST-FS3', 'ITEST-S3', 'ITEST Active Distributor', 1), ('ITEST-FS4', 'ITEST-S4', 'ITEST Closed Distributor', 0)`); err != nil {
		t.Fatalf("insert distributors: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO distributor_outlets (distributor_id, outlet_id) VALUES ('ITEST-FS3', 'ITEST-FS1')`); err != nil {
		t.Fatalf("map outlet: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sales_divisions (id, name, is_active) VALUES ('ITEST-SDV1', 'ITEST Division', 1), ('ITEST-SDV2', 'ITEST Closed Division', 0)`); err != nil {
		t.Fatalf("insert divisions: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM distributors WHERE id LIKE 'ITEST-FS%'`)
		_, _ = db.Exec(`DELETE FROM outlets WHERE id LIKE 'ITEST-FS%'`)
		_, _ = db.Exec(`DELETE FROM sales_divisions WHERE id LIKE 'ITEST-SDV%'`)
	})

	data, err := source.FormMasterData(ctx)
	if err != nil {
		t.Fatalf("form data: %v", err)
	}
	has := func(names []string, want string) bool {
		for _, n := range names {
			if n == want {
				return true
			}
		}
		return false
	}
	var distributors, outlets []string
	for _, d := range data.Distributors {
		distributors = append(distributors, d.Name)
		if d.Name == "ITEST Active Distributor" && (len(d.Outlets) != 1 || d.Outlets[0].Name != "ITEST Active Outlet") {
			t.Errorf("mapped outlets = %+v", d.Outlets)
		}
	}
	for _, o := range data.Outlets {
		outlets = append(outlets, o.Name)
	}
	if !has(distributors, "ITEST Active Distributor") || has(distributors, "ITEST Closed Distributor") {
		t.Errorf("distributors = %v; want the active one only", distributors)
	}
	if !has(outlets, "ITEST Active Outlet") || has(outlets, "ITEST Closed Outlet") {
		t.Errorf("outlets = %v; want the active one only", outlets)
	}
	if !has(data.SalesDivisions, "ITEST Division") || has(data.SalesDivisions, "ITEST Closed Division") {
		t.Errorf("sales divisions = %v; want the active one only", data.SalesDivisions)
	}
	for _, seeded := range []string{"M1 BIS", "M1 CWC", "M245", "M3"} {
		if !has(data.SalesDivisions, seeded) {
			t.Errorf("seeded division %q is missing from %v", seeded, data.SalesDivisions)
		}
	}
}

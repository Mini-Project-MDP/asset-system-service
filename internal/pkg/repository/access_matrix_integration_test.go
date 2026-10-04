package repository

import (
	"sort"
	"testing"

	"github.com/lib/pq"
)

// The module access matrix of the requirement document (User Requirement,
// "Matriks akses modul per role"), expressed as the permission each module's
// menu and endpoints need. Runs the real seed against a disposable PostgreSQL;
// skipped unless TEST_DATABASE_URL is set (see openTestDB).
//
//	Modul        Admin  Asset Team  Sales Admin  SS/RSM/GRSM/NSM/SD
//	Dashboard      ✓        ✓           —              —          dashboard:read
//	Requests       ✓        ✓           ✓              —          request:read, request:create
//	Approvals      ✓        —           —              ✓          approvals:read
//	Fulfillment    ✓        ✓           —              —          fulfillment:read
//	Settings       ✓        ✓ (part)    —              —          masterdata:manage (Outlet & Distributor), settings:manage (the rest, Admin only)
func TestSeededRolesFollowTheAccessMatrix(t *testing.T) {
	db := openTestDB(t)

	modulePermissions := []string{
		"dashboard:read", "request:read", "request:create", "approvals:read",
		"fulfillment:read", "masterdata:manage", "settings:manage",
	}
	want := map[string][]string{
		"MASTER_ADMIN":  {"dashboard:read", "request:read", "request:create", "approvals:read", "fulfillment:read", "masterdata:manage", "settings:manage"},
		"ASSET_MANAGER": {"dashboard:read", "request:read", "request:create", "fulfillment:read", "masterdata:manage"},
		"SA":            {"request:read", "request:create"},
		"SS":            {"approvals:read"},
		"RSM":           {"approvals:read"},
		"GRSM":          {"approvals:read"},
		"NSM":           {"approvals:read"},
		"SD":            {"approvals:read"},
		"Cabang":        {}, // a label for requests made on a branch's behalf, not a login role
	}

	for role, expected := range want {
		rows, err := db.Query(`
			SELECT p.code FROM role_permissions rp
			JOIN roles r ON r.id = rp.role_id
			JOIN permissions p ON p.id = rp.permission_id
			WHERE r.code = $1 AND p.code = ANY($2)`, role, pq.Array(modulePermissions))
		if err != nil {
			t.Fatalf("%s: query: %v", role, err)
		}
		got := []string{}
		for rows.Next() {
			var code string
			if err := rows.Scan(&code); err != nil {
				t.Fatalf("%s: scan: %v", role, err)
			}
			got = append(got, code)
		}
		rows.Close()

		sort.Strings(got)
		sorted := append([]string{}, expected...)
		sort.Strings(sorted)
		if len(got) != len(sorted) {
			t.Errorf("%s: module permissions = %v, want %v", role, got, sorted)
			continue
		}
		for i := range got {
			if got[i] != sorted[i] {
				t.Errorf("%s: module permissions = %v, want %v", role, got, sorted)
				break
			}
		}
	}

	t.Run("every approver can still act on a request", func(t *testing.T) {
		for _, role := range []string{"MASTER_ADMIN", "SS", "RSM", "GRSM", "NSM", "SD"} {
			var n int
			if err := db.QueryRow(`SELECT COUNT(*) FROM role_permissions rp JOIN roles r ON r.id = rp.role_id
				JOIN permissions p ON p.id = rp.permission_id WHERE r.code = $1 AND p.code = 'request:approve'`, role).Scan(&n); err != nil || n != 1 {
				t.Errorf("%s: request:approve rows = %d, err %v; want 1", role, n, err)
			}
		}
	})
}

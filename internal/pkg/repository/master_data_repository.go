package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type masterDataRepository struct {
	db *sql.DB
}

// NewMasterDataRepository creates a PostgreSQL-backed domain.MasterDataRepository.
func NewMasterDataRepository(db *sql.DB) domain.MasterDataRepository {
	return &masterDataRepository{db: db}
}

// unfinishedRequests is the status filter for "still referencing master data".
const unfinishedRequests = `status NOT IN ('COMPLETED','REJECTED')`

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// mapWriteError converts a PostgreSQL unique violation into domain.ErrDuplicate.
func mapWriteError(err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return domain.ErrDuplicate
	}
	return err
}

// regionSlug derives a stable regions.code from a region name.
func regionSlug(name string) string {
	return strings.ToUpper(strings.Join(strings.Fields(name), "_"))
}

// ensureRegion returns the id of the region with the given name, creating it
// when it does not exist yet.
func ensureRegion(ctx context.Context, tx *sql.Tx, name string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id FROM regions WHERE name = $1 LIMIT 1`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	code := regionSlug(name)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO regions (id, code, name) VALUES ($1, $2, $3) ON CONFLICT (code) DO NOTHING`,
		uuid.NewString(), code, name); err != nil {
		return "", err
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM regions WHERE code = $1`, code).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func (r *masterDataRepository) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (r *masterDataRepository) exists(ctx context.Context, query string, args ...any) (bool, error) {
	var found bool
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&found); err != nil {
		return false, err
	}
	return found, nil
}

// --- Outlets ---

const outletSelect = `
	SELECT o.id, o.code, o.name, COALESCE(r.name, ''), o.is_active
	FROM outlets o LEFT JOIN regions r ON r.id = o.region_id`

func scanOutlet(s interface{ Scan(...any) error }) (domain.Outlet, error) {
	var o domain.Outlet
	var active int
	if err := s.Scan(&o.ID, &o.Code, &o.Name, &o.Region, &active); err != nil {
		return domain.Outlet{}, err
	}
	o.IsActive = active == 1
	return o, nil
}

func (r *masterDataRepository) ListOutlets(ctx context.Context, includeInactive bool) ([]domain.Outlet, error) {
	query := outletSelect
	if !includeInactive {
		query += ` WHERE o.is_active = 1`
	}
	rows, err := r.db.QueryContext(ctx, query+` ORDER BY o.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Outlet, 0)
	for rows.Next() {
		o, err := scanOutlet(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, o)
	}
	return items, rows.Err()
}

func (r *masterDataRepository) GetOutlet(ctx context.Context, id string) (*domain.Outlet, error) {
	o, err := scanOutlet(r.db.QueryRowContext(ctx, outletSelect+` WHERE o.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *masterDataRepository) CreateOutlet(ctx context.Context, in domain.SaveOutletInput, isActive bool) (string, error) {
	id := uuid.NewString()
	err := r.inTx(ctx, func(tx *sql.Tx) error {
		regionID, err := ensureRegion(ctx, tx, in.Region)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO outlets (id, code, name, region_id, is_active) VALUES ($1, $2, $3, $4, $5)`,
			id, in.Code, in.Name, regionID, boolToInt(isActive))
		return err
	})
	if err != nil {
		return "", mapWriteError(err)
	}
	return id, nil
}

func (r *masterDataRepository) UpdateOutlet(ctx context.Context, id string, in domain.SaveOutletInput, isActive bool) error {
	err := r.inTx(ctx, func(tx *sql.Tx) error {
		regionID, err := ensureRegion(ctx, tx, in.Region)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`UPDATE outlets SET code = $1, name = $2, region_id = $3, is_active = $4,
			 version = version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $5`,
			in.Code, in.Name, regionID, boolToInt(isActive), id)
		return err
	})
	return mapWriteError(err)
}

func (r *masterDataRepository) OutletInUse(ctx context.Context, id string) (bool, error) {
	return r.exists(ctx, `SELECT EXISTS(SELECT 1 FROM asset_requests WHERE outlet_id = $1 AND `+unfinishedRequests+`)`, id)
}

// --- Distributors ---

func (r *masterDataRepository) loadDistributors(ctx context.Context, where string, args ...any) ([]domain.Distributor, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT d.id, d.code, d.name, d.is_active, COALESCE(o.id, ''), COALESCE(o.name, '')
		FROM distributors d
		LEFT JOIN distributor_outlets dof ON dof.distributor_id = d.id
		LEFT JOIN outlets o ON o.id = dof.outlet_id
		`+where+` ORDER BY d.name, o.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.Distributor, 0)
	index := map[string]int{}
	for rows.Next() {
		var d domain.Distributor
		var active int
		var outletID, outletName string
		if err := rows.Scan(&d.ID, &d.Code, &d.Name, &active, &outletID, &outletName); err != nil {
			return nil, err
		}
		i, seen := index[d.ID]
		if !seen {
			d.IsActive = active == 1
			d.Outlets = []domain.OutletRef{}
			items = append(items, d)
			i = len(items) - 1
			index[d.ID] = i
		}
		if outletID != "" {
			items[i].Outlets = append(items[i].Outlets, domain.OutletRef{ID: outletID, Name: outletName})
		}
	}
	return items, rows.Err()
}

func (r *masterDataRepository) ListDistributors(ctx context.Context, includeInactive bool) ([]domain.Distributor, error) {
	where := ``
	if !includeInactive {
		where = `WHERE d.is_active = 1`
	}
	return r.loadDistributors(ctx, where)
}

func (r *masterDataRepository) GetDistributor(ctx context.Context, id string) (*domain.Distributor, error) {
	items, err := r.loadDistributors(ctx, `WHERE d.id = $1`, id)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

func mapOutlets(ctx context.Context, tx *sql.Tx, distributorID string, outletIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM distributor_outlets WHERE distributor_id = $1`, distributorID); err != nil {
		return err
	}
	for _, outletID := range outletIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO distributor_outlets (distributor_id, outlet_id) VALUES ($1, $2)`,
			distributorID, outletID); err != nil {
			return err
		}
	}
	return nil
}

func (r *masterDataRepository) CreateDistributor(ctx context.Context, in domain.SaveDistributorInput, isActive bool) (string, error) {
	id := uuid.NewString()
	err := r.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO distributors (id, code, name, is_active) VALUES ($1, $2, $3, $4)`,
			id, in.Code, in.Name, boolToInt(isActive)); err != nil {
			return err
		}
		return mapOutlets(ctx, tx, id, in.OutletIDs)
	})
	if err != nil {
		return "", mapWriteError(err)
	}
	return id, nil
}

func (r *masterDataRepository) UpdateDistributor(ctx context.Context, id string, in domain.SaveDistributorInput, isActive bool) error {
	err := r.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE distributors SET code = $1, name = $2, is_active = $3,
			 version = version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $4`,
			in.Code, in.Name, boolToInt(isActive), id); err != nil {
			return err
		}
		return mapOutlets(ctx, tx, id, in.OutletIDs)
	})
	return mapWriteError(err)
}

func (r *masterDataRepository) DistributorInUse(ctx context.Context, id string) (bool, error) {
	return r.exists(ctx, `SELECT EXISTS(SELECT 1 FROM asset_requests WHERE distributor_id = $1 AND `+unfinishedRequests+`)`, id)
}

func (r *masterDataRepository) MissingOutlets(ctx context.Context, outletIDs []string) ([]string, error) {
	if len(outletIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT u.id FROM UNNEST($1::text[]) AS u(id) WHERE NOT EXISTS (SELECT 1 FROM outlets o WHERE o.id = u.id)`,
		pq.Array(outletIDs))
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func (r *masterDataRepository) OutletsMappedElsewhere(ctx context.Context, outletIDs []string, exceptDistributorID string) ([]string, error) {
	if len(outletIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT outlet_id FROM distributor_outlets WHERE outlet_id = ANY($1::text[]) AND distributor_id <> $2`,
		pq.Array(outletIDs), exceptDistributorID)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func scanIDs(rows *sql.Rows) ([]string, error) {
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// --- Asset types ---

const assetTypeSelect = `
	SELECT id, code, name, identifier_type, identifier_required, is_active FROM asset_types`

func scanAssetType(s interface{ Scan(...any) error }) (domain.AssetType, error) {
	var t domain.AssetType
	var required, active int
	if err := s.Scan(&t.ID, &t.Code, &t.Name, &t.IdentifierType, &required, &active); err != nil {
		return domain.AssetType{}, err
	}
	t.IdentifierRequired = required == 1
	t.IsActive = active == 1
	return t, nil
}

func (r *masterDataRepository) ListAssetTypes(ctx context.Context, includeInactive bool) ([]domain.AssetType, error) {
	query := assetTypeSelect
	if !includeInactive {
		query += ` WHERE is_active = 1`
	}
	rows, err := r.db.QueryContext(ctx, query+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.AssetType, 0)
	for rows.Next() {
		t, err := scanAssetType(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

func (r *masterDataRepository) GetAssetType(ctx context.Context, id string) (*domain.AssetType, error) {
	t, err := scanAssetType(r.db.QueryRowContext(ctx, assetTypeSelect+` WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *masterDataRepository) CreateAssetType(ctx context.Context, in domain.SaveAssetTypeInput, isActive bool) (string, error) {
	id := uuid.NewString()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO asset_types (id, code, name, identifier_type, identifier_required, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		id, in.Code, in.Name, in.Identifier, boolToInt(in.Identifier != domain.IdentifierNone), boolToInt(isActive))
	if err != nil {
		return "", mapWriteError(err)
	}
	return id, nil
}

func (r *masterDataRepository) UpdateAssetType(ctx context.Context, id string, in domain.SaveAssetTypeInput, isActive bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE asset_types SET code = $1, name = $2, identifier_type = $3, identifier_required = $4,
		 is_active = $5, version = version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $6`,
		in.Code, in.Name, in.Identifier, boolToInt(in.Identifier != domain.IdentifierNone), boolToInt(isActive), id)
	return mapWriteError(err)
}

func (r *masterDataRepository) AssetTypeInUse(ctx context.Context, id string) (bool, error) {
	return r.exists(ctx, `SELECT EXISTS(SELECT 1 FROM asset_requests WHERE asset_type_id = $1 AND `+unfinishedRequests+`)`, id)
}

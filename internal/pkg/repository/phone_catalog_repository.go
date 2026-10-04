package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/google/uuid"
)

type phoneCatalogRepository struct {
	db *sql.DB
}

// NewPhoneCatalogRepository creates a PostgreSQL-backed domain.PhoneCatalogRepository.
func NewPhoneCatalogRepository(db *sql.DB) domain.PhoneCatalogRepository {
	return &phoneCatalogRepository{db: db}
}

// loadBrands reads brands with their models. Inactive models are filtered in
// the JOIN condition (not WHERE) so an active brand without active models is
// still returned with an empty model list.
func (r *phoneCatalogRepository) loadBrands(ctx context.Context, includeInactive bool, brandID string) ([]domain.PhoneBrand, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT b.id, b.name, b.is_active, COALESCE(m.id, ''), COALESCE(m.name, ''), COALESCE(m.is_active, 0)
		FROM phone_brands b
		LEFT JOIN phone_models m ON m.brand_id = b.id AND ($1::boolean OR m.is_active = 1)
		WHERE ($1::boolean OR b.is_active = 1) AND ($2 = '' OR b.id = $2)
		ORDER BY b.name, m.name`, includeInactive, brandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.PhoneBrand, 0)
	index := map[string]int{}
	for rows.Next() {
		var b domain.PhoneBrand
		var brandActive, modelActive int
		var m domain.PhoneModel
		if err := rows.Scan(&b.ID, &b.Name, &brandActive, &m.ID, &m.Name, &modelActive); err != nil {
			return nil, err
		}
		i, seen := index[b.ID]
		if !seen {
			b.IsActive = brandActive == 1
			b.Models = []domain.PhoneModel{}
			items = append(items, b)
			i = len(items) - 1
			index[b.ID] = i
		}
		if m.ID != "" {
			m.BrandID = b.ID
			m.IsActive = modelActive == 1
			items[i].Models = append(items[i].Models, m)
		}
	}
	return items, rows.Err()
}

func (r *phoneCatalogRepository) ListBrands(ctx context.Context, includeInactive bool) ([]domain.PhoneBrand, error) {
	return r.loadBrands(ctx, includeInactive, "")
}

func (r *phoneCatalogRepository) GetBrand(ctx context.Context, id string) (*domain.PhoneBrand, error) {
	items, err := r.loadBrands(ctx, true, id)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

func (r *phoneCatalogRepository) CreateBrand(ctx context.Context, in domain.SavePhoneBrandInput, isActive bool) (string, error) {
	id := uuid.NewString()
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO phone_brands (id, name, is_active) VALUES ($1, $2, $3)`,
		id, in.Name, boolToInt(isActive)); err != nil {
		return "", mapWriteError(err)
	}
	return id, nil
}

func (r *phoneCatalogRepository) UpdateBrand(ctx context.Context, id string, in domain.SavePhoneBrandInput, isActive bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE phone_brands SET name = $1, is_active = $2, version = version + 1,
		 updated_at = CURRENT_TIMESTAMP WHERE id = $3`,
		in.Name, boolToInt(isActive), id)
	return mapWriteError(err)
}

func (r *phoneCatalogRepository) GetModel(ctx context.Context, id string) (*domain.PhoneModel, error) {
	var m domain.PhoneModel
	var active int
	err := r.db.QueryRowContext(ctx,
		`SELECT id, brand_id, name, is_active FROM phone_models WHERE id = $1`, id).
		Scan(&m.ID, &m.BrandID, &m.Name, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.IsActive = active == 1
	return &m, nil
}

func (r *phoneCatalogRepository) CreateModel(ctx context.Context, in domain.SavePhoneModelInput, isActive bool) (string, error) {
	id := uuid.NewString()
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO phone_models (id, brand_id, name, is_active) VALUES ($1, $2, $3, $4)`,
		id, in.BrandID, in.Name, boolToInt(isActive)); err != nil {
		return "", mapWriteError(err)
	}
	return id, nil
}

func (r *phoneCatalogRepository) UpdateModel(ctx context.Context, id string, in domain.SavePhoneModelInput, isActive bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE phone_models SET brand_id = $1, name = $2, is_active = $3, version = version + 1,
		 updated_at = CURRENT_TIMESTAMP WHERE id = $4`,
		in.BrandID, in.Name, boolToInt(isActive), id)
	return mapWriteError(err)
}

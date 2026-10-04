package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// requestFormSource reads the master data the New Request form chooses from.
type requestFormSource struct {
	db     *sql.DB
	master domain.MasterDataRepository
}

// NewRequestFormSource creates a domain.RequestFormSource. Distributors and
// outlets come from the master data repository, sales divisions from their table.
func NewRequestFormSource(db *sql.DB, master domain.MasterDataRepository) domain.RequestFormSource {
	return &requestFormSource{db: db, master: master}
}

func (s *requestFormSource) FormMasterData(ctx context.Context) (domain.FormMasterData, error) {
	var data domain.FormMasterData
	var err error
	if data.Distributors, err = s.master.ListDistributors(ctx, false); err != nil {
		return domain.FormMasterData{}, fmt.Errorf("form distributors: %w", err)
	}
	if data.Outlets, err = s.master.ListOutlets(ctx, false); err != nil {
		return domain.FormMasterData{}, fmt.Errorf("form outlets: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `SELECT name FROM sales_divisions WHERE is_active = 1 ORDER BY name`)
	if err != nil {
		return domain.FormMasterData{}, fmt.Errorf("form sales divisions: %w", err)
	}
	defer rows.Close()
	data.SalesDivisions = []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return domain.FormMasterData{}, fmt.Errorf("scan sales division: %w", err)
		}
		data.SalesDivisions = append(data.SalesDivisions, name)
	}
	return data, rows.Err()
}

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

type imeiReferenceRepository struct {
	db *sql.DB
}

// NewImeiReferenceRepository creates a PostgreSQL-backed domain.ImeiReferenceRepository.
func NewImeiReferenceRepository(db *sql.DB) domain.ImeiReferenceRepository {
	return &imeiReferenceRepository{db: db}
}

func (r *imeiReferenceRepository) LookupTAC(ctx context.Context, tac string) (*domain.ImeiInfo, error) {
	var info domain.ImeiInfo
	err := r.db.QueryRowContext(ctx,
		`SELECT brand, model, release_year FROM imei_reference WHERE tac = $1`, tac).
		Scan(&info.Brand, &info.Model, &info.ReleaseYear)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// importChunkSize is how many rows go into one INSERT: 4 parameters per row
// keeps a chunk far below PostgreSQL's 65535-parameter limit, and chunking
// (rather than one INSERT per row) matters because every round trip to a
// remote database is slow.
const importChunkSize = 500

// ImportImeiReference inserts or updates TAC reference rows (matched by TAC) in
// a single transaction and returns how many rows were written. Either every
// row is imported or none is.
func ImportImeiReference(ctx context.Context, db *sql.DB, rows []domain.ImeiReferenceRow) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	for start := 0; start < len(rows); start += importChunkSize {
		end := start + importChunkSize
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]

		var query strings.Builder
		query.WriteString(`INSERT INTO imei_reference (tac, brand, model, release_year) VALUES `)
		args := make([]any, 0, len(chunk)*4)
		for i, row := range chunk {
			if i > 0 {
				query.WriteString(", ")
			}
			n := i * 4
			fmt.Fprintf(&query, "($%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4)
			args = append(args, row.TAC, row.Brand, row.Model, row.ReleaseYear)
		}
		query.WriteString(` ON CONFLICT (tac) DO UPDATE SET brand = EXCLUDED.brand, model = EXCLUDED.model, release_year = EXCLUDED.release_year`)

		if _, err := tx.ExecContext(ctx, query.String(), args...); err != nil {
			return 0, fmt.Errorf("import rows %d-%d: %w", start+1, end, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(rows), nil
}

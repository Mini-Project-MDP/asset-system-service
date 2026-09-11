package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// Open creates a PostgreSQL database handle, configures connection pooling, and verifies connectivity.
func Open(ctx context.Context, databaseURL string) (*sql.DB, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("database URL cannot be empty")
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres database: %w", err)
	}

	// Connection pooling configuration suitable for Supabase pooler (port 5432 / 6543)
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(15 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

// OpenPostgres is an alias for Open.
func OpenPostgres(ctx context.Context, databaseURL string) (*sql.DB, error) {
	return Open(ctx, databaseURL)
}

// OpenTurso is maintained for backwards compatibility.
func OpenTurso(ctx context.Context, databaseURL, authToken string) (*sql.DB, error) {
	return Open(ctx, databaseURL)
}

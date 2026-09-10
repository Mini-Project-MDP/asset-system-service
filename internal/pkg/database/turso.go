package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/tursodatabase/libsql-client-go/libsql"
)

// OpenTurso creates a database handle and verifies the remote connection.
func OpenTurso(ctx context.Context, databaseURL, authToken string) (*sql.DB, error) {
	connector, err := libsql.NewConnector(
		databaseURL,
		libsql.WithAuthToken(authToken),
	)
	if err != nil {
		return nil, fmt.Errorf("create Turso connector: %w", err)
	}

	db := sql.OpenDB(connector)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping Turso database: %w", err)
	}

	return db, nil
}

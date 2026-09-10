package database

import (
	"context"
	"strings"
	"testing"
)

func TestOpenTursoRejectsUnsupportedURLWithoutLeakingToken(t *testing.T) {
	const secret = "do-not-leak-this-token"

	db, err := OpenTurso(context.Background(), "postgres://example.com/database", secret)
	if db != nil {
		_ = db.Close()
		t.Fatal("expected no database handle")
	}
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("error contains the database auth token")
	}
}

package database

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestOpenRejectsEmptyURL(t *testing.T) {
	db, err := Open(context.Background(), "")
	if db != nil {
		_ = db.Close()
		t.Fatal("expected nil db handle")
	}
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestOpenRejectsUnreachableHostWithoutLeakingPassword(t *testing.T) {
	const secret = "my-secret-password-123"
	connStr := "postgres://user:" + secret + "@127.0.0.1:59999/nonexistent?sslmode=disable&connect_timeout=1"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	db, err := Open(ctx, connStr)
	if db != nil {
		_ = db.Close()
		t.Fatal("expected no database handle for unreachable host")
	}
	if err == nil {
		t.Fatal("expected connection error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("error leaked password")
	}
}

package auth

import (
	"testing"
	"time"
)

func TestPasswordHashing(t *testing.T) {
	password := "Password123!"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error hashing password: %v", err)
	}

	if hash == password {
		t.Fatalf("expected hashed password to differ from plain text")
	}

	if !CheckPasswordHash(password, hash) {
		t.Fatalf("expected password verification to succeed")
	}

	if CheckPasswordHash("WrongPassword!", hash) {
		t.Fatalf("expected wrong password verification to fail")
	}
}

func TestJWTTokenManager(t *testing.T) {
	secret := "test-secret-key-12345"
	expiry := 1 * time.Hour
	tm := NewTokenManager(secret, expiry)

	userID := "usr_test123"
	email := "test@mayora.com"
	name := "Test User"
	empNo := "EMP999"
	isMaster := true
	roles := []string{"MASTER_ADMIN"}
	perms := []string{"user:read", "user:write"}

	tokenStr, exp, err := tm.GenerateToken(userID, email, name, empNo, isMaster, roles, perms)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	if tokenStr == "" {
		t.Fatalf("expected token string to be non-empty")
	}

	if exp.Before(time.Now()) {
		t.Fatalf("expected token expiration to be in the future")
	}

	claims, err := tm.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("unexpected error validating token: %v", err)
	}

	if claims.UserID != userID || claims.Email != email || !claims.IsMaster {
		t.Fatalf("claims mismatch: got %+v", claims)
	}

	// Test invalid token
	invalidTM := NewTokenManager("different-secret-key", expiry)
	_, err = invalidTM.ValidateToken(tokenStr)
	if err == nil {
		t.Fatalf("expected token validation to fail with wrong secret key")
	}
}

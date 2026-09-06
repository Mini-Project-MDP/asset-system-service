package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// UserClaims holds JWT claim data.
type UserClaims struct {
	UserID      string   `json:"user_id"`
	Email       string   `json:"email"`
	Name        string   `json:"name"`
	EmployeeNo  string   `json:"employee_no"`
	IsMaster    bool     `json:"is_master"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	jwt.RegisteredClaims
}

// TokenManager handles JWT token creation and validation.
type TokenManager struct {
	secretKey []byte
	expiry    time.Duration
}

// NewTokenManager initializes a TokenManager.
func NewTokenManager(secretKey string, expiry time.Duration) *TokenManager {
	return &TokenManager{
		secretKey: []byte(secretKey),
		expiry:    expiry,
	}
}

// GenerateToken generates a signed JWT token string for a user.
func (tm *TokenManager) GenerateToken(userID, email, name, employeeNo string, isMaster bool, roles, permissions []string) (string, time.Time, error) {
	expirationTime := time.Now().Add(tm.expiry)
	claims := &UserClaims{
		UserID:      userID,
		Email:       email,
		Name:        name,
		EmployeeNo:  employeeNo,
		IsMaster:    isMaster,
		Roles:       roles,
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(tm.secretKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign JWT token: %w", err)
	}

	return tokenString, expirationTime, nil
}

// ValidateToken parses and validates a JWT token string.
func (tm *TokenManager) ValidateToken(tokenString string) (*UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return tm.secretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*UserClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	return claims, nil
}

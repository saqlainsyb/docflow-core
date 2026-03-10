package utils

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the payload inside every JWT we issue.
// All three token types share the same claims structure.
type Claims struct {
	UserID     string `json:"user_id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	TokenType  string `json:"token_type"`            // "access" | "refresh" | "document"
	DocumentID string `json:"document_id,omitempty"` // only set on document tokens
	Color      string `json:"color,omitempty"`       // only set on document tokens
	jwt.RegisteredClaims
}

// GenerateAccessToken creates a short-lived JWT for API authentication.
// Expires in whatever duration is configured (default 15 minutes).
func GenerateAccessToken(userID, email, name, secret string, expiry time.Duration) (string, error) {
	claims := Claims{
		UserID:    userID,
		Email:     email,
		Name:      name,
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GenerateDocumentToken creates a short-lived JWT scoped to a single document.
// Used to authenticate WebSocket connections to a document room.
// Signed with a separate secret from access tokens.
func GenerateDocumentToken(userID, documentID, color, secret string, expiry time.Duration) (string, error) {
	claims := Claims{
		UserID:     userID,
		TokenType:  "document",
		DocumentID: documentID,
		Color:      color,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateToken parses and validates a JWT using the provided secret.
// Returns the claims if valid, or an error if the token is expired,
// has an invalid signature, or is malformed.
func ValidateToken(tokenString, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		func(token *jwt.Token) (interface{}, error) {
			// make sure the signing method is what we expect
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(secret), nil
		},
	)

	if err != nil {
		// surface expiry as a distinct error so the middleware
		// can return TOKEN_EXPIRED instead of a generic 401
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// sentinel errors — checked by middleware and handlers
var (
	ErrTokenExpired = errors.New("token expired")
	ErrInvalidToken = errors.New("invalid token")
)
package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword takes a plain text password and returns a bcrypt hash.
// Cost factor 12 means it takes ~250ms — intentionally slow to make
// brute force attacks impractical.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CheckPassword compares a plain text password against a bcrypt hash.
// Returns nil if they match, an error if they don't.
// This is constant-time — it takes the same amount of time regardless
// of whether the password is correct or not, preventing timing attacks.
func CheckPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// GenerateRefreshToken creates a cryptographically random 32-byte token
// encoded as a hex string. This is the raw token returned to the client.
func GenerateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// HashToken takes a raw token string and returns its SHA-256 hash as hex.
// We store the hash in the database, never the raw token.
// When validating, we hash what the client sent and compare against stored hashes.
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
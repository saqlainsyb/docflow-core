// internal/utils/validator.go
package utils

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

// emailRegex matches valid email addresses.
// This is a pragmatic pattern — strict enough for real addresses,
// not so strict it rejects valid edge cases.
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// ValidateEmail returns true if the string is a valid email address.
// Used in the auth service in addition to Gin's binding tag as a
// second layer of validation with a consistent pattern.
func ValidateEmail(email string) bool {
	return emailRegex.MatchString(strings.TrimSpace(email))
}

// ValidatePassword checks that a password meets the minimum requirements:
//   - At least 8 characters
//   - At least one digit (0-9)
//   - At least one special character (!@#$%^&* etc.)
//
// Called in the auth service during registration.
// Gin's binding tag only enforces min=8 — the digit and special char
// checks are enforced here at the service layer.
func ValidatePassword(password string) (ok bool, reason string) {
	if len(password) < 8 {
		return false, "password must be at least 8 characters"
	}

	var hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSpecial = true
		}
	}

	if !hasDigit {
		return false, "password must contain at least one digit"
	}

	if !hasSpecial {
		return false, "password must contain at least one special character"
	}

	return true, ""
}

// IsValidUUID returns true if the string is a valid UUID v4.
// Used by middleware before hitting the database with a garbage ID.
func IsValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
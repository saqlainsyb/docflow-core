package repositories

import "errors"

// ErrNotFound is returned when a database query finds no matching row.
// Repositories return this instead of pgx.ErrNoRows so the rest of the
// application never needs to import pgx just to check for a missing record.
var ErrNotFound = errors.New("record not found")
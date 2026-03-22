package repositories

import (
	"context"
	"errors"
	
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saqlainsyb/docflow-core/internal/models"
)

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

// FindByID looks up a user by their UUID.
// Returns ErrNotFound if no user exists with that ID.
func (r *UserRepository) FindByID(ctx context.Context, id string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, name, avatar_url, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	user := &models.User{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Name,
		&user.AvatarURL,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return user, nil
}

// FindByEmail looks up a user by their email address.
// Used during login to find the user before checking their password.
// Returns ErrNotFound if no user exists with that email.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, name, avatar_url, created_at, updated_at
		FROM users
		WHERE email = $1
	`

	user := &models.User{}
	err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Name,
		&user.AvatarURL,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return user, nil
}

// Create inserts a new user into the database.
// The password must already be hashed before calling this —
// this method stores whatever hash it receives, no hashing happens here.
func (r *UserRepository) Create(ctx context.Context, email, passwordHash, name string) (*models.User, error) {
	query := `
		INSERT INTO users (email, password_hash, name)
		VALUES ($1, $2, $3)
		RETURNING id, email, password_hash, name, avatar_url, created_at, updated_at
	`

	user := &models.User{}
	err := r.db.QueryRow(ctx, query, email, passwordHash, name).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Name,
		&user.AvatarURL,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// Update changes a user's name and/or avatar_url.
// Uses COALESCE so nil fields are left unchanged — only provided fields are updated.
// Returns the updated user as a UserPublic DTO (no password hash).
func (r *UserRepository) Update(ctx context.Context, id string, name, avatarURL *string) (*models.UserPublic, error) {
	user := &models.UserPublic{}
	err := r.db.QueryRow(ctx, `
		UPDATE users
		SET
			name       = COALESCE($1, name),
			avatar_url = COALESCE($2, avatar_url),
			updated_at = NOW()
		WHERE id = $3
		RETURNING id, email, name, avatar_url, created_at
	`, name, avatarURL, id).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.AvatarURL,
		&user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return user, nil
}

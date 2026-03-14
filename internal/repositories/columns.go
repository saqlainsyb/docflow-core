package repositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saqlainsyb/docflow-core/internal/models"
)

type ColumnRepository struct {
	db *pgxpool.Pool
}

func NewColumnRepository(db *pgxpool.Pool) *ColumnRepository {
	return &ColumnRepository{db: db}
}

// Create inserts a new column into a board at the given position.
func (r *ColumnRepository) Create(ctx context.Context, boardID, title string, position float64) (*models.Column, error) {
	col := &models.Column{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO columns (board_id, title, position)
		VALUES ($1, $2, $3)
		RETURNING id, board_id, title, position, created_at, updated_at
	`, boardID, title, position).Scan(
		&col.ID,
		&col.BoardID,
		&col.Title,
		&col.Position,
		&col.CreatedAt,
		&col.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return col, nil
}

// FindByID looks up a single column by UUID.
// Returns ErrNotFound if no column exists with that ID.
func (r *ColumnRepository) FindByID(ctx context.Context, id string) (*models.Column, error) {
	col := &models.Column{}
	err := r.db.QueryRow(ctx, `
		SELECT id, board_id, title, position, created_at, updated_at
		FROM columns
		WHERE id = $1
	`, id).Scan(
		&col.ID,
		&col.BoardID,
		&col.Title,
		&col.Position,
		&col.CreatedAt,
		&col.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return col, nil
}

// FindByBoard returns all columns for a board ordered by position ascending.
func (r *ColumnRepository) FindByBoard(ctx context.Context, boardID string) ([]models.Column, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, board_id, title, position, created_at, updated_at
		FROM columns
		WHERE board_id = $1
		ORDER BY position ASC
	`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []models.Column
	for rows.Next() {
		var col models.Column
		if err := rows.Scan(
			&col.ID,
			&col.BoardID,
			&col.Title,
			&col.Position,
			&col.CreatedAt,
			&col.UpdatedAt,
		); err != nil {
			return nil, err
		}
		columns = append(columns, col)
	}

	return columns, rows.Err()
}

// Update changes a column's title and/or position.
// Only updates fields that are provided — nil fields are left unchanged.
func (r *ColumnRepository) Update(ctx context.Context, id string, title *string, position *float64) (*models.Column, error) {
	col := &models.Column{}
	err := r.db.QueryRow(ctx, `
		UPDATE columns
		SET
			title      = COALESCE($1, title),
			position   = COALESCE($2, position),
			updated_at = NOW()
		WHERE id = $3
		RETURNING id, board_id, title, position, created_at, updated_at
	`, title, position, id).Scan(
		&col.ID,
		&col.BoardID,
		&col.Title,
		&col.Position,
		&col.CreatedAt,
		&col.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return col, nil
}

// Delete removes a column and all its cards by cascade.
// The ON DELETE CASCADE on cards.column_id handles card deletion.
// Documents and document_updates cascade from cards automatically.
func (r *ColumnRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.Exec(ctx, `DELETE FROM columns WHERE id = $1`, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// GetMaxPosition returns the highest position value among all columns
// on a board. Returns 0 if the board has no columns yet.
// Used to compute the position for a newly appended column:
// new position = max + 1000
func (r *ColumnRepository) GetMaxPosition(ctx context.Context, boardID string) (float64, error) {
	var max float64
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(MAX(position), 0)
		FROM columns
		WHERE board_id = $1
	`, boardID).Scan(&max)

	return max, err
}
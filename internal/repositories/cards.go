package repositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saqlainsyb/docflow-core/internal/models"
)

type CardRepository struct {
	db *pgxpool.Pool
}

func NewCardRepository(db *pgxpool.Pool) *CardRepository {
	return &CardRepository{db: db}
}

// CreateWithDocument inserts a card and its associated document in a single
// transaction. Every card gets exactly one document — this is enforced at
// both the DB level (UNIQUE on card_id) and here at the application level.
// If either insert fails, both are rolled back.
func (r *CardRepository) CreateWithDocument(ctx context.Context, boardID, columnID, title, createdBy string, position float64, color *string) (*models.Card, string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback(ctx)

	card := &models.Card{}
	err = tx.QueryRow(ctx, `
		INSERT INTO cards (board_id, column_id, title, position, color, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, board_id, column_id, title, position, color, assignee_id, archived, created_by, created_at, updated_at
	`, boardID, columnID, title, position, color, createdBy).Scan(
		&card.ID,
		&card.BoardID,
		&card.ColumnID,
		&card.Title,
		&card.Position,
		&card.Color,
		&card.AssigneeID,
		&card.Archived,
		&card.CreatedBy,
		&card.CreatedAt,
		&card.UpdatedAt,
	)
	if err != nil {
		return nil, "", err
	}

	// auto-create the document for this card
	var documentID string
	err = tx.QueryRow(ctx, `
		INSERT INTO documents (card_id)
		VALUES ($1)
		RETURNING id
	`, card.ID).Scan(&documentID)
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", err
	}

	return card, documentID, nil
}

// FindByID looks up a single card by UUID.
// Returns ErrNotFound if no card exists with that ID.
func (r *CardRepository) FindByID(ctx context.Context, id string) (*models.Card, error) {
	card := &models.Card{}
	err := r.db.QueryRow(ctx, `
		SELECT id, board_id, column_id, title, position, color, assignee_id, archived, created_by, created_at, updated_at
		FROM cards
		WHERE id = $1
	`, id).Scan(
		&card.ID,
		&card.BoardID,
		&card.ColumnID,
		&card.Title,
		&card.Position,
		&card.Color,
		&card.AssigneeID,
		&card.Archived,
		&card.CreatedBy,
		&card.CreatedAt,
		&card.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return card, nil
}

// FindByColumn returns all non-archived cards in a column ordered by position.
func (r *CardRepository) FindByColumn(ctx context.Context, columnID string) ([]models.Card, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, board_id, column_id, title, position, color, assignee_id, archived, created_by, created_at, updated_at
		FROM cards
		WHERE column_id = $1 AND archived = FALSE
		ORDER BY position ASC
	`, columnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []models.Card
	for rows.Next() {
		var card models.Card
		if err := rows.Scan(
			&card.ID,
			&card.BoardID,
			&card.ColumnID,
			&card.Title,
			&card.Position,
			&card.Color,
			&card.AssigneeID,
			&card.Archived,
			&card.CreatedBy,
			&card.CreatedAt,
			&card.UpdatedAt,
		); err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}

	return cards, rows.Err()
}

// Update changes a card's title, color, and/or assignee.
// Only updates fields that are provided — nil fields are left unchanged.
func (r *CardRepository) Update(ctx context.Context, id string, title, color, assigneeID *string) (*models.Card, error) {
	card := &models.Card{}
	err := r.db.QueryRow(ctx, `
		UPDATE cards
		SET
			title       = COALESCE($1, title),
			color       = COALESCE($2, color),
			assignee_id = COALESCE($3, assignee_id),
			updated_at  = NOW()
		WHERE id = $4
		RETURNING id, board_id, column_id, title, position, color, assignee_id, archived, created_by, created_at, updated_at
	`, title, color, assigneeID, id).Scan(
		&card.ID,
		&card.BoardID,
		&card.ColumnID,
		&card.Title,
		&card.Position,
		&card.Color,
		&card.AssigneeID,
		&card.Archived,
		&card.CreatedBy,
		&card.CreatedAt,
		&card.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return card, nil
}

// Move updates the column and position of a card in a single statement.
// The caller is responsible for computing the correct position value
// using fractional indexing before calling this.
func (r *CardRepository) Move(ctx context.Context, id, columnID string, position float64) error {
	result, err := r.db.Exec(ctx, `
		UPDATE cards
		SET column_id = $1, position = $2, updated_at = NOW()
		WHERE id = $3
	`, columnID, position, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// Archive sets archived = true on a card (soft delete).
// Archived cards disappear from board loads but can be restored.
func (r *CardRepository) Archive(ctx context.Context, id string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE cards SET archived = TRUE, updated_at = NOW() WHERE id = $1
	`, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// Unarchive restores an archived card.
func (r *CardRepository) Unarchive(ctx context.Context, id string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE cards SET archived = FALSE, updated_at = NOW() WHERE id = $1
	`, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// Delete permanently removes a card and its document via cascade.
func (r *CardRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.Exec(ctx, `DELETE FROM cards WHERE id = $1`, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// GetMaxPositionInColumn returns the highest position among non-archived
// cards in a column. Returns 0 if the column has no cards yet.
// Used to compute the position for a newly appended card: max + 1000.
func (r *CardRepository) GetMaxPositionInColumn(ctx context.Context, columnID string) (float64, error) {
	var max float64
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(MAX(position), 0)
		FROM cards
		WHERE column_id = $1 AND archived = FALSE
	`, columnID).Scan(&max)

	return max, err
}

// GetNeighborPositions returns the positions of the cards immediately above
// and below a given reference position in a column.
// Used by the service to check if the gap is large enough for a new card,
// and to compute the rebalance trigger.
// Returns (below, above) where below < refPos < above.
// If inserting at the start, below = 0.
// If inserting at the end, above = refPos + 1000.
func (r *CardRepository) GetNeighborPositions(ctx context.Context, columnID string, refPos float64) (below float64, above float64, err error) {
	// position immediately below refPos
	err = r.db.QueryRow(ctx, `
		SELECT COALESCE(MAX(position), 0)
		FROM cards
		WHERE column_id = $1 AND archived = FALSE AND position < $2
	`, columnID, refPos).Scan(&below)
	if err != nil {
		return
	}

	// position immediately above refPos
	err = r.db.QueryRow(ctx, `
		SELECT COALESCE(MIN(position), $2 + 1000)
		FROM cards
		WHERE column_id = $1 AND archived = FALSE AND position > $2
	`, columnID, refPos).Scan(&above)

	return
}

// RebalanceColumn reassigns positions for all non-archived cards in a column
// as 1000, 2000, 3000... preserving their current order.
// Called when the gap between neighbors drops below the threshold (0.001).
// All updates run in a single transaction.
func (r *CardRepository) RebalanceColumn(ctx context.Context, columnID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// fetch all non-archived cards in this column ordered by current position
	rows, err := tx.Query(ctx, `
		SELECT id FROM cards
		WHERE column_id = $1 AND archived = FALSE
		ORDER BY position ASC
	`, columnID)
	if err != nil {
		return err
	}

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()

	if err := rows.Err(); err != nil {
		return err
	}

	// reassign clean positions: 1000, 2000, 3000...
	for i, id := range ids {
		newPos := float64((i + 1) * 1000)
		_, err := tx.Exec(ctx, `
			UPDATE cards SET position = $1, updated_at = NOW() WHERE id = $2
		`, newPos, id)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// GetDocumentID returns the document ID associated with a card.
// Used when building CardResponse — document_id is always present.
func (r *CardRepository) GetDocumentID(ctx context.Context, cardID string) (string, error) {
	var documentID string
	err := r.db.QueryRow(ctx, `
		SELECT id FROM documents WHERE card_id = $1
	`, cardID).Scan(&documentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}

	return documentID, nil
}

// GetAssignee returns the assignee UserPublic for a card.
// Returns nil, nil when the card has no assignee (assignee_id IS NULL).
// Returns ErrNotFound only if the card itself does not exist —
// a missing user row (data integrity issue) is treated as no assignee.
//
// This exists so CardService can build a full CardResponse without
// needing a UserRepository dependency injected into it.
func (r *CardRepository) GetAssignee(ctx context.Context, cardID string) (*models.UserPublic, error) {
	var assignee models.UserPublic
	var avatarURL *string
 
	err := r.db.QueryRow(ctx, `
		SELECT u.id, u.email, u.name, u.avatar_url, u.created_at
		FROM cards c
		JOIN users u ON u.id = c.assignee_id
		WHERE c.id = $1
		  AND c.assignee_id IS NOT NULL
	`, cardID).Scan(
		&assignee.ID,
		&assignee.Email,
		&assignee.Name,
		&avatarURL,
		&assignee.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Card has no assignee (or card doesn't exist — FindByID
			// should have caught the latter already).
			return nil, nil
		}
		return nil, err
	}
 
	assignee.AvatarURL = avatarURL
	return &assignee, nil
}
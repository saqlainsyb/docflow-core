package repositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saqlainsyb/docflow-core/internal/models"
)

type DocumentRepository struct {
	db *pgxpool.Pool
}

func NewDocumentRepository(db *pgxpool.Pool) *DocumentRepository {
	return &DocumentRepository{db: db}
}

// FindByID looks up a document by its own UUID.
// Returns ErrNotFound if no document exists with that ID.
func (r *DocumentRepository) FindByID(ctx context.Context, id string) (*models.Document, error) {
	doc := &models.Document{}
	err := r.db.QueryRow(ctx, `
		SELECT id, card_id, snapshot, snapshot_clock, created_at, updated_at
		FROM documents
		WHERE id = $1
	`, id).Scan(
		&doc.ID,
		&doc.CardID,
		&doc.Snapshot,
		&doc.SnapshotClock,
		&doc.CreatedAt,
		&doc.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return doc, nil
}

// FindByCardID looks up the document associated with a card.
// Returns ErrNotFound if no document exists for that card.
func (r *DocumentRepository) FindByCardID(ctx context.Context, cardID string) (*models.Document, error) {
	doc := &models.Document{}
	err := r.db.QueryRow(ctx, `
		SELECT id, card_id, snapshot, snapshot_clock, created_at, updated_at
		FROM documents
		WHERE card_id = $1
	`, cardID).Scan(
		&doc.ID,
		&doc.CardID,
		&doc.Snapshot,
		&doc.SnapshotClock,
		&doc.CreatedAt,
		&doc.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return doc, nil
}

// IncrementClock atomically increments the document clock and inserts a new
// update row in a single transaction.
// Uses SELECT FOR UPDATE to prevent duplicate clock values under concurrency —
// two simultaneous writes cannot get the same clock value.
// Returns the new clock value so the hub can track it.
func (r *DocumentRepository) IncrementClock(ctx context.Context, documentID string, updateData []byte) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// lock the document row — prevents concurrent writes getting the same clock
	var currentClock int
	err = tx.QueryRow(ctx, `
		SELECT snapshot_clock FROM documents WHERE id = $1 FOR UPDATE
	`, documentID).Scan(&currentClock)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}

	newClock := currentClock + 1

	// update the document clock
	_, err = tx.Exec(ctx, `
		UPDATE documents SET snapshot_clock = $1, updated_at = NOW() WHERE id = $2
	`, newClock, documentID)
	if err != nil {
		return 0, err
	}

	// append the update
	_, err = tx.Exec(ctx, `
		INSERT INTO document_updates (document_id, update_data, clock)
		VALUES ($1, $2, $3)
	`, documentID, updateData, newClock)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	return newClock, nil
}

// GetUpdatesSinceClock returns all updates with clock > the given value.
// Called on reconnect — the client sends its last known clock and we
// return everything it missed.
func (r *DocumentRepository) GetUpdatesSinceClock(ctx context.Context, documentID string, clock int) ([]models.DocumentUpdate, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, document_id, update_data, clock, created_at
		FROM document_updates
		WHERE document_id = $1 AND clock > $2
		ORDER BY clock ASC
	`, documentID, clock)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var updates []models.DocumentUpdate
	for rows.Next() {
		var u models.DocumentUpdate
		if err := rows.Scan(&u.ID, &u.DocumentID, &u.UpdateData, &u.Clock, &u.CreatedAt); err != nil {
			return nil, err
		}
		updates = append(updates, u)
	}

	return updates, rows.Err()
}

// GetSnapshot returns the latest compacted snapshot and its clock value.
// Snapshot may be nil if the document has never been compacted.
// The WebSocket hub uses clock to know which updates still need to be
// sent to a reconnecting client on top of the snapshot.
func (r *DocumentRepository) GetSnapshot(ctx context.Context, documentID string) ([]byte, int, error) {
	var snapshot []byte
	var clock int

	err := r.db.QueryRow(ctx, `
		SELECT snapshot, snapshot_clock FROM documents WHERE id = $1
	`, documentID).Scan(&snapshot, &clock)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}

	return snapshot, clock, nil
}

// UpdateSnapshot compacts all updates up to the given clock into a single
// snapshot binary and deletes the individual update rows.
// Updates with clock > snapshotClock are untouched — safe to run while
// live edits are coming in.
// All operations run in a single transaction.
func (r *DocumentRepository) UpdateSnapshot(ctx context.Context, documentID string, snapshot []byte, snapshotClock int) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE documents
		SET snapshot = $1, snapshot_clock = $2, updated_at = NOW()
		WHERE id = $3
	`, snapshot, snapshotClock, documentID)
	if err != nil {
		return err
	}

	// delete only the updates we compacted — not any that arrived after
	_, err = tx.Exec(ctx, `
		DELETE FROM document_updates
		WHERE document_id = $1 AND clock <= $2
	`, documentID, snapshotClock)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saqlainsyb/docflow-core/internal/models"
)

type BoardRepository struct {
	db *pgxpool.Pool
}

func NewBoardRepository(db *pgxpool.Pool) *BoardRepository {
	return &BoardRepository{db: db}
}

// Create inserts a new workspace-visibility board.
// No transaction needed — single insert.
func (r *BoardRepository) Create(ctx context.Context, workspaceID, title, visibility, createdBy string) (*models.Board, error) {
	board := &models.Board{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO boards (workspace_id, title, visibility, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, workspace_id, title, visibility, share_token, created_by, created_at, updated_at
	`, workspaceID, title, visibility, createdBy).Scan(
		&board.ID,
		&board.WorkspaceID,
		&board.Title,
		&board.Visibility,
		&board.ShareToken,
		&board.CreatedBy,
		&board.CreatedAt,
		&board.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return board, nil
}

// CreatePrivate inserts a private board and adds the creator to board_members
// in a single transaction. If either insert fails, both are rolled back —
// preventing a private board with no members.
func (r *BoardRepository) CreatePrivate(ctx context.Context, workspaceID, title, createdBy string) (*models.Board, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	board := &models.Board{}
	err = tx.QueryRow(ctx, `
		INSERT INTO boards (workspace_id, title, visibility, created_by)
		VALUES ($1, $2, 'private', $3)
		RETURNING id, workspace_id, title, visibility, share_token, created_by, created_at, updated_at
	`, workspaceID, title, createdBy).Scan(
		&board.ID,
		&board.WorkspaceID,
		&board.Title,
		&board.Visibility,
		&board.ShareToken,
		&board.CreatedBy,
		&board.CreatedAt,
		&board.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO board_members (board_id, user_id)
		VALUES ($1, $2)
	`, board.ID, createdBy)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return board, nil
}

// FindByID looks up a board by UUID.
// Returns ErrNotFound if no board exists with that ID.
func (r *BoardRepository) FindByID(ctx context.Context, id string) (*models.Board, error) {
	board := &models.Board{}
	err := r.db.QueryRow(ctx, `
		SELECT id, workspace_id, title, visibility, share_token, created_by, created_at, updated_at
		FROM boards
		WHERE id = $1
	`, id).Scan(
		&board.ID,
		&board.WorkspaceID,
		&board.Title,
		&board.Visibility,
		&board.ShareToken,
		&board.CreatedBy,
		&board.CreatedAt,
		&board.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return board, nil
}

// FindByWorkspace returns all boards in a workspace visible to the given user.
// Workspace-visibility boards: visible to all workspace members.
// Private boards: only visible if the user is an explicit board member,
// or if they are an owner/admin (checked in the service layer via role).
// We return all boards here and let the service filter by role if needed —
// but we do filter out private boards the user has no membership in.
func (r *BoardRepository) FindByWorkspace(ctx context.Context, workspaceID, userID string) ([]models.BoardResponse, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			b.id,
			b.workspace_id,
			b.title,
			b.visibility,
			b.created_at,
			(SELECT COUNT(*) FROM board_members bm WHERE bm.board_id = b.id) AS member_count,
			(SELECT COUNT(*) FROM cards c WHERE c.board_id = b.id AND c.archived = FALSE) AS card_count
		FROM boards b
		WHERE b.workspace_id = $1
		  AND (
		    b.visibility = 'workspace'
		    OR EXISTS (
		      SELECT 1 FROM board_members bm
		      WHERE bm.board_id = b.id AND bm.user_id = $2
		    )
		    OR EXISTS (
		      SELECT 1 FROM workspace_members wm
		      WHERE wm.workspace_id = $1 AND wm.user_id = $2 AND wm.role IN ('owner', 'admin')
		    )
		  )
		ORDER BY b.created_at ASC
	`, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var boards []models.BoardResponse
	for rows.Next() {
		var b models.BoardResponse
		if err := rows.Scan(
			&b.ID,
			&b.WorkspaceID,
			&b.Title,
			&b.Visibility,
			&b.CreatedAt,
			&b.MemberCount,
			&b.CardCount,
		); err != nil {
			return nil, err
		}
		boards = append(boards, b)
	}

	return boards, rows.Err()
}

// Update changes a board's title and/or visibility.
// Only updates fields that are provided — nil fields are left unchanged.
func (r *BoardRepository) Update(ctx context.Context, id string, title, visibility *string) (*models.Board, error) {
	board := &models.Board{}
	err := r.db.QueryRow(ctx, `
		UPDATE boards
		SET
			title      = COALESCE($1, title),
			visibility = COALESCE($2, visibility),
			updated_at = NOW()
		WHERE id = $3
		RETURNING id, workspace_id, title, visibility, share_token, created_by, created_at, updated_at
	`, title, visibility, id).Scan(
		&board.ID,
		&board.WorkspaceID,
		&board.Title,
		&board.Visibility,
		&board.ShareToken,
		&board.CreatedBy,
		&board.CreatedAt,
		&board.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return board, nil
}

// Delete removes a board. Cascading FK constraints handle columns, cards,
// documents, document_updates, and board_members automatically.
func (r *BoardRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.Exec(ctx, `DELETE FROM boards WHERE id = $1`, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// GetBoardDetail returns the full board with nested columns and cards.
// Columns and cards are fetched in a single JOIN query ordered by position.
// The flat rows are assembled into the nested ColumnWithCards structure in Go.
// Cards include the assignee (nullable) and document_id.
func (r *BoardRepository) GetBoardDetail(ctx context.Context, boardID string) (*models.BoardDetailResponse, error) {
	// first fetch the board itself
	board, err := r.FindByID(ctx, boardID)
	if err != nil {
		return nil, err
	}

	// fetch board members
	memberRows, err := r.db.Query(ctx, `
		SELECT u.id, u.name, u.email, u.avatar_url, wm.role, wm.joined_at
		FROM board_members bm
		JOIN users u ON u.id = bm.user_id
		JOIN workspace_members wm ON wm.user_id = bm.user_id AND wm.workspace_id = $1
		WHERE bm.board_id = $2
		ORDER BY bm.added_at ASC
	`, board.WorkspaceID, boardID)
	if err != nil {
		return nil, err
	}
	defer memberRows.Close()

	var members []models.MemberResponse
	for memberRows.Next() {
		var m models.MemberResponse
		if err := memberRows.Scan(&m.UserID, &m.Name, &m.Email, &m.AvatarURL, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	if err := memberRows.Err(); err != nil {
		return nil, err
	}

	// fetch columns + cards in one query ordered by position
	// LEFT JOIN means columns with no cards still appear
	rows, err := r.db.Query(ctx, `
		SELECT
			col.id,
			col.title,
			col.position,
			col.created_at,
			c.id,
			c.title,
			c.position,
			c.color,
			c.archived,
			c.created_at,
			c.assignee_id,
			u.name,
			u.email,
			u.avatar_url,
			d.id
		FROM columns col
		LEFT JOIN cards c
			ON c.column_id = col.id AND c.archived = FALSE
		LEFT JOIN users u
			ON u.id = c.assignee_id
		LEFT JOIN documents d
			ON d.card_id = c.id
		WHERE col.board_id = $1
		ORDER BY col.position ASC, c.position ASC
	`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// assemble flat rows into nested columns → cards
	columnMap := make(map[string]*models.ColumnWithCards)
	columnOrder := []string{} // preserve position order

	for rows.Next() {
		var (
			colID, colTitle string
			colPos          float64
			colCreatedAt    time.Time

			// card fields — all nullable because of LEFT JOIN
			cardID         *string
			cardTitle      *string
			cardPos        *float64
			cardColor      *string
			cardArchived   *bool
			cardCreatedAt  *time.Time
			assigneeID     *string
			assigneeName   *string
			assigneeEmail  *string
			assigneeAvatar *string
			documentID     *string
		)

		if err := rows.Scan(
			&colID, &colTitle, &colPos, &colCreatedAt,
			&cardID, &cardTitle, &cardPos, &cardColor, &cardArchived, &cardCreatedAt,
			&assigneeID, &assigneeName, &assigneeEmail, &assigneeAvatar,
			&documentID,
		); err != nil {
			return nil, err
		}

		// register column if first time seeing it
		if _, exists := columnMap[colID]; !exists {
			columnMap[colID] = &models.ColumnWithCards{
				ID:        colID,
				BoardID:   boardID,
				Title:     colTitle,
				Position:  colPos,
				CreatedAt: colCreatedAt,
				Cards:     []models.CardResponse{},
			}
			columnOrder = append(columnOrder, colID)
		}

		// only add a card row if a card actually exists (LEFT JOIN)
		if cardID == nil {
			continue
		}

		card := models.CardResponse{
			ID:       *cardID,
			BoardID:  boardID,
			ColumnID: colID,
			Title:    *cardTitle,
			Position: *cardPos,
			Color:    cardColor,
			Archived: *cardArchived,
		}

		if cardCreatedAt != nil {
			card.CreatedAt = *cardCreatedAt
		}

		if documentID != nil {
			card.DocumentID = *documentID
		}

		if assigneeID != nil {
			card.Assignee = &models.UserPublic{
				ID:        *assigneeID,
				Name:      *assigneeName,
				Email:     *assigneeEmail,
				AvatarURL: assigneeAvatar,
			}
		}

		columnMap[colID].Cards = append(columnMap[colID].Cards, card)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// build ordered slice from map
	columns := make([]models.ColumnWithCards, 0, len(columnOrder))
	for _, id := range columnOrder {
		columns = append(columns, *columnMap[id])
	}

	if members == nil {
		members = []models.MemberResponse{}
	}

	return &models.BoardDetailResponse{
		ID:          board.ID,
		WorkspaceID: board.WorkspaceID,
		Title:       board.Title,
		Visibility:  board.Visibility,
		Columns:     columns,
		Members:     members,
		CreatedAt:   board.CreatedAt,
	}, nil
}

// FindByShareToken looks up a board by its public share token.
// Returns ErrNotFound if the token doesn't exist or has been revoked (set to NULL).
func (r *BoardRepository) FindByShareToken(ctx context.Context, token string) (*models.Board, error) {
	board := &models.Board{}
	err := r.db.QueryRow(ctx, `
		SELECT id, workspace_id, title, visibility, share_token, created_by, created_at, updated_at
		FROM boards
		WHERE share_token = $1
	`, token).Scan(
		&board.ID,
		&board.WorkspaceID,
		&board.Title,
		&board.Visibility,
		&board.ShareToken,
		&board.CreatedBy,
		&board.CreatedAt,
		&board.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return board, nil
}

// SetShareToken sets or clears the share token on a board.
// Pass nil to revoke (sets share_token = NULL).
func (r *BoardRepository) SetShareToken(ctx context.Context, boardID string, token *string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE boards SET share_token = $1, updated_at = NOW() WHERE id = $2
	`, token, boardID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// AddBoardMember inserts an explicit board membership row.
// Used for private boards — workspace-visibility boards don't need this.
func (r *BoardRepository) AddBoardMember(ctx context.Context, boardID, userID string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO board_members (board_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, boardID, userID)

	return err
}

// RemoveBoardMember deletes an explicit board membership row.
func (r *BoardRepository) RemoveBoardMember(ctx context.Context, boardID, userID string) error {
	result, err := r.db.Exec(ctx, `
		DELETE FROM board_members WHERE board_id = $1 AND user_id = $2
	`, boardID, userID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// IsBoardMember returns true if the user has an explicit board_members row.
// Used to check access for private boards.
func (r *BoardRepository) IsBoardMember(ctx context.Context, boardID, userID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM board_members
			WHERE board_id = $1 AND user_id = $2
		)
	`, boardID, userID).Scan(&exists)

	return exists, err
}

// ListBoardMembers returns all explicit members of a board with their details.
func (r *BoardRepository) ListBoardMembers(ctx context.Context, boardID, workspaceID string) ([]models.MemberResponse, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.name, u.email, u.avatar_url, wm.role, bm.added_at
		FROM board_members bm
		JOIN users u ON u.id = bm.user_id
		JOIN workspace_members wm ON wm.user_id = bm.user_id AND wm.workspace_id = $1
		WHERE bm.board_id = $2
		ORDER BY bm.added_at ASC
	`, workspaceID, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.MemberResponse
	for rows.Next() {
		var m models.MemberResponse
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &m.AvatarURL, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}

	return members, rows.Err()
}

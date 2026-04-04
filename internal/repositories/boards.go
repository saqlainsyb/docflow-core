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

// ── Board CRUD ────────────────────────────────────────────────────────────────

// Create inserts a new board and adds the creator as 'owner' in board_members,
// both in a single transaction. This applies regardless of visibility — every
// board always has exactly one owner row in board_members.
func (r *BoardRepository) Create(ctx context.Context, workspaceID, title, visibility, createdBy string) (*models.Board, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	board := &models.Board{}
	err = tx.QueryRow(ctx, `
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

	// Always insert creator as board owner.
	// For private boards this is the access gate; for workspace-visibility
	// boards it still records who owns/controls the board.
	_, err = tx.Exec(ctx, `
		INSERT INTO board_members (board_id, user_id, role)
		VALUES ($1, $2, 'owner')
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
// Workspace-visibility boards are visible to all workspace members.
// Private boards are visible only to explicit board_members or workspace owner/admin.
func (r *BoardRepository) FindByWorkspace(ctx context.Context, workspaceID, userID string) ([]models.BoardResponse, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			b.id,
			b.workspace_id,
			b.title,
			b.visibility,
			b.created_at,
			(SELECT COUNT(*) FROM board_members bm WHERE bm.board_id = b.id) AS member_count,
			(SELECT COUNT(*) FROM cards c WHERE c.board_id = b.id AND c.archived = FALSE) AS card_count,
			b.updated_at
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
			&b.UpdatedAt,
		); err != nil {
			return nil, err
		}
		boards = append(boards, b)
	}

	return boards, rows.Err()
}

// Update changes a board's title and/or visibility.
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

// GetBoardDetail returns the full board with nested columns, cards, and members.
func (r *BoardRepository) GetBoardDetail(ctx context.Context, boardID string) (*models.BoardDetailResponse, error) {
	board, err := r.FindByID(ctx, boardID)
	if err != nil {
		return nil, err
	}

	members, err := r.ListBoardMembers(ctx, boardID)
	if err != nil {
		return nil, err
	}

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

	columnMap := make(map[string]*models.ColumnWithCards)
	columnOrder := []string{}

	for rows.Next() {
		var (
			colID, colTitle string
			colPos          float64
			colCreatedAt    time.Time

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

	columns := make([]models.ColumnWithCards, 0, len(columnOrder))
	for _, id := range columnOrder {
		columns = append(columns, *columnMap[id])
	}

	if members == nil {
		members = []models.BoardMember{}
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

// ── Share link ────────────────────────────────────────────────────────────────

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

// ── Board membership ──────────────────────────────────────────────────────────

// GetBoardMemberRole returns the board-level role for a specific user.
// Returns ErrNotFound if the user has no row in board_members.
func (r *BoardRepository) GetBoardMemberRole(ctx context.Context, boardID, userID string) (string, error) {
	var role string
	err := r.db.QueryRow(ctx, `
		SELECT role FROM board_members
		WHERE board_id = $1 AND user_id = $2
	`, boardID, userID).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}

	return role, nil
}

// IsBoardMember returns true if the user has an explicit board_members row.
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

// AddBoardMember inserts an explicit board membership row with the given role.
// ON CONFLICT DO NOTHING — callers should check IsBoardMember first if they
// need to distinguish "already member" from a successful insert.
func (r *BoardRepository) AddBoardMember(ctx context.Context, boardID, userID, role string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO board_members (board_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (board_id, user_id) DO NOTHING
	`, boardID, userID, role)

	return err
}

// UpdateBoardMemberRole changes the board role of an existing member.
// Returns ErrNotFound if the user is not a board member.
func (r *BoardRepository) UpdateBoardMemberRole(ctx context.Context, boardID, userID, role string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE board_members SET role = $1
		WHERE board_id = $2 AND user_id = $3
	`, role, boardID, userID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// RemoveBoardMember deletes an explicit board membership row.
// Returns ErrNotFound if the user is not a board member.
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

// TransferOwnership atomically moves the 'owner' role from one user to another.
// The previous owner is downgraded to 'admin'.
// The new owner must already be a board member.
// Uses a single transaction with row-level locking to prevent races.
func (r *BoardRepository) TransferOwnership(ctx context.Context, boardID, fromUserID, toUserID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Lock both rows upfront to prevent concurrent transfers.
	var fromRole, toRole string
	err = tx.QueryRow(ctx, `
		SELECT role FROM board_members
		WHERE board_id = $1 AND user_id = $2
		FOR UPDATE
	`, boardID, fromUserID).Scan(&fromRole)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	err = tx.QueryRow(ctx, `
		SELECT role FROM board_members
		WHERE board_id = $1 AND user_id = $2
		FOR UPDATE
	`, boardID, toUserID).Scan(&toRole)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound // target user must already be a board member
		}
		return err
	}

	// Downgrade current owner to admin.
	if _, err = tx.Exec(ctx, `
		UPDATE board_members SET role = 'admin'
		WHERE board_id = $1 AND user_id = $2
	`, boardID, fromUserID); err != nil {
		return err
	}

	// Elevate new owner.
	if _, err = tx.Exec(ctx, `
		UPDATE board_members SET role = 'owner'
		WHERE board_id = $1 AND user_id = $2
	`, boardID, toUserID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ListBoardMembers returns all explicit members of a board with their board role.
// The board role (owner/admin/editor) is read directly from board_members —
// it does NOT reflect the workspace role.
func (r *BoardRepository) ListBoardMembers(ctx context.Context, boardID string) ([]models.BoardMember, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.name, u.email, u.avatar_url, bm.role, bm.added_at
		FROM board_members bm
		JOIN users u ON u.id = bm.user_id
		WHERE bm.board_id = $1
		ORDER BY
			CASE bm.role
				WHEN 'owner'  THEN 1
				WHEN 'admin'  THEN 2
				WHEN 'editor' THEN 3
			END,
			bm.added_at ASC
	`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.BoardMember
	for rows.Next() {
		var m models.BoardMember
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &m.AvatarURL, &m.BoardRole, &m.AddedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}

	return members, rows.Err()
}
package services

import (
	"context"
	"errors"

	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
)

type ColumnService struct {
	columnRepo   *repositories.ColumnRepository
	boardService *BoardService
	hub          BoardBroadcaster
}

func NewColumnService(
	columnRepo *repositories.ColumnRepository,
	boardService *BoardService,
	hub BoardBroadcaster,
) *ColumnService {
	return &ColumnService{
		columnRepo:   columnRepo,
		boardService: boardService,
		hub:          hub,
	}
}

// CreateColumn adds a new column to the end of a board.
// Position is computed as max existing position + 1000.
// If the board has no columns yet, position starts at 1000.
func (s *ColumnService) CreateColumn(ctx context.Context, boardID, userID, memberRole string, req models.CreateColumnRequest) (*models.Column, error) {
	if err := s.boardService.checkAccess(ctx, boardID, userID, memberRole); err != nil {
		return nil, err
	}

	maxPos, err := s.columnRepo.GetMaxPosition(ctx, boardID)
	if err != nil {
		return nil, err
	}

	position := maxPos + 1000.0

	col, err := s.columnRepo.Create(ctx, boardID, req.Title, position)
	if err != nil {
		return nil, err
	}

	s.hub.BroadcastToBoard(boardID, map[string]any{
		"type":   "COLUMN_CREATED",
		"column": columnToResponse(col),
	})

	return col, nil
}

// UpdateColumn renames a column and/or repositions it.
// Both fields are optional — only provided fields are updated.
func (s *ColumnService) UpdateColumn(ctx context.Context, columnID, userID, memberRole string, req models.UpdateColumnRequest) (*models.Column, error) {
	// resolve the board this column belongs to
	col, err := s.columnRepo.FindByID(ctx, columnID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if err := s.boardService.checkAccess(ctx, col.BoardID, userID, memberRole); err != nil {
		return nil, err
	}

	updated, err := s.columnRepo.Update(ctx, columnID, req.Title, req.Position)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// broadcast the specific event type that changed —
	// rename and reorder are separate event shapes so the frontend
	// can handle them with targeted UI updates rather than a full reload
	if req.Title != nil && req.Position != nil {
		// both changed — send two events so each subscriber can handle
		// exactly what it cares about
		s.hub.BroadcastToBoard(col.BoardID, map[string]any{
			"type":      "COLUMN_RENAMED",
			"column_id": columnID,
			"title":     *req.Title,
		})
		s.hub.BroadcastToBoard(col.BoardID, map[string]any{
			"type":      "COLUMN_REORDERED",
			"column_id": columnID,
			"position":  *req.Position,
		})
	} else if req.Title != nil {
		s.hub.BroadcastToBoard(col.BoardID, map[string]any{
			"type":      "COLUMN_RENAMED",
			"column_id": columnID,
			"title":     *req.Title,
		})
	} else if req.Position != nil {
		s.hub.BroadcastToBoard(col.BoardID, map[string]any{
			"type":      "COLUMN_REORDERED",
			"column_id": columnID,
			"position":  *req.Position,
		})
	}

	return updated, nil
}

// DeleteColumn removes a column and all its cards.
// Cascading FK constraints in the schema handle cards, documents,
// and document_updates automatically.
func (s *ColumnService) DeleteColumn(ctx context.Context, columnID, userID, memberRole string) error {
	col, err := s.columnRepo.FindByID(ctx, columnID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	if err := s.boardService.checkAccess(ctx, col.BoardID, userID, memberRole); err != nil {
		return err
	}

	// capture boardID before the column is deleted —
	// same pattern as DeleteCard: get the ID you need before the row is gone
	boardID := col.BoardID

	if err := s.columnRepo.Delete(ctx, columnID); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	s.hub.BroadcastToBoard(boardID, map[string]any{
		"type":      "COLUMN_DELETED",
		"column_id": columnID,
	})

	return nil
}

// columnToResponse converts a Column db model into a ColumnResponse DTO.
// Used when broadcasting COLUMN_CREATED so the frontend gets the full
// shape including the server-assigned ID and position.
func columnToResponse(col *models.Column) models.ColumnResponse {
	return models.ColumnResponse{
		ID:        col.ID,
		BoardID:   col.BoardID,
		Title:     col.Title,
		Position:  col.Position,
		CreatedAt: col.CreatedAt,
	}
}
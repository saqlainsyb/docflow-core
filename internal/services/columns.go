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
}

func NewColumnService(
	columnRepo *repositories.ColumnRepository,
	boardService *BoardService,
) *ColumnService {
	return &ColumnService{
		columnRepo:   columnRepo,
		boardService: boardService,
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

	return s.columnRepo.Create(ctx, boardID, req.Title, position)
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

	if err := s.columnRepo.Delete(ctx, columnID); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	return nil
}
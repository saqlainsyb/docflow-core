package services

import (
	"context"
	"errors"

	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

type CardService struct {
	cardRepo     *repositories.CardRepository
	columnRepo   *repositories.ColumnRepository
	boardService *BoardService
}

func NewCardService(
	cardRepo *repositories.CardRepository,
	columnRepo *repositories.ColumnRepository,
	boardService *BoardService,
) *CardService {
	return &CardService{
		cardRepo:     cardRepo,
		columnRepo:   columnRepo,
		boardService: boardService,
	}
}

// CreateCard creates a new card at the end of a column.
// Steps:
// 1. Verify board access
// 2. Verify the column belongs to the board
// 3. Compute position = max position in column + 1000
// 4. Insert card + document in a single transaction
// 5. Return CardResponse (includes document_id)
func (s *CardService) CreateCard(ctx context.Context, columnID, userID, memberRole string, req models.CreateCardRequest) (*models.CardResponse, error) {
	// resolve the column to get its board_id
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

	// compute position — append to end of column
	maxPos, err := s.cardRepo.GetMaxPositionInColumn(ctx, columnID)
	if err != nil {
		return nil, err
	}
	position := maxPos + 1000.0

	card, documentID, err := s.cardRepo.CreateWithDocument(ctx, col.BoardID, columnID, req.Title, userID, position, req.Color)
	if err != nil {
		return nil, err
	}

	resp := cardToResponse(card, documentID, nil)

	// TODO: broadcast CARD_CREATED event to board WebSocket room

	return resp, nil
}

// UpdateCard changes a card's title, color, and/or assignee.
// All fields are optional — only provided fields are updated.
func (s *CardService) UpdateCard(ctx context.Context, cardID, userID, memberRole string, req models.UpdateCardRequest) (*models.CardResponse, error) {
	card, err := s.cardRepo.FindByID(ctx, cardID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if err := s.boardService.checkAccess(ctx, card.BoardID, userID, memberRole); err != nil {
		return nil, err
	}

	updated, err := s.cardRepo.Update(ctx, cardID, req.Title, req.Color, req.AssigneeID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	documentID, err := s.cardRepo.GetDocumentID(ctx, cardID)
	if err != nil {
		return nil, err
	}

	resp := cardToResponse(updated, documentID, nil)

	// TODO: broadcast CARD_UPDATED event to board WebSocket room

	return resp, nil
}

// MoveCard moves a card to a new column and/or position.
// Steps:
// 1. Verify board access
// 2. Verify target column belongs to the same board (prevents cross-board moves)
// 3. Check gap between neighbors — rebalance column if gap too small
// 4. Update card in DB
// 5. Return updated card
func (s *CardService) MoveCard(ctx context.Context, cardID, userID, memberRole string, req models.MoveCardRequest) (*models.CardResponse, error) {
	card, err := s.cardRepo.FindByID(ctx, cardID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if err := s.boardService.checkAccess(ctx, card.BoardID, userID, memberRole); err != nil {
		return nil, err
	}

	// verify target column belongs to the same board — prevents cross-board moves
	targetCol, err := s.columnRepo.FindByID(ctx, req.ColumnID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if targetCol.BoardID != card.BoardID {
		return nil, ErrInsufficientPermissions
	}

	// check the gap around the requested position
	// if too small, rebalance the target column first
	below, above, err := s.cardRepo.GetNeighborPositions(ctx, req.ColumnID, req.Position)
	if err != nil {
		return nil, err
	}

	if utils.NeedsRebalance(below, above) {
		if err := s.cardRepo.RebalanceColumn(ctx, req.ColumnID); err != nil {
			return nil, err
		}
		// after rebalance, positions have shifted — recompute neighbors
		below, above, err = s.cardRepo.GetNeighborPositions(ctx, req.ColumnID, req.Position)
		if err != nil {
			return nil, err
		}
	}

	// use the midpoint of the actual gap rather than the raw requested position —
	// this is safe even if the frontend sent a slightly stale position value
	finalPosition := utils.Between(below, above)

	if err := s.cardRepo.Move(ctx, cardID, req.ColumnID, finalPosition); err != nil {
		return nil, err
	}

	documentID, err := s.cardRepo.GetDocumentID(ctx, cardID)
	if err != nil {
		return nil, err
	}

	// reload card to get updated column_id and position
	moved, err := s.cardRepo.FindByID(ctx, cardID)
	if err != nil {
		return nil, err
	}

	resp := cardToResponse(moved, documentID, nil)

	// TODO: broadcast CARD_MOVED event to board WebSocket room

	return resp, nil
}

// DeleteCard permanently removes a card and its document.
// Document and document_updates are removed via cascade.
func (s *CardService) DeleteCard(ctx context.Context, cardID, userID, memberRole string) error {
	card, err := s.cardRepo.FindByID(ctx, cardID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	if err := s.boardService.checkAccess(ctx, card.BoardID, userID, memberRole); err != nil {
		return err
	}

	if err := s.cardRepo.Delete(ctx, cardID); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	// TODO: broadcast CARD_DELETED event to board WebSocket room

	return nil
}

// ArchiveCard soft-deletes a card — it disappears from board loads
// but can be restored with UnarchiveCard.
func (s *CardService) ArchiveCard(ctx context.Context, cardID, userID, memberRole string) error {
	card, err := s.cardRepo.FindByID(ctx, cardID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	if err := s.boardService.checkAccess(ctx, card.BoardID, userID, memberRole); err != nil {
		return err
	}

	if err := s.cardRepo.Archive(ctx, cardID); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	// TODO: broadcast CARD_ARCHIVED event to board WebSocket room

	return nil
}

// UnarchiveCard restores an archived card back to its column.
func (s *CardService) UnarchiveCard(ctx context.Context, cardID, userID, memberRole string) error {
	card, err := s.cardRepo.FindByID(ctx, cardID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	if err := s.boardService.checkAccess(ctx, card.BoardID, userID, memberRole); err != nil {
		return err
	}

	if err := s.cardRepo.Unarchive(ctx, cardID); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	// TODO: broadcast CARD_UNARCHIVED event to board WebSocket room

	return nil
}

// cardToResponse converts a Card db model into a CardResponse DTO.
// assignee is optional — pass nil if no assignee lookup was done.
// document_id is always required — every card has exactly one document.
func cardToResponse(card *models.Card, documentID string, assignee *models.UserPublic) *models.CardResponse {
	return &models.CardResponse{
		ID:         card.ID,
		BoardID:    card.BoardID,
		ColumnID:   card.ColumnID,
		Title:      card.Title,
		Position:   card.Position,
		Color:      card.Color,
		Assignee:   assignee,
		DocumentID: documentID,
		Archived:   card.Archived,
		CreatedAt:  card.CreatedAt,
	}
}
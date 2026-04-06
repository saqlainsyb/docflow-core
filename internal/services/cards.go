package services

import (
	"context"
	"errors"

	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

// BoardBroadcaster is the narrow interface the card and column services
// need from the WebSocket hub. Defined here in the services package so
// services never import the ws package — that would create a circular
// dependency (ws already imports services for DocumentService).
//
// *ws.Hub satisfies this interface automatically via its BroadcastToBoard
// method — no changes needed in the ws package.
type BoardBroadcaster interface {
	BroadcastToBoard(boardID string, payload any)
}

type CardService struct {
	cardRepo     *repositories.CardRepository
	columnRepo   *repositories.ColumnRepository
	boardService *BoardService
	hub          BoardBroadcaster
}

func NewCardService(
	cardRepo *repositories.CardRepository,
	columnRepo *repositories.ColumnRepository,
	boardService *BoardService,
	hub BoardBroadcaster,
) *CardService {
	return &CardService{
		cardRepo:     cardRepo,
		columnRepo:   columnRepo,
		boardService: boardService,
		hub:          hub,
	}
}

// CreateCard creates a new card at the end of a column.
// Steps:
// 1. Verify board access
// 2. Verify the column belongs to the board
// 3. Compute position = max position in column + 1000
// 4. Insert card + document in a single transaction
// 5. Broadcast CARD_CREATED to the board room
// 6. Return CardResponse (includes document_id)
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

	// broadcast to every tab that has this board open
	s.hub.BroadcastToBoard(col.BoardID, map[string]any{
		"type": "CARD_CREATED",
		"card": resp,
	})

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

	// build a changes object containing only the fields that were sent —
	// the frontend merges this into its local card copy
	changes := map[string]any{}
	if req.Title != nil {
		changes["title"] = *req.Title
	}
	if req.Color != nil {
		changes["color"] = *req.Color
	}
	if req.AssigneeID != nil {
		changes["assignee_id"] = *req.AssigneeID
	}

	s.hub.BroadcastToBoard(card.BoardID, map[string]any{
		"type":    "CARD_UPDATED",
		"card_id": cardID,
		"changes": changes,
	})

	return resp, nil
}

// MoveCard moves a card to a new column and/or position.
// Steps:
// 1. Verify board access
// 2. Verify target column belongs to the same board (prevents cross-board moves)
// 3. Check gap between neighboring positions — rebalance column if gap too small
// 4. Update card in DB
// 5. Broadcast CARD_MOVED to the board room
// 6. Return updated card
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

	s.hub.BroadcastToBoard(card.BoardID, map[string]any{
		"type":      "CARD_MOVED",
		"card_id":   cardID,
		"column_id": req.ColumnID,
		"position":  finalPosition,
	})

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

	// capture boardID before the card is deleted
	boardID := card.BoardID

	if err := s.cardRepo.Delete(ctx, cardID); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	s.hub.BroadcastToBoard(boardID, map[string]any{
		"type":    "CARD_DELETED",
		"card_id": cardID,
	})

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

	s.hub.BroadcastToBoard(card.BoardID, map[string]any{
		"type":    "CARD_ARCHIVED",
		"card_id": cardID,
	})

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

 	// Reload card so archived = false is reflected in the broadcast.
 	restored, err := s.cardRepo.FindByID(ctx, cardID)
 	if err != nil {
 		return err
 	}

 	documentID, err := s.cardRepo.GetDocumentID(ctx, cardID)
 	if err != nil {
 		return err
 	}

 	// Fetch assignee via the card repo so we don't need a userRepo
 	// dependency on CardService.
 	assignee, err := s.cardRepo.GetAssignee(ctx, cardID)
 	if err != nil {
 		return err
 	}

 	resp := cardToResponse(restored, documentID, assignee)

 	s.hub.BroadcastToBoard(card.BoardID, map[string]any{
 		"type": "CARD_UNARCHIVED",
 		"card": resp,
 	})

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
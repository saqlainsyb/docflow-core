package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/services"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

type CardHandler struct {
	cardService *services.CardService
}

func NewCardHandler(cardService *services.CardService) *CardHandler {
	return &CardHandler{cardService: cardService}
}

// CreateCard handles POST /api/v1/columns/:id/cards
// Column middleware has already run — :id is the column ID, member_role in context.
func (h *CardHandler) CreateCard(c *gin.Context) {
	var req models.CreateCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	columnID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	card, err := h.cardService.CreateCard(c.Request.Context(), columnID, userID, memberRole, req)
	if err != nil {
		handleCardError(c, err)
		return
	}

	c.JSON(http.StatusCreated, card)
}

// UpdateCard handles PATCH /api/v1/cards/:id
// Card middleware has already run — member_role in context.
func (h *CardHandler) UpdateCard(c *gin.Context) {
	var req models.UpdateCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	cardID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	card, err := h.cardService.UpdateCard(c.Request.Context(), cardID, userID, memberRole, req)
	if err != nil {
		handleCardError(c, err)
		return
	}

	c.JSON(http.StatusOK, card)
}

// MoveCard handles POST /api/v1/cards/:id/move
// Moves a card to a new column and/or position.
func (h *CardHandler) MoveCard(c *gin.Context) {
	var req models.MoveCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	cardID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	card, err := h.cardService.MoveCard(c.Request.Context(), cardID, userID, memberRole, req)
	if err != nil {
		handleCardError(c, err)
		return
	}

	c.JSON(http.StatusOK, card)
}

// DeleteCard handles DELETE /api/v1/cards/:id
// Permanently removes the card and its document via cascade.
func (h *CardHandler) DeleteCard(c *gin.Context) {
	cardID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	if err := h.cardService.DeleteCard(c.Request.Context(), cardID, userID, memberRole); err != nil {
		handleCardError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ArchiveCard handles POST /api/v1/cards/:id/archive
// Soft-deletes a card — disappears from board loads but recoverable.
func (h *CardHandler) ArchiveCard(c *gin.Context) {
	cardID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	if err := h.cardService.ArchiveCard(c.Request.Context(), cardID, userID, memberRole); err != nil {
		handleCardError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// UnarchiveCard handles POST /api/v1/cards/:id/unarchive
// Restores an archived card back to its column.
func (h *CardHandler) UnarchiveCard(c *gin.Context) {
	cardID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	if err := h.cardService.UnarchiveCard(c.Request.Context(), cardID, userID, memberRole); err != nil {
		handleCardError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// handleCardError maps service errors to the correct HTTP response.
func handleCardError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrNotFound):
		utils.ErrorResponse(c, http.StatusNotFound, "CARD_NOT_FOUND", "card not found")
	case errors.Is(err, services.ErrBoardAccessDenied):
		utils.ErrorResponse(c, http.StatusForbidden, "BOARD_ACCESS_DENIED", "you do not have access to this board")
	case errors.Is(err, services.ErrInsufficientPermissions):
		utils.ErrorResponse(c, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "you do not have permission to perform this action")
	default:
		utils.ErrInternal(c)
	}
}
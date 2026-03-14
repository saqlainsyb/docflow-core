package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/services"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

type BoardHandler struct {
	boardService *services.BoardService
}

func NewBoardHandler(boardService *services.BoardService) *BoardHandler {
	return &BoardHandler{boardService: boardService}
}

// ListBoards handles GET /api/v1/workspaces/:id/boards
// Returns all boards in the workspace visible to the requesting user.
func (h *BoardHandler) ListBoards(c *gin.Context) {
	workspaceID := c.GetString("workspace_id")
	userID := c.GetString("user_id")

	boards, err := h.boardService.ListBoards(c.Request.Context(), workspaceID, userID)
	if err != nil {
		utils.ErrInternal(c)
		return
	}

	if boards == nil {
		boards = []models.BoardResponse{}
	}

	c.JSON(http.StatusOK, boards)
}

// CreateBoard handles POST /api/v1/workspaces/:id/boards
// Any workspace member can create a board.
func (h *BoardHandler) CreateBoard(c *gin.Context) {
	var req models.CreateBoardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	workspaceID := c.GetString("workspace_id")
	userID := c.GetString("user_id")

	board, err := h.boardService.CreateBoard(c.Request.Context(), workspaceID, userID, req)
	if err != nil {
		utils.ErrInternal(c)
		return
	}

	c.JSON(http.StatusCreated, board)
}

// GetBoardDetail handles GET /api/v1/boards/:id
// Returns the full board with nested columns and cards.
// Board ID comes from the URL — not from the workspace middleware.
func (h *BoardHandler) GetBoardDetail(c *gin.Context) {
	boardID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	board, err := h.boardService.GetBoardDetail(c.Request.Context(), boardID, userID, memberRole)
	if err != nil {
		handleBoardError(c, err)
		return
	}

	c.JSON(http.StatusOK, board)
}

// UpdateBoard handles PATCH /api/v1/boards/:id
// Updates title and/or visibility.
func (h *BoardHandler) UpdateBoard(c *gin.Context) {
	var req models.UpdateBoardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	boardID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	board, err := h.boardService.UpdateBoard(c.Request.Context(), boardID, userID, memberRole, req)
	if err != nil {
		handleBoardError(c, err)
		return
	}

	c.JSON(http.StatusOK, board)
}

// DeleteBoard handles DELETE /api/v1/boards/:id
// Requires workspace owner role.
func (h *BoardHandler) DeleteBoard(c *gin.Context) {
	boardID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	if err := h.boardService.DeleteBoard(c.Request.Context(), boardID, userID, memberRole); err != nil {
		handleBoardError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListBoardMembers handles GET /api/v1/boards/:id/members
func (h *BoardHandler) ListBoardMembers(c *gin.Context) {
	boardID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	members, err := h.boardService.ListBoardMembers(c.Request.Context(), boardID, userID, memberRole)
	if err != nil {
		handleBoardError(c, err)
		return
	}

	if members == nil {
		members = []models.MemberResponse{}
	}

	c.JSON(http.StatusOK, members)
}

// AddBoardMember handles POST /api/v1/boards/:id/members
// Requires admin or owner workspace role.
func (h *BoardHandler) AddBoardMember(c *gin.Context) {
	var req models.AddBoardMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	boardID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	if err := h.boardService.AddBoardMember(c.Request.Context(), boardID, userID, memberRole, req); err != nil {
		handleBoardError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// RemoveBoardMember handles DELETE /api/v1/boards/:id/members/:uid
// Requires admin or owner workspace role.
func (h *BoardHandler) RemoveBoardMember(c *gin.Context) {
	boardID := c.Param("id")
	targetUserID := c.Param("uid")
	requesterID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	if err := h.boardService.RemoveBoardMember(c.Request.Context(), boardID, targetUserID, requesterID, memberRole); err != nil {
		handleBoardError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// GenerateShareLink handles POST /api/v1/boards/:id/share-link
// Requires admin or owner workspace role.
func (h *BoardHandler) GenerateShareLink(c *gin.Context) {
	boardID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	resp, err := h.boardService.GenerateShareLink(c.Request.Context(), boardID, userID, memberRole)
	if err != nil {
		handleBoardError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// RevokeShareLink handles DELETE /api/v1/boards/:id/share-link
// Requires admin or owner workspace role.
func (h *BoardHandler) RevokeShareLink(c *gin.Context) {
	boardID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	if err := h.boardService.RevokeShareLink(c.Request.Context(), boardID, userID, memberRole); err != nil {
		handleBoardError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// GetPublicBoard handles GET /api/v1/share/:token
// No authentication required — public read-only view.
func (h *BoardHandler) GetPublicBoard(c *gin.Context) {
	token := c.Param("token")

	board, err := h.boardService.GetPublicBoard(c.Request.Context(), token)
	if err != nil {
		handleBoardError(c, err)
		return
	}

	c.JSON(http.StatusOK, board)
}

// handleBoardError maps service errors to the correct HTTP response.
func handleBoardError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrNotFound):
		utils.ErrorResponse(c, http.StatusNotFound, "BOARD_NOT_FOUND", "board not found or not accessible")
	case errors.Is(err, services.ErrBoardAccessDenied):
		utils.ErrorResponse(c, http.StatusForbidden, "BOARD_ACCESS_DENIED", "you do not have access to this board")
	case errors.Is(err, services.ErrInsufficientPermissions):
		utils.ErrorResponse(c, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "you do not have permission to perform this action")
	default:
		utils.ErrInternal(c)
	}
}
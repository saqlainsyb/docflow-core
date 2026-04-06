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
// Any workspace member can create a board. Creator is automatically board owner.
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
func (h *BoardHandler) GetBoardDetail(c *gin.Context) {
	boardID := c.Param("id")
	userID := c.GetString("user_id")
	workspaceRole := c.GetString("member_role")
	boardRole := c.GetString("board_role")

	board, err := h.boardService.GetBoardDetail(c.Request.Context(), boardID, userID, workspaceRole, boardRole)
	if err != nil {
		handleBoardError(c, err)
		return
	}

	c.JSON(http.StatusOK, board)
}

// UpdateBoard handles PATCH /api/v1/boards/:id
// Rename: board owner or admin. Change visibility: board owner only.
func (h *BoardHandler) UpdateBoard(c *gin.Context) {
	var req models.UpdateBoardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	boardID := c.Param("id")
	userID := c.GetString("user_id")
	workspaceRole := c.GetString("member_role")
	boardRole := c.GetString("board_role")

	board, err := h.boardService.UpdateBoard(c.Request.Context(), boardID, userID, workspaceRole, boardRole, req)
	if err != nil {
		handleBoardError(c, err)
		return
	}

	c.JSON(http.StatusOK, board)
}

// DeleteBoard handles DELETE /api/v1/boards/:id
// Board owner or workspace owner only.
func (h *BoardHandler) DeleteBoard(c *gin.Context) {
	boardID := c.Param("id")
	userID := c.GetString("user_id")
	workspaceRole := c.GetString("member_role")
	boardRole := c.GetString("board_role")

	if err := h.boardService.DeleteBoard(c.Request.Context(), boardID, userID, workspaceRole, boardRole); err != nil {
		handleBoardError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListBoardMembers handles GET /api/v1/boards/:id/members
func (h *BoardHandler) ListBoardMembers(c *gin.Context) {
	boardID := c.Param("id")
	userID := c.GetString("user_id")
	workspaceRole := c.GetString("member_role")

	members, err := h.boardService.ListBoardMembers(c.Request.Context(), boardID, userID, workspaceRole)
	if err != nil {
		handleBoardError(c, err)
		return
	}

	if members == nil {
		members = []models.BoardMember{}
	}

	c.JSON(http.StatusOK, members)
}

// AddBoardMember handles POST /api/v1/boards/:id/members
// Board owner or admin. Admins can only add editors.
func (h *BoardHandler) AddBoardMember(c *gin.Context) {
	var req models.AddBoardMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	boardID := c.Param("id")
	requesterID := c.GetString("user_id")
	workspaceRole := c.GetString("member_role")
	boardRole := c.GetString("board_role")

	if err := h.boardService.AddBoardMember(c.Request.Context(), boardID, requesterID, workspaceRole, boardRole, req); err != nil {
		handleBoardError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// RemoveBoardMember handles DELETE /api/v1/boards/:id/members/:uid
// Board owner or admin. Cannot remove the board owner.
func (h *BoardHandler) RemoveBoardMember(c *gin.Context) {
	boardID := c.Param("id")
	targetUserID := c.Param("uid")
	requesterID := c.GetString("user_id")
	workspaceRole := c.GetString("member_role")
	boardRole := c.GetString("board_role")

	if err := h.boardService.RemoveBoardMember(c.Request.Context(), boardID, targetUserID, requesterID, workspaceRole, boardRole); err != nil {
		handleBoardError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// UpdateBoardMemberRole handles PATCH /api/v1/boards/:id/members/:uid
// Board owner only. Changes a member's role between admin and editor.
// To change the owner, use the transfer-ownership endpoint instead.
func (h *BoardHandler) UpdateBoardMemberRole(c *gin.Context) {
	var req models.UpdateBoardMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	boardID := c.Param("id")
	targetUserID := c.Param("uid")
	requesterID := c.GetString("user_id")
	workspaceRole := c.GetString("member_role")
	boardRole := c.GetString("board_role")

	if err := h.boardService.UpdateBoardMemberRole(c.Request.Context(), boardID, targetUserID, requesterID, workspaceRole, boardRole, req); err != nil {
		handleBoardError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// TransferOwnership handles POST /api/v1/boards/:id/transfer
// Board owner only. Target must already be a board member.
// Previous owner is downgraded to admin atomically.
func (h *BoardHandler) TransferOwnership(c *gin.Context) {
	var req models.TransferOwnershipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	boardID := c.Param("id")
	requesterID := c.GetString("user_id")
	workspaceRole := c.GetString("member_role")
	boardRole := c.GetString("board_role")

	if err := h.boardService.TransferOwnership(c.Request.Context(), boardID, requesterID, workspaceRole, boardRole, req); err != nil {
		handleBoardError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// GenerateShareLink handles POST /api/v1/boards/:id/share-link
// Board owner or admin.
func (h *BoardHandler) GenerateShareLink(c *gin.Context) {
	boardID := c.Param("id")
	userID := c.GetString("user_id")
	workspaceRole := c.GetString("member_role")
	boardRole := c.GetString("board_role")

	resp, err := h.boardService.GenerateShareLink(c.Request.Context(), boardID, userID, workspaceRole, boardRole)
	if err != nil {
		handleBoardError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// RevokeShareLink handles DELETE /api/v1/boards/:id/share-link
// Board owner or admin.
func (h *BoardHandler) RevokeShareLink(c *gin.Context) {
	boardID := c.Param("id")
	userID := c.GetString("user_id")
	workspaceRole := c.GetString("member_role")
	boardRole := c.GetString("board_role")

	if err := h.boardService.RevokeShareLink(c.Request.Context(), boardID, userID, workspaceRole, boardRole); err != nil {
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

// GetArchivedCards handles GET /api/v1/boards/:id/archived-cards
//
// Returns all archived cards for the board, ordered by column position
// then archived-at descending (most recently archived first within each
// column).
//
// Auth: board middleware has already run — user_id and member_role are
// injected into context. The service layer enforces private-board access
// using the workspace member_role.
//
// Response: 200 []ArchivedCardResponse  (empty array when none archived)
func (h *BoardHandler) GetArchivedCards(c *gin.Context) {
	boardID       := c.Param("id")
	userID        := c.GetString("user_id")
	workspaceRole := c.GetString("member_role")
 
	cards, err := h.boardService.GetArchivedCards(c.Request.Context(), boardID, userID, workspaceRole)
	if err != nil {
		handleBoardError(c, err)
		return
	}
 
	c.JSON(http.StatusOK, cards)
}

// handleBoardError maps service errors to HTTP responses.
func handleBoardError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrNotFound):
		utils.ErrorResponse(c, http.StatusNotFound, "BOARD_NOT_FOUND", "board not found or not accessible")
	case errors.Is(err, services.ErrBoardAccessDenied):
		utils.ErrorResponse(c, http.StatusForbidden, "BOARD_ACCESS_DENIED", "you do not have access to this board")
	case errors.Is(err, services.ErrInsufficientPermissions):
		utils.ErrorResponse(c, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "you do not have permission to perform this action")
	case errors.Is(err, services.ErrAlreadyBoardMember):
		utils.ErrorResponse(c, http.StatusConflict, "ALREADY_BOARD_MEMBER", "user is already a member of this board")
	case errors.Is(err, services.ErrCannotRemoveBoardOwner):
		utils.ErrorResponse(c, http.StatusForbidden, "CANNOT_REMOVE_BOARD_OWNER", "cannot remove the board owner — transfer ownership first")
	case errors.Is(err, services.ErrTargetNotBoardMember):
		utils.ErrorResponse(c, http.StatusUnprocessableEntity, "TARGET_NOT_BOARD_MEMBER", "target user must already be a board member")
	case errors.Is(err, services.ErrUserNotFound):
		utils.ErrorResponse(c, http.StatusNotFound, "USER_NOT_FOUND", "user not found or not a workspace member")
	default:
		utils.ErrInternal(c)
	}
}
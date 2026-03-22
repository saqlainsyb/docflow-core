package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/services"
	"github.com/saqlainsyb/docflow-core/internal/utils"
	"github.com/saqlainsyb/docflow-core/internal/ws"
)

type DocumentHandler struct {
	documentService *services.DocumentService
	hub             *ws.Hub
}

func NewDocumentHandler(documentService *services.DocumentService, hub *ws.Hub) *DocumentHandler {
	return &DocumentHandler{documentService: documentService, hub: hub}
}

// IssueToken handles POST /api/v1/documents/:id/token
// Issues a short-lived document JWT scoped to this document only.
// The frontend uses this token to authenticate the WebSocket connection.
// connectedCount is hardcoded to 0 until the WebSocket hub is wired in —
// at that point the handler will call hub.RoomSize(documentID) instead.
func (h *DocumentHandler) IssueToken(c *gin.Context) {
	documentID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	connectedCount := h.hub.RoomSize(documentID)
	resp, err := h.documentService.IssueToken(c.Request.Context(), documentID, userID, memberRole, connectedCount)
	if err != nil {
		handleDocumentError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetSnapshot handles GET /api/v1/documents/:id/snapshot
// Returns the current document state as base64-encoded Yjs binary.
// The frontend uses this to bootstrap the editor before connecting via WebSocket.
func (h *DocumentHandler) GetSnapshot(c *gin.Context) {
	documentID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	resp, err := h.documentService.GetSnapshot(
		c.Request.Context(),
		documentID,
		userID,
		memberRole,
	)
	if err != nil {
		handleDocumentError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// handleDocumentError maps service errors to the correct HTTP response.
func handleDocumentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrNotFound):
		utils.ErrorResponse(c, http.StatusNotFound, "DOCUMENT_NOT_FOUND", "document not found")
	case errors.Is(err, services.ErrBoardAccessDenied):
		utils.ErrorResponse(c, http.StatusForbidden, "BOARD_ACCESS_DENIED", "you do not have access to this board")
	case errors.Is(err, services.ErrInsufficientPermissions):
		utils.ErrorResponse(c, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "you do not have permission to perform this action")
	default:
		utils.ErrInternal(c)
	}
}

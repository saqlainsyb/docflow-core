package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/services"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

type ColumnHandler struct {
	columnService *services.ColumnService
}

func NewColumnHandler(columnService *services.ColumnService) *ColumnHandler {
	return &ColumnHandler{columnService: columnService}
}

// CreateColumn handles POST /api/v1/boards/:id/columns
// Appends a new column to the end of the board.
// board middleware has already run — board_id is in :id, member_role in context.
func (h *ColumnHandler) CreateColumn(c *gin.Context) {
	var req models.CreateColumnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	boardID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	col, err := h.columnService.CreateColumn(c.Request.Context(), boardID, userID, memberRole, req)
	if err != nil {
		handleColumnError(c, err)
		return
	}

	c.JSON(http.StatusCreated, col)
}

// UpdateColumn handles PATCH /api/v1/columns/:id
// Renames and/or repositions a column.
// Both fields are optional — only provided fields are updated.
func (h *ColumnHandler) UpdateColumn(c *gin.Context) {
	var req models.UpdateColumnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	columnID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	col, err := h.columnService.UpdateColumn(c.Request.Context(), columnID, userID, memberRole, req)
	if err != nil {
		handleColumnError(c, err)
		return
	}

	c.JSON(http.StatusOK, col)
}

// DeleteColumn handles DELETE /api/v1/columns/:id
// Removes the column and all its cards via cascade.
func (h *ColumnHandler) DeleteColumn(c *gin.Context) {
	columnID := c.Param("id")
	userID := c.GetString("user_id")
	memberRole := c.GetString("member_role")

	if err := h.columnService.DeleteColumn(c.Request.Context(), columnID, userID, memberRole); err != nil {
		handleColumnError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// handleColumnError maps service errors to the correct HTTP response.
func handleColumnError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrNotFound):
		utils.ErrorResponse(c, http.StatusNotFound, "COLUMN_NOT_FOUND", "column not found")
	case errors.Is(err, services.ErrBoardAccessDenied):
		utils.ErrorResponse(c, http.StatusForbidden, "BOARD_ACCESS_DENIED", "you do not have access to this board")
	case errors.Is(err, services.ErrInsufficientPermissions):
		utils.ErrorResponse(c, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "you do not have permission to perform this action")
	default:
		utils.ErrInternal(c)
	}
}
package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/services"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

type WorkspaceHandler struct {
	workspaceService *services.WorkspaceService
}

func NewWorkspaceHandler(workspaceService *services.WorkspaceService) *WorkspaceHandler {
	return &WorkspaceHandler{workspaceService: workspaceService}
}

// ListWorkspaces handles GET /api/v1/workspaces
// Returns all workspaces the authenticated user belongs to.
func (h *WorkspaceHandler) ListWorkspaces(c *gin.Context) {
	userID := c.GetString("user_id")

	workspaces, err := h.workspaceService.ListWorkspaces(c.Request.Context(), userID)
	if err != nil {
		utils.ErrInternal(c)
		return
	}

	// return empty array instead of null when user has no workspaces
	if workspaces == nil {
		workspaces = []models.WorkspaceResponse{}
	}

	c.JSON(http.StatusOK, workspaces)
}

// CreateWorkspace handles POST /api/v1/workspaces
// Creates a new workspace owned by the authenticated user.
func (h *WorkspaceHandler) CreateWorkspace(c *gin.Context) {
	var req models.CreateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	userID := c.GetString("user_id")

	ws, err := h.workspaceService.CreateWorkspace(c.Request.Context(), userID, req)
	if err != nil {
		utils.ErrInternal(c)
		return
	}

	c.JSON(http.StatusCreated, ws)
}

// GetWorkspace handles GET /api/v1/workspaces/:id
// Returns workspace detail with full member list.
// workspace_id is injected by the workspace middleware.
func (h *WorkspaceHandler) GetWorkspace(c *gin.Context) {
	workspaceID := c.GetString("workspace_id")

	ws, err := h.workspaceService.GetWorkspace(c.Request.Context(), workspaceID)
	if err != nil {
		handleWorkspaceError(c, err)
		return
	}

	c.JSON(http.StatusOK, ws)
}

// RenameWorkspace handles PATCH /api/v1/workspaces/:id
// Requires admin or owner role — enforced in the service.
func (h *WorkspaceHandler) RenameWorkspace(c *gin.Context) {
	var req models.UpdateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if req.Name == nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}

	workspaceID := c.GetString("workspace_id")
	memberRole := c.GetString("member_role")

	ws, err := h.workspaceService.RenameWorkspace(c.Request.Context(), workspaceID, memberRole, req)
	if err != nil {
		handleWorkspaceError(c, err)
		return
	}

	c.JSON(http.StatusOK, ws)
}

// DeleteWorkspace handles DELETE /api/v1/workspaces/:id
// Requires owner role — enforced in the service.
// Cascades to all boards, columns, cards, and documents.
func (h *WorkspaceHandler) DeleteWorkspace(c *gin.Context) {
	workspaceID := c.GetString("workspace_id")
	memberRole := c.GetString("member_role")

	if err := h.workspaceService.DeleteWorkspace(c.Request.Context(), workspaceID, memberRole); err != nil {
		handleWorkspaceError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListMembers handles GET /api/v1/workspaces/:id/members
func (h *WorkspaceHandler) ListMembers(c *gin.Context) {
	workspaceID := c.GetString("workspace_id")

	members, err := h.workspaceService.ListMembers(c.Request.Context(), workspaceID)
	if err != nil {
		utils.ErrInternal(c)
		return
	}

	if members == nil {
		members = []models.MemberResponse{}
	}

	c.JSON(http.StatusOK, members)
}

// InviteMember handles POST /api/v1/workspaces/:id/members
// Requires admin or owner role — enforced in the service.
func (h *WorkspaceHandler) InviteMember(c *gin.Context) {
	var req models.InviteMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	workspaceID := c.GetString("workspace_id")
	memberRole := c.GetString("member_role")

	if err := h.workspaceService.InviteMember(c.Request.Context(), workspaceID, memberRole, req); err != nil {
		handleWorkspaceError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// RemoveMember handles DELETE /api/v1/workspaces/:id/members/:uid
// Requires admin or owner role — enforced in the service.
func (h *WorkspaceHandler) RemoveMember(c *gin.Context) {
	workspaceID := c.GetString("workspace_id")
	requesterID := c.GetString("user_id")
	memberRole := c.GetString("member_role")
	targetUserID := c.Param("uid")

	if err := h.workspaceService.RemoveMember(c.Request.Context(), workspaceID, requesterID, memberRole, targetUserID); err != nil {
		handleWorkspaceError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// UpdateMemberRole handles PATCH /api/v1/workspaces/:id/members/:uid
// Requires owner role — enforced in the service.
func (h *WorkspaceHandler) UpdateMemberRole(c *gin.Context) {
	var req models.UpdateMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	workspaceID := c.GetString("workspace_id")
	requesterID := c.GetString("user_id")
	memberRole := c.GetString("member_role")
	targetUserID := c.Param("uid")

	if err := h.workspaceService.UpdateMemberRole(c.Request.Context(), workspaceID, requesterID, memberRole, targetUserID, req); err != nil {
		handleWorkspaceError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// handleWorkspaceError maps service errors to the correct HTTP response.
func handleWorkspaceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrNotFound):
		utils.ErrorResponse(c, http.StatusNotFound, "WORKSPACE_NOT_FOUND", "workspace or member not found")
	case errors.Is(err, services.ErrInsufficientPermissions):
		utils.ErrorResponse(c, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "you do not have permission to perform this action")
	case errors.Is(err, services.ErrAlreadyMember):
		utils.ErrConflict(c, "ALREADY_WORKSPACE_MEMBER", "user is already a member of this workspace")
	case errors.Is(err, services.ErrCannotRemoveOwner):
		utils.ErrorResponse(c, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "cannot remove the workspace owner")
	case errors.Is(err, services.ErrCannotChangeSelfRole):
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", "cannot change your own role")
	default:
		utils.ErrInternal(c)
	}
}
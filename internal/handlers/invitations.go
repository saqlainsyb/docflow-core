// internal/handlers/invitations.go
package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/services"
	"github.com/saqlainsyb/docflow-core/internal/utils"
	"go.uber.org/zap"
)

type InvitationHandler struct {
	invitationService *services.InvitationService
	logger            *zap.Logger
}

func NewInvitationHandler(invitationService *services.InvitationService, logger *zap.Logger) *InvitationHandler {
	return &InvitationHandler{
		invitationService: invitationService,
		logger:            logger,
	}
}

// SendInvitation handles POST /api/v1/workspaces/:id/invitations
// Requires admin or owner role (enforced in service).
// Body: { "email": "user@example.com", "role": "member" }
func (h *InvitationHandler) SendInvitation(c *gin.Context) {
	var req models.SendInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	workspaceID  := c.GetString("workspace_id")
	requesterID  := c.GetString("user_id")
	memberRole   := c.GetString("member_role")

	resp, err := h.invitationService.SendInvitation(c.Request.Context(), workspaceID, requesterID, memberRole, req)
	if err != nil {
		// Email delivery failure is non-fatal: the invitation row exists.
		// Return 202 Accepted instead of 201 Created to signal partial success.
		if errors.Is(err, services.ErrEmailDeliveryFailed) {
			h.logger.Warn("invitation created but email delivery failed",
				zap.String("workspace_id", workspaceID),
				zap.String("invited_email", req.Email),
				zap.Error(err),
			)
			utils.ErrorResponse(c, http.StatusAccepted, "EMAIL_DELIVERY_FAILED",
				"invitation was created but the email could not be delivered; the invitee can still accept via direct link")
			return
		}
		handleInvitationError(c, err)
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// GetInvitation handles GET /api/v1/invitations/:token
// Public — no authentication required.
// Returns workspace name, inviter name, and expiry for the accept page.
func (h *InvitationHandler) GetInvitation(c *gin.Context) {
	rawToken := c.Param("token")

	detail, err := h.invitationService.GetInvitationByToken(c.Request.Context(), rawToken)
	if err != nil {
		handleInvitationError(c, err)
		return
	}

	c.JSON(http.StatusOK, detail)
}

// AcceptInvitation handles POST /api/v1/invitations/:token/accept
// Requires authentication — the caller must be logged in.
// On success, returns the workspace ID so the frontend can navigate there.
func (h *InvitationHandler) AcceptInvitation(c *gin.Context) {
	rawToken   := c.Param("token")
	callerID   := c.GetString("user_id")
	callerEmail := c.GetString("user_email") // injected by auth middleware

	workspaceID, err := h.invitationService.AcceptInvitation(c.Request.Context(), rawToken, callerID, callerEmail)
	if err != nil {
		handleInvitationError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"workspace_id": workspaceID})
}

// ListInvitations handles GET /api/v1/workspaces/:id/invitations
// Returns all live pending invitations. Requires admin or owner role.
func (h *InvitationHandler) ListInvitations(c *gin.Context) {
	workspaceID := c.GetString("workspace_id")
	memberRole  := c.GetString("member_role")

	invitations, err := h.invitationService.ListPendingInvitations(c.Request.Context(), workspaceID, memberRole)
	if err != nil {
		handleInvitationError(c, err)
		return
	}

	c.JSON(http.StatusOK, invitations)
}

// CancelInvitation handles DELETE /api/v1/workspaces/:id/invitations/:invitationId
// Requires admin or owner role (enforced in service).
func (h *InvitationHandler) CancelInvitation(c *gin.Context) {
	workspaceID  := c.GetString("workspace_id")
	memberRole   := c.GetString("member_role")
	invitationID := c.Param("invitationId")

	if err := h.invitationService.CancelInvitation(c.Request.Context(), workspaceID, invitationID, memberRole); err != nil {
		handleInvitationError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// handleInvitationError maps service-layer invitation errors to HTTP responses.
func handleInvitationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrInvitationInvalid):
		utils.ErrorResponse(c, http.StatusNotFound, "INVITATION_INVALID",
			"this invitation link is invalid or has already been used")
	case errors.Is(err, services.ErrInvitationExpired):
		utils.ErrorResponse(c, http.StatusGone, "INVITATION_EXPIRED",
			"this invitation has expired; ask the workspace admin to send a new one")
	case errors.Is(err, services.ErrInvitationAlreadyPending):
		utils.ErrConflict(c, "INVITATION_ALREADY_PENDING",
			"a pending invitation for this email already exists")
	case errors.Is(err, services.ErrInvitationEmailMismatch):
		utils.ErrorResponse(c, http.StatusForbidden, "INVITATION_EMAIL_MISMATCH",
			"this invitation was sent to a different email address")
	case errors.Is(err, services.ErrAlreadyMember):
		utils.ErrConflict(c, "ALREADY_WORKSPACE_MEMBER",
			"this user is already a member of the workspace")
	case errors.Is(err, services.ErrInsufficientPermissions):
		utils.ErrorResponse(c, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS",
			"you do not have permission to manage invitations")
	case errors.Is(err, services.ErrNotFound):
		utils.ErrorResponse(c, http.StatusNotFound, "NOT_FOUND",
			"workspace or invitation not found")
	default:
		utils.ErrInternal(c)
	}
}
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

// Document resolves the document from :id in the URL, walks up to its
// card, board, and workspace, checks that the authenticated user is a
// workspace member, and injects workspace_id and member_role into context.
// Must run after the Auth middleware — depends on user_id being present.
func Document(
	documentRepo  *repositories.DocumentRepository,
	cardRepo      *repositories.CardRepository,
	boardRepo     *repositories.BoardRepository,
	workspaceRepo *repositories.WorkspaceRepository,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		documentID := c.Param("id")
		if documentID == "" {
			utils.ErrorResponse(c, 400, "INVALID_DOCUMENT_ID", "document id is required")
			c.Abort()
			return
		}

		// validate UUID format before hitting the database
		if _, err := uuid.Parse(documentID); err != nil {
			utils.ErrorResponse(c, 400, "INVALID_UUID", "document id is not a valid UUID")
			c.Abort()
			return
		}


		userID := c.GetString("user_id")

		// look up the document to get its card_id
		doc, err := documentRepo.FindByID(c.Request.Context(), documentID)
		if err != nil {
			if err == repositories.ErrNotFound {
				utils.ErrorResponse(c, 404, "DOCUMENT_NOT_FOUND", "document not found")
				c.Abort()
				return
			}
			utils.ErrInternal(c)
			c.Abort()
			return
		}

		// look up the card to get its board_id
		card, err := cardRepo.FindByID(c.Request.Context(), doc.CardID)
		if err != nil {
			if err == repositories.ErrNotFound {
				utils.ErrorResponse(c, 404, "CARD_NOT_FOUND", "card not found")
				c.Abort()
				return
			}
			utils.ErrInternal(c)
			c.Abort()
			return
		}

		// look up the board to get its workspace_id
		board, err := boardRepo.FindByID(c.Request.Context(), card.BoardID)
		if err != nil {
			if err == repositories.ErrNotFound {
				utils.ErrorResponse(c, 404, "BOARD_NOT_FOUND", "board not found")
				c.Abort()
				return
			}
			utils.ErrInternal(c)
			c.Abort()
			return
		}

		// check the user is a workspace member and get their role
		member, err := workspaceRepo.GetMember(c.Request.Context(), board.WorkspaceID, userID)
		if err != nil {
			if err == repositories.ErrNotFound {
				utils.ErrorResponse(c, 403, "NOT_WORKSPACE_MEMBER", "you are not a member of this workspace")
				c.Abort()
				return
			}
			utils.ErrInternal(c)
			c.Abort()
			return
		}

		c.Set("workspace_id", board.WorkspaceID)
		c.Set("member_role", member.Role)

		c.Next()
	}
}
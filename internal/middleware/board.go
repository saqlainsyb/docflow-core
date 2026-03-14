package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

// Board resolves the board from :id in the URL, finds its workspace,
// checks that the authenticated user is a workspace member, and injects
// workspace_id and member_role into the Gin context.
// Must run after the Auth middleware — depends on user_id being present.
// Board-level access control (private vs workspace visibility) is handled
// in the service layer after this middleware passes.
func Board(
	boardRepo *repositories.BoardRepository,
	workspaceRepo *repositories.WorkspaceRepository,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		boardID := c.Param("id")
		if boardID == "" {
			utils.ErrorResponse(c, 400, "INVALID_BOARD_ID", "board id is required")
			c.Abort()
			return
		}

		userID := c.GetString("user_id")

		// look up the board to get its workspace_id
		board, err := boardRepo.FindByID(c.Request.Context(), boardID)
		if err != nil {
			if err == repositories.ErrNotFound {
				utils.ErrorResponse(c, 404, "BOARD_NOT_FOUND", "board not found or not accessible")
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

		// inject so handlers and services can read them
		c.Set("workspace_id", board.WorkspaceID)
		c.Set("member_role", member.Role)

		c.Next()
	}
}
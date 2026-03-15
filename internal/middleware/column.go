package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

// Column resolves the column from :id in the URL, finds its board and
// workspace, checks that the authenticated user is a workspace member,
// and injects workspace_id and member_role into the Gin context.
// Must run after the Auth middleware — depends on user_id being present.
func Column(
	columnRepo *repositories.ColumnRepository,
	boardRepo *repositories.BoardRepository,
	workspaceRepo *repositories.WorkspaceRepository,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		columnID := c.Param("id")
		if columnID == "" {
			utils.ErrorResponse(c, 400, "INVALID_COLUMN_ID", "column id is required")
			c.Abort()
			return
		}

		if _, err := uuid.Parse(columnID); err != nil {
			utils.ErrorResponse(c, 400, "INVALID_UUID", "column id is not a valid UUID")
			c.Abort()
			return
		}

		userID := c.GetString("user_id")

		// look up the column to get its board_id
		col, err := columnRepo.FindByID(c.Request.Context(), columnID)
		if err != nil {
			if err == repositories.ErrNotFound {
				utils.ErrorResponse(c, 404, "COLUMN_NOT_FOUND", "column not found")
				c.Abort()
				return
			}
			utils.ErrInternal(c)
			c.Abort()
			return
		}

		// look up the board to get its workspace_id
		board, err := boardRepo.FindByID(c.Request.Context(), col.BoardID)
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

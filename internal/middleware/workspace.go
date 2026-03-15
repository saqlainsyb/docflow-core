package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

// Workspace validates that the authenticated user is a member of the
// workspace identified by :id in the URL.
// On success it injects workspace_id and member_role into the Gin context.
// Must run after the Auth middleware — depends on user_id being present.
func Workspace(workspaceRepo *repositories.WorkspaceRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		workspaceID := c.Param("id")
		if workspaceID == "" {
			utils.ErrorResponse(c, 400, "INVALID_WORKSPACE_ID", "workspace id is required")
			c.Abort()
			return
		}

		if _, err := uuid.Parse(workspaceID); err != nil {
			utils.ErrorResponse(c, 400, "INVALID_UUID", "workspace id is not a valid UUID")
			c.Abort()
			return
		}

		userID := c.GetString("user_id")

		member, err := workspaceRepo.GetMember(c.Request.Context(), workspaceID, userID)
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

		c.Set("workspace_id", workspaceID)
		c.Set("member_role", member.Role)

		c.Next()
	}
}

package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

// Board resolves the board from :id in the URL, verifies the caller is a
// workspace member, then resolves and injects the caller's effective board role.
//
// Injected context keys:
//   - "workspace_id"  — the board's parent workspace UUID
//   - "member_role"   — the caller's workspace role (owner / admin / member)
//   - "board_role"    — the caller's effective board role (owner / admin / editor)
//
// Board-level access for private boards is enforced in the service layer —
// this middleware only confirms workspace membership and prepares the roles.
// Must run after the Auth middleware (depends on "user_id" being present).
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

		if _, err := uuid.Parse(boardID); err != nil {
			utils.ErrorResponse(c, 400, "INVALID_UUID", "board id is not a valid UUID")
			c.Abort()
			return
		}

		userID := c.GetString("user_id")

		// Fetch the board to get its workspace.
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

		// Confirm workspace membership and get workspace role.
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

		// Look up the caller's explicit board role (may not exist).
		boardMemberRole, err := boardRepo.GetBoardMemberRole(c.Request.Context(), boardID, userID)
		if err != nil && err != repositories.ErrNotFound {
			utils.ErrInternal(c)
			c.Abort()
			return
		}

		// Resolve effective board role.
		//
		//   explicit owner in board_members → "owner"
		//   workspace owner/admin            → "admin" (minimum)
		//   explicit board role              → that role
		//   workspace member, no board role  → "editor" (default)
		//
		// Note: for private boards, a workspace member with boardMemberRole == ""
		// (no board_members row) will be denied access by the service layer;
		// we still inject "editor" here but the service checkAccess() rejects them.
		effectiveBoardRole := resolveBoardRole(member.Role, boardMemberRole)

		c.Set("workspace_id", board.WorkspaceID)
		c.Set("member_role", member.Role)       // workspace role — used by workspace-level checks
		c.Set("board_role", effectiveBoardRole)  // board role    — used by board-level checks

		c.Next()
	}
}

// resolveBoardRole combines the workspace role and the explicit board_members role
// into a single effective board role.
func resolveBoardRole(workspaceRole, boardMemberRole string) string {
	if boardMemberRole == "owner" {
		return "owner"
	}
	if workspaceRole == "owner" || workspaceRole == "admin" {
		return "admin"
	}
	if boardMemberRole != "" {
		return boardMemberRole
	}
	return "editor"
}
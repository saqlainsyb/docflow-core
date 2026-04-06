// internal/router/router.go
package router

import (
	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/config"
	"github.com/saqlainsyb/docflow-core/internal/handlers"
	"github.com/saqlainsyb/docflow-core/internal/middleware"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
)

func Setup(
	cfg               *config.Config,
	healthHandler     *handlers.HealthHandler,
	authHandler       *handlers.AuthHandler,
	workspaceHandler  *handlers.WorkspaceHandler,
	invitationHandler *handlers.InvitationHandler,
	boardHandler      *handlers.BoardHandler,
	columnHandler     *handlers.ColumnHandler,
	cardHandler       *handlers.CardHandler,
	documentHandler   *handlers.DocumentHandler,
	wsHandler         *handlers.WSHandler,
	workspaceRepo     *repositories.WorkspaceRepository,
	boardRepo         *repositories.BoardRepository,
	columnRepo        *repositories.ColumnRepository,
	cardRepo          *repositories.CardRepository,
	documentRepo      *repositories.DocumentRepository,
) *gin.Engine {

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())
	r.Use(middleware.CORS(cfg))
	r.Use(middleware.RateLimit(cfg))

	// ── Health ────────────────────────────────────────────────────────────────
	r.GET("/health", healthHandler.Check)

	// ── WebSocket routes ──────────────────────────────────────────────────────
	r.GET("/ws/documents/:id", wsHandler.HandleDocumentWS)
	r.GET("/ws/boards/:id", wsHandler.HandleBoardWS)

	api := r.Group("/api/v1")

	// ── Public routes ─────────────────────────────────────────────────────────
	auth := api.Group("/auth")
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)
	}

	api.GET("/share/:token", boardHandler.GetPublicBoard)

	// ── Public invitation lookup — no auth required ───────────────────────────
	// The accept page calls this to get the workspace name / inviter name before
	// the user decides whether to accept. Revealing this minimal info to anyone
	// with the token is intentional and acceptable — the token is a secret itself.
	api.GET("/invitations/:token", invitationHandler.GetInvitation)

	// ── Protected routes — JWT required ───────────────────────────────────────
	protected := api.Group("")
	protected.Use(middleware.Auth(cfg))
	{
		protected.POST("/auth/logout", authHandler.Logout)
		protected.GET("/users/me", authHandler.GetMe)
		protected.PATCH("/users/me", authHandler.UpdateMe)

		// Accept an invitation — requires auth because we need to know who is
		// accepting. The invitation's invited_email is compared against the
		// authenticated user's email in the service layer.
		protected.POST("/invitations/:token/accept", invitationHandler.AcceptInvitation)

		// Workspace routes
		protected.GET("/workspaces", workspaceHandler.ListWorkspaces)
		protected.POST("/workspaces", workspaceHandler.CreateWorkspace)

		// Workspace-scoped routes
		ws := protected.Group("/workspaces/:id")
		ws.Use(middleware.Workspace(workspaceRepo))
		{
			ws.GET("", workspaceHandler.GetWorkspace)
			ws.PATCH("", workspaceHandler.RenameWorkspace)
			ws.DELETE("", workspaceHandler.DeleteWorkspace)

			// Members
			ws.GET("/members", workspaceHandler.ListMembers)
			ws.POST("/members", workspaceHandler.InviteMember)
			ws.PATCH("/members/:uid", workspaceHandler.UpdateMemberRole)
			ws.DELETE("/members/:uid", workspaceHandler.RemoveMember)

			// Email invitations
			ws.GET("/invitations", invitationHandler.ListInvitations)
			ws.POST("/invitations", invitationHandler.SendInvitation)
			ws.DELETE("/invitations/:invitationId", invitationHandler.CancelInvitation)

			// Boards
			ws.GET("/boards", boardHandler.ListBoards)
			ws.POST("/boards", boardHandler.CreateBoard)

			// Board-scoped routes
			board := ws.Group("/boards/:boardId")
			board.Use(middleware.Board(boardRepo, workspaceRepo))
			{
				board.GET("", boardHandler.GetBoard)
				board.PATCH("", boardHandler.UpdateBoard)
				board.DELETE("", boardHandler.DeleteBoard)
				board.GET("/members", boardHandler.ListBoardMembers)
				board.POST("/members", boardHandler.AddBoardMember)
				board.PATCH("/members/:uid", boardHandler.UpdateBoardMemberRole)
				board.DELETE("/members/:uid", boardHandler.RemoveBoardMember)
				board.POST("/transfer", boardHandler.TransferBoardOwnership)
				board.POST("/share-link", boardHandler.GenerateShareLink)
				board.DELETE("/share-link", boardHandler.RevokeShareLink)
				board.GET("/archived-cards", boardHandler.GetArchivedCards)

				// Column-scoped routes
				column := board.Group("/columns")
				{
					column.POST("", columnHandler.CreateColumn)
					column.PATCH("/reorder", columnHandler.ReorderColumns)
				}

				col := board.Group("")
				col.Use(middleware.Column(columnRepo))
				{
					col.PATCH("/columns/:columnId", columnHandler.UpdateColumn)
					col.DELETE("/columns/:columnId", columnHandler.DeleteColumn)

					// Card-scoped routes
					card := col.Group("/columns/:columnId/cards")
					{
						card.POST("", cardHandler.CreateCard)
					}
				}

				cardRoutes := board.Group("")
				cardRoutes.Use(middleware.Card(cardRepo))
				{
					cardRoutes.PATCH("/cards/:cardId", cardHandler.UpdateCard)
					cardRoutes.DELETE("/cards/:cardId", cardHandler.DeleteCard)
					cardRoutes.POST("/cards/:cardId/move", cardHandler.MoveCard)
					cardRoutes.POST("/cards/:cardId/archive", cardHandler.ArchiveCard)
					cardRoutes.POST("/cards/:cardId/unarchive", cardHandler.UnarchiveCard)
				}

				// Document routes
				doc := board.Group("/cards/:cardId/documents")
				doc.Use(middleware.Card(cardRepo))
				{
					doc.GET("/:documentId/token", documentHandler.IssueToken)
					doc.GET("/:documentId/snapshot", documentHandler.GetSnapshot)
				}
			}
		}
	}

	return r
}
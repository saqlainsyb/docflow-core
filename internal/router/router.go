// internal/router/router.go
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/config"
	"github.com/saqlainsyb/docflow-core/internal/handlers"
	"github.com/saqlainsyb/docflow-core/internal/middleware"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
)

func Setup(
	cfg              *config.Config,
	authHandler      *handlers.AuthHandler,
	workspaceHandler *handlers.WorkspaceHandler,
	boardHandler     *handlers.BoardHandler,
	columnHandler    *handlers.ColumnHandler,
	cardHandler      *handlers.CardHandler,
	documentHandler  *handlers.DocumentHandler,
	wsHandler        *handlers.WSHandler,
	workspaceRepo    *repositories.WorkspaceRepository,
	boardRepo        *repositories.BoardRepository,
	columnRepo       *repositories.ColumnRepository,
	cardRepo         *repositories.CardRepository,
	documentRepo     *repositories.DocumentRepository,
) *gin.Engine {

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())
	r.Use(middleware.CORS(cfg))
	r.Use(middleware.RateLimit(cfg))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// ── WebSocket routes ──────────────────────────────────────────────────────
	r.GET("/ws/documents/:id", wsHandler.HandleDocumentWS)
	r.GET("/ws/boards/:id",    wsHandler.HandleBoardWS)

	api := r.Group("/api/v1")

	// ── Public routes ─────────────────────────────────────────────────────────
	auth := api.Group("/auth")
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login",    authHandler.Login)
		auth.POST("/refresh",  authHandler.Refresh)
	}

	api.GET("/share/:token", boardHandler.GetPublicBoard)

	// ── Protected routes — JWT required ───────────────────────────────────────
	protected := api.Group("")
	protected.Use(middleware.Auth(cfg))
	{
		protected.POST("/auth/logout", authHandler.Logout)
		protected.GET("/users/me",     authHandler.GetMe)
		protected.PATCH("/users/me",   authHandler.UpdateMe)

		// Workspace routes
		protected.GET("/workspaces",  workspaceHandler.ListWorkspaces)
		protected.POST("/workspaces", workspaceHandler.CreateWorkspace)

		// Workspace-scoped routes
		ws := protected.Group("/workspaces/:id")
		ws.Use(middleware.Workspace(workspaceRepo))
		{
			ws.GET("",                 workspaceHandler.GetWorkspace)
			ws.PATCH("",               workspaceHandler.RenameWorkspace)
			ws.DELETE("",              workspaceHandler.DeleteWorkspace)
			ws.GET("/members",         workspaceHandler.ListMembers)
			ws.POST("/members",        workspaceHandler.InviteMember)
			ws.PATCH("/members/:uid",  workspaceHandler.UpdateMemberRole)
			ws.DELETE("/members/:uid", workspaceHandler.RemoveMember)
			ws.GET("/boards",          boardHandler.ListBoards)
			ws.POST("/boards",         boardHandler.CreateBoard)
		}

		// Board-scoped routes
		// middleware.Board resolves workspace membership AND effective board role,
		// injecting both "member_role" (workspace) and "board_role" into context.
		board := protected.Group("/boards/:id")
		board.Use(middleware.Board(boardRepo, workspaceRepo))
		{
			board.GET("",                  boardHandler.GetBoardDetail)
			board.PATCH("",                boardHandler.UpdateBoard)
			board.DELETE("",               boardHandler.DeleteBoard)
			board.GET("/members",          boardHandler.ListBoardMembers)
			board.POST("/members",         boardHandler.AddBoardMember)
			board.PATCH("/members/:uid",   boardHandler.UpdateBoardMemberRole) // new
			board.DELETE("/members/:uid",  boardHandler.RemoveBoardMember)
			board.POST("/transfer",        boardHandler.TransferOwnership)     // new
			board.POST("/share-link",      boardHandler.GenerateShareLink)
			board.DELETE("/share-link",    boardHandler.RevokeShareLink)
			board.POST("/columns",         columnHandler.CreateColumn)
		}

		// Column-scoped routes
		col := protected.Group("/columns/:id")
		col.Use(middleware.Column(columnRepo, boardRepo, workspaceRepo))
		{
			col.PATCH("",      columnHandler.UpdateColumn)
			col.DELETE("",     columnHandler.DeleteColumn)
			col.POST("/cards", cardHandler.CreateCard)
		}

		// Card-scoped routes
		card := protected.Group("/cards/:id")
		card.Use(middleware.Card(cardRepo, boardRepo, workspaceRepo))
		{
			card.PATCH("",          cardHandler.UpdateCard)
			card.DELETE("",         cardHandler.DeleteCard)
			card.POST("/move",      cardHandler.MoveCard)
			card.POST("/archive",   cardHandler.ArchiveCard)
			card.POST("/unarchive", cardHandler.UnarchiveCard)
		}

		// Document-scoped routes
		doc := protected.Group("/documents/:id")
		doc.Use(middleware.Document(documentRepo, cardRepo, boardRepo, workspaceRepo))
		{
			doc.POST("/token",   documentHandler.IssueToken)
			doc.GET("/snapshot", documentHandler.GetSnapshot)
		}
	}

	return r
}
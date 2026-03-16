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
	workspaceRepo    *repositories.WorkspaceRepository,
	boardRepo        *repositories.BoardRepository,
	columnRepo       *repositories.ColumnRepository,
	cardRepo         *repositories.CardRepository,
	documentRepo     *repositories.DocumentRepository,
) *gin.Engine {

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	// health check — no auth required
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")

	// ── public routes ─────────────────────────────────────────────────────
	auth := api.Group("/auth")
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login",    authHandler.Login)
		auth.POST("/refresh",  authHandler.Refresh)
	}

	// public share link — no auth required
	api.GET("/share/:token", boardHandler.GetPublicBoard)

	// ── protected routes — JWT required ───────────────────────────────────
	protected := api.Group("")
	protected.Use(middleware.Auth(cfg))
	{
		protected.POST("/auth/logout", authHandler.Logout)
		protected.GET("/users/me",     authHandler.GetMe)

		// workspace routes
		protected.GET("/workspaces",  workspaceHandler.ListWorkspaces)
		protected.POST("/workspaces", workspaceHandler.CreateWorkspace)

		// workspace-scoped routes
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

		// board-scoped routes
		board := protected.Group("/boards/:id")
		board.Use(middleware.Board(boardRepo, workspaceRepo))
		{
			board.GET("",                 boardHandler.GetBoardDetail)
			board.PATCH("",               boardHandler.UpdateBoard)
			board.DELETE("",              boardHandler.DeleteBoard)
			board.GET("/members",         boardHandler.ListBoardMembers)
			board.POST("/members",        boardHandler.AddBoardMember)
			board.DELETE("/members/:uid", boardHandler.RemoveBoardMember)
			board.POST("/share-link",     boardHandler.GenerateShareLink)
			board.DELETE("/share-link",   boardHandler.RevokeShareLink)
			board.POST("/columns",        columnHandler.CreateColumn)
		}

		// column-scoped routes
		col := protected.Group("/columns/:id")
		col.Use(middleware.Column(columnRepo, boardRepo, workspaceRepo))
		{
			col.PATCH("",      columnHandler.UpdateColumn)
			col.DELETE("",     columnHandler.DeleteColumn)
			col.POST("/cards", cardHandler.CreateCard)
		}

		// card-scoped routes
		card := protected.Group("/cards/:id")
		card.Use(middleware.Card(cardRepo, boardRepo, workspaceRepo))
		{
			card.PATCH("",          cardHandler.UpdateCard)
			card.DELETE("",         cardHandler.DeleteCard)
			card.POST("/move",      cardHandler.MoveCard)
			card.POST("/archive",   cardHandler.ArchiveCard)
			card.POST("/unarchive", cardHandler.UnarchiveCard)
		}

		// document-scoped routes
		doc := protected.Group("/documents/:id")
		doc.Use(middleware.Document(documentRepo, cardRepo, boardRepo, workspaceRepo))
		{
			doc.POST("/token",    documentHandler.IssueToken)
			doc.GET("/snapshot",  documentHandler.GetSnapshot)
		}
	}

	return r
}
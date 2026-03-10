package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/config"
	"github.com/saqlainsyb/docflow-core/internal/handlers"
	"github.com/saqlainsyb/docflow-core/internal/middleware"
)

func Setup(
	cfg *config.Config,
	authHandler *handlers.AuthHandler,
) *gin.Engine {

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	// health check — no auth required
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")

	// ── public routes — no auth ───────────────────────────────────────────
	auth := api.Group("/auth")
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login",    authHandler.Login)
		auth.POST("/refresh",  authHandler.Refresh)
	}

	// ── protected routes — JWT required ───────────────────────────────────
	protected := api.Group("")
	protected.Use(middleware.Auth(cfg))
	{
		protected.POST("/auth/logout", authHandler.Logout)
		protected.GET("/users/me",     authHandler.GetMe)
	}

	return r
}
package main

import (
	"log"

	"github.com/saqlainsyb/docflow-core/internal/config"
	"github.com/saqlainsyb/docflow-core/internal/db"
	"github.com/saqlainsyb/docflow-core/internal/handlers"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
	"github.com/saqlainsyb/docflow-core/internal/router"
	"github.com/saqlainsyb/docflow-core/internal/services"
)

func main() {
	// config
	cfg := config.Load()
	log.Printf("starting docflow in %s mode on port %s", cfg.AppEnv, cfg.AppPort)

	// database connections
	dbPool := db.Connect(cfg)
	defer dbPool.Close()

	redisClient := db.ConnectRedis(cfg)
	defer redisClient.Close()

	// repositories
	userRepo         := repositories.NewUserRepository(dbPool)
	refreshTokenRepo := repositories.NewRefreshTokenRepository(dbPool)
	workspaceRepo    := repositories.NewWorkspaceRepository(dbPool)
	boardRepo        := repositories.NewBoardRepository(dbPool)
	columnRepo       := repositories.NewColumnRepository(dbPool)

	// services
	authService      := services.NewAuthService(userRepo, refreshTokenRepo, workspaceRepo, cfg)
	workspaceService := services.NewWorkspaceService(workspaceRepo, userRepo)
	boardService     := services.NewBoardService(boardRepo, workspaceRepo, cfg)
	columnService    := services.NewColumnService(columnRepo, boardService)

	// handlers
	authHandler      := handlers.NewAuthHandler(authService)
	workspaceHandler := handlers.NewWorkspaceHandler(workspaceService)
	boardHandler     := handlers.NewBoardHandler(boardService)
	columnHandler    := handlers.NewColumnHandler(columnService)

	// router
	r := router.Setup(cfg, authHandler, workspaceHandler, boardHandler, columnHandler, workspaceRepo, boardRepo, columnRepo)

	log.Printf("server running on port %s", cfg.AppPort)
	if err := r.Run(":" + cfg.AppPort); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
}
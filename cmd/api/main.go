// cmd/api/main.go
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/saqlainsyb/docflow-core/internal/config"
	"github.com/saqlainsyb/docflow-core/internal/db"
	"github.com/saqlainsyb/docflow-core/internal/email"
	"github.com/saqlainsyb/docflow-core/internal/handlers"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
	"github.com/saqlainsyb/docflow-core/internal/router"
	"github.com/saqlainsyb/docflow-core/internal/services"
	"github.com/saqlainsyb/docflow-core/internal/ws"
)

func main() {
	// ── config ────────────────────────────────────────────────────────────
	cfg := config.Load()
	log.Printf("starting docflow in %s mode on port %s", cfg.AppEnv, cfg.AppPort)

	// ── Logger ────────────────────────────────────────────────────────────────
	var logger *zap.Logger
	var err error
	if cfg.IsDevelopment() {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// ── database connections ──────────────────────────────────────────────
	dbPool := db.Connect(cfg)
	redisClient := db.ConnectRedis(cfg)

	// ── repositories ─────────────────────────────────────────────────────
	userRepo         := repositories.NewUserRepository(dbPool)
	refreshTokenRepo := repositories.NewRefreshTokenRepository(dbPool)
	workspaceRepo    := repositories.NewWorkspaceRepository(dbPool)
	boardRepo        := repositories.NewBoardRepository(dbPool)
	columnRepo       := repositories.NewColumnRepository(dbPool)
	cardRepo         := repositories.NewCardRepository(dbPool)
	documentRepo     := repositories.NewDocumentRepository(dbPool)
	invitationRepo   := repositories.NewInvitationRepository(dbPool)

	// ── Email client ──────────────────────────────────────────────────────────
	// emailClient is always constructed — if RESEND_API_KEY is empty the client
	// exists but all send calls will return an error (handled as non-fatal in
	// the invitation service). This avoids nil checks everywhere.
	emailClient := email.NewClient(cfg.ResendAPIKey, cfg.ResendFromAddr)
 
	if !cfg.IsEmailConfigured() {
		logger.Warn("RESEND_API_KEY is not set — invitation emails will not be delivered")
	}

	// ── services (partial — documentService needed by hub) ───────────────
	authService      := services.NewAuthService(userRepo, refreshTokenRepo, workspaceRepo, cfg)
	workspaceService := services.NewWorkspaceService(workspaceRepo, userRepo)
	boardService     := services.NewBoardService(boardRepo, workspaceRepo, cfg)
	documentService  := services.NewDocumentService(documentRepo, cardRepo, columnRepo, boardService, cfg)
	invitationService := services.NewInvitationService(invitationRepo, workspaceRepo, userRepo, emailClient, cfg.FrontendURL)

	// ── websocket hub ─────────────────────────────────────────────────────
	hub := ws.NewHub(documentService)

	// ── services (remainder — depend on hub) ──────────────────────────────
	columnService := services.NewColumnService(columnRepo, boardService, hub)
	cardService   := services.NewCardService(cardRepo, columnRepo, boardService, hub)

	// ── handlers ─────────────────────────────────────────────────────────
	healthHandler    := handlers.NewHealthHandler(dbPool, redisClient)
	authHandler      := handlers.NewAuthHandler(authService, cfg)
	workspaceHandler := handlers.NewWorkspaceHandler(workspaceService)
	invitationHandler := handlers.NewInvitationHandler(invitationService, logger)
	boardHandler     := handlers.NewBoardHandler(boardService)
	columnHandler    := handlers.NewColumnHandler(columnService)
	cardHandler      := handlers.NewCardHandler(cardService)
	documentHandler  := handlers.NewDocumentHandler(documentService, hub)
	wsHandler        := handlers.NewWSHandler(hub, cfg)

	// ── HTTP server ───────────────────────────────────────────────────────
	// Use net/http.Server directly instead of r.Run() so we can call
	// server.Shutdown() during graceful shutdown. r.Run() blocks forever
	// with no shutdown hook.
	ginRouter := router.Setup(
		cfg,
		healthHandler,
		authHandler,
		workspaceHandler,
		invitationHandler,
		boardHandler,
		columnHandler,
		cardHandler,
		documentHandler,
		wsHandler,
		workspaceRepo,
		boardRepo,
		columnRepo,
		cardRepo,
		documentRepo,
	)

	server := &http.Server{
		Addr:    ":" + cfg.AppPort,
		Handler: ginRouter,

		// Defensive timeouts — guards against slow clients holding connections
		// open indefinitely. WebSocket connections are long-lived but upgrades
		// happen quickly; these timeouts only apply to the HTTP handshake phase.
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// ── signal listener ───────────────────────────────────────────────────
	// Buffer 1 so the OS signal is never dropped if we're momentarily busy.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// ── start serving ─────────────────────────────────────────────────────
	// Run in a goroutine so main() can proceed to the shutdown block below.
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("docflow started", zap.String("port", cfg.AppPort))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	// ── block until signal or server error ────────────────────────────────
	select {
	case err := <-serverErr:
		log.Fatalf("server error: %v", err)

	case sig := <-quit:
		log.Printf("received signal %s — starting graceful shutdown", sig)
	}

	// ── ordered shutdown sequence ─────────────────────────────────────────
	// Order matters: stop accepting new connections first, drain WebSocket
	// rooms second, then close backing resources last.

	// Step 1 — stop accepting new HTTP/WebSocket connections.
	// In-flight HTTP handlers have 30 s to finish. Upgrade requests that
	// arrived before this point continue normally; new ones are refused.
	httpCtx, httpCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer httpCancel()

	if err := server.Shutdown(httpCtx); err != nil {
		log.Printf("HTTP shutdown did not complete cleanly: %v", err)
	} else {
		log.Println("HTTP server shut down")
	}

	// Step 2 — drain the WebSocket hub.
	// Sends close frames to every connected document and board client and
	// waits for all room goroutines to exit. Bounded by its own timeout so
	// a stuck client can't block the process forever.
	wsCtx, wsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer wsCancel()

	hub.Shutdown(wsCtx)
	log.Println("WebSocket hub shut down")

	// Step 3 — close backing resources.
	// By this point no goroutine is touching the pool or the Redis client,
	// so these calls are safe and will not block.
	redisClient.Close()
	log.Println("Redis connection closed")

	dbPool.Close()
	log.Println("database connection pool closed")

	log.Println("shutdown complete — goodbye")
}
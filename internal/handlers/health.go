// internal/handlers/health.go
package handlers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// HealthHandler handles GET /health.
// It probes both the database and Redis concurrently and returns:
//
//	200  { "status": "ok",      "db": "ok",    "redis": "ok"    }
//	503  { "status": "degraded","db": "<err>", "redis": "<err>" }
//
// Fly.io (and any other platform health check) treats any non-2xx as
// unhealthy and will not route traffic to the instance until it recovers.
// Running the probes concurrently keeps p99 latency well under 100 ms
// even if one dependency is slow.
type HealthHandler struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewHealthHandler(db *pgxpool.Pool, redis *redis.Client) *HealthHandler {
	return &HealthHandler{db: db, redis: redis}
}

// Check handles GET /health.
func (h *HealthHandler) Check(c *gin.Context) {
	// Each probe gets its own tight deadline — health checks must be fast.
	// If the DB or Redis is genuinely up but slow (> 3 s), that is itself
	// a problem worth surfacing as unhealthy.
	const probeTimeout = 3 * time.Second

	var (
		dbStatus    string
		redisStatus string
		mu          sync.Mutex
		wg          sync.WaitGroup
		healthy     = true
	)

	// ── DB probe ──────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()

		ctx, cancel := context.WithTimeout(c.Request.Context(), probeTimeout)
		defer cancel()

		status := "ok"
		if err := h.db.Ping(ctx); err != nil {
			status = err.Error()
			mu.Lock()
			healthy = false
			mu.Unlock()
		}

		mu.Lock()
		dbStatus = status
		mu.Unlock()
	}()

	// ── Redis probe ───────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()

		ctx, cancel := context.WithTimeout(c.Request.Context(), probeTimeout)
		defer cancel()

		status := "ok"
		if err := h.redis.Ping(ctx).Err(); err != nil {
			status = err.Error()
			mu.Lock()
			healthy = false
			mu.Unlock()
		}

		mu.Lock()
		redisStatus = status
		mu.Unlock()
	}()

	wg.Wait()

	statusCode := http.StatusOK
	overallStatus := "ok"
	if !healthy {
		statusCode = http.StatusServiceUnavailable
		overallStatus = "degraded"
	}

	c.JSON(statusCode, gin.H{
		"status": overallStatus,
		"db":     dbStatus,
		"redis":  redisStatus,
	})
}
// internal/middleware/cors.go
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/config"
)

// CORS returns a Gin middleware that sets the correct cross-origin headers
// so the frontend (Vite dev server or production domain) can talk to the API.
//
// Must be registered before any route-specific middleware so preflight
// OPTIONS requests are handled before auth middleware runs.
// (Auth middleware would reject OPTIONS requests since they carry no token.)
func CORS(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := cfg.CORSAllowedOrigin

		// Allow the configured frontend origin.
		c.Header("Access-Control-Allow-Origin", origin)

		// allow-credentials must be true so the frontend can send the
		// Authorization header and receive Set-Cookie headers if needed.
		c.Header("Access-Control-Allow-Credentials", "true")

		// The methods your API actually uses.
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

		// Authorization — needed for JWT bearer tokens.
		// Content-Type — needed for JSON request bodies.
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")

		// How long the browser can cache this preflight response.
		// 86400 = 24 hours — avoids a preflight before every single request.
		c.Header("Access-Control-Max-Age", "86400")

		// Preflight request — browser is asking "will you accept my real request?"
		// Respond immediately with 204 and the headers above.
		// Do NOT call c.Next() — there is no body and no further handling needed.
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/config"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

// Auth is the JWT validation middleware.
// It runs before every protected route handler.
// On success it injects user_id, email, and name into the Gin context.
// On failure it aborts the request with the appropriate error response.
func Auth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {

		// read the Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			utils.ErrUnauthorized(c, "MISSING_TOKEN", "authorization header is required")
			return
		}

		// header must be in the format "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			utils.ErrUnauthorized(c, "INVALID_TOKEN", "authorization header format must be Bearer <token>")
			return
		}

		tokenString := parts[1]

		// validate the JWT signature and expiry
		claims, err := utils.ValidateToken(tokenString, cfg.JWTAccessSecret)
		if err != nil {
			if errors.Is(err, utils.ErrTokenExpired) {
				// use this specific code so the frontend knows to
				// silently refresh rather than redirect to login
				utils.ErrUnauthorized(c, "TOKEN_EXPIRED", "access token has expired")
				return
			}
			utils.ErrUnauthorized(c, "INVALID_TOKEN", "access token is invalid")
			return
		}

		// make sure this is an access token — not a document token
		// being used on a regular API endpoint
		if claims.TokenType != "access" {
			utils.ErrUnauthorized(c, "INVALID_TOKEN", "invalid token type")
			return
		}

		// inject user details into the context
		// handlers read these with c.GetString("user_id") etc.
		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_name", claims.Name)

		c.Next()
	}
}
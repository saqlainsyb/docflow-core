package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/config"
	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/services"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

const refreshCookieName = "refresh_token"

type AuthHandler struct {
	authService *services.AuthService
	cfg         *config.Config
}

func NewAuthHandler(authService *services.AuthService, cfg *config.Config) *AuthHandler {
	return &AuthHandler{authService: authService, cfg: cfg}
}

// setRefreshCookie writes the refresh token to an HttpOnly cookie.
// HttpOnly: JS cannot read this cookie — XSS cannot steal the refresh token.
// Secure: only sent over HTTPS — disabled in development (http://localhost).
// SameSite=Strict: cookie is never sent on cross-origin requests — prevents CSRF.
// Path=/api/v1/auth: browser only attaches the cookie to auth endpoints,
// not to every API call, which reduces unnecessary exposure.
func (h *AuthHandler) setRefreshCookie(c *gin.Context, rawToken string) {
	maxAge := int(h.cfg.JWTRefreshExpiry.Seconds())
	secure := !h.cfg.IsDevelopment()

	c.SetSameSite(http.SameSiteNoneMode)
	c.SetCookie(
		refreshCookieName,
		rawToken,
		maxAge,
		"/api/v1/auth", // path — scoped to auth endpoints only
		"",             // domain — empty means current host
		secure,         // secure — false in dev, true in production
		true,           // httpOnly — JS cannot read this
	)
}

// clearRefreshCookie expires the cookie immediately.
// Called on logout and on theft detection so the browser discards it.
func (h *AuthHandler) clearRefreshCookie(c *gin.Context) {
	secure := !h.cfg.IsDevelopment()

	c.SetSameSite(http.SameSiteNoneMode)
	c.SetCookie(
		refreshCookieName,
		"",
		-1,            // maxAge -1 tells the browser to delete immediately
		"/api/v1/auth",
		"",
		secure,
		true,
	)
}

// Register handles POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req models.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	resp, err := h.authService.Register(c.Request.Context(), req)
	if err != nil {
		handleAuthError(c, err)
		return
	}

	// write refresh token to HttpOnly cookie — never in the response body
	h.setRefreshCookie(c, resp.RefreshToken)

	// resp.RefreshToken is json:"-" so it won't appear in the JSON output
	c.JSON(http.StatusCreated, resp)
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	resp, err := h.authService.Login(c.Request.Context(), req)
	if err != nil {
		handleAuthError(c, err)
		return
	}

	h.setRefreshCookie(c, resp.RefreshToken)
	c.JSON(http.StatusOK, resp)
}

// Refresh handles POST /api/v1/auth/refresh
// Reads the refresh token from the HttpOnly cookie — no request body needed.
// On success, overwrites the cookie with the new refresh token (rotation).
func (h *AuthHandler) Refresh(c *gin.Context) {
	rawToken, err := c.Cookie(refreshCookieName)
	if err != nil || rawToken == "" {
		utils.ErrUnauthorized(c, "REFRESH_TOKEN_INVALID", "refresh token cookie is missing")
		return
	}

	resp, err := h.authService.Refresh(c.Request.Context(), rawToken)
	if err != nil {
		// on theft detection, also clear the cookie so the browser
		// doesn't keep retrying with the compromised token
		if errors.Is(err, services.ErrTokenTheftDetected) {
			h.clearRefreshCookie(c)
		}
		handleAuthError(c, err)
		return
	}

	// overwrite cookie with the newly rotated refresh token
	h.setRefreshCookie(c, resp.RefreshToken)
	c.JSON(http.StatusOK, resp)
}

// Logout handles POST /api/v1/auth/logout
// Reads the refresh token from the cookie, revokes it, then clears the cookie.
// No request body required — the access token in the Authorization header
// is enough to authenticate the request (auth middleware handles that).
func (h *AuthHandler) Logout(c *gin.Context) {
	rawToken, _ := c.Cookie(refreshCookieName)

	// always clear the cookie first — even if the token isn't found in the DB
	h.clearRefreshCookie(c)

	if err := h.authService.Logout(c.Request.Context(), rawToken); err != nil {
		utils.ErrInternal(c)
		return
	}

	c.Status(http.StatusNoContent)
}

// GetMe handles GET /api/v1/users/me
func (h *AuthHandler) GetMe(c *gin.Context) {
	userID := c.GetString("user_id")

	user, err := h.authService.GetMe(c.Request.Context(), userID)
	if err != nil {
		utils.ErrInternal(c)
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateMe handles PATCH /api/v1/users/me
func (h *AuthHandler) UpdateMe(c *gin.Context) {
	var req models.UpdateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	userID := c.GetString("user_id")

	user, err := h.authService.UpdateMe(c.Request.Context(), userID, req)
	if err != nil {
		handleAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, user)
}

// handleAuthError maps service errors to the correct HTTP response.
func handleAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrEmailAlreadyExists):
		utils.ErrConflict(c, "EMAIL_ALREADY_EXISTS", "an account with this email already exists")
	case errors.Is(err, services.ErrInvalidCredentials):
		utils.ErrUnauthorized(c, "INVALID_CREDENTIALS", "invalid email or password")
	case errors.Is(err, services.ErrInvalidRefreshToken):
		utils.ErrUnauthorized(c, "REFRESH_TOKEN_INVALID", "refresh token is invalid or has been revoked")
	case errors.Is(err, services.ErrRefreshTokenExpired):
		utils.ErrUnauthorized(c, "REFRESH_TOKEN_EXPIRED", "refresh token has expired")
	case errors.Is(err, services.ErrTokenTheftDetected):
		utils.ErrUnauthorized(c, "TOKEN_THEFT_DETECTED", "suspicious activity detected, please login again")
	case errors.Is(err, services.ErrUserNotFound):
		utils.ErrorResponse(c, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
	case errors.Is(err, services.ErrWeakPassword):
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		utils.ErrInternal(c)
	}
}